package ingest

import (
	"strings"

	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricdatapb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// DefaultRedactMask replaces a sensitive value.
const DefaultRedactMask = "[REDACTED]"

// DefaultRedactKeys are the attribute keys masked by default. Keys are matched
// case-insensitively and with '-' or '.' treated as '_'.
var DefaultRedactKeys = []string{
	"credit_card",
	"card_number",
	"card.number",
	"cc_number",
	"pan",
	"cvv",
	"cvc",
}

// RedactionConfig configures the pipeline's redaction stage.
type RedactionConfig struct {
	// Keys are attribute keys whose values are always replaced with Mask.
	Keys []string
	// Mask replaces a sensitive value. Empty uses DefaultRedactMask.
	Mask string
}

// DefaultRedactionConfig returns the redaction rules shipped with the gateway.
func DefaultRedactionConfig() RedactionConfig {
	return RedactionConfig{
		Keys: append([]string(nil), DefaultRedactKeys...),
		Mask: DefaultRedactMask,
	}
}

type redactor struct {
	keys map[string]struct{}
	mask string
}

func newRedactor(cfg RedactionConfig) *redactor {
	mask := strings.TrimSpace(cfg.Mask)
	if mask == "" {
		mask = DefaultRedactMask
	}

	keys := cfg.Keys
	if len(keys) == 0 {
		keys = DefaultRedactKeys
	}
	normalized := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key = normalizeRedactKey(key); key != "" {
			normalized[key] = struct{}{}
		}
	}

	return &redactor{keys: normalized, mask: mask}
}

// apply masks the batch in place, before it reaches the sink (the WAL). Trace
// and span IDs are never touched, so log/trace correlation survives redaction.
func (r *redactor) apply(batch *Batch) {
	if batch == nil {
		return
	}
	switch batch.Signal {
	case SignalTraces:
		r.applyTraces(batch.Traces)
	case SignalLogs:
		r.applyLogs(batch.Logs)
	case SignalMetrics:
		r.applyMetrics(batch.Metrics)
	}
}

func (r *redactor) applyLogs(req *logspb.ExportLogsServiceRequest) {
	if req == nil {
		return
	}
	for _, resource := range req.GetResourceLogs() {
		r.maskAttributes(resource.GetResource().GetAttributes())
		for _, scope := range resource.GetScopeLogs() {
			r.maskAttributes(scope.GetScope().GetAttributes())
			for _, record := range scope.GetLogRecords() {
				r.maskAttributes(record.GetAttributes())
				r.maskAnyValue(record.GetBody())
			}
		}
	}
}

func (r *redactor) applyTraces(req *tracepb.ExportTraceServiceRequest) {
	if req == nil {
		return
	}
	for _, resource := range req.GetResourceSpans() {
		r.maskAttributes(resource.GetResource().GetAttributes())
		for _, scope := range resource.GetScopeSpans() {
			r.maskAttributes(scope.GetScope().GetAttributes())
			for _, span := range scope.GetSpans() {
				r.maskAttributes(span.GetAttributes())
				for _, event := range span.GetEvents() {
					r.maskAttributes(event.GetAttributes())
				}
				for _, link := range span.GetLinks() {
					r.maskAttributes(link.GetAttributes())
				}
			}
		}
	}
}

func (r *redactor) applyMetrics(req *metricpb.ExportMetricsServiceRequest) {
	if req == nil {
		return
	}
	for _, resource := range req.GetResourceMetrics() {
		r.maskAttributes(resource.GetResource().GetAttributes())
		for _, scope := range resource.GetScopeMetrics() {
			r.maskAttributes(scope.GetScope().GetAttributes())
			for _, metric := range scope.GetMetrics() {
				r.maskMetric(metric)
			}
		}
	}
}

