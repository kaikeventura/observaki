package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/observaki/observaki/internal/id"
	"github.com/observaki/observaki/internal/ingest"
	"github.com/observaki/observaki/internal/ingestpb"
	"google.golang.org/protobuf/proto"
)

const (
	defaultQueueDepth     = 8192
	publishAttempts       = 3
	publishRetryBackoff   = 50 * time.Millisecond
	maxEnvelopeOverhead   = 4096
	defaultNATSName       = "observaki-gateway"
	defaultReconnectDelay = time.Second
)

// Config configures the NATS JetStream WAL adapter.
type Config struct {
	URL             string
	CredentialsFile string
	SubjectPrefix   string
	StreamPrefix    string
	Shards          int
	StreamReplicas  int
	StreamStorage   string
	StreamMaxAge    time.Duration
	StreamMaxBytes  int64
	BatchMaxEvents  int
	BatchMaxBytes   int
	BatchLinger     time.Duration
	ConnectTimeout  time.Duration
	PublishTimeout  time.Duration
	MaxPendingBytes int64
	PublishWorkers  int
	Logger          *slog.Logger
}

func (c Config) withDefaults() Config {
	if c.URL == "" {
		c.URL = "nats://127.0.0.1:4222"
	}
	if c.SubjectPrefix == "" {
		c.SubjectPrefix = "observaki.ingest.v1"
	}
	if c.StreamPrefix == "" {
		c.StreamPrefix = "OBSERVAKI"
	}
	if c.Shards <= 0 {
		c.Shards = 32
	}
	if c.StreamReplicas <= 0 {
		c.StreamReplicas = 1
	}
	if c.StreamStorage == "" {
		c.StreamStorage = "file"
	}
	if c.StreamMaxAge <= 0 {
		c.StreamMaxAge = 24 * time.Hour
	}
	if c.StreamMaxBytes <= 0 {
		c.StreamMaxBytes = 32 << 30
	}
	if c.BatchMaxEvents <= 0 {
		c.BatchMaxEvents = 100_000
	}
	if c.BatchMaxBytes <= 0 {
		c.BatchMaxBytes = 4 << 20
	}
	if c.BatchLinger <= 0 {
		c.BatchLinger = 5 * time.Millisecond
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.PublishTimeout <= 0 {
		c.PublishTimeout = 5 * time.Second
	}
	if c.MaxPendingBytes <= 0 {
		c.MaxPendingBytes = 256 << 20
	}
	if c.PublishWorkers <= 0 {
		c.PublishWorkers = 8
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.DiscardHandler)
	}
	return c
}

func (c Config) validate() error {
	if c.Shards < 1 || c.Shards > 1024 {
		return fmt.Errorf("nats: shards must be between 1 and 1024")
	}
	if c.BatchMaxEvents < 1 || c.BatchMaxBytes < 1 {
		return fmt.Errorf("nats: batch limits must be positive")
	}
	if c.BatchLinger <= 0 || c.PublishTimeout <= 0 || c.ConnectTimeout <= 0 {
		return fmt.Errorf("nats: timeouts must be positive")
	}
	if c.MaxPendingBytes < 1 {
		return fmt.Errorf("nats: max pending bytes must be positive")
	}
	if c.PublishWorkers < 1 {
		return fmt.Errorf("nats: publish workers must be positive")
	}
	switch c.StreamStorage {
	case "file", "memory":
	default:
		return fmt.Errorf("nats: stream storage %q must be file or memory", c.StreamStorage)
	}
	return nil
}

type pending struct {
	request    *ingestpb.Request
	events     int
	size       int
	subject    string
	tenantID   string
	projectID  string
	configVer  string
	generation int64
	signal     ingest.Signal
	shard      int
	done       chan error
}

type group struct {
	key           string
	subject       string
	tenantID      string
	projectID     string
	configVersion string
	generation    int64
	signal        ingest.Signal
	shard         int
	items         []*pending
	events        int
	bytes         int
	created       time.Time
}

// Sink is the JetStream-backed WAL. It implements ingest.Sink,
// ingest.SinkHealth and ingest.SinkStatsProvider.
type Sink struct {
	cfg        Config
	nc         *natsgo.Conn
	js         jetstream.JetStream
	logger     *slog.Logger
	maxPayload int

	inbox chan *pending
	jobs  chan *group

	sendMu      sync.RWMutex
	closedFlag  atomic.Bool
	closeOnce   sync.Once
	closeErr    error
	batcherDone chan struct{}
	workersDone chan struct{}

	pendingBytes  atomic.Int64
	pendingEvents atomic.Int64
	inFlight      atomic.Int64

	publishedBatches atomic.Uint64
	publishedEvents  atomic.Uint64
	publishErrors    atomic.Uint64
	publishRetries   atomic.Uint64
	backpressure     atomic.Uint64

	window *publishWindow
}

