package writer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed is returned after Close starts.
var ErrClosed = errors.New("clickhouse writer is closed")

// ErrWrongSignal is returned when a logs writer receives spans or vice versa.
var ErrWrongSignal = errors.New("clickhouse writer received the wrong signal")

// Config configures one writer. Table and Signal are filled by OpenLogs and
// OpenTraces; the remaining fields have safe defaults.
type Config struct {
	Table         string
	Signal        string
	SpoolDir      string
	QueueDepth    int
	DedupeWindow  int
	MaxSkew       time.Duration
	MaxRetries    int
	RetryBackoff  time.Duration
	EncodeWorkers int
	Logger        *slog.Logger
	OnAck         func(Ack)
}

// Ack describes a batch that ClickHouse confirmed.
type Ack struct {
	Seq            uint64        `json:"seq"`
	Rows           int           `json:"rows"`
	WrittenRows    uint64        `json:"written_rows"`
	Lag            time.Duration `json:"lag"`
	OldestLag      time.Duration `json:"oldest_lag"`
	AcknowledgedAt time.Time     `json:"acknowledged_at"`
	Recovered      bool          `json:"recovered"`
}

// SubmitResult describes a batch that was durably spooled. It does not mean
// ClickHouse has flushed the batch yet; Ack or Stats.AckedBatches does.
type SubmitResult struct {
	Seq          uint64 `json:"seq"`
	Rows         int    `json:"rows"`
	Duplicates   int    `json:"duplicates"`
	Clamped      int    `json:"clamped"`
	SpooledBytes int    `json:"spooled_bytes"`
}

type queuedBatch struct {
	meta     *batchMeta
	path     string
	data     []byte
	replayed bool
}

// Writer is the storage ingestion path for one signal. Submit is safe for
// concurrent callers; batches are spooled and inserted in sequence order.
type Writer struct {
	cfg      Config
	inserter Inserter
	spool    *spool
	dedupe   *dedupeWindow
	stats    counters

	queue  chan *queuedBatch
	runCtx context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	seq    atomic.Uint64
	closed atomic.Bool

	lifecycleMu sync.Mutex
	stopCh      chan struct{}
	submitWG    sync.WaitGroup

	fatalOnce sync.Once
	fatalCh   chan struct{}
	fatalMu   sync.Mutex
	fatalErr  error
}

// OpenLogs opens a writer for otel_logs, replays any unacknowledged batch from
// the spool and starts the insert worker.
func OpenLogs(ctx context.Context, cfg Config, inserter Inserter) (*Writer, error) {
	cfg.Signal = SignalLogs
	if cfg.Table == "" {
		cfg.Table = "otel_logs"
	}
	return open(ctx, cfg, inserter)
}

// OpenTraces opens a writer for otel_traces with the same replay contract.
func OpenTraces(ctx context.Context, cfg Config, inserter Inserter) (*Writer, error) {
	cfg.Signal = SignalTraces
	if cfg.Table == "" {
		cfg.Table = "otel_traces"
	}
	return open(ctx, cfg, inserter)
}

