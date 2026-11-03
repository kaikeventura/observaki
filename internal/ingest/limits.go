package ingest

import (
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	metricdatapb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// countSpans returns the number of spans in an OTLP traces request.
func countSpans(req *tracepb.ExportTraceServiceRequest) int {
	if req == nil {
		return 0
	}
	total := 0
	for _, resource := range req.GetResourceSpans() {
		for _, scope := range resource.GetScopeSpans() {
			total += len(scope.GetSpans())
		}
	}
	return total
}

// countLogRecords returns the number of log records in an OTLP logs request.
func countLogRecords(req *logspb.ExportLogsServiceRequest) int {
	if req == nil {
		return 0
	}
	total := 0
	for _, resource := range req.GetResourceLogs() {
		for _, scope := range resource.GetScopeLogs() {
			total += len(scope.GetLogRecords())
		}
	}
	return total
}

// countDataPoints returns the number of data points in an OTLP metrics
// request. Every metric type contributes its points, so a histogram with one
// point counts as one event, matching the benchmark's event definition.
func countDataPoints(req *metricpb.ExportMetricsServiceRequest) int {
	if req == nil {
		return 0
	}
	total := 0
	for _, resource := range req.GetResourceMetrics() {
		for _, scope := range resource.GetScopeMetrics() {
			for _, metric := range scope.GetMetrics() {
				total += countMetricDataPoints(metric)
			}
		}
	}
	return total
}

func countMetricDataPoints(metric *metricdatapb.Metric) int {
	if metric == nil {
		return 0
	}
	switch data := metric.GetData().(type) {
	case *metricdatapb.Metric_Gauge:
		return len(data.Gauge.GetDataPoints())
	case *metricdatapb.Metric_Sum:
		return len(data.Sum.GetDataPoints())
	case *metricdatapb.Metric_Histogram:
		return len(data.Histogram.GetDataPoints())
	case *metricdatapb.Metric_ExponentialHistogram:
		return len(data.ExponentialHistogram.GetDataPoints())
	case *metricdatapb.Metric_Summary:
		return len(data.Summary.GetDataPoints())
	default:
		return 0
	}
}
