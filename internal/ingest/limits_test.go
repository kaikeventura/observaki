package ingest

import (
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestCountSpans(t *testing.T) {
	request := tracesRequest(
		&tracepb.Span{Name: "one"},
		&tracepb.Span{Name: "two"},
	)
	request.ResourceSpans = append(request.ResourceSpans, &tracepb.ResourceSpans{
		ScopeSpans: []*tracepb.ScopeSpans{{
			Spans: []*tracepb.Span{{Name: "three"}},
		}},
	})
	if got := countSpans(request); got != 3 {
		t.Fatalf("countSpans() = %d, want 3", got)
	}
	if got := countSpans(nil); got != 0 {
		t.Fatalf("countSpans(nil) = %d, want 0", got)
	}
}

func TestCountLogRecords(t *testing.T) {
	request := logsRequest(
		&logspb.LogRecord{Body: stringValue("one")},
		&logspb.LogRecord{Body: stringValue("two")},
	)
	request.ResourceLogs = append(request.ResourceLogs, &logspb.ResourceLogs{
		ScopeLogs: []*logspb.ScopeLogs{{
			LogRecords: []*logspb.LogRecord{{Body: stringValue("three")}},
		}},
	})
	if got := countLogRecords(request); got != 3 {
		t.Fatalf("countLogRecords() = %d, want 3", got)
	}
	if got := countLogRecords(nil); got != 0 {
		t.Fatalf("countLogRecords(nil) = %d, want 0", got)
	}
}

func TestCountDataPoints(t *testing.T) {
	point := func() *metricpb.NumberDataPoint {
		return &metricpb.NumberDataPoint{
			Attributes: []*commonpb.KeyValue{stringAttribute("host", "a")},
		}
	}
	request := metricsRequest(
		gaugeMetric("gauge", point(), point()),
		&metricpb.Metric{Name: "sum", Data: &metricpb.Metric_Sum{Sum: &metricpb.Sum{DataPoints: []*metricpb.NumberDataPoint{point()}}}},
		&metricpb.Metric{Name: "histogram", Data: &metricpb.Metric_Histogram{Histogram: &metricpb.Histogram{DataPoints: []*metricpb.HistogramDataPoint{{}}}}},
		&metricpb.Metric{Name: "exp", Data: &metricpb.Metric_ExponentialHistogram{ExponentialHistogram: &metricpb.ExponentialHistogram{DataPoints: []*metricpb.ExponentialHistogramDataPoint{{}}}}},
		&metricpb.Metric{Name: "summary", Data: &metricpb.Metric_Summary{Summary: &metricpb.Summary{DataPoints: []*metricpb.SummaryDataPoint{{}}}}},
	)
	if got := countDataPoints(request); got != 6 {
		t.Fatalf("countDataPoints() = %d, want 6", got)
	}
	if got := countDataPoints(nil); got != 0 {
		t.Fatalf("countDataPoints(nil) = %d, want 0", got)
	}
}

func TestBatchEvents(t *testing.T) {
	tests := []struct {
		name  string
		batch *Batch
		want  int
	}{
		{name: "nil", batch: nil, want: 0},
		{name: "traces", batch: &Batch{Signal: SignalTraces, Traces: tracesRequest(&tracepb.Span{}, &tracepb.Span{})}, want: 2},
		{name: "logs", batch: &Batch{Signal: SignalLogs, Logs: logsRequest(&logspb.LogRecord{})}, want: 1},
		{name: "metrics", batch: &Batch{Signal: SignalMetrics, Metrics: metricsRequest(gaugeMetric("m", &metricpb.NumberDataPoint{}))}, want: 1},
		{name: "unknown signal", batch: &Batch{Signal: Signal("other")}, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.batch.Events(); got != test.want {
				t.Fatalf("Events() = %d, want %d", got, test.want)
			}
		})
	}
}
