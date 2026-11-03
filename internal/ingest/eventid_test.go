package ingest

import (
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestSynthesizeEventIDsAreStableForLogs(t *testing.T) {
	record := &logspb.LogRecord{
		TimeUnixNano: 1_700_000_000_000_000_000,
		Body:         stringValue("checkout failed"),
		Attributes:   []*commonpb.KeyValue{stringAttribute("http.status", "500"), stringAttribute("service", "api")},
	}
	batch := &Batch{Signal: SignalLogs, TenantID: "tenant-a", ProjectID: "project-a", Logs: logsRequest(record)}
	first := SynthesizeEventIDs(batch)
	second := SynthesizeEventIDs(batch)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("event ids = %v / %v, want one each", first, second)
	}
	if first[0] != second[0] {
		t.Fatalf("event id changed between identical batches: %q != %q", first[0], second[0])
	}
	if len(first[0]) != 36 {
		t.Fatalf("event id %q is not UUID-shaped", first[0])
	}

	otherTenant := *batch
	otherTenant.TenantID = "tenant-b"
	if got := SynthesizeEventIDs(&otherTenant)[0]; got == first[0] {
		t.Fatal("event id did not include the tenant")
	}

	changed := *batch
	changed.Logs = logsRequest(&logspb.LogRecord{
		TimeUnixNano: record.TimeUnixNano,
		Body:         stringValue("checkout failed"),
		Attributes:   []*commonpb.KeyValue{stringAttribute("http.status", "501"), stringAttribute("service", "api")},
	})
	if got := SynthesizeEventIDs(&changed)[0]; got == first[0] {
		t.Fatal("event id did not include the log body attributes")
	}
}

func TestSynthesizeEventIDsIgnoreAttributeOrder(t *testing.T) {
	attributes := []*commonpb.KeyValue{
		stringAttribute("a", "1"),
		stringAttribute("b", "2"),
		stringAttribute("c", "3"),
	}
	reordered := []*commonpb.KeyValue{attributes[2], attributes[0], attributes[1]}
	first := SynthesizeEventIDs(&Batch{
		Signal: SignalLogs, TenantID: "t", ProjectID: "p",
		Logs: logsRequest(&logspb.LogRecord{Body: stringValue("x"), Attributes: attributes}),
	})
	second := SynthesizeEventIDs(&Batch{
		Signal: SignalLogs, TenantID: "t", ProjectID: "p",
		Logs: logsRequest(&logspb.LogRecord{Body: stringValue("x"), Attributes: reordered}),
	})
	if first[0] != second[0] {
		t.Fatalf("attribute order changed the event id: %q != %q", first[0], second[0])
	}
}

func TestSynthesizeEventIDsForSpans(t *testing.T) {
	span := &tracepb.Span{
		TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
		Name:              "GET /checkout",
		StartTimeUnixNano: 10,
		EndTimeUnixNano:   20,
	}
	batch := &Batch{Signal: SignalTraces, TenantID: "t", ProjectID: "p", Traces: tracesRequest(span)}
	first := SynthesizeEventIDs(batch)
	second := SynthesizeEventIDs(batch)
	if len(first) != 1 || first[0] != second[0] {
		t.Fatalf("span event ids = %v / %v", first, second)
	}

	// A malformed span without IDs still gets a stable content-derived ID.
	malformed := &tracepb.Span{Name: "orphan", StartTimeUnixNano: 1, EndTimeUnixNano: 2}
	broken := &Batch{Signal: SignalTraces, TenantID: "t", ProjectID: "p", Traces: tracesRequest(malformed)}
	if got := SynthesizeEventIDs(broken); len(got) != 1 || got[0] == "" {
		t.Fatalf("malformed span event ids = %v", got)
	}
}

func TestSynthesizeEventIDsForMetrics(t *testing.T) {
	point := &metricpb.NumberDataPoint{
		Attributes:        []*commonpb.KeyValue{stringAttribute("host", "a")},
		StartTimeUnixNano: 1,
		TimeUnixNano:      2,
		Value:             &metricpb.NumberDataPoint_AsDouble{AsDouble: 0.5},
	}
	metric := gaugeMetric("cpu.usage", point)
	batch := &Batch{Signal: SignalMetrics, TenantID: "t", ProjectID: "p", Metrics: metricsRequest(metric)}
	ids := SynthesizeEventIDs(batch)
	if len(ids) != 1 {
		t.Fatalf("event ids = %v, want one", ids)
	}
	if got := SynthesizeEventIDs(batch)[0]; got != ids[0] {
		t.Fatalf("metric event id changed: %q != %q", got, ids[0])
	}

	intPoint := &metricpb.NumberDataPoint{
		TimeUnixNano: 2,
		Value:        &metricpb.NumberDataPoint_AsInt{AsInt: 1},
	}
	intBatch := &Batch{Signal: SignalMetrics, TenantID: "t", ProjectID: "p", Metrics: metricsRequest(gaugeMetric("cpu.usage", intPoint))}
	intIDs := SynthesizeEventIDs(intBatch)
	if len(intIDs) != 1 || intIDs[0] == ids[0] {
		t.Fatalf("int metric event ids = %v (double %v), want a distinct ID", intIDs, ids)
	}
}

func TestSynthesizeEventIDsCountMatchesEvents(t *testing.T) {
	batch := &Batch{
		Signal:    SignalMetrics,
		TenantID:  "t",
		ProjectID: "p",
		Metrics: metricsRequest(
			gaugeMetric("a", &metricpb.NumberDataPoint{TimeUnixNano: 1, Value: &metricpb.NumberDataPoint_AsInt{AsInt: 1}}),
			gaugeMetric("b",
				&metricpb.NumberDataPoint{TimeUnixNano: 1, Value: &metricpb.NumberDataPoint_AsInt{AsInt: 1}},
				&metricpb.NumberDataPoint{TimeUnixNano: 2, Value: &metricpb.NumberDataPoint_AsInt{AsInt: 2}},
			),
		),
	}
	if got, want := len(SynthesizeEventIDs(batch)), batch.Events(); got != want {
		t.Fatalf("event ids = %d, want %d", got, want)
	}
}

func BenchmarkSynthesizeEventIDs(b *testing.B) {
	record := &logspb.LogRecord{
		TimeUnixNano: 1_700_000_000_000_000_000,
		Body:         stringValue("checkout failed"),
		Attributes:   []*commonpb.KeyValue{stringAttribute("http.status", "500"), stringAttribute("service", "api")},
	}
	batch := &Batch{Signal: SignalLogs, TenantID: "tenant-a", ProjectID: "project-a"}
	batch.Logs = logsRequest()
	for index := 0; index < 1000; index++ {
		batch.Logs.ResourceLogs[0].ScopeLogs[0].LogRecords = append(batch.Logs.ResourceLogs[0].ScopeLogs[0].LogRecords, record)
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if got := len(SynthesizeEventIDs(batch)); got != 1000 {
			b.Fatalf("ids = %d", got)
		}
	}
}
