package loadgen

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	logscollectorpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricscollectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

const (
	metricAttributeSets = 32
	traceAttributeSets  = 8
)

// Builder creates deterministic OTLP requests from event indices. Resources,
// scopes and fixed attribute sets are shared across records to keep allocations
// bounded; protobuf marshaling only reads them.
type Builder struct {
	cfg     Config
	pattern []Signal

	resource *resourcepb.Resource
	scope    *commonpb.InstrumentationScope

	logAttributes    []*commonpb.KeyValue
	metricAttributes [metricAttributeSets][]*commonpb.KeyValue
	traceAttributes  [traceAttributeSets][]*commonpb.KeyValue
	traceStatuses    [traceAttributeSets]*tracepb.Status

	metricName string
	spanName   string

	metricPool string
	logPool    string
	tracePool  string
}

// NewBuilder validates the payload configuration and precomputes immutable
// templates.
func NewBuilder(cfg Config) (*Builder, error) {
	if err := cfg.Mix.Validate(); err != nil {
		return nil, err
	}
	if cfg.Rate <= 0 {
		return nil, fmt.Errorf("rate must be greater than zero")
	}
	if cfg.Payload.MetricPadding < 0 || cfg.Payload.LogBody < 0 || cfg.Payload.TracePadding < 0 {
		return nil, fmt.Errorf("payload sizes must be non-negative")
	}
	if cfg.StartTime.IsZero() {
		cfg.StartTime = time.Now().UTC()
	}

	b := &Builder{
		cfg:        cfg,
		pattern:    cfg.Mix.Pattern(),
		metricName: "bench.http.server.duration",
		spanName:   "GET /api/v1/checkout",
	}

	b.resource = &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
		stringKV("service.name", "checkout"),
		stringKV("service.instance.id", fmt.Sprintf("bench-%d", cfg.Seed)),
		stringKV("deployment.environment", "benchmark"),
		stringKV("host.name", "bench-host-01"),
	}}
	b.scope = &commonpb.InstrumentationScope{
		Name:    "observaki.bench",
		Version: "1.0.0",
	}

	b.logAttributes = []*commonpb.KeyValue{
		stringKV("log.file.name", "checkout.log"),
		stringKV("http.method", "GET"),
		stringKV("http.route", "/api/v1/checkout"),
		intKV("http.status_code", 200),
	}

	services := [8]string{"checkout", "payment", "cart", "inventory", "search", "auth", "shipping", "catalog"}
	routes := [8]string{
		"/api/v1/checkout", "/api/v1/payment", "/api/v1/cart", "/api/v1/inventory",
		"/api/v1/search", "/api/v1/login", "/api/v1/shipping", "/api/v1/catalog",
	}
	regions := [2]string{"us-east-1", "eu-west-1"}

	b.metricPool = makePool(uint64(cfg.Seed)^0x6d6574726963, poolSize(cfg.Payload.MetricPadding, 1<<20), metricHexEvery)
	b.logPool = makePool(uint64(cfg.Seed)^0x6c6f6773, poolSize(cfg.Payload.LogBody, 4<<20), logHexEvery)
	b.tracePool = makePool(uint64(cfg.Seed)^0x7472616365, poolSize(cfg.Payload.TracePadding, 4<<20), traceHexEvery)

	for i := 0; i < metricAttributeSets; i++ {
		statusCode := int64(200)
		if i%4 == 3 {
			statusCode = 500
		}
		metricAttrs := []*commonpb.KeyValue{
			stringKV("http.route", routes[i%len(routes)]),
			intKV("http.status_code", statusCode),
			stringKV("region", regions[i%len(regions)]),
		}
		if padding := poolWindow(b.metricPool, int64(i), cfg.Payload.MetricPadding); padding != "" {
			metricAttrs = append(metricAttrs, stringKV("bench.payload", padding))
		}
		b.metricAttributes[i] = metricAttrs
	}

	for i := 0; i < traceAttributeSets; i++ {
		statusCode := int64(200)
		if i%4 == 3 {
			statusCode = 500
		}
		b.traceAttributes[i] = []*commonpb.KeyValue{
			stringKV("service.name", services[i]),
			stringKV("http.method", "GET"),
			stringKV("http.route", routes[i]),
			intKV("http.status_code", statusCode),
			stringKV("net.peer.name", services[i]+".internal"),
		}

		statusCodeEnum := tracepb.Status_STATUS_CODE_OK
		if statusCode >= 500 {
			statusCodeEnum = tracepb.Status_STATUS_CODE_ERROR
		}
		b.traceStatuses[i] = &tracepb.Status{Code: statusCodeEnum}
	}

	return b, nil
}

// SignalFor returns the deterministic signal for a global event index.
func (b *Builder) SignalFor(index int64) Signal {
	return b.pattern[int(index%int64(len(b.pattern)))]
}

// EventTime returns the deterministic event timestamp for a global event index.
func (b *Builder) EventTime(index int64) uint64 {
	step := int64(time.Second) / int64(b.cfg.EffectiveEventTimeRate())
	if step < 1 {
		step = 1
	}
	return uint64(b.cfg.StartTime.UnixNano() + index*step)
}

// BuildLogs builds an OTLP logs request for the supplied global indices.
func (b *Builder) BuildLogs(indices []int64) *logscollectorpb.ExportLogsServiceRequest {
	records := make([]*logspb.LogRecord, len(indices))
	for i, index := range indices {
		timestamp := b.EventTime(index)
		records[i] = &logspb.LogRecord{
			TimeUnixNano:         timestamp,
			ObservedTimeUnixNano: timestamp,
			SeverityNumber:       logspb.SeverityNumber_SEVERITY_NUMBER_INFO,
			SeverityText:         "INFO",
			Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{
				StringValue: poolWindow(b.logPool, index, b.cfg.Payload.LogBody),
			}},
			Attributes: b.logAttributes,
			TraceId:    traceID(b.cfg.Seed, index),
			SpanId:     spanID(b.cfg.Seed, index),
		}
	}
	return &logscollectorpb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource:  b.resource,
			ScopeLogs: []*logspb.ScopeLogs{{Scope: b.scope, LogRecords: records}},
		}},
	}
}