func (r *redactor) maskMetric(metric *metricdatapb.Metric) {
	if metric == nil {
		return
	}
	switch data := metric.GetData().(type) {
	case *metricdatapb.Metric_Gauge:
		for _, point := range data.Gauge.GetDataPoints() {
			r.maskAttributes(point.GetAttributes())
			r.maskExemplars(point.GetExemplars())
		}
	case *metricdatapb.Metric_Sum:
		for _, point := range data.Sum.GetDataPoints() {
			r.maskAttributes(point.GetAttributes())
			r.maskExemplars(point.GetExemplars())
		}
	case *metricdatapb.Metric_Histogram:
		for _, point := range data.Histogram.GetDataPoints() {
			r.maskAttributes(point.GetAttributes())
			r.maskExemplars(point.GetExemplars())
		}
	case *metricdatapb.Metric_ExponentialHistogram:
		for _, point := range data.ExponentialHistogram.GetDataPoints() {
			r.maskAttributes(point.GetAttributes())
			r.maskExemplars(point.GetExemplars())
		}
	case *metricdatapb.Metric_Summary:
		for _, point := range data.Summary.GetDataPoints() {
			r.maskAttributes(point.GetAttributes())
		}
	}
}

func (r *redactor) maskExemplars(exemplars []*metricdatapb.Exemplar) {
	for _, exemplar := range exemplars {
		r.maskAttributes(exemplar.GetFilteredAttributes())
	}
}

func (r *redactor) maskAttributes(attributes []*commonpb.KeyValue) {
	for _, attribute := range attributes {
		r.maskKeyValue(attribute)
	}
}

func (r *redactor) maskKeyValue(attribute *commonpb.KeyValue) {
	if attribute == nil {
		return
	}
	if _, sensitive := r.keys[normalizeRedactKey(attribute.GetKey())]; sensitive {
		attribute.Value = &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{StringValue: r.mask},
		}
		return
	}
	r.maskAnyValue(attribute.GetValue())
}

func (r *redactor) maskAnyValue(value *commonpb.AnyValue) {
	if value == nil {
		return
	}
	switch typed := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		typed.StringValue = r.maskString(typed.StringValue)
	case *commonpb.AnyValue_ArrayValue:
		for _, item := range typed.ArrayValue.GetValues() {
			r.maskAnyValue(item)
		}
	case *commonpb.AnyValue_KvlistValue:
		for _, item := range typed.KvlistValue.GetValues() {
			r.maskKeyValue(item)
		}
	}
}

func (r *redactor) maskString(value string) string {
	if value == "" {
		return value
	}
	return maskCardNumbers(value, r.mask)
}

// maskCardNumbers replaces Luhn-valid 13-19 digit sequences with mask. It is a
// linear scan instead of a backtracking regex: redaction runs on every string
// of every event before the WAL, and the benchmark payload is large.
func maskCardNumbers(value, mask string) string {
	firstMatch := -1
	lastWritten := 0
	var out strings.Builder
	for index := 0; index < len(value); {
		if !isASCIIDigit(value[index]) {
			index++
			continue
		}
		start := index
		digits := 0
		end := index
		for end < len(value) {
			char := value[end]
			if isASCIIDigit(char) {
				digits++
				end++
				continue
			}
			if (char == ' ' || char == '-') && end+1 < len(value) && isASCIIDigit(value[end+1]) && digits < 19 {
				end++
				continue
			}
			break
		}
		if digits >= 13 && digits <= 19 && cardBoundaries(value, start, end) && luhnValid(value[start:end]) {
			if firstMatch < 0 {
				firstMatch = start
				out.Grow(len(value))
			}
			out.WriteString(value[lastWritten:start])
			out.WriteString(mask)
			lastWritten = end
		}
		index = end
	}
	if firstMatch < 0 {
		return value
	}
	out.WriteString(value[lastWritten:])
	return out.String()
}

func isASCIIDigit(char byte) bool { return char >= '0' && char <= '9' }

func isASCIIWord(char byte) bool {
	return isASCIIDigit(char) ||
		char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char == '_'
}

func cardBoundaries(value string, start, end int) bool {
	if start > 0 && isASCIIWord(value[start-1]) {
		return false
	}
	if end < len(value) && isASCIIWord(value[end]) {
		return false
	}
	return true
}

func normalizeRedactKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, ".", "_")
	return key
}

// luhnValid reports whether value contains a Luhn-valid card number. Spaces
// and dashes are ignored so grouped numbers are validated correctly.
func luhnValid(value string) bool {
	sum := 0
	double := false
	digits := 0
	for i := len(value) - 1; i >= 0; i-- {
		c := value[i]
		if c < '0' || c > '9' {
			continue
		}
		digits++
		digit := int(c - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return digits >= 13 && sum%10 == 0
}
