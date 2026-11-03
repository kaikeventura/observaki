package ingest

import (
	"strings"
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestRedactLogsMasksCreditCard(t *testing.T) {
	traceID := []byte{0x5b, 0x8e, 0xff, 0xf7, 0x98, 0x03, 0x81, 0x03, 0xd2, 0x69, 0xb6, 0x33, 0x81, 0x3f, 0xc6, 0x0c}
	spanID := []byte{0xee, 0x48, 0x02, 0xb5, 0xb8, 0xc4, 0xc0, 0xb1}
	record := &logspb.LogRecord{
		TraceId: traceID,
		SpanId:  spanID,
		Body:    stringValue("card=4111 1111 1111 1111 approved"),
		Attributes: []*commonpb.KeyValue{
			stringAttribute("credit_card", "4111 1111 1111 1111"),
			stringAttribute("Credit-Card", "4111 1111 1111 1111"),
			stringAttribute("order_id", "1234567890123"),
			{Key: "nested", Value: kvListValue(stringAttribute("card.number", "4111111111111111"))},
			{Key: "list", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{
				Values: []*commonpb.AnyValue{stringValue("5500 0000 0000 0004")},
			}}}},
		},
	}

	redactor := newRedactor(DefaultRedactionConfig())
	redactor.applyLogs(logsRequest(record))

	if got := record.Body.GetStringValue(); got != "card=[REDACTED] approved" {
		t.Errorf("body = %q, want the card number masked", got)
	}
	if got := record.Attributes[0].GetValue().GetStringValue(); got != "[REDACTED]" {
		t.Errorf("credit_card = %q, want [REDACTED]", got)
	}
	if got := record.Attributes[1].GetValue().GetStringValue(); got != "[REDACTED]" {
		t.Errorf("Credit-Card = %q, want [REDACTED]", got)
	}
	if got := record.Attributes[2].GetValue().GetStringValue(); got != "1234567890123" {
		t.Errorf("order_id = %q, want it unchanged", got)
	}
	nested := record.Attributes[3].GetValue().GetKvlistValue().GetValues()[0]
	if got := nested.GetValue().GetStringValue(); got != "[REDACTED]" {
		t.Errorf("nested card.number = %q, want [REDACTED]", got)
	}
	item := record.Attributes[4].GetValue().GetArrayValue().GetValues()[0]
	if got := item.GetStringValue(); got != "[REDACTED]" {
		t.Errorf("array card = %q, want [REDACTED]", got)
	}
	if string(record.GetTraceId()) != string(traceID) || string(record.GetSpanId()) != string(spanID) {
		t.Error("trace/span IDs changed during redaction")
	}
}

func TestRedactConfigurableMaskAndKeys(t *testing.T) {
	record := &logspb.LogRecord{
		Body: stringValue("paid with 4111-1111-1111-1111"),
		Attributes: []*commonpb.KeyValue{
			stringAttribute("secret", "hunter2"),
			stringAttribute("credit_card", "not-a-card"),
		},
	}

	redactor := newRedactor(RedactionConfig{Keys: []string{"secret"}, Mask: "***"})
	redactor.applyLogs(logsRequest(record))

	if got := record.Attributes[0].GetValue().GetStringValue(); got != "***" {
		t.Errorf("secret = %q, want ***", got)
	}
	if got := record.Attributes[1].GetValue().GetStringValue(); got != "not-a-card" {
		t.Errorf("credit_card = %q, want it unchanged because the key is not configured", got)
	}
	if got := record.Body.GetStringValue(); got != "paid with ***" {
		t.Errorf("body = %q, want ***", got)
	}
}

