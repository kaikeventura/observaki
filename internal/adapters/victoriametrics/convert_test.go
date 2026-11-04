package victoriametrics

import (
	"testing"
	"time"

	"github.com/observaki/observaki/internal/promrw"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	otlpmetricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func metricRequest(metric *otlpmetricpb.Metric) *metricpb.ExportMetricsServiceRequest {
	return &metricpb.ExportMetricsServiceRequest{ResourceMetrics: []*otlpmetricpb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			stringValue("service.name", "checkout"),
			stringValue("host.name", "bench-01"),
		}},
		ScopeMetrics: []*otlpmetricpb.ScopeMetrics{{Metrics: []*otlpmetricpb.Metric{metric}}},
	}}}
}

func gaugeMetric(points ...*otlpmetricpb.NumberDataPoint) *otlpmetricpb.Metric {
	return &otlpmetricpb.Metric{
		Name: "bench.http.server.duration",
		Data: &otlpmetricpb.Metric_Gauge{Gauge: &otlpmetricpb.Gauge{DataPoints: points}},
	}
}

func numberPoint(value float64, timestamp uint64, attrs ...*commonpb.KeyValue) *otlpmetricpb.NumberDataPoint {
	return &otlpmetricpb.NumberDataPoint{
		TimeUnixNano: timestamp,
		Attributes:   attrs,
		Value:        &otlpmetricpb.NumberDataPoint_AsDouble{AsDouble: value},
	}
}

func stringValue(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func intValue(key string, value int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}}}
}

func TestConvertGauge(t *testing.T) {
	receivedAt := time.Unix(1_700_000_000, 0).UTC()
	request := metricRequest(gaugeMetric(
		numberPoint(12.5, uint64(receivedAt.UnixNano()), stringValue("http.route", "/checkout"), intValue("http.status_code", 200)),
	))
	events, skipped, err := ConvertMetrics("tenant-a", "project-a", request, []string{"event-1"}, receivedAt)
	if err != nil {
		t.Fatalf("ConvertMetrics() error = %v", err)
	}
	if skipped != 0 || len(events) != 1 {
		t.Fatalf("events/skipped = %d/%d", len(events), skipped)
	}
	event := events[0]
	if event.EventID != "event-1" || len(event.Series) != 1 {
		t.Fatalf("event = %+v", event)
	}
	series := event.Series[0]
	if len(series.Samples) != 1 || series.Samples[0].Value != 12.5 {
		t.Fatalf("samples = %+v", series.Samples)
	}
	if series.Samples[0].Timestamp != receivedAt.UnixMilli() {
		t.Fatalf("timestamp = %d, want %d", series.Samples[0].Timestamp, receivedAt.UnixMilli())
	}
	labels := map[string]string{}
	for _, label := range series.Labels {
		labels[label.Name] = label.Value
	}
	want := map[string]string{
		"__name__":         "bench.http.server.duration",
		"http_route":       "/checkout",
		"http_status_code": "200",
		"service_name":     "checkout",
		"host_name":        "bench-01",
		"tenant_id":        "tenant-a",
		"project_id":       "project-a",
	}
	for key, value := range want {
		if labels[key] != value {
			t.Fatalf("label %q = %q, want %q (labels %v)", key, labels[key], value, labels)
		}
	}
	for index := 1; index < len(series.Labels); index++ {
		if series.Labels[index-1].Name >= series.Labels[index].Name {
			t.Fatalf("labels are not sorted: %v", series.Labels)
		}
	}
}