// Open connects to NATS, ensures the per-signal JetStream streams and starts
// the batching publishers. It fails closed when the WAL cannot be initialized.
func Open(ctx context.Context, cfg Config) (*Sink, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := cfg.Logger

	options := []natsgo.Option{
		natsgo.Name(defaultNATSName),
		natsgo.Timeout(cfg.ConnectTimeout),
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectWait(defaultReconnectDelay),
		// JetStream publishes are synchronous and acknowledged; buffering
		// them in the client during an outage would hide backpressure.
		natsgo.ReconnectBufSize(-1),
		natsgo.ErrorHandler(func(_ *natsgo.Conn, _ *natsgo.Subscription, err error) {
			logger.Warn("nats error", "error", err)
		}),
		natsgo.DisconnectErrHandler(func(_ *natsgo.Conn, err error) {
			logger.Warn("nats disconnected", "error", err)
		}),
		natsgo.ReconnectHandler(func(nc *natsgo.Conn) {
			logger.Info("nats reconnected", "url", nc.ConnectedUrl())
		}),
	}
	if cfg.CredentialsFile != "" {
		options = append(options, natsgo.UserCredentials(cfg.CredentialsFile))
	}
	nc, err := natsgo.Connect(cfg.URL, options...)
	if err != nil {
		return nil, fmt.Errorf("nats: connect to %q: %w", cfg.URL, err)
	}

	sink := &Sink{
		cfg:         cfg,
		nc:          nc,
		logger:      logger,
		inbox:       make(chan *pending, defaultQueueDepth),
		jobs:        make(chan *group, max(64, cfg.PublishWorkers*4)),
		batcherDone: make(chan struct{}),
		workersDone: make(chan struct{}),
		window:      &publishWindow{},
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: create JetStream context: %w", err)
	}
	sink.js = js

	if payload := nc.MaxPayload(); payload > 0 {
		sink.maxPayload = int(payload) - maxEnvelopeOverhead
	}
	if sink.maxPayload < 16<<10 {
		nc.Close()
		return nil, fmt.Errorf("nats: server max payload %d is too small", nc.MaxPayload())
	}

	streamCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := sink.clampStreamBudget(streamCtx); err != nil {
		nc.Close()
		return nil, err
	}
	if err := sink.ensureStreams(streamCtx); err != nil {
		nc.Close()
		return nil, err
	}

	var workers sync.WaitGroup
	workers.Add(cfg.PublishWorkers)
	for worker := 0; worker < cfg.PublishWorkers; worker++ {
		go func() {
			defer workers.Done()
			for job := range sink.jobs {
				sink.publishGroup(job)
			}
		}()
	}
	go func() {
		workers.Wait()
		close(sink.workersDone)
	}()
	go sink.runBatcher()

	logger.Info(
		"nats WAL ready",
		"url", nc.ConnectedUrl(),
		"subject_prefix", cfg.SubjectPrefix,
		"shards", cfg.Shards,
		"batch_max_events", cfg.BatchMaxEvents,
		"batch_max_bytes", cfg.BatchMaxBytes,
		"batch_linger", cfg.BatchLinger,
		"max_pending_bytes", cfg.MaxPendingBytes,
		"max_payload", sink.maxPayload,
	)
	return sink, nil
}

func (s *Sink) clampStreamBudget(ctx context.Context) error {
	account, err := s.js.AccountInfo(ctx)
	if err != nil {
		return fmt.Errorf("nats: read account limits: %w", err)
	}
	maxBytes := account.Limits.MaxStore
	if s.cfg.StreamStorage == "memory" {
		maxBytes = account.Limits.MaxMemory
	}
	if maxBytes <= 0 {
		return nil
	}
	perStream := (maxBytes / 3) * 8 / 10
	if s.cfg.StreamMaxBytes > perStream {
		s.logger.Warn(
			"nats stream max bytes clamped to the account budget",
			"configured", s.cfg.StreamMaxBytes,
			"account_limit", maxBytes,
			"per_stream", perStream,
		)
		s.cfg.StreamMaxBytes = perStream
	}
	return nil
}

