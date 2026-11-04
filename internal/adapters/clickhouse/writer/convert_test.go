package writer

import (
	"testing"
	"time"

	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	otlplogspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestLogsFromOTLP(t *testing.T) {
	timestamp := time.Unix(1_700_000_000, 123).UTC()
	request := &logspb.ExportLogsServiceRequest{ResourceLogs: []*otlplogspb.ResourceLogs{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			kv("service.name", "checkout"),
			kv("host.name", "bench-01"),
		}},
		ScopeLogs: []*otlplogspb.ScopeLogs{{LogRecords: []*otlplogspb.LogRecord{{
			TimeUnixNano:         uint64(timestamp.UnixNano()),
			ObservedTimeUnixNano: uint64(timestamp.Add(time.Second).UnixNano()),
			SeverityNumber:       otlplogspb.SeverityNumber_SEVERITY_NUMBER_WARN,
			Body:                 &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "checkout failed"}},
			Attributes: []*commonpb.KeyValue{
				kv("http.status", "500"),
				{Key: "retry", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}},
			},
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}}}},
	}}}
	records, err := LogsFromOTLP("tenant-a", "project-a", 1, 2, request, []string{"event-1"}, timestamp)
	if err != nil {
		t.Fatalf("LogsFromOTLP() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.TenantID != "tenant-a" || record.ProjectID != "project-a" || record.EventID != "event-1" {
		t.Fatalf("identity = %+v", record)
	}
	if !record.Timestamp.Equal(timestamp) || !record.ObservedTimestamp.Equal(timestamp.Add(time.Second)) {
		t.Fatalf("timestamps = %v/%v", record.Timestamp, record.ObservedTimestamp)
	}
	if record.Severity != "WARN" || record.SeverityNumber != uint8(otlplogspb.SeverityNumber_SEVERITY_NUMBER_WARN) {
		t.Fatalf("severity = %q/%d", record.Severity, record.SeverityNumber)
	}
	if record.ServiceName != "checkout" || record.Body != "checkout failed" {
		t.Fatalf("service/body = %q/%q", record.ServiceName, record.Body)
	}
	if record.Attributes["http.status"] != "500" || record.Attributes["retry"] != "true" {
		t.Fatalf("attributes = %v", record.Attributes)
	}
	if record.ResourceAttributes["host.name"] != "bench-01" {
		t.Fatalf("resource attributes = %v", record.ResourceAttributes)
	}
	if record.TraceID != "0102030405060708090a0b0c0d0e0f10" || record.SpanID != "0102030405060708" {
		t.Fatalf("trace/span = %q/%q", record.TraceID, record.SpanID)
	}
}

func TestLogsFromOTLPFallsBackToReceiveTime(t *testing.T) {
	receivedAt := time.Unix(1_700_000_000, 0).UTC()
	request := &logspb.ExportLogsServiceRequest{ResourceLogs: []*otlplogspb.ResourceLogs{{
		ScopeLogs: []*otlplogspb.ScopeLogs{{LogRecords: []*otlplogspb.LogRecord{{Body: &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{StringValue: "no timestamp"},
		}}}}},
	}}}
	records, err := LogsFromOTLP("t", "p", 1, 2, request, []string{"event"}, receivedAt)
	if err != nil {
		t.Fatalf("LogsFromOTLP() error = %v", err)
	}
	if !records[0].Timestamp.Equal(receivedAt) || !records[0].ObservedTimestamp.Equal(receivedAt) {
		t.Fatalf("timestamps = %v/%v", records[0].Timestamp, records[0].ObservedTimestamp)
	}
}

func TestTracesFromOTLP(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	request := &tracepb.ExportTraceServiceRequest{ResourceSpans: []*otlptracepb.ResourceSpans{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{kv("service.name", "cart")}},
		ScopeSpans: []*otlptracepb.ScopeSpans{{Spans: []*otlptracepb.Span{{
			TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
			ParentSpanId:      []byte{8, 7, 6, 5, 4, 3, 2, 1},
			Name:              "GET /cart",
			Kind:              otlptracepb.Span_SPAN_KIND_SERVER,
			StartTimeUnixNano: uint64(start.UnixNano()),
			EndTimeUnixNano:   uint64(start.Add(50 * time.Millisecond).UnixNano()),
			Status:            &otlptracepb.Status{Code: otlptracepb.Status_STATUS_CODE_ERROR, Message: "boom"},
			Attributes:        []*commonpb.KeyValue{kv("http.status_code", "500")},
			Events: []*otlptracepb.Span_Event{{
				Name:         "exception",
				TimeUnixNano: uint64(start.Add(10 * time.Millisecond).UnixNano()),
				Attributes:   []*commonpb.KeyValue{kv("exception.type", "timeout")},
			}},
			Links: []*otlptracepb.Span_Link{{
				TraceId: []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9},
				SpanId:  []byte{9, 9, 9, 9, 9, 9, 9, 9},
			}},
		}}}},
	}}}
	records, err := TracesFromOTLP("t", "p", 1, 2, request, []string{"span-event"}, start)
	if err != nil {
		t.Fatalf("TracesFromOTLP() error = %v", err)
	}
	record := records[0]
	if record.TraceID == "" || record.SpanID == "" || record.ParentSpanID == "" {
		t.Fatalf("span ids = %+v", record)
	}
	if record.ServiceName != "cart" || record.OperationName != "GET /cart" {
		t.Fatalf("service/operation = %q/%q", record.ServiceName, record.OperationName)
	}
	if record.SpanKind != "SERVER" || record.StatusCode != "ERROR" || record.StatusMessage != "boom" {
		t.Fatalf("kind/status = %q/%q/%q", record.SpanKind, record.StatusCode, record.StatusMessage)
	}
	if record.DurationNs != uint64(50*time.Millisecond) {
		t.Fatalf("duration = %d", record.DurationNs)
	}
	if len(record.Events) != 1 || record.Events[0].Name != "exception" || record.Events[0].Attributes["exception.type"] != "timeout" {
		t.Fatalf("events = %+v", record.Events)
	}
	if len(record.Links) != 1 || record.Links[0].TraceID == "" {
		t.Fatalf("links = %+v", record.Links)
	}
}

func TestConvertRejectsEventIDMismatch(t *testing.T) {
	request := &logspb.ExportLogsServiceRequest{ResourceLogs: []*otlplogspb.ResourceLogs{{
		ScopeLogs: []*otlplogspb.ScopeLogs{{LogRecords: []*otlplogspb.LogRecord{{}}}},
	}}}
	if _, err := LogsFromOTLP("t", "p", 1, 2, request, nil, time.Now()); err == nil {
		t.Fatal("LogsFromOTLP() error = nil, want mismatch")
	}
	traceRequest := &tracepb.ExportTraceServiceRequest{ResourceSpans: []*otlptracepb.ResourceSpans{{
		ScopeSpans: []*otlptracepb.ScopeSpans{{Spans: []*otlptracepb.Span{{}}}},
	}}}
	if _, err := TracesFromOTLP("t", "p", 1, 2, traceRequest, nil, time.Now()); err == nil {
		t.Fatal("TracesFromOTLP() error = nil, want mismatch")
	}
}

func kv(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}