// BuildMetrics builds an OTLP metrics request for the supplied global indices.
func (b *Builder) BuildMetrics(indices []int64) *metricscollectorpb.ExportMetricsServiceRequest {
	points := make([]*metricspb.NumberDataPoint, len(indices))
	for i, index := range indices {
		timestamp := b.EventTime(index)
		start := timestamp
		if timestamp > uint64(time.Minute) {
			start = timestamp - uint64(time.Minute)
		}
		points[i] = &metricspb.NumberDataPoint{
			Attributes:        b.metricAttributes[int(index%metricAttributeSets)],
			StartTimeUnixNano: start,
			TimeUnixNano:      timestamp,
			Value: &metricspb.NumberDataPoint_AsDouble{
				AsDouble: float64(index%1000) / 10,
			},
		}
	}
	metric := &metricspb.Metric{
		Name:        b.metricName,
		Description: "synthetic benchmark metric",
		Unit:        "ms",
		Data: &metricspb.Metric_Gauge{
			Gauge: &metricspb.Gauge{DataPoints: points},
		},
	}
	return &metricscollectorpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource:     b.resource,
			ScopeMetrics: []*metricspb.ScopeMetrics{{Scope: b.scope, Metrics: []*metricspb.Metric{metric}}},
		}},
	}
}

// BuildTraces builds an OTLP traces request for the supplied global indices.
func (b *Builder) BuildTraces(indices []int64) *tracecollectorpb.ExportTraceServiceRequest {
	spans := make([]*tracepb.Span, len(indices))
	for i, index := range indices {
		start := b.EventTime(index)
		attributes := make([]*commonpb.KeyValue, 0, len(b.traceAttributes[0])+1)
		attributes = append(attributes, b.traceAttributes[int(index%traceAttributeSets)]...)
		if padding := poolWindow(b.tracePool, index, b.cfg.Payload.TracePadding); padding != "" {
			attributes = append(attributes, stringKV("bench.payload", padding))
		}
		spans[i] = &tracepb.Span{
			TraceId:           traceID(b.cfg.Seed, index),
			SpanId:            spanID(b.cfg.Seed, index),
			Name:              b.spanName,
			Kind:              tracepb.Span_SPAN_KIND_SERVER,
			StartTimeUnixNano: start,
			EndTimeUnixNano:   start + uint64(50*time.Millisecond),
			Attributes:        attributes,
			Status:            b.traceStatuses[int(index%traceAttributeSets)],
		}
	}
	return &tracecollectorpb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource:   b.resource,
			ScopeSpans: []*tracepb.ScopeSpans{{Scope: b.scope, Spans: spans}},
		}},
	}
}

func poolSize(payloadSize, minimum int) int {
	size := payloadSize * 1024
	if size < minimum {
		size = minimum
	}
	return size
}

func poolWindow(pool string, index int64, size int) string {
	if size <= 0 || len(pool) == 0 {
		return ""
	}
	if size >= len(pool) {
		return pool[:size]
	}
	start := int((uint64(index) * 0x9e3779b97f4a7c15) % uint64(len(pool)-size+1))
	return pool[start : start+size]
}

func stringKV(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}},
	}
}

func intKV(key string, value int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}},
	}
}

func traceID(seed, index int64) []byte {
	return idBytes(seed, index, 16, 0x7472616365)
}

func spanID(seed, index int64) []byte {
	return idBytes(seed, index, 8, 0x7370616e)
}

func idBytes(seed, index int64, size int, salt uint64) []byte {
	id := make([]byte, size)
	state := splitmix64(uint64(seed) ^ uint64(index)*0x9e3779b97f4a7c15 ^ salt)
	for offset := 0; offset < size; offset += 8 {
		binary.LittleEndian.PutUint64(id[offset:], state)
		state = splitmix64(state)
	}
	return id
}

func splitmix64(value uint64) uint64 {
	value += 0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

var fillerTokens = [...]string{
	"checkout", "payment", "cart", "inventory", "request", "response",
	"latency", "status", "region", "us-east-1", "eu-west-1", "timeout",
	"retry", "cache", "database", "query", "http", "grpc", "ok", "error",
}

// Entropy knobs used by the reference workload. A lower value inserts a
// 16-character hex token more often, reducing the gzip compression ratio.
var (
	metricHexEvery = 8
	logHexEvery    = 256
	traceHexEvery  = 256
)

func makePool(seed uint64, size, hexEvery int) string {
	if size <= 0 {
		return ""
	}
	var builder strings.Builder
	builder.Grow(size + 32)
	state := seed
	tokens := 0
	for builder.Len() < size {
		if hexEvery > 0 && tokens%hexEvery == 0 {
			appendHex(&builder, state, 16)
		} else {
			state = splitmix64(state)
			builder.WriteString(fillerTokens[state%uint64(len(fillerTokens))])
		}
		builder.WriteByte(' ')
		tokens++
	}
	return builder.String()[:size]
}

func appendHex(builder *strings.Builder, state uint64, size int) {
	const digits = "0123456789abcdef"
	for i := 0; i < size; i++ {
		state = splitmix64(state)
		builder.WriteByte(digits[state&0xf])
	}
}