func (s *Sink) ensureStreams(ctx context.Context) error {
	signals := []ingest.Signal{ingest.SignalTraces, ingest.SignalLogs, ingest.SignalMetrics}
	for _, signal := range signals {
		subject := streamSubject(s.cfg.SubjectPrefix, signal)
		config := jetstream.StreamConfig{
			Name:        streamName(s.cfg.StreamPrefix, signal),
			Description: "Observaki ingest WAL (" + signal.String() + ")",
			Subjects:    []string{subject},
			Retention:   jetstream.LimitsPolicy,
			Discard:     jetstream.DiscardNew,
			MaxAge:      s.cfg.StreamMaxAge,
			MaxBytes:    s.cfg.StreamMaxBytes,
			// The duplicate window only covers publisher retries of the same
			// envelope. Event-level dedupe is by event_id downstream.
			Duplicates: 2 * time.Minute,
			Storage:    storageType(s.cfg.StreamStorage),
			Replicas:   s.cfg.StreamReplicas,
		}
		existing, lookupErr := s.js.Stream(ctx, config.Name)
		if lookupErr == nil {
			info, err := existing.Info(ctx)
			if err != nil {
				return fmt.Errorf("nats: inspect existing stream %s: %w", config.Name, err)
			}
			if !containsSubject(info.Config.Subjects, subject) {
				return fmt.Errorf(
					"nats: stream %s exists without the expected subject %q (found %v)",
					config.Name, subject, info.Config.Subjects,
				)
			}
			s.logger.Info(
				"nats stream reused",
				"stream", config.Name,
				"messages", info.State.Msgs,
				"bytes", info.State.Bytes,
				"max_bytes", info.Config.MaxBytes,
			)
			continue
		}
		if !errors.Is(lookupErr, jetstream.ErrStreamNotFound) {
			return fmt.Errorf("nats: look up stream %s: %w", config.Name, lookupErr)
		}
		if _, err := s.js.CreateStream(ctx, config); err != nil {
			return fmt.Errorf("nats: create stream %s: %w", config.Name, err)
		}
		s.logger.Info("nats stream created", "stream", config.Name, "subject", subject)
	}
	return nil
}

func containsSubject(subjects []string, want string) bool {
	for _, subject := range subjects {
		if subject == want {
			return true
		}
	}
	return false
}

func storageType(value string) jetstream.StorageType {
	if value == "memory" {
		return jetstream.MemoryStorage
	}
	return jetstream.FileStorage
}

// Consume implements ingest.Sink. It returns only after JetStream confirms the
// envelope that contains the request (AD-4).
func (s *Sink) Consume(ctx context.Context, batch *ingest.Batch) (ingest.Result, error) {
	if batch == nil || batch.Events() == 0 {
		return ingest.Result{}, nil
	}
	if batch.TenantID == "" || batch.ProjectID == "" {
		return ingest.Result{}, fmt.Errorf("%w: tenant_id and project_id are required", ErrInvalidEnvelope)
	}
	if batch.RequestID == "" {
		batch.RequestID = id.NewUUIDv7()
	}
	events := batch.Events()
	payload, err := marshalPayload(batch)
	if err != nil {
		return ingest.Result{}, err
	}
	eventIDs := ingest.SynthesizeEventIDs(batch)
	if len(eventIDs) != events {
		return ingest.Result{}, fmt.Errorf("nats: synthesized %d event ids for %d events", len(eventIDs), events)
	}
	shard := shardFor(batch, batch.RequestID, s.cfg.Shards)
	subject := subjectFor(s.cfg.SubjectPrefix, batch, shard)

	request := &ingestpb.Request{
		RequestId:          batch.RequestID,
		OtlpProtobuf:       payload,
		EventIds:           eventIDs,
		ReceivedAtUnixNano: batch.ReceivedAt.UnixNano(),
	}
	if batch.ReceivedAt.IsZero() {
		request.ReceivedAtUnixNano = time.Now().UnixNano()
	}
	item := &pending{
		request:    request,
		events:     events,
		size:       requestSize(request),
		subject:    subject,
		tenantID:   batch.TenantID,
		projectID:  batch.ProjectID,
		configVer:  batch.ConfigVersion,
		generation: batch.ConfigGeneration,
		signal:     batch.Signal,
		shard:      shard,
		done:       make(chan error, 1),
	}
	if !s.reserve(item.size, events) {
		s.backpressure.Add(1)
		return ingest.Result{}, fmt.Errorf("%w: %d pending bytes", ingest.ErrBackpressure, s.pendingBytes.Load())
	}
	if err := s.enqueue(item); err != nil {
		s.release(item)
		return ingest.Result{}, err
	}

	select {
	case err := <-item.done:
		if err != nil {
			return ingest.Result{}, err
		}
		return ingest.Result{Accepted: events}, nil
	case <-ctx.Done():
		// The envelope is already in the WAL pipeline and may still publish.
		// The request is not confirmed, so a client retry is safe and the
		// downstream event_id dedupe absorbs any duplicate.
		return ingest.Result{}, ctx.Err()
	}
}

