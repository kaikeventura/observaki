package writer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeInserter struct {
	mu       sync.Mutex
	requests []InsertRequest
	handler  func(InsertRequest) (InsertResult, error)
}

func (f *fakeInserter) Insert(_ context.Context, request InsertRequest) (InsertResult, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	handler := f.handler
	f.mu.Unlock()
	if handler != nil {
		return handler(request)
	}
	return InsertResult{WrittenRows: uint64(request.Rows)}, nil
}

func (f *fakeInserter) setHandler(handler func(InsertRequest) (InsertResult, error)) {
	f.mu.Lock()
	f.handler = handler
	f.mu.Unlock()
}

func (f *fakeInserter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakeInserter) request(index int) InsertRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[index]
}

func testConfig(dir string) Config {
	return Config{
		SpoolDir:     dir,
		QueueDepth:   2,
		DedupeWindow: 10,
		MaxSkew:      24 * time.Hour,
		RetryBackoff: 5 * time.Millisecond,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func testLog(eventID string) LogRecord {
	return LogRecord{
		TenantID:       "tenant-0",
		ProjectID:      "project-0",
		Timestamp:      time.Now().UTC(),
		Severity:       "INFO",
		SeverityNumber: 9,
		ServiceName:    "checkout",
		Body:           "request completed",
		Attributes:     map[string]string{"http.method": "GET"},
		EventID:        eventID,
	}
}

func waitFor(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func TestSubmitAckAndCheckpoint(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	w, err := OpenLogs(context.Background(), testConfig(dir), inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		_ = w.Close(context.Background())
	}()

	result, err := w.SubmitLogs(context.Background(), []LogRecord{testLog("e1"), testLog("e2")})
	if err != nil {
		t.Fatalf("SubmitLogs() error = %v", err)
	}
	if result.Rows != 2 || result.Duplicates != 0 || result.Seq != 1 {
		t.Fatalf("SubmitLogs() = %+v, want 2 rows, no duplicates, seq 1", result)
	}
	waitFor(t, "batch ack", func() bool { return w.Stats().AckedBatches == 1 })

	stats := w.Stats()
	if stats.InsertedEvents != 2 || stats.SubmittedEvents != 2 {
		t.Fatalf("stats = %+v, want 2 submitted and 2 inserted", stats)
	}
	if stats.PendingBatches != 0 || stats.PendingBytes != 0 {
		t.Fatalf("stats = %+v, want no pending batch or bytes", stats)
	}
	if len(inserter.requests) != 1 {
		t.Fatalf("insert requests = %d, want 1", len(inserter.requests))
	}
	request := inserter.request(0)
	if request.Table != "otel_logs" || request.Rows != 2 || request.DedupeToken == "" {
		t.Fatalf("insert request = %+v, want otel_logs with 2 rows and a token", request)
	}
	if len(request.Data) == 0 {
		t.Fatal("insert request has no RowBinary payload")
	}

	spool, err := openSpool(dir)
	if err != nil {
		t.Fatalf("openSpool() error = %v", err)
	}
	checkpoint, err := spool.readCheckpoint()
	if err != nil {
		t.Fatalf("readCheckpoint() error = %v", err)
	}
	if checkpoint != result.Seq {
		t.Fatalf("checkpoint = %d, want %d", checkpoint, result.Seq)
	}
	files, err := spool.all()
	if err != nil {
		t.Fatalf("spool.all() error = %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("spool still has %d batches after ack", len(files))
	}
}

func TestDeduplicatesEventIDs(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	w, err := OpenLogs(context.Background(), testConfig(dir), inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		_ = w.Close(context.Background())
	}()

	first, err := w.SubmitLogs(context.Background(), []LogRecord{testLog("e1"), testLog("e1")})
	if err != nil {
		t.Fatalf("SubmitLogs(first) error = %v", err)
	}
	if first.Rows != 1 || first.Duplicates != 1 {
		t.Fatalf("first SubmitLogs() = %+v, want 1 row and 1 duplicate", first)
	}

	second, err := w.SubmitLogs(context.Background(), []LogRecord{testLog("e1")})
	if err != nil {
		t.Fatalf("SubmitLogs(second) error = %v", err)
	}
	if second.Rows != 0 || second.Duplicates != 1 {
		t.Fatalf("second SubmitLogs() = %+v, want 0 rows and 1 duplicate", second)
	}
	waitFor(t, "first batch ack", func() bool { return w.Stats().AckedBatches == 1 })

	stats := w.Stats()
	if stats.SubmittedEvents != 1 || stats.DuplicateEvents != 2 {
		t.Fatalf("stats = %+v, want 1 submitted and 2 duplicates", stats)
	}
	if inserter.count() != 1 {
		t.Fatalf("insert requests = %d, want 1", inserter.count())
	}
}

func TestDedupeWindowEvictsOldest(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	cfg := testConfig(dir)
	cfg.DedupeWindow = 2
	w, err := OpenLogs(context.Background(), cfg, inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		_ = w.Close(context.Background())
	}()

	for _, eventID := range []string{"e1", "e2", "e3"} {
		result, err := w.SubmitLogs(context.Background(), []LogRecord{testLog(eventID)})
		if err != nil {
			t.Fatalf("SubmitLogs(%s) error = %v", eventID, err)
		}
		if result.Rows != 1 {
			t.Fatalf("SubmitLogs(%s) rows = %d, want 1", eventID, result.Rows)
		}
	}
	// e1 was evicted by e3, so it is accepted again.
	result, err := w.SubmitLogs(context.Background(), []LogRecord{testLog("e1")})
	if err != nil {
		t.Fatalf("SubmitLogs(e1 again) error = %v", err)
	}
	if result.Rows != 1 || result.Duplicates != 0 {
		t.Fatalf("SubmitLogs(e1 again) = %+v, want 1 row and 0 duplicates", result)
	}
	waitFor(t, "four batches ack", func() bool { return w.Stats().AckedBatches == 4 })
}

func TestInvalidRecordRejectsWholeBatch(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	w, err := OpenLogs(context.Background(), testConfig(dir), inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		_ = w.Close(context.Background())
	}()

	bad := testLog("bad")
	bad.EventID = ""
	_, err = w.SubmitLogs(context.Background(), []LogRecord{testLog("ok"), bad})
	if err == nil {
		t.Fatal("SubmitLogs() error = nil, want a validation error")
	}
	var encodeErr *encodeError
	if !errors.As(err, &encodeErr) || encodeErr.index != 1 {
		t.Fatalf("SubmitLogs() error = %v, want encodeError at index 1", err)
	}
	if w.Stats().SubmittedBatches != 0 || inserter.count() != 0 {
		t.Fatal("invalid batch reached the spool or the inserter")
	}
}

func TestClampsTimestampsOutsideSkew(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	w, err := OpenLogs(context.Background(), testConfig(dir), inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		_ = w.Close(context.Background())
	}()

	record := testLog("old")
	record.Timestamp = time.Now().Add(-48 * time.Hour)
	result, err := w.SubmitLogs(context.Background(), []LogRecord{record})
	if err != nil {
		t.Fatalf("SubmitLogs() error = %v", err)
	}
	if result.Clamped != 1 {
		t.Fatalf("SubmitLogs() = %+v, want 1 clamped timestamp", result)
	}
	waitFor(t, "clamped batch ack", func() bool { return w.Stats().AckedBatches == 1 })
	if w.Stats().ClampedTimestamps != 1 {
		t.Fatalf("ClampedTimestamps = %d, want 1", w.Stats().ClampedTimestamps)
	}
}

func TestOpenReplaysUnackedBatch(t *testing.T) {
	dir := t.TempDir()
	spool, err := openSpool(dir)
	if err != nil {
		t.Fatalf("openSpool() error = %v", err)
	}
	data := []byte("rowbinary-payload")
	sum := sha256.Sum256(data)
	meta := &batchMeta{
		Seq:                  7,
		Table:                "otel_logs",
		Signal:               SignalLogs,
		Rows:                 1,
		DedupeToken:          "logs-7-deadbeef",
		MinTimestampUnixNano: time.Now().Add(-time.Second).UnixNano(),
		MaxTimestampUnixNano: time.Now().UnixNano(),
		DataSHA256:           hex.EncodeToString(sum[:]),
	}
	if _, err := spool.write(meta, data); err != nil {
		t.Fatalf("spool.write() error = %v", err)
	}

	inserter := &fakeInserter{}
	w, err := OpenLogs(context.Background(), testConfig(dir), inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		_ = w.Close(context.Background())
	}()

	stats := w.Stats()
	if stats.RecoveredBatches != 1 || stats.RecoveredEvents != 1 {
		t.Fatalf("recovered stats = %+v, want one batch and one event", stats)
	}
	if stats.InsertedEvents != 1 || stats.AckedBatches != 1 {
		t.Fatalf("runtime stats = %+v, want one inserted event and one ack", stats)
	}
	if inserter.count() != 1 || inserter.request(0).DedupeToken != "logs-7-deadbeef" {
		t.Fatalf("insert requests = %+v, want the stored dedupe token", inserter.requests)
	}
	checkpoint, err := spool.readCheckpoint()
	if err != nil {
		t.Fatalf("readCheckpoint() error = %v", err)
	}
	if checkpoint != 7 {
		t.Fatalf("checkpoint = %d, want 7", checkpoint)
	}
}

func TestReplayCountsRecoveredRows(t *testing.T) {
	dir := t.TempDir()
	spool, err := openSpool(dir)
	if err != nil {
		t.Fatalf("openSpool() error = %v", err)
	}
	data := []byte("already-committed")
	sum := sha256.Sum256(data)
	meta := &batchMeta{
		Seq:                  3,
		Table:                "otel_logs",
		Signal:               SignalLogs,
		Rows:                 100,
		DedupeToken:          "logs-3-already",
		MinTimestampUnixNano: time.Now().UnixNano(),
		MaxTimestampUnixNano: time.Now().UnixNano(),
		DataSHA256:           hex.EncodeToString(sum[:]),
	}
	if _, err := spool.write(meta, data); err != nil {
		t.Fatalf("spool.write() error = %v", err)
	}

	inserter := &fakeInserter{}
	inserter.setHandler(func(InsertRequest) (InsertResult, error) {
		return InsertResult{WrittenRows: 0}, nil
	})
	w, err := OpenLogs(context.Background(), testConfig(dir), inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		_ = w.Close(context.Background())
	}()

	stats := w.Stats()
	if stats.RecoveredEvents != 100 {
		t.Fatalf("RecoveredEvents = %d, want 100", stats.RecoveredEvents)
	}
	if stats.InsertedEvents != 0 {
		t.Fatalf("InsertedEvents = %d, want 0", stats.InsertedEvents)
	}
	if stats.RecoveredBatches != 1 {
		t.Fatalf("RecoveredBatches = %d, want 1", stats.RecoveredBatches)
	}
}

func TestPermanentErrorMovesBatchToDLQ(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	inserter.setHandler(func(InsertRequest) (InsertResult, error) {
		return InsertResult{}, &InsertError{StatusCode: 400, Code: "62", Message: "syntax error", Permanent: true}
	})
	w, err := OpenLogs(context.Background(), testConfig(dir), inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		_ = w.Close(context.Background())
	}()

	result, err := w.SubmitLogs(context.Background(), []LogRecord{testLog("bad")})
	if err != nil {
		t.Fatalf("SubmitLogs() error = %v", err)
	}
	waitFor(t, "dlq", func() bool { return w.Stats().DLQEvents == 1 })

	spool, err := openSpool(dir)
	if err != nil {
		t.Fatalf("openSpool() error = %v", err)
	}
	files, err := spool.all()
	if err != nil {
		t.Fatalf("spool.all() error = %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("spool still has %d batches after dlq", len(files))
	}
	entries, err := os.ReadDir(filepath.Join(dir, "dlq"))
	if err != nil {
		t.Fatalf("read dlq directory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("dlq entries = %d, want the batch and its error record", len(entries))
	}
	checkpoint, err := spool.readCheckpoint()
	if err != nil {
		t.Fatalf("readCheckpoint() error = %v", err)
	}
	if checkpoint != result.Seq {
		t.Fatalf("checkpoint = %d, want %d", checkpoint, result.Seq)
	}
}

func TestCloseDuringRetryLeavesBatchForReplay(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	inserter.setHandler(func(InsertRequest) (InsertResult, error) {
		return InsertResult{}, &InsertError{StatusCode: 503, Message: "storage down", Permanent: false}
	})
	w, err := OpenLogs(context.Background(), testConfig(dir), inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	if _, err := w.SubmitLogs(context.Background(), []LogRecord{testLog("e1")}); err != nil {
		t.Fatalf("SubmitLogs() error = %v", err)
	}
	closeContext, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := w.Close(closeContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() error = %v, want context deadline exceeded", err)
	}

	spool, err := openSpool(dir)
	if err != nil {
		t.Fatalf("openSpool() error = %v", err)
	}
	files, err := spool.all()
	if err != nil {
		t.Fatalf("spool.all() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("spool batches = %d, want the unacknowledged batch", len(files))
	}

	recoveryInserter := &fakeInserter{}
	recovered, err := OpenLogs(context.Background(), testConfig(dir), recoveryInserter)
	if err != nil {
		t.Fatalf("OpenLogs(recovery) error = %v", err)
	}
	defer func() {
		_ = recovered.Close(context.Background())
	}()
	if recovered.Stats().RecoveredBatches != 1 || recovered.Stats().InsertedEvents != 1 {
		t.Fatalf("recovery stats = %+v, want one recovered and inserted event", recovered.Stats())
	}
}

func TestConcurrentSubmitsKeepSequenceAndCheckpoint(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	cfg := testConfig(dir)
	cfg.QueueDepth = 4
	w, err := OpenLogs(context.Background(), cfg, inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		_ = w.Close(context.Background())
	}()

	const goroutines = 8
	const perGoroutine = 5
	var group sync.WaitGroup
	errs := make(chan error, goroutines)
	for index := 0; index < goroutines; index++ {
		group.Add(1)
		go func(goroutine int) {
			defer group.Done()
			for batch := 0; batch < perGoroutine; batch++ {
				eventID := eventIDFor(goroutine, batch)
				if _, err := w.SubmitLogs(context.Background(), []LogRecord{testLog(eventID)}); err != nil {
					errs <- err
					return
				}
			}
		}(index)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("SubmitLogs() error = %v", err)
	}
	waitFor(t, "all batches ack", func() bool {
		return w.Stats().AckedBatches == goroutines*perGoroutine
	})
	stats := w.Stats()
	if stats.SubmittedBatches != goroutines*perGoroutine {
		t.Fatalf("SubmittedBatches = %d, want %d", stats.SubmittedBatches, goroutines*perGoroutine)
	}
	if stats.InsertedEvents != goroutines*perGoroutine {
		t.Fatalf("InsertedEvents = %d, want %d", stats.InsertedEvents, goroutines*perGoroutine)
	}
}

func eventIDFor(goroutine, batch int) string {
	return "event-" + string(rune('a'+goroutine)) + "-" + string(rune('0'+batch))
}
