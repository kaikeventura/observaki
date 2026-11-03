package ingest

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"sort"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// eventIDVersion prefixes every canonical event payload. Changing the hash
// inputs requires a new version so an old replay keeps the same event_id.
const eventIDVersion = "observaki.event-id.v1"

// SynthesizeEventIDs returns one deterministic event_id per leaf event in the
// batch, in the same traversal order used by Batch.Events (spans, log records
// or data points). The gateway synthesizes IDs after redaction so a retry of
// the same redacted request keeps the same IDs (AD-2, AD-18).
//
// IDs are UUID-shaped strings derived from the first 16 bytes of SHA-256. They
// are not UUIDv7 and carry no timestamp: determinism is the contract, not
// ordering.
func SynthesizeEventIDs(batch *Batch) []string {
	if batch == nil {
		return nil
	}
	ids := make([]string, 0, batch.Events())
	hasher := &canonicalHasher{buf: make([]byte, 0, 512)}

	switch batch.Signal {
	case SignalTraces:
		for _, resourceSpans := range batch.Traces.GetResourceSpans() {
			resource := resourceSpans.GetResource()
			for _, scopeSpans := range resourceSpans.GetScopeSpans() {
				scope := scopeSpans.GetScope()
				for _, span := range scopeSpans.GetSpans() {
					ids = append(ids, spanEventID(hasher, batch, resource, scope, span))
				}
			}
		}
	case SignalLogs:
		for _, resourceLogs := range batch.Logs.GetResourceLogs() {
			resource := resourceLogs.GetResource()
			for _, scopeLogs := range resourceLogs.GetScopeLogs() {
				scope := scopeLogs.GetScope()
				for _, record := range scopeLogs.GetLogRecords() {
					ids = append(ids, logEventID(hasher, batch, resource, scope, record))
				}
			}
		}
	case SignalMetrics:
		for _, resourceMetrics := range batch.Metrics.GetResourceMetrics() {
			resource := resourceMetrics.GetResource()
			for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
				scope := scopeMetrics.GetScope()
				for _, metric := range scopeMetrics.GetMetrics() {
					ids = appendMetricEventIDs(ids, hasher, batch, resource, scope, metric)
				}
			}
		}
	}
	return ids
}

func spanEventID(hasher *canonicalHasher, batch *Batch, resource *resourcepb.Resource, scope *commonpb.InstrumentationScope, span *tracepb.Span) string {
	hasher.reset()
	hasher.str(eventIDVersion)
	hasher.str(batch.Signal.String())
	hasher.str(batch.TenantID)
	hasher.str(batch.ProjectID)

	// Trace and span IDs uniquely identify a span; the fast path avoids
	// hashing every attribute on the hot path. Malformed spans without both
	// IDs fall back to the full canonical payload.
	if len(span.GetTraceId()) == 16 && len(span.GetSpanId()) == 8 {
		hasher.bytes(span.GetTraceId())
		hasher.bytes(span.GetSpanId())
		return hasher.eventID()
	}

	hasher.attributes(resource.GetAttributes())
	hasher.str(scope.GetName())
	hasher.str(scope.GetVersion())
	hasher.str(span.GetName())
	hasher.bytes(span.GetTraceId())
	hasher.bytes(span.GetSpanId())
	hasher.bytes(span.GetParentSpanId())
	hasher.u32(uint32(span.GetKind()))
	hasher.u64(span.GetStartTimeUnixNano())
	hasher.u64(span.GetEndTimeUnixNano())
	hasher.attributes(span.GetAttributes())
	hasher.u32(span.GetDroppedAttributesCount())
	hasher.u32(span.GetFlags())
	hasher.u32(uint32(span.GetStatus().GetCode()))
	hasher.str(span.GetStatus().GetMessage())
	for _, event := range span.GetEvents() {
		hasher.str(event.GetName())
		hasher.u64(event.GetTimeUnixNano())
		hasher.attributes(event.GetAttributes())
		hasher.u32(event.GetDroppedAttributesCount())
	}
	for _, link := range span.GetLinks() {
		hasher.bytes(link.GetTraceId())
		hasher.bytes(link.GetSpanId())
		hasher.attributes(link.GetAttributes())
		hasher.u32(link.GetDroppedAttributesCount())
		hasher.u32(link.GetFlags())
	}
	return hasher.eventID()
}

