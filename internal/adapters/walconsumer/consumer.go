package walconsumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	natsadapter "github.com/observaki/observaki/internal/adapters/nats"
	"github.com/observaki/observaki/internal/ingest"
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	otlplogspb "go.opentelemetry.io/proto/otlp/logs/v1"
	otlpmetricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlptracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// Sink is the downstream storage port. The production implementation is
// Storage; tests use an in-memory fake.
type Sink interface {
	WriteLogs(ctx context.Context, tenantID, projectID string, generation, authzEpoch uint64, request *logspb.ExportLogsServiceRequest, eventIDs []string, receivedAt time.Time) (WriteResult, error)
	WriteTraces(ctx context.Context, tenantID, projectID string, generation, authzEpoch uint64, request *tracepb.ExportTraceServiceRequest, eventIDs []string, receivedAt time.Time) (WriteResult, error)
	WriteMetrics(ctx context.Context, tenantID, projectID string, generation, authzEpoch uint64, request *metricpb.ExportMetricsServiceRequest, eventIDs []string, receivedAt time.Time) (WriteResult, error)
}

// Config configures the WAL consumer.
type Config struct {
	URL             string
	CredentialsFile string
	SubjectPrefix   string
	StreamPrefix    string
	ConsumerPrefix  string
	DLQSubject      string
	DLQStream       string
	DLQMaxAge       time.Duration
	DLQMaxBytes     int64

	BatchMaxEvents   int
	BatchMaxBytes    int
	FetchMaxMessages int
	MaxAckPending    int
	AckWait          time.Duration
	BatchLinger      time.Duration
	ConnectTimeout   time.Duration
	RetryBackoff     time.Duration
	MaxRetryBackoff  time.Duration
	Logger           *slog.Logger
}

const (
	defaultConsumerPrefix = "observaki-writer"
	defaultDLQSubject     = "observaki.dlq.v1"
	defaultDLQStream      = "OBSERVAKI_DLQ"
)

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
	if c.ConsumerPrefix == "" {
		c.ConsumerPrefix = defaultConsumerPrefix
	}
	if c.DLQSubject == "" {
		c.DLQSubject = defaultDLQSubject
	}
	if c.DLQStream == "" {
		c.DLQStream = defaultDLQStream
	}
	if c.DLQMaxAge <= 0 {
		c.DLQMaxAge = 7 * 24 * time.Hour
	}
	if c.DLQMaxBytes <= 0 {
		c.DLQMaxBytes = 8 << 30
	}
	if c.BatchMaxEvents <= 0 {
		c.BatchMaxEvents = 50_000
	}
	if c.BatchMaxBytes <= 0 {
		c.BatchMaxBytes = 64 << 20
	}
	if c.FetchMaxMessages <= 0 {
		c.FetchMaxMessages = 64
	}
	if c.MaxAckPending <= 0 {
		c.MaxAckPending = 4096
	}
	if c.AckWait <= 0 {
		c.AckWait = 30 * time.Minute
	}
	if c.BatchLinger <= 0 {
		c.BatchLinger = time.Second
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = 500 * time.Millisecond
	}
	if c.MaxRetryBackoff <= 0 {
		c.MaxRetryBackoff = 30 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.DiscardHandler)
	}
	return c
}

func (c Config) validate() error {
	if c.BatchMaxEvents < 1 || c.BatchMaxBytes < 1024 {
		return errors.New("walconsumer: batch limits must be positive")
	}
	if c.FetchMaxMessages < 1 || c.MaxAckPending < 1 {
		return errors.New("walconsumer: fetch and ack limits must be positive")
	}
	if c.AckWait < time.Second || c.BatchLinger < 10*time.Millisecond {
		return errors.New("walconsumer: ack wait and linger are too small")
	}
	if c.RetryBackoff <= 0 || c.MaxRetryBackoff < c.RetryBackoff {
		return errors.New("walconsumer: retry backoff is invalid")
	}
	return nil
}

// Consumer owns one durable JetStream consumer per signal and acknowledges a
// message only after its group was written (or moved to the DLQ).
type Consumer struct {
	cfg     Config
	logger  *slog.Logger
	nc      *nats.Conn
	js      jetstream.JetStream
	sink    Sink
	metrics *writerMetrics

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	consumers map[ingest.Signal]jetstream.Consumer

	closeOnce sync.Once
	closeErr  error
	healthy   atomic.Bool
}