func marshalPayload(batch *ingest.Batch) ([]byte, error) {
	var (
		payload []byte
		err     error
	)
	switch batch.Signal {
	case ingest.SignalTraces:
		if batch.Traces == nil {
			return nil, fmt.Errorf("%w: traces payload is missing", ErrInvalidEnvelope)
		}
		payload, err = proto.Marshal(batch.Traces)
	case ingest.SignalLogs:
		if batch.Logs == nil {
			return nil, fmt.Errorf("%w: logs payload is missing", ErrInvalidEnvelope)
		}
		payload, err = proto.Marshal(batch.Logs)
	case ingest.SignalMetrics:
		if batch.Metrics == nil {
			return nil, fmt.Errorf("%w: metrics payload is missing", ErrInvalidEnvelope)
		}
		payload, err = proto.Marshal(batch.Metrics)
	default:
		return nil, fmt.Errorf("%w: unsupported signal %q", ErrInvalidEnvelope, batch.Signal)
	}
	if err != nil {
		return nil, fmt.Errorf("marshal OTLP payload: %w", err)
	}
	return payload, nil
}

func requestSize(request *ingestpb.Request) int {
	size := len(request.GetOtlpProtobuf()) + 128
	for _, eventID := range request.GetEventIds() {
		size += len(eventID) + 2
	}
	return size
}

func (s *Sink) reserve(size int, events int) bool {
	if s.pendingBytes.Add(int64(size)) > s.cfg.MaxPendingBytes {
		s.pendingBytes.Add(-int64(size))
		return false
	}
	s.pendingEvents.Add(int64(events))
	return true
}

func (s *Sink) release(item *pending) {
	s.pendingBytes.Add(-int64(item.size))
	s.pendingEvents.Add(-int64(item.events))
}

func (s *Sink) enqueue(item *pending) error {
	s.sendMu.RLock()
	defer s.sendMu.RUnlock()
	if s.closedFlag.Load() {
		return ingest.ErrSinkClosed
	}
	select {
	case s.inbox <- item:
		return nil
	default:
		s.backpressure.Add(1)
		return fmt.Errorf("%w: queue is full", ingest.ErrBackpressure)
	}
}

func (s *Sink) runBatcher() {
	defer close(s.batcherDone)
	ticker := time.NewTicker(s.cfg.BatchLinger)
	defer ticker.Stop()

	groups := make(map[string]*group)
	flush := func(g *group) {
		delete(groups, g.key)
		s.inFlight.Add(1)
		s.jobs <- g
	}

	for {
		select {
		case item, ok := <-s.inbox:
			if !ok {
				for _, group := range groups {
					flush(group)
				}
				close(s.jobs)
				return
			}
			key := item.subject + "|" + item.configVer
			current := groups[key]
			if current == nil {
				current = &group{
					key:           key,
					subject:       item.subject,
					tenantID:      item.tenantID,
					projectID:     item.projectID,
					configVersion: item.configVer,
					generation:    item.generation,
					signal:        item.signal,
					shard:         item.shard,
					created:       time.Now(),
				}
				groups[key] = current
			}
			current.items = append(current.items, item)
			current.events += item.events
			current.bytes += item.size
			if current.events >= s.cfg.BatchMaxEvents || current.bytes >= s.cfg.BatchMaxBytes {
				flush(current)
			}
		case <-ticker.C:
			now := time.Now()
			for _, current := range groups {
				if now.Sub(current.created) >= s.cfg.BatchLinger {
					flush(current)
				}
			}
		}
	}
}

func (s *Sink) publishGroup(g *group) {
	defer s.inFlight.Add(-1)
	s.publishSubset(g, g.items)
}