func logEventID(hasher *canonicalHasher, batch *Batch, resource *resourcepb.Resource, scope *commonpb.InstrumentationScope, record *logspb.LogRecord) string {
	hasher.reset()
	hasher.str(eventIDVersion)
	hasher.str(batch.Signal.String())
	hasher.str(batch.TenantID)
	hasher.str(batch.ProjectID)
	hasher.attributes(resource.GetAttributes())
	hasher.str(scope.GetName())
	hasher.str(scope.GetVersion())
	hasher.u64(record.GetTimeUnixNano())
	hasher.u64(record.GetObservedTimeUnixNano())
	hasher.u32(uint32(record.GetSeverityNumber()))
	hasher.str(record.GetSeverityText())
	hasher.value(record.GetBody())
	hasher.attributes(record.GetAttributes())
	hasher.u32(record.GetDroppedAttributesCount())
	hasher.u32(record.GetFlags())
	hasher.bytes(record.GetTraceId())
	hasher.bytes(record.GetSpanId())
	hasher.str(record.GetEventName())
	return hasher.eventID()
}

func appendMetricEventIDs(ids []string, hasher *canonicalHasher, batch *Batch, resource *resourcepb.Resource, scope *commonpb.InstrumentationScope, metric *metricpb.Metric) []string {
	prefix := func(pointCommon func()) {
		hasher.reset()
		hasher.str(eventIDVersion)
		hasher.str(batch.Signal.String())
		hasher.str(batch.TenantID)
		hasher.str(batch.ProjectID)
		hasher.attributes(resource.GetAttributes())
		hasher.str(scope.GetName())
		hasher.str(scope.GetVersion())
		hasher.str(metric.GetName())
		hasher.str(metric.GetDescription())
		hasher.str(metric.GetUnit())
		pointCommon()
	}

	switch data := metric.GetData().(type) {
	case *metricpb.Metric_Gauge:
		for _, point := range data.Gauge.GetDataPoints() {
			prefix(func() {
				hasher.u8(1)
				hasher.metricPoint(point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), point.GetFlags())
			})
			hasher.numberValue(point)
			ids = append(ids, hasher.eventID())
		}
	case *metricpb.Metric_Sum:
		for _, point := range data.Sum.GetDataPoints() {
			prefix(func() {
				hasher.u8(2)
				hasher.bool(data.Sum.GetIsMonotonic())
				hasher.u32(uint32(data.Sum.GetAggregationTemporality()))
				hasher.metricPoint(point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), point.GetFlags())
			})
			hasher.numberValue(point)
			ids = append(ids, hasher.eventID())
		}
	case *metricpb.Metric_Histogram:
		for _, point := range data.Histogram.GetDataPoints() {
			prefix(func() {
				hasher.u8(3)
				hasher.u32(uint32(data.Histogram.GetAggregationTemporality()))
				hasher.metricPoint(point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), point.GetFlags())
				hasher.u64(point.GetCount())
				hasher.optionalF64(point.Sum)
				hasher.optionalF64(point.Min)
				hasher.optionalF64(point.Max)
				hasher.u64s(point.GetBucketCounts())
				hasher.f64s(point.GetExplicitBounds())
				hasher.exemplars(point.GetExemplars())
			})
			ids = append(ids, hasher.eventID())
		}
	case *metricpb.Metric_ExponentialHistogram:
		for _, point := range data.ExponentialHistogram.GetDataPoints() {
			prefix(func() {
				hasher.u8(4)
				hasher.u32(uint32(data.ExponentialHistogram.GetAggregationTemporality()))
				hasher.metricPoint(point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), point.GetFlags())
				hasher.u64(point.GetCount())
				hasher.optionalF64(point.Sum)
				hasher.optionalF64(point.Min)
				hasher.optionalF64(point.Max)
				hasher.i32(point.GetScale())
				hasher.u64(point.GetZeroCount())
				hasher.i64s(point.GetPositive().GetBucketCounts())
				hasher.i32(point.GetPositive().GetOffset())
				hasher.i64s(point.GetNegative().GetBucketCounts())
				hasher.i32(point.GetNegative().GetOffset())
				hasher.exemplars(point.GetExemplars())
			})
			ids = append(ids, hasher.eventID())
		}
	case *metricpb.Metric_Summary:
		for _, point := range data.Summary.GetDataPoints() {
			prefix(func() {
				hasher.u8(5)
				hasher.metricPoint(point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), point.GetFlags())
				hasher.u64(point.GetCount())
				hasher.f64(point.GetSum())
				for _, quantile := range point.GetQuantileValues() {
					hasher.f64(quantile.GetQuantile())
					hasher.f64(quantile.GetValue())
				}
			})
			ids = append(ids, hasher.eventID())
		}
	}
	return ids
}

type canonicalHasher struct {
	buf []byte
}

func (h *canonicalHasher) reset() {
	h.buf = h.buf[:0]
}

func (h *canonicalHasher) u8(value uint8) {
	h.buf = append(h.buf, value)
}