func TestConvertHistogramCumulativeBuckets(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	sum := 7.5
	request := metricRequest(&otlpmetricpb.Metric{
		Name: "http_request_duration_seconds",
		Data: &otlpmetricpb.Metric_Histogram{Histogram: &otlpmetricpb.Histogram{
			AggregationTemporality: otlpmetricpb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
			DataPoints: []*otlpmetricpb.HistogramDataPoint{{
				TimeUnixNano:   uint64(start.UnixNano()),
				Count:          10,
				Sum:            &sum,
				ExplicitBounds: []float64{0.1, 1},
				BucketCounts:   []uint64{2, 3, 5},
			}},
		}},
	})
	events, skipped, err := ConvertMetrics("t", "p", request, []string{"e"}, start)
	if err != nil {
		t.Fatalf("ConvertMetrics() error = %v", err)
	}
	if skipped != 0 || len(events) != 1 {
		t.Fatalf("events/skipped = %d/%d", len(events), skipped)
	}
	values := map[string]float64{}
	for _, series := range events[0].Series {
		name := labelValue(series.Labels, "__name__")
		if le := labelValue(series.Labels, "le"); le != "" {
			name += "{le=" + le + "}"
		}
		values[name] = series.Samples[0].Value
	}
	if values["http_request_duration_seconds{le=0.1}"] != 2 {
		t.Fatalf("0.1 bucket = %v", values["http_request_duration_seconds{le=0.1}"])
	}
	if values["http_request_duration_seconds{le=1}"] != 5 {
		t.Fatalf("1 bucket = %v", values["http_request_duration_seconds{le=1}"])
	}
	if values["http_request_duration_seconds{le=+Inf}"] != 10 {
		t.Fatalf("+Inf bucket = %v", values["http_request_duration_seconds{le=+Inf}"])
	}
	if values["http_request_duration_seconds_sum"] != 7.5 || values["http_request_duration_seconds_count"] != 10 {
		t.Fatalf("sum/count = %v", values)
	}
}

func TestConvertSummary(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	request := metricRequest(&otlpmetricpb.Metric{
		Name: "rpc_duration",
		Data: &otlpmetricpb.Metric_Summary{Summary: &otlpmetricpb.Summary{
			DataPoints: []*otlpmetricpb.SummaryDataPoint{{
				TimeUnixNano: uint64(start.UnixNano()),
				Count:        4,
				Sum:          10,
				QuantileValues: []*otlpmetricpb.SummaryDataPoint_ValueAtQuantile{
					{Quantile: 0.5, Value: 2},
					{Quantile: 0.99, Value: 4},
				},
			}},
		}},
	})
	events, _, err := ConvertMetrics("t", "p", request, []string{"e"}, start)
	if err != nil {
		t.Fatalf("ConvertMetrics() error = %v", err)
	}
	values := map[string]float64{}
	for _, series := range events[0].Series {
		name := labelValue(series.Labels, "__name__")
		if quantile := labelValue(series.Labels, "quantile"); quantile != "" {
			name += "{q=" + quantile + "}"
		}
		values[name] = series.Samples[0].Value
	}
	if values["rpc_duration{q=0.5}"] != 2 || values["rpc_duration{q=0.99}"] != 4 {
		t.Fatalf("quantiles = %v", values)
	}
	if values["rpc_duration_sum"] != 10 || values["rpc_duration_count"] != 4 {
		t.Fatalf("sum/count = %v", values)
	}
}

func TestConvertSkipsDeltaHistograms(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	request := metricRequest(&otlpmetricpb.Metric{
		Name: "delta_histogram",
		Data: &otlpmetricpb.Metric_Histogram{Histogram: &otlpmetricpb.Histogram{
			AggregationTemporality: otlpmetricpb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
			DataPoints: []*otlpmetricpb.HistogramDataPoint{{
				TimeUnixNano: uint64(start.UnixNano()),
				Count:        1,
			}},
		}},
	})
	events, skipped, err := ConvertMetrics("t", "p", request, []string{"e"}, start)
	if err != nil {
		t.Fatalf("ConvertMetrics() error = %v", err)
	}
	if len(events) != 0 || skipped != 1 {
		t.Fatalf("events/skipped = %d/%d, want 0/1", len(events), skipped)
	}
}

func TestConvertRejectsEventIDMismatch(t *testing.T) {
	request := metricRequest(gaugeMetric(numberPoint(1, 1)))
	if _, _, err := ConvertMetrics("t", "p", request, nil, time.Now()); err == nil {
		t.Fatal("ConvertMetrics() error = nil, want mismatch")
	}
}

func TestGroupEventsMergesSeries(t *testing.T) {
	labels := []promrw.Label{{Name: "__name__", Value: "m"}, {Name: "tenant_id", Value: "t"}}
	events := []MetricEvent{
		{EventID: "1", Series: []promrw.TimeSeries{{Labels: labels, Samples: []promrw.Sample{{Value: 1, Timestamp: 1}}}}},
		{EventID: "2", Series: []promrw.TimeSeries{{Labels: labels, Samples: []promrw.Sample{{Value: 2, Timestamp: 2}}}}},
	}
	series := GroupEvents(events)
	if len(series) != 1 || len(series[0].Samples) != 2 {
		t.Fatalf("series = %+v", series)
	}
}

func labelValue(labels []promrw.Label, name string) string {
	for _, label := range labels {
		if label.Name == name {
			return label.Value
		}
	}
	return ""
}