func (s *Sink) publishSubset(g *group, items []*pending) {
	if len(items) == 0 {
		return
	}
	envelope := &ingestpb.Envelope{
		SchemaVersion:     SchemaVersion,
		MessageId:         id.NewUUIDv7(),
		TenantId:          g.tenantID,
		ProjectId:         g.projectID,
		ConfigVersion:     g.configVersion,
		ConfigGeneration:  g.generation,
		AuthzEpoch:        g.generation,
		Signal:            g.signal.String(),
		Shard:             uint32(g.shard),
		CreatedAtUnixNano: time.Now().UnixNano(),
		Requests:          make([]*ingestpb.Request, 0, len(items)),
	}
	for _, item := range items {
		envelope.Requests = append(envelope.Requests, item.request)
	}
	data, err := EncodeEnvelope(envelope)
	if err != nil {
		s.publishErrors.Add(1)
		s.finish(items, err)
		return
	}
	if len(data) > s.maxPayload {
		if len(items) == 1 {
			s.publishErrors.Add(1)
			s.finish(items, fmt.Errorf("%w: %d bytes", ErrEnvelopeTooLarge, len(data)))
			return
		}
		middle := len(items) / 2
		s.publishSubset(g, items[:middle])
		s.publishSubset(g, items[middle:])
		return
	}

	for attempt := 1; attempt <= publishAttempts; attempt++ {
		started := time.Now()
		err = s.publish(g.subject, envelope, data)
		s.window.observe(time.Since(started))
		if err == nil {
			s.publishedBatches.Add(1)
			for _, item := range items {
				s.publishedEvents.Add(uint64(item.events))
			}
			s.finish(items, nil)
			return
		}
		if attempt < publishAttempts {
			s.publishRetries.Add(1)
			time.Sleep(publishRetryBackoff * time.Duration(attempt))
		}
	}
	s.publishErrors.Add(1)
	s.logger.Warn(
		"nats WAL publish failed",
		"subject", g.subject,
		"tenant_id", g.tenantID,
		"project_id", g.projectID,
		"signal", g.signal.String(),
		"requests", len(items),
		"bytes", len(data),
		"error", err,
	)
	s.finish(items, err)
}

func (s *Sink) publish(subject string, envelope *ingestpb.Envelope, data []byte) error {
	message := natsgo.NewMsg(subject)
	message.Data = data
	message.Header.Set(EncodingHeader, EncodingZstd)
	message.Header.Set(SchemaHeader, fmt.Sprintf("%d", envelope.GetSchemaVersion()))
	message.Header.Set(TenantHeader, envelope.GetTenantId())
	message.Header.Set(ProjectHeader, envelope.GetProjectId())
	message.Header.Set(GenerationHeader, fmt.Sprintf("%d", envelope.GetAuthzEpoch()))
	message.Header.Set(SignalHeader, envelope.GetSignal())
	message.Header.Set(ShardHeader, fmt.Sprintf("%d", envelope.GetShard()))
	message.Header.Set(RequestCountHeader, fmt.Sprintf("%d", len(envelope.GetRequests())))

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.PublishTimeout)
	defer cancel()
	_, err := s.js.PublishMsg(ctx, message, jetstream.WithMsgID(envelope.GetMessageId()))
	if err != nil {
		return fmt.Errorf("jetstream publish: %w", err)
	}
	return nil
}

func (s *Sink) finish(items []*pending, err error) {
	for _, item := range items {
		s.release(item)
		item.done <- err
	}
}

// Healthy implements ingest.SinkHealth.
func (s *Sink) Healthy() error {
	if s.closedFlag.Load() {
		return ingest.ErrSinkClosed
	}
	if s.nc == nil || !s.nc.IsConnected() {
		return fmt.Errorf("%w: nats is disconnected", ingest.ErrBackpressure)
	}
	return nil
}

func (s *Sink) isClosed() bool { return s.closedFlag.Load() }

// Close stops accepting requests, flushes every buffered group, waits for
// in-flight publishes and closes the NATS connection. It is idempotent.
func (s *Sink) Close() error {
	s.closeOnce.Do(func() {
		s.sendMu.Lock()
		s.closedFlag.Store(true)
		close(s.inbox)
		s.sendMu.Unlock()

		<-s.batcherDone
		<-s.workersDone

		if err := s.nc.Drain(); err != nil && !errors.Is(err, natsgo.ErrConnectionClosed) {
			s.closeErr = fmt.Errorf("nats: drain connection: %w", err)
		}
		s.nc.Close()
	})
	return s.closeErr
}

// StreamName returns the JetStream stream name for a signal.
func StreamName(prefix string, signal ingest.Signal) string {
	return streamName(prefix, signal)
}