func (h *canonicalHasher) bool(value bool) {
	if value {
		h.u8(1)
		return
	}
	h.u8(0)
}

func (h *canonicalHasher) u32(value uint32) {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	h.buf = append(h.buf, raw[:]...)
}

func (h *canonicalHasher) i32(value int32) {
	h.u32(uint32(value))
}

func (h *canonicalHasher) u64(value uint64) {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	h.buf = append(h.buf, raw[:]...)
}

func (h *canonicalHasher) i64(value int64) {
	h.u64(uint64(value))
}

func (h *canonicalHasher) f64(value float64) {
	h.u64(math.Float64bits(value))
}

func (h *canonicalHasher) optionalF64(value *float64) {
	if value == nil {
		h.u8(0)
		return
	}
	h.u8(1)
	h.f64(*value)
}

func (h *canonicalHasher) str(value string) {
	h.u64(uint64(len(value)))
	h.buf = append(h.buf, value...)
}

func (h *canonicalHasher) bytes(value []byte) {
	h.u64(uint64(len(value)))
	h.buf = append(h.buf, value...)
}

func (h *canonicalHasher) u64s(values []uint64) {
	h.u64(uint64(len(values)))
	for _, value := range values {
		h.u64(value)
	}
}

func (h *canonicalHasher) i64s(values []uint64) {
	h.u64s(values)
}

func (h *canonicalHasher) f64s(values []float64) {
	h.u64(uint64(len(values)))
	for _, value := range values {
		h.f64(value)
	}
}

func (h *canonicalHasher) numberValue(point *metricpb.NumberDataPoint) {
	switch value := point.GetValue().(type) {
	case *metricpb.NumberDataPoint_AsDouble:
		h.u8(1)
		h.f64(value.AsDouble)
	case *metricpb.NumberDataPoint_AsInt:
		h.u8(2)
		h.i64(value.AsInt)
	default:
		h.u8(0)
	}
}

func (h *canonicalHasher) metricPoint(attributes []*commonpb.KeyValue, start, observed uint64, flags uint32) {
	h.attributes(attributes)
	h.u64(start)
	h.u64(observed)
	h.u32(flags)
}

func (h *canonicalHasher) exemplars(exemplars []*metricpb.Exemplar) {
	h.u64(uint64(len(exemplars)))
	for _, exemplar := range exemplars {
		h.u64(exemplar.GetTimeUnixNano())
		h.f64(exemplar.GetAsDouble())
		h.bytes(exemplar.GetSpanId())
		h.bytes(exemplar.GetTraceId())
		h.attributes(exemplar.GetFilteredAttributes())
	}
}

func (h *canonicalHasher) attributes(attributes []*commonpb.KeyValue) {
	if len(attributes) > 1 {
		attributes = append([]*commonpb.KeyValue(nil), attributes...)
		sort.SliceStable(attributes, func(i, j int) bool {
			return attributes[i].GetKey() < attributes[j].GetKey()
		})
	}
	h.u64(uint64(len(attributes)))
	for _, attribute := range attributes {
		h.str(attribute.GetKey())
		h.value(attribute.GetValue())
	}
}

func (h *canonicalHasher) value(value *commonpb.AnyValue) {
	switch typed := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		h.u8(1)
		h.str(typed.StringValue)
	case *commonpb.AnyValue_BoolValue:
		h.u8(2)
		h.bool(typed.BoolValue)
	case *commonpb.AnyValue_IntValue:
		h.u8(3)
		h.i64(typed.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		h.u8(4)
		h.f64(typed.DoubleValue)
	case *commonpb.AnyValue_ArrayValue:
		h.u8(5)
		h.u64(uint64(len(typed.ArrayValue.GetValues())))
		for _, item := range typed.ArrayValue.GetValues() {
			h.value(item)
		}
	case *commonpb.AnyValue_KvlistValue:
		h.u8(6)
		h.attributes(typed.KvlistValue.GetValues())
	case *commonpb.AnyValue_BytesValue:
		h.u8(7)
		h.bytes(typed.BytesValue)
	default:
		h.u8(0)
	}
}

func (h *canonicalHasher) eventID() string {
	sum := sha256.Sum256(h.buf)
	var uuid [16]byte
	copy(uuid[:], sum[:16])
	// RFC 9562 version 5 and variant bits keep the ID UUID-shaped.
	uuid[6] = (uuid[6] & 0x0f) | 0x50
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return formatUUID(uuid)
}

func formatUUID(value [16]byte) string {
	var out [36]byte
	hex.Encode(out[0:8], value[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], value[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], value[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], value[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], value[10:16])
	return string(out[:])
}
