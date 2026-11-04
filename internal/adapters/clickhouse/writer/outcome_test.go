package writer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDedupeTokenFromEventsIsStableAcrossSubmissions(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	cfg := testConfig(dir)
	cfg.DedupeWindow = 0
	cfg.DedupeTokenFromEvents = true
	outcomes := make(chan Outcome, 4)
	cfg.OnDone = func(outcome Outcome) { outcomes <- outcome }

	w, err := OpenLogs(context.Background(), cfg, inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() { _ = w.Close(context.Background()) }()

	records := []LogRecord{testLog("e1"), testLog("e2")}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := w.SubmitLogs(context.Background(), records); err != nil {
			t.Fatalf("SubmitLogs() attempt %d error = %v", attempt, err)
		}
		select {
		case outcome := <-outcomes:
			if outcome.DLQ || outcome.Rows != 2 {
				t.Fatalf("outcome = %+v", outcome)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for outcome")
		}
	}
	if inserter.count() != 2 {
		t.Fatalf("inserts = %d, want 2", inserter.count())
	}
	first := inserter.request(0)
	second := inserter.request(1)
	if first.DedupeToken == "" || first.DedupeToken != second.DedupeToken {
		t.Fatalf("tokens = %q / %q, want equal", first.DedupeToken, second.DedupeToken)
	}
}

func TestOnDoneReportsDLQ(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	inserter.setHandler(func(InsertRequest) (InsertResult, error) {
		return InsertResult{}, &InsertError{StatusCode: 400, Code: "62", Message: "syntax error", Permanent: true}
	})
	cfg := testConfig(dir)
	cfg.DedupeWindow = 0
	outcomes := make(chan Outcome, 1)
	cfg.OnDone = func(outcome Outcome) { outcomes <- outcome }

	w, err := OpenLogs(context.Background(), cfg, inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() { _ = w.Close(context.Background()) }()

	if _, err := w.SubmitLogs(context.Background(), []LogRecord{testLog("bad")}); err != nil {
		t.Fatalf("SubmitLogs() error = %v", err)
	}
	select {
	case outcome := <-outcomes:
		if !outcome.DLQ || outcome.Rows != 1 || outcome.Error == "" {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for dlq outcome")
	}
	stats := w.Stats()
	if stats.DLQEvents != 1 {
		t.Fatalf("dlq events = %d, want 1", stats.DLQEvents)
	}
}

func TestOnDoneReportsAttempts(t *testing.T) {
	dir := t.TempDir()
	inserter := &fakeInserter{}
	var (
		mu       sync.Mutex
		attempts int
	)
	inserter.setHandler(func(request InsertRequest) (InsertResult, error) {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if attempts == 1 {
			return InsertResult{}, &InsertError{StatusCode: 503, Message: "storage is busy"}
		}
		return InsertResult{WrittenRows: uint64(request.Rows)}, nil
	})
	cfg := testConfig(dir)
	cfg.DedupeWindow = 0
	cfg.RetryBackoff = time.Millisecond
	outcomes := make(chan Outcome, 1)
	cfg.OnDone = func(outcome Outcome) { outcomes <- outcome }

	w, err := OpenLogs(context.Background(), cfg, inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() { _ = w.Close(context.Background()) }()

	if _, err := w.SubmitLogs(context.Background(), []LogRecord{testLog("retry")}); err != nil {
		t.Fatalf("SubmitLogs() error = %v", err)
	}
	select {
	case outcome := <-outcomes:
		if outcome.DLQ || outcome.Attempts != 2 || outcome.Rows != 1 {
			t.Fatalf("outcome = %+v, want two attempts", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for outcome")
	}
	if err := w.Close(context.Background()); err != nil && !errors.Is(err, ErrClosed) {
		t.Fatalf("Close() error = %v", err)
	}
}