// Open connects to NATS, validates the three WAL streams, creates the durable
// consumers and starts the fetch workers.
func Open(ctx context.Context, cfg Config, sink Sink) (*Consumer, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if sink == nil {
		return nil, errors.New("walconsumer: sink is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logger := cfg.Logger
	options := []nats.Option{
		nats.Name("observaki-writer"),
		nats.Timeout(cfg.ConnectTimeout),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			logger.Warn("nats writer error", "error", err)
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			logger.Warn("nats writer disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("nats writer reconnected", "url", nc.ConnectedUrl())
		}),
	}
	if cfg.CredentialsFile != "" {
		options = append(options, nats.UserCredentials(cfg.CredentialsFile))
	}
	nc, err := nats.Connect(cfg.URL, options...)
	if err != nil {
		return nil, fmt.Errorf("walconsumer: connect to %q: %w", cfg.URL, err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("walconsumer: create JetStream context: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		cfg:       cfg,
		logger:    logger,
		nc:        nc,
		js:        js,
		sink:      sink,
		metrics:   newWriterMetrics(),
		ctx:       runCtx,
		cancel:    cancel,
		consumers: make(map[ingest.Signal]jetstream.Consumer, 3),
	}
	consumer.metrics.connected.Store(true)
	consumer.healthy.Store(true)

	setupCtx, cancelSetup := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancelSetup()
	if err := consumer.ensureDLQStream(setupCtx); err != nil {
		cancel()
		nc.Close()
		return nil, err
	}
	for _, signal := range metricSignals {
		stream, err := js.Stream(setupCtx, natsadapter.StreamName(cfg.StreamPrefix, signal))
		if err != nil {
			cancel()
			nc.Close()
			if errors.Is(err, jetstream.ErrStreamNotFound) {
				return nil, fmt.Errorf("walconsumer: WAL stream for %s is missing; start the gateway first", signal)
			}
			return nil, fmt.Errorf("walconsumer: look up WAL stream for %s: %w", signal, err)
		}
		jsConsumer, err := stream.CreateOrUpdateConsumer(setupCtx, jetstream.ConsumerConfig{
			Durable:       consumerDurable(cfg.ConsumerPrefix, signal),
			Description:   "Observaki WAL writer (" + signal.String() + ")",
			AckPolicy:     jetstream.AckExplicitPolicy,
			DeliverPolicy: jetstream.DeliverAllPolicy,
			FilterSubject: natsadapter.StreamSubject(cfg.SubjectPrefix, signal),
			MaxAckPending: cfg.MaxAckPending,
			AckWait:       cfg.AckWait,
			MaxDeliver:    -1,
			ReplayPolicy:  jetstream.ReplayInstantPolicy,
		})
		if err != nil {
			cancel()
			nc.Close()
			return nil, fmt.Errorf("walconsumer: create consumer for %s: %w", signal, err)
		}
		consumer.consumers[signal] = jsConsumer
	}

	consumer.metrics.consumerRunning.Store(true)
	for _, signal := range metricSignals {
		consumer.wg.Add(1)
		go consumer.runSignal(runCtx, signal, consumer.consumers[signal])
	}
	consumer.wg.Add(1)
	go consumer.pollStatus(runCtx)

	logger.Info(
		"WAL writer consumers ready",
		"url", nc.ConnectedUrl(),
		"consumer_prefix", cfg.ConsumerPrefix,
		"batch_max_events", cfg.BatchMaxEvents,
		"batch_max_bytes", cfg.BatchMaxBytes,
		"ack_wait", cfg.AckWait,
		"dlq_subject", cfg.DLQSubject,
	)
	return consumer, nil
}

func (c *Consumer) ensureDLQStream(ctx context.Context) error {
	if _, err := c.js.Stream(ctx, c.cfg.DLQStream); err == nil {
		return nil
	} else if !errors.Is(err, jetstream.ErrStreamNotFound) {
		return fmt.Errorf("walconsumer: look up dlq stream: %w", err)
	}
	_, err := c.js.CreateStream(ctx, jetstream.StreamConfig{
		Name:        c.cfg.DLQStream,
		Description: "Observaki permanent ingestion rejections",
		Subjects:    []string{c.cfg.DLQSubject + ".>"},
		Retention:   jetstream.LimitsPolicy,
		MaxAge:      c.cfg.DLQMaxAge,
		MaxBytes:    c.cfg.DLQMaxBytes,
		Storage:     jetstream.FileStorage,
	})
	if err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		return fmt.Errorf("walconsumer: create dlq stream: %w", err)
	}
	return nil
}

func consumerDurable(prefix string, signal ingest.Signal) string {
	return prefix + "_" + signal.String()
}

type pendingRequest struct {
	receivedAt time.Time
	eventIDs   []string
	logs       *logspb.ExportLogsServiceRequest
	traces     *tracepb.ExportTraceServiceRequest
	metrics    *metricpb.ExportMetricsServiceRequest
	bytes      int
}

type pendingMessage struct {
	msg        jetstream.Msg
	subject    string
	headers    nats.Header
	data       []byte
	tenantID   string
	projectID  string
	generation uint64
	authzEpoch uint64
	signal     ingest.Signal
	events     int
	receivedAt time.Time
	requests   []*pendingRequest
}

type pendingGroup struct {
	key        string
	tenantID   string
	projectID  string
	generation uint64
	authzEpoch uint64
	signal     ingest.Signal
	messages   []*pendingMessage
	logs       []*otlplogspb.ResourceLogs
	traces     []*otlptracepb.ResourceSpans
	metrics    []*otlpmetricpb.ResourceMetrics
	eventIDs   []string
	events     int
	bytes      int
	oldest     time.Time
}

type pendingBatch struct {
	groups map[string]*pendingGroup
}

func newPendingBatch() *pendingBatch {
	return &pendingBatch{groups: make(map[string]*pendingGroup)}
}

func (b *pendingBatch) group(message *pendingMessage) *pendingGroup {
	key := message.tenantID + "\xff" + message.projectID + "\xff" +
		strconv.FormatUint(message.generation, 10) + "\xff" +
		strconv.FormatUint(message.authzEpoch, 10) + "\xff" + message.signal.String()
	group := b.groups[key]
	if group == nil {
		group = &pendingGroup{
			key:        key,
			tenantID:   message.tenantID,
			projectID:  message.projectID,
			generation: message.generation,
			authzEpoch: message.authzEpoch,
			signal:     message.signal,
		}
		b.groups[key] = group
	}
	return group
}

func (c *Consumer) runSignal(ctx context.Context, signal ingest.Signal, consumer jetstream.Consumer) {
	defer c.wg.Done()
	for {
		if ctx.Err() != nil {
			return
		}
		c.metrics.connected.Store(c.nc.IsConnected())
		batch, err := consumer.Fetch(c.cfg.FetchMaxMessages, jetstream.FetchMaxWait(c.cfg.BatchLinger))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.metrics.observeError(err)
			c.logger.Warn("WAL fetch failed", "signal", signal, "error", err)
			if !sleepContext(ctx, c.cfg.RetryBackoff) {
				return
			}
			continue
		}
		pending := newPendingBatch()
		for message := range batch.Messages() {
			if ctx.Err() != nil {
				// Do not ack: the unacked messages are delivered again.
				return
			}
			c.consumeMessage(ctx, pending, message)
			for key, group := range pending.groups {
				if group.events >= c.cfg.BatchMaxEvents || group.bytes >= c.cfg.BatchMaxBytes {
					if !c.flushGroup(ctx, group) {
						return
					}
					delete(pending.groups, key)
				}
			}
		}
		if err := batch.Error(); err != nil && !errors.Is(err, nats.ErrTimeout) && !errors.Is(err, jetstream.ErrNoMessages) {
			c.metrics.observeError(err)
			c.logger.Warn("WAL fetch batch failed", "signal", signal, "error", err)
		}
		for key, group := range pending.groups {
			if !c.flushGroup(ctx, group) {
				return
			}
			delete(pending.groups, key)
		}
	}
}

// consumeMessage decodes one envelope and adds it to the pending batch. A
// permanent decode failure goes to the DLQ and is acknowledged immediately.
func (c *Consumer) consumeMessage(ctx context.Context, pending *pendingBatch, msg jetstream.Msg) {
	envelope, err := natsadapter.DecodeMessage(&nats.Msg{
		Subject: msg.Subject(),
		Header:  msg.Headers(),
		Data:    msg.Data(),
	})
	if err != nil {
		signal := signalFromSubject(msg.Subject())
		if header := ingest.Signal(msg.Headers().Get(natsadapter.SignalHeader)); header != "" {
			signal = header
		}
		c.logger.Error("WAL message is invalid; moving to dlq", "subject", msg.Subject(), "error", err)
		c.rejectMessage(ctx, signal, msg, err)
		return
	}

	signal := ingest.Signal(envelope.GetSignal())
	message := &pendingMessage{
		msg:        msg,
		subject:    msg.Subject(),
		headers:    cloneHeader(msg.Headers()),
		data:       append([]byte(nil), msg.Data()...),
		tenantID:   envelope.GetTenantId(),
		projectID:  envelope.GetProjectId(),
		generation: uint64(nonNegative(envelope.GetConfigGeneration())),
		authzEpoch: uint64(nonNegative(envelope.GetAuthzEpoch())),
		signal:     signal,
	}
	createdAt := time.Unix(0, envelope.GetCreatedAtUnixNano()).UTC()
	for _, request := range envelope.GetRequests() {
		receivedAt := time.Unix(0, request.GetReceivedAtUnixNano()).UTC()
		if request.GetReceivedAtUnixNano() == 0 {
			receivedAt = createdAt
		}
		if message.receivedAt.IsZero() || receivedAt.Before(message.receivedAt) {
			message.receivedAt = receivedAt
		}
		converted := &pendingRequest{
			receivedAt: receivedAt,
			eventIDs:   request.GetEventIds(),
			bytes:      len(request.GetOtlpProtobuf()),
		}
		switch signal {
		case ingest.SignalLogs:
			payload := &logspb.ExportLogsServiceRequest{}
			if err := proto.Unmarshal(request.GetOtlpProtobuf(), payload); err != nil {
				c.rejectMessage(ctx, signal, msg, fmt.Errorf("decode OTLP logs: %w", err))
				return
			}
			converted.logs = payload
		case ingest.SignalTraces:
			payload := &tracepb.ExportTraceServiceRequest{}
			if err := proto.Unmarshal(request.GetOtlpProtobuf(), payload); err != nil {
				c.rejectMessage(ctx, signal, msg, fmt.Errorf("decode OTLP traces: %w", err))
				return
			}
			converted.traces = payload
		case ingest.SignalMetrics:
			payload := &metricpb.ExportMetricsServiceRequest{}
			if err := proto.Unmarshal(request.GetOtlpProtobuf(), payload); err != nil {
				c.rejectMessage(ctx, signal, msg, fmt.Errorf("decode OTLP metrics: %w", err))
				return
			}
			converted.metrics = payload
		default:
			c.rejectMessage(ctx, signal, msg, fmt.Errorf("unsupported signal %q", signal))
			return
		}
		message.requests = append(message.requests, converted)
		message.events += len(converted.eventIDs)
	}

	group := pending.group(message)
	group.messages = append(group.messages, message)
	group.events += message.events
	group.bytes += message.bytes()
	group.eventIDs = append(group.eventIDs, message.eventIDs()...)
	if group.oldest.IsZero() || message.receivedAt.Before(group.oldest) {
		group.oldest = message.receivedAt
	}
	for _, request := range message.requests {
		switch signal {
		case ingest.SignalLogs:
			group.logs = append(group.logs, request.logs.GetResourceLogs()...)
		case ingest.SignalTraces:
			group.traces = append(group.traces, request.traces.GetResourceSpans()...)
		case ingest.SignalMetrics:
			group.metrics = append(group.metrics, request.metrics.GetResourceMetrics()...)
		}
	}
	c.metrics.observeAccepted(message.tenantID, signal, message.events)
	c.metrics.inFlightEvents.Add(int64(message.events))
}

func (m *pendingMessage) bytes() int {
	total := 0
	for _, request := range m.requests {
		total += request.bytes
	}
	return total
}

func (m *pendingMessage) eventIDs() []string {
	ids := make([]string, 0, m.events)
	for _, request := range m.requests {
		ids = append(ids, request.eventIDs...)
	}
	return ids
}

// flushGroup writes the merged OTLP payload and acks the messages only after
// the downstream confirmed the flush. A transient failure is retried with
// exponential backoff; a permanent failure moves every original message to the
// DLQ and only then acks.
func (c *Consumer) flushGroup(ctx context.Context, group *pendingGroup) bool {
	started := time.Now()
	attempt := 0
	for {
		attempt++
		result, err := c.writeGroup(ctx, group)
		if err == nil {
			c.metrics.observeResult(group.tenantID, group.signal, result, 0)
			for _, message := range group.messages {
				c.metrics.observeLag(group.tenantID, group.signal, time.Since(message.receivedAt))
				if ackErr := message.msg.Ack(); ackErr != nil {
					c.metrics.observeError(ackErr)
					c.logger.Warn("WAL ack failed", "subject", message.subject, "error", ackErr)
				}
			}
			c.metrics.inFlightEvents.Add(-int64(group.events))
			c.metrics.observeFlush(group.signal, time.Since(started))
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		if IsPermanent(err) {
			c.logger.Error("WAL batch rejected; moving to dlq",
				"tenant_id", group.tenantID,
				"project_id", group.projectID,
				"signal", group.signal,
				"events", group.events,
				"error", err,
			)
			if !c.dlqGroup(ctx, group, err) {
				return false
			}
			c.metrics.observeResult(group.tenantID, group.signal, WriteResult{
				Events:  group.events,
				Failed:  group.events,
				Retries: result.Retries,
			}, time.Since(group.oldest))
			c.metrics.inFlightEvents.Add(-int64(group.events))
			return true
		}
		c.metrics.observeError(err)
		c.metrics.observeRetry(group.tenantID, group.signal)
		delay := retryDelay(c.cfg.RetryBackoff, c.cfg.MaxRetryBackoff, attempt)
		c.logger.Warn("WAL write failed; retrying",
			"tenant_id", group.tenantID,
			"project_id", group.projectID,
			"signal", group.signal,
			"events", group.events,
			"error", err,
		)
		if !sleepContext(ctx, delay) {
			return false
		}
	}
}

func (c *Consumer) writeGroup(ctx context.Context, group *pendingGroup) (WriteResult, error) {
	switch group.signal {
	case ingest.SignalLogs:
		return c.sink.WriteLogs(ctx, group.tenantID, group.projectID, group.generation, group.authzEpoch,
			&logspb.ExportLogsServiceRequest{ResourceLogs: group.logs}, group.eventIDs, group.oldest)
	case ingest.SignalTraces:
		return c.sink.WriteTraces(ctx, group.tenantID, group.projectID, group.generation, group.authzEpoch,
			&tracepb.ExportTraceServiceRequest{ResourceSpans: group.traces}, group.eventIDs, group.oldest)
	case ingest.SignalMetrics:
		return c.sink.WriteMetrics(ctx, group.tenantID, group.projectID, group.generation, group.authzEpoch,
			&metricpb.ExportMetricsServiceRequest{ResourceMetrics: group.metrics}, group.eventIDs, group.oldest)
	default:
		return WriteResult{}, &PermanentError{Err: fmt.Errorf("unsupported signal %q", group.signal)}
	}
}

func (c *Consumer) dlqGroup(ctx context.Context, group *pendingGroup, cause error) bool {
	for _, message := range group.messages {
		if !c.dlqMessage(ctx, group.signal, message.msg, cause) {
			return false
		}
		c.metrics.observeDLQ(group.signal)
		if ackErr := message.msg.Ack(); ackErr != nil {
			c.metrics.observeError(ackErr)
			c.logger.Warn("WAL ack failed after dlq", "subject", message.subject, "error", ackErr)
		}
	}
	return true
}

func (c *Consumer) rejectMessage(ctx context.Context, signal ingest.Signal, msg jetstream.Msg, cause error) {
	if !c.dlqMessage(ctx, signal, msg, cause) {
		return
	}
	c.metrics.observeDLQ(signal)
	if err := msg.Ack(); err != nil {
		c.metrics.observeError(err)
		c.logger.Warn("WAL ack failed after dlq", "subject", msg.Subject(), "error", err)
	}
}

func (c *Consumer) dlqMessage(ctx context.Context, signal ingest.Signal, msg jetstream.Msg, cause error) bool {
	if signal == "" {
		signal = signalFromSubject(msg.Subject())
	}
	if signal == "" {
		signal = "unknown"
	}
	for {
		subject := c.cfg.DLQSubject + "." + signal.String()
		headers := cloneHeader(msg.Headers())
		if headers == nil {
			headers = nats.Header{}
		}
		headers.Set("Observaki-Original-Subject", msg.Subject())
		headers.Set("Observaki-DLQ-Reason", truncate(cause.Error(), 1024))
		headers.Set("Observaki-DLQ-At", time.Now().UTC().Format(time.RFC3339Nano))
		dlq := &nats.Msg{Subject: subject, Header: headers, Data: msg.Data()}
		publishCtx, cancel := context.WithTimeout(ctx, c.cfg.ConnectTimeout)
		_, err := c.js.PublishMsg(publishCtx, dlq)
		cancel()
		if err == nil {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		c.metrics.observeError(err)
		c.logger.Warn("DLQ publish failed; retrying", "subject", subject, "error", err)
		if !sleepContext(ctx, c.cfg.RetryBackoff) {
			return false
		}
	}
}

func (c *Consumer) pollStatus(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.metrics.connected.Store(c.nc.IsConnected())
			for signal, consumer := range c.consumers {
				infoCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				info, err := consumer.Info(infoCtx)
				cancel()
				if err != nil {
					continue
				}
				c.metrics.setPending(signal, int64(info.NumPending)+int64(info.NumAckPending))
			}
		}
	}
}

// Healthy reports whether the consumers are running and NATS is connected. A
// downstream outage is not a readiness failure: the messages stay unacked in
// the WAL and the writer retries.
func (c *Consumer) Healthy() error {
	if !c.healthy.Load() && c.ctx.Err() != nil {
		return errors.New("walconsumer: closed")
	}
	if c.nc == nil || !c.nc.IsConnected() {
		return errors.New("walconsumer: nats is disconnected")
	}
	if !c.metrics.consumerRunning.Load() {
		return errors.New("walconsumer: workers are not running")
	}
	return nil
}

// Stats returns the current per-tenant/signal snapshot.
func (c *Consumer) Stats() Stats {
	return c.metrics.snapshot()
}

// Close stops fetching and closes the NATS connection. Unacknowledged
// messages are redelivered to the next writer instance.
func (c *Consumer) Close(ctx context.Context) error {
	c.closeOnce.Do(func() {
		c.healthy.Store(false)
		c.metrics.consumerRunning.Store(false)
		c.cancel()
		done := make(chan struct{})
		go func() {
			c.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			c.closeErr = ctx.Err()
		}
		c.metrics.connected.Store(false)
		if err := c.nc.Drain(); err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
			c.closeErr = errors.Join(c.closeErr, err)
		}
		c.nc.Close()
	})
	return c.closeErr
}

func retryDelay(base, maximum time.Duration, attempt int) time.Duration {
	delay := base
	for index := 1; index < attempt; index++ {
		delay *= 2
		if delay >= maximum {
			return maximum
		}
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func cloneHeader(header nats.Header) nats.Header {
	if header == nil {
		return nil
	}
	cloned := make(nats.Header, len(header))
	for key, values := range header {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// signalFromSubject reads the signal token from the WAL subject layout
// <prefix>.<tenant>.<project>.e<epoch>.<signal>.<shard>. It is used when a
// malformed message has no (or the wrong) signal header.
func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func signalFromSubject(subject string) ingest.Signal {
	tokens := strings.Split(subject, ".")
	if len(tokens) < 2 {
		return ""
	}
	candidate := ingest.Signal(tokens[len(tokens)-2])
	switch candidate {
	case ingest.SignalTraces, ingest.SignalLogs, ingest.SignalMetrics:
		return candidate
	default:
		return ""
	}
}