func TestRedactTracesAndMetrics(t *testing.T) {
	span := &tracepb.Span{
		TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		Attributes: []*commonpb.KeyValue{
			stringAttribute("credit_card", "4111 1111 1111 1111"),
			stringAttribute("http.route", "/checkout"),
		},
	}
	traceRequest := tracesRequest(span)
	traceRequest.ResourceSpans[0].Resource.Attributes = []*commonpb.KeyValue{stringAttribute("card_number", "4111111111111111")}
	traceRequest.ResourceSpans[0].ScopeSpans[0].Spans[0].Events = []*tracepb.Span_Event{{
		Attributes: []*commonpb.KeyValue{stringAttribute("cc_number", "5500 0000 0000 0004")},
	}}

	point := &metricpb.NumberDataPoint{
		Attributes: []*commonpb.KeyValue{stringAttribute("pan", "4111111111111111")},
		Exemplars: []*metricpb.Exemplar{{
			TraceId: []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9},
			FilteredAttributes: []*commonpb.KeyValue{
				stringAttribute("credit_card", "4111111111111111"),
			},
		}},
	}
	metricRequest := metricsRequest(gaugeMetric("checkout", point))

	redactor := newRedactor(DefaultRedactionConfig())
	redactor.applyTraces(traceRequest)
	redactor.applyMetrics(metricRequest)

	if got := span.Attributes[0].GetValue().GetStringValue(); got != "[REDACTED]" {
		t.Errorf("span credit_card = %q", got)
	}
	if got := span.Attributes[1].GetValue().GetStringValue(); got != "/checkout" {
		t.Errorf("span http.route = %q, want it unchanged", got)
	}
	if got := traceRequest.ResourceSpans[0].Resource.Attributes[0].GetValue().GetStringValue(); got != "[REDACTED]" {
		t.Errorf("resource card_number = %q", got)
	}
	if got := span.Events[0].Attributes[0].GetValue().GetStringValue(); got != "[REDACTED]" {
		t.Errorf("span event cc_number = %q", got)
	}
	if got := point.Attributes[0].GetValue().GetStringValue(); got != "[REDACTED]" {
		t.Errorf("metric pan = %q", got)
	}
	if got := point.Exemplars[0].FilteredAttributes[0].GetValue().GetStringValue(); got != "[REDACTED]" {
		t.Errorf("exemplar credit_card = %q", got)
	}
	if string(point.Exemplars[0].GetTraceId()) != string([]byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}) {
		t.Error("exemplar trace ID changed during redaction")
	}
}

func TestRedactDoesNotTouchNonCardNumbers(t *testing.T) {
	record := &logspb.LogRecord{
		Body: stringValue("order 1234567890123 and timestamp 1700000000000000000"),
	}
	redactor := newRedactor(DefaultRedactionConfig())
	redactor.applyLogs(logsRequest(record))
	if got := record.Body.GetStringValue(); strings.Contains(got, "[REDACTED]") {
		t.Fatalf("body = %q, want ordinary numbers unchanged", got)
	}
}

func TestLuhnValid(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "4111111111111111", want: true},
		{value: "4111 1111 1111 1111", want: true},
		{value: "5500-0000-0000-0004", want: true},
		{value: "4111111111111112", want: false},
		{value: "1234567890123", want: false},
		{value: "4111", want: false},
	}
	for _, test := range tests {
		if got := luhnValid(test.value); got != test.want {
			t.Errorf("luhnValid(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestRedactNilBatch(t *testing.T) {
	newRedactor(DefaultRedactionConfig()).apply(nil)
}

func BenchmarkMaskCardNumbers(b *testing.B) {
	payload := strings.Repeat("0123456789abcdef", 64)
	b.SetBytes(int64(len(payload)))
	for index := 0; index < b.N; index++ {
		_ = maskCardNumbers(payload, DefaultRedactMask)
	}
}

func TestMaskCardNumbersBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "plain", value: "4111111111111111", want: "[REDACTED]"},
		{name: "grouped", value: "card 4111 1111 1111 1111 ok", want: "card [REDACTED] ok"},
		{name: "dashed", value: "5500-0000-0000-0004", want: "[REDACTED]"},
		{name: "two cards", value: "4111111111111111,5500000000000004", want: "[REDACTED],[REDACTED]"},
		{name: "embedded in a word", value: "x4111111111111111", want: "x4111111111111111"},
		{name: "20 digits", value: "41111111111111111111", want: "41111111111111111111"},
		{name: "ordinary number", value: "1234567890123", want: "1234567890123"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := maskCardNumbers(test.value, DefaultRedactMask); got != test.want {
				t.Fatalf("maskCardNumbers(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}