func open(ctx context.Context, cfg Config, inserter Inserter) (*Writer, error) {
	if inserter == nil {
		return nil, errors.New("clickhouse inserter is nil")
	}
	if cfg.Signal != SignalLogs && cfg.Signal != SignalTraces {
		return nil, fmt.Errorf("clickhouse writer signal = %q, want logs or traces", cfg.Signal)
	}
	if cfg.Table == "" {
		return nil, errors.New("clickhouse writer table is empty")
	}
	if err := validateTable(cfg.Table); err != nil {
		return nil, err
	}
	if cfg.SpoolDir == "" {
		return nil, errors.New("clickhouse writer spool directory is empty")
	}
	if cfg.QueueDepth <= 0 {
		cfg.QueueDepth = 2
	}
	if cfg.DedupeWindow < 0 {
		return nil, errors.New("clickhouse writer dedupe window cannot be negative")
	}
	if cfg.MaxSkew < 0 {
		return nil, errors.New("clickhouse writer max skew cannot be negative")
	}
	if cfg.MaxRetries < 0 {
		return nil, errors.New("clickhouse writer max retries cannot be negative")
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 500 * time.Millisecond
	}
	if cfg.EncodeWorkers <= 0 {
		cfg.EncodeWorkers = runtime.GOMAXPROCS(0)
	}
	if cfg.EncodeWorkers > 32 {
		cfg.EncodeWorkers = 32
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	spool, err := openSpool(cfg.SpoolDir)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	w := &Writer{
		cfg:      cfg,
		inserter: inserter,
		spool:    spool,
		dedupe:   newDedupeWindow(cfg.DedupeWindow),
		queue:    make(chan *queuedBatch, cfg.QueueDepth),
		runCtx:   runCtx,
		cancel:   cancel,
		stopCh:   make(chan struct{}),
		fatalCh:  make(chan struct{}),
	}

	if err := w.replay(ctx); err != nil {
		cancel()
		return nil, err
	}
	w.wg.Add(1)
	go w.worker()
	return w, nil
}

// replay inserts every batch above the checkpoint in sequence order before
// accepting new submissions. Replaying synchronously preserves checkpoint
// monotonicity; a crash during recovery is safe because the token is stable.
func (w *Writer) replay(ctx context.Context) error {
	acked, err := w.spool.readCheckpoint()
	if err != nil {
		return err
	}
	files, err := w.spool.all()
	if err != nil {
		return err
	}
	maxSeq := acked
	for _, file := range files {
		if file.seq <= acked {
			if err := w.spool.remove(file.path); err != nil {
				return err
			}
			continue
		}
		meta, data, err := w.spool.read(file.path)
		if err != nil {
			return err
		}
		if meta.Seq != file.seq {
			return fmt.Errorf("spool batch %s has seq %d, want %d", file.path, meta.Seq, file.seq)
		}
		if meta.Table != w.cfg.Table || meta.Signal != w.cfg.Signal {
			return fmt.Errorf("spool batch %d targets %s/%s, want %s/%s", meta.Seq, meta.Signal, meta.Table, w.cfg.Signal, w.cfg.Table)
		}
		if file.seq > maxSeq {
			maxSeq = file.seq
		}
		w.stats.pendingBatches.Add(1)
		w.stats.pendingBytes.Add(int64(len(data)))
		if err := w.insert(ctx, &queuedBatch{meta: meta, path: file.path, data: data, replayed: true}); err != nil {
			return fmt.Errorf("replay spooled batch %d: %w", meta.Seq, err)
		}
		w.stats.recoveredBatches.Add(1)
		w.stats.recoveredEvents.Add(uint64(meta.Rows))
	}
	w.seq.Store(maxSeq)
	return nil
}

// SubmitLogs validates, deduplicates, encodes and durably spools a log batch.
func (w *Writer) SubmitLogs(ctx context.Context, records []LogRecord) (SubmitResult, error) {
	if w.cfg.Signal != SignalLogs {
		return SubmitResult{}, ErrWrongSignal
	}
	return submitRecords(w, ctx, records)
}

// SubmitTraces validates, deduplicates, encodes and durably spools a span
// batch.
func (w *Writer) SubmitTraces(ctx context.Context, records []SpanRecord) (SubmitResult, error) {
	if w.cfg.Signal != SignalTraces {
		return SubmitResult{}, ErrWrongSignal
	}
	return submitRecords(w, ctx, records)
}

func submitRecords[T record](w *Writer, ctx context.Context, records []T) (SubmitResult, error) {
	if len(records) == 0 {
		return SubmitResult{}, nil
	}
	if err := w.beginSubmit(); err != nil {
		return SubmitResult{}, err
	}
	defer w.submitWG.Done()

	// Validate the whole batch before touching the dedupe window. A rejected
	// batch must not mark any event as seen.
	for index := range records {
		if err := records[index].validate(); err != nil {
			return SubmitResult{}, &encodeError{index: index, err: err}
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return SubmitResult{}, err
	}

	ingestedAt := time.Now().UTC()
	accepted := make([]int, 0, len(records))
	eventTimes := make([]time.Time, 0, len(records))
	duplicates := 0
	clamped := 0
	var minTimestamp, maxTimestamp time.Time

	for index := range records {
		identity := records[index].identity()
		key := dedupeKey{
			tenantID:  identity.tenantID,
			projectID: identity.projectID,
			signal:    w.cfg.Signal,
			eventID:   identity.eventID,
		}
		if !w.dedupe.add(key) {
			duplicates++
			continue
		}

		eventTime, wasClamped := clampTimestamp(records[index].eventTime(), ingestedAt, w.cfg.MaxSkew)
		if wasClamped {
			clamped++
		}
		if len(accepted) == 0 || eventTime.Before(minTimestamp) {
			minTimestamp = eventTime
		}
		if len(accepted) == 0 || eventTime.After(maxTimestamp) {
			maxTimestamp = eventTime
		}
		accepted = append(accepted, index)
		eventTimes = append(eventTimes, eventTime)
	}

	if len(accepted) == 0 {
		w.stats.duplicateEvents.Add(uint64(duplicates))
		return SubmitResult{Duplicates: duplicates}, nil
	}

	buffer := encodeAccepted(records, accepted, eventTimes, ingestedAt, w.cfg.EncodeWorkers)
	sum := sha256.Sum256(buffer)
	seq := w.seq.Add(1)
	meta := &batchMeta{
		Seq:                  seq,
		Table:                w.cfg.Table,
		Signal:               w.cfg.Signal,
		Rows:                 len(accepted),
		DedupeToken:          batchToken(w.cfg.Signal, seq, sum),
		MinTimestampUnixNano: minTimestamp.UnixNano(),
		MaxTimestampUnixNano: maxTimestamp.UnixNano(),
		DataSHA256:           hex.EncodeToString(sum[:]),
	}
	path, err := w.spool.write(meta, buffer)
	if err != nil {
		return SubmitResult{}, err
	}

	w.stats.submittedBatches.Add(1)
	w.stats.submittedEvents.Add(uint64(len(accepted)))
	w.stats.duplicateEvents.Add(uint64(duplicates))
	w.stats.clampedTimestamps.Add(uint64(clamped))
	w.stats.pendingBatches.Add(1)
	w.stats.pendingBytes.Add(int64(len(buffer)))

	result := SubmitResult{
		Seq:          seq,
		Rows:         len(accepted),
		Duplicates:   duplicates,
		Clamped:      clamped,
		SpooledBytes: len(buffer),
	}
	batch := &queuedBatch{meta: meta, path: path, data: buffer}
	select {
	case w.queue <- batch:
		return result, nil
	case <-w.fatalCh:
		return result, w.fatal()
	case <-w.stopCh:
		// Close started while the batch was being spooled. The batch is
		// durable; the next start replays it.
		return result, nil
	case <-ctx.Done():
		// The batch is already durable. The caller can retry the same events;
		// the event_id window and the stable token make the retry safe.
		return result, ctx.Err()
	}
}

func (w *Writer) worker() {
	defer w.wg.Done()
	for {
		select {
		case batch, ok := <-w.queue:
			if !ok {
				return
			}
			if err := w.insert(w.runCtx, batch); err != nil {
				if w.runCtx.Err() != nil {
					continue
				}
				w.setFatal(err)
				return
			}
		case <-w.stopCh:
			// Close waits for in-flight submitters and then closes the queue.
			// Drain everything that was already spooled before returning.
			for batch := range w.queue {
				if err := w.insert(w.runCtx, batch); err != nil {
					if w.runCtx.Err() != nil {
						continue
					}
					w.setFatal(err)
					return
				}
			}
			return
		}
	}
}

func (w *Writer) insert(ctx context.Context, batch *queuedBatch) error {
	attempts := 0
	for {
		result, err := w.inserter.Insert(ctx, InsertRequest{
			Table:       w.cfg.Table,
			Data:        batch.data,
			Rows:        batch.meta.Rows,
			DedupeToken: batch.meta.DedupeToken,
		})
		if err == nil {
			w.stats.insertedEvents.Add(result.WrittenRows)
			if err := w.spool.writeCheckpoint(batch.meta.Seq); err != nil {
				return fmt.Errorf("checkpoint batch %d: %w", batch.meta.Seq, err)
			}
			if err := w.spool.remove(batch.path); err != nil {
				return fmt.Errorf("remove spooled batch %d: %w", batch.meta.Seq, err)
			}
			acknowledgedAt := time.Now()
			lag := acknowledgedAt.Sub(time.Unix(0, batch.meta.MaxTimestampUnixNano))
			oldestLag := acknowledgedAt.Sub(time.Unix(0, batch.meta.MinTimestampUnixNano))
			w.stats.ackedBatches.Add(1)
			w.stats.observeLag(oldestLag)
			w.stats.pendingBatches.Add(-1)
			w.stats.pendingBytes.Add(-int64(len(batch.data)))
			if w.cfg.OnAck != nil {
				w.cfg.OnAck(Ack{
					Seq:            batch.meta.Seq,
					Rows:           batch.meta.Rows,
					WrittenRows:    result.WrittenRows,
					Lag:            lag,
					OldestLag:      oldestLag,
					AcknowledgedAt: acknowledgedAt,
					Recovered:      batch.replayed,
				})
			}
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
		if IsPermanent(err) {
			destination, moveErr := w.spool.moveToDLQ(batch.path, err)
			if moveErr != nil {
				return fmt.Errorf("move batch %d to dlq: %w", batch.meta.Seq, moveErr)
			}
			if err := w.spool.writeCheckpoint(batch.meta.Seq); err != nil {
				return fmt.Errorf("checkpoint dlq batch %d: %w", batch.meta.Seq, err)
			}
			w.stats.dlqEvents.Add(uint64(batch.meta.Rows))
			w.stats.pendingBatches.Add(-1)
			w.stats.pendingBytes.Add(-int64(len(batch.data)))
			w.cfg.Logger.Error("clickhouse batch moved to dlq",
				"seq", batch.meta.Seq,
				"rows", batch.meta.Rows,
				"path", destination,
				"error", err,
			)
			return nil
		}

		attempts++
		w.stats.retryAttempts.Add(1)
		if attempts == 1 {
			w.stats.retriedBatches.Add(1)
		}
		if w.cfg.MaxRetries > 0 && attempts >= w.cfg.MaxRetries {
			return fmt.Errorf("insert batch %d failed after %d attempts: %w", batch.meta.Seq, attempts, err)
		}
		delay := retryDelay(w.cfg.RetryBackoff, attempts)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Close stops accepting submissions, drains the queue and waits for the
// in-flight batch. If ctx expires, the worker is canceled and any unacked
// batch remains in the spool for the next start.
func (w *Writer) Close(ctx context.Context) error {
	w.lifecycleMu.Lock()
	if w.closed.Swap(true) {
		w.lifecycleMu.Unlock()
		return w.fatal()
	}
	close(w.stopCh)
	w.lifecycleMu.Unlock()

	done := make(chan struct{})
	go func() {
		w.submitWG.Wait()
		close(w.queue)
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		w.cancel()
		return w.fatal()
	case <-ctx.Done():
		w.cancel()
		<-done
		return ctx.Err()
	}
}

func (w *Writer) beginSubmit() error {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	if w.closed.Load() {
		return ErrClosed
	}
	w.submitWG.Add(1)
	return nil
}

// Stats returns a point-in-time snapshot.
func (w *Writer) Stats() Stats {
	return w.stats.snapshot(w.cfg.Table, w.cfg.Signal, w.dedupe, w.cfg.SpoolDir, w.fatalString())
}

func (w *Writer) checkOpen() error {
	if w.closed.Load() {
		return ErrClosed
	}
	select {
	case <-w.fatalCh:
		return w.fatal()
	default:
		return nil
	}
}

func (w *Writer) setFatal(err error) {
	w.fatalOnce.Do(func() {
		w.fatalMu.Lock()
		w.fatalErr = err
		w.fatalMu.Unlock()
		close(w.fatalCh)
	})
}

func (w *Writer) fatal() error {
	w.fatalMu.Lock()
	defer w.fatalMu.Unlock()
	if w.fatalErr == nil {
		return nil
	}
	return w.fatalErr
}

func (w *Writer) fatalString() string {
	err := w.fatal()
	if err == nil {
		return ""
	}
	return err.Error()
}

func clampTimestamp(eventTime, ingestedAt time.Time, maxSkew time.Duration) (time.Time, bool) {
	if maxSkew <= 0 {
		return eventTime, false
	}
	delta := eventTime.Sub(ingestedAt)
	if delta > maxSkew || delta < -maxSkew {
		return ingestedAt, true
	}
	return eventTime, false
}

func batchToken(signal string, seq uint64, dataHash [sha256.Size]byte) string {
	return fmt.Sprintf("%s-%d-%s", signal, seq, hex.EncodeToString(dataHash[:16]))
}

func retryDelay(base time.Duration, attempt int) time.Duration {
	if attempt > 6 {
		attempt = 6
	}
	return base * time.Duration(1<<(attempt-1))
}

// encodeAccepted splits the accepted rows across workers. The maps and other
// read-only fields in the records are shared, so the workers only read.
func encodeAccepted[T record](records []T, accepted []int, eventTimes []time.Time, ingestedAt time.Time, workers int) []byte {
	if workers < 1 {
		workers = 1
	}
	if workers > len(accepted) {
		workers = len(accepted)
	}
	if workers <= 1 || len(accepted) < 4096 {
		buffer := make([]byte, 0, len(accepted)*256)
		for position, index := range accepted {
			buffer = records[index].appendRowBinary(buffer, eventTimes[position], ingestedAt)
		}
		return buffer
	}

	chunk := (len(accepted) + workers - 1) / workers
	chunkCount := (len(accepted) + chunk - 1) / chunk
	chunks := make([][]byte, chunkCount)
	var group sync.WaitGroup
	for slot := 0; slot < chunkCount; slot++ {
		begin := slot * chunk
		end := begin + chunk
		if end > len(accepted) {
			end = len(accepted)
		}
		group.Add(1)
		go func(slot, begin, end int) {
			defer group.Done()
			buffer := make([]byte, 0, (end-begin)*256)
			for position := begin; position < end; position++ {
				buffer = records[accepted[position]].appendRowBinary(buffer, eventTimes[position], ingestedAt)
			}
			chunks[slot] = buffer
		}(slot, begin, end)
	}
	group.Wait()
	total := 0
	for _, buffer := range chunks {
		total += len(buffer)
	}
	combined := make([]byte, 0, total)
	for _, buffer := range chunks {
		combined = append(combined, buffer...)
	}
	return combined
}
