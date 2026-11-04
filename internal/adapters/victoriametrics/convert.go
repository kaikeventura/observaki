package victoriametrics

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/observaki/observaki/internal/otlputil"
	"github.com/observaki/observaki/internal/promrw"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	otlpmetricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// MetricEvent is one OTLP data point after conversion. A gauge or sum point
// carries one series; a histogram or summary point carries several. The event
// ID is the gateway's deterministic dedupe key for the data point.
type MetricEvent struct {
	EventID string
	Series  []promrw.TimeSeries
}

// ConvertMetrics converts one redacted OTLP metrics request into one event per
// data point, in OTLP traversal order. eventIDs must match that order.
// Delta histograms are not accumulated; they are counted in skipped instead of
// being stored with a wrong cumulative value. Exponential histogram buckets
// are not exported, but their _sum and _count are.
func ConvertMetrics(
	tenantID, projectID string,
	request *metricpb.ExportMetricsServiceRequest,
	eventIDs []string,
	receivedAt time.Time,
) (events []MetricEvent, skipped int, err error) {
	if request == nil {
		return nil, 0, fmt.Errorf("metrics request is nil")
	}
	total := countOtlpPoints(request)
	if total != len(eventIDs) {
		return nil, 0, fmt.Errorf("metrics request has %d data points but %d event ids", total, len(eventIDs))
	}
	if total == 0 {
		return nil, 0, fmt.Errorf("metrics request has no data points")
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	events = make([]MetricEvent, 0, total)
	position := 0
	for _, resourceMetrics := range request.GetResourceMetrics() {
		resourceLabels := otlputil.ResourceLabels(resourceMetrics.GetResource())
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			for _, metric := range scopeMetrics.GetMetrics() {
				pointEvents, pointSkipped, err := convertMetric(
					tenantID, projectID, metric, resourceLabels, eventIDs[position:], receivedAt,
				)
				if err != nil {
					return nil, skipped, err
				}
				events = append(events, pointEvents...)
				position += len(pointEvents) + pointSkipped
				skipped += pointSkipped
			}
		}
	}
	if position != len(eventIDs) {
		return nil, skipped, fmt.Errorf("metrics conversion consumed %d event ids, want %d", position, len(eventIDs))
	}
	return events, skipped, nil
}

func convertMetric(
	tenantID, projectID string,
	metric *otlpmetricpb.Metric,
	resourceLabels []otlputil.Label,
	eventIDs []string,
	receivedAt time.Time,
) ([]MetricEvent, int, error) {
	switch data := metric.GetData().(type) {
	case *otlpmetricpb.Metric_Gauge:
		return convertNumberPoints(tenantID, projectID, metric.GetName(), resourceLabels, data.Gauge.GetDataPoints(), eventIDs, receivedAt)
	case *otlpmetricpb.Metric_Sum:
		return convertNumberPoints(tenantID, projectID, metric.GetName(), resourceLabels, data.Sum.GetDataPoints(), eventIDs, receivedAt)
	case *otlpmetricpb.Metric_Histogram:
		return convertHistogramPoints(tenantID, projectID, metric.GetName(), resourceLabels, data.Histogram, eventIDs, receivedAt)
	case *otlpmetricpb.Metric_ExponentialHistogram:
		return convertExponentialHistogramPoints(tenantID, projectID, metric.GetName(), resourceLabels, data.ExponentialHistogram, eventIDs, receivedAt)
	case *otlpmetricpb.Metric_Summary:
		return convertSummaryPoints(tenantID, projectID, metric.GetName(), resourceLabels, data.Summary, eventIDs, receivedAt)
	default:
		return nil, 0, fmt.Errorf("metric %q has no data", metric.GetName())
	}
}

func convertNumberPoints(
	tenantID, projectID, name string,
	resourceLabels []otlputil.Label,
	points []*otlpmetricpb.NumberDataPoint,
	eventIDs []string,
	receivedAt time.Time,
) ([]MetricEvent, int, error) {
	if len(points) > len(eventIDs) {
		return nil, 0, fmt.Errorf("metric %q has %d points but %d event ids", name, len(points), len(eventIDs))
	}
	events := make([]MetricEvent, 0, len(points))
	for index, point := range points {
		timestamp := pointTime(point.GetTimeUnixNano(), receivedAt)
		value, err := numberValue(point)
		if err != nil {
			return nil, 0, fmt.Errorf("metric %q: %w", name, err)
		}
		labels := metricLabels(name, resourceLabels, point.GetAttributes(), tenantID, projectID, nil)
		events = append(events, MetricEvent{
			EventID: eventIDs[index],
			Series: []promrw.TimeSeries{{
				Labels:  labels,
				Samples: []promrw.Sample{{Value: value, Timestamp: timestamp.UnixMilli()}},
			}},
		})
	}
	return events, 0, nil
}

func convertHistogramPoints(
	tenantID, projectID, name string,
	resourceLabels []otlputil.Label,
	histogram *otlpmetricpb.Histogram,
	eventIDs []string,
	receivedAt time.Time,
) ([]MetricEvent, int, error) {
	points := histogram.GetDataPoints()
	if histogram.GetAggregationTemporality() == otlpmetricpb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
		// Prometheus remote write expects cumulative histograms. Accumulating
		// deltas needs per-series state that the MVP writer does not have.
		return nil, len(points), nil
	}
	if len(points) > len(eventIDs) {
		return nil, 0, fmt.Errorf("metric %q has %d points but %d event ids", name, len(points), len(eventIDs))
	}
	events := make([]MetricEvent, 0, len(points))
	for index, point := range points {
		timestamp := pointTime(point.GetTimeUnixNano(), receivedAt)
		labels := metricLabels(name, resourceLabels, point.GetAttributes(), tenantID, projectID, nil)
		series := make([]promrw.TimeSeries, 0, len(point.GetExplicitBounds())+3)
		cumulative := uint64(0)
		for bucket, bound := range point.GetExplicitBounds() {
			if bucket < len(point.GetBucketCounts()) {
				cumulative += point.GetBucketCounts()[bucket]
			}
			series = append(series, seriesWithSample(
				appendLabel(labels, "le", formatFloat(bound)),
				float64(cumulative),
				timestamp,
			))
		}
		series = append(series, seriesWithSample(
			appendLabel(labels, "le", "+Inf"),
			float64(point.GetCount()),
			timestamp,
		))
		if point.Sum != nil {
			series = append(series, seriesWithSample(
				appendLabel(labels, "__name__", name+"_sum"),
				*point.Sum,
				timestamp,
			))
		}
		series = append(series, seriesWithSample(
			appendLabel(labels, "__name__", name+"_count"),
			float64(point.GetCount()),
			timestamp,
		))
		events = append(events, MetricEvent{EventID: eventIDs[index], Series: series})
	}
	return events, 0, nil
}

func convertExponentialHistogramPoints(
	tenantID, projectID, name string,
	resourceLabels []otlputil.Label,
	histogram *otlpmetricpb.ExponentialHistogram,
	eventIDs []string,
	receivedAt time.Time,
) ([]MetricEvent, int, error) {
	points := histogram.GetDataPoints()
	if len(points) > len(eventIDs) {
		return nil, 0, fmt.Errorf("metric %q has %d points but %d event ids", name, len(points), len(eventIDs))
	}
	events := make([]MetricEvent, 0, len(points))
	for index, point := range points {
		timestamp := pointTime(point.GetTimeUnixNano(), receivedAt)
		labels := metricLabels(name, resourceLabels, point.GetAttributes(), tenantID, projectID, nil)
		series := make([]promrw.TimeSeries, 0, 2)
		if point.Sum != nil {
			series = append(series, seriesWithSample(
				appendLabel(labels, "__name__", name+"_sum"),
				*point.Sum,
				timestamp,
			))
		}
		series = append(series, seriesWithSample(
			appendLabel(labels, "__name__", name+"_count"),
			float64(point.GetCount()),
			timestamp,
		))
		events = append(events, MetricEvent{EventID: eventIDs[index], Series: series})
	}
	return events, 0, nil
}

func convertSummaryPoints(
	tenantID, projectID, name string,
	resourceLabels []otlputil.Label,
	summary *otlpmetricpb.Summary,
	eventIDs []string,
	receivedAt time.Time,
) ([]MetricEvent, int, error) {
	points := summary.GetDataPoints()
	if len(points) > len(eventIDs) {
		return nil, 0, fmt.Errorf("metric %q has %d points but %d event ids", name, len(points), len(eventIDs))
	}
	events := make([]MetricEvent, 0, len(points))
	for index, point := range points {
		timestamp := pointTime(point.GetTimeUnixNano(), receivedAt)
		labels := metricLabels(name, resourceLabels, point.GetAttributes(), tenantID, projectID, nil)
		series := make([]promrw.TimeSeries, 0, len(point.GetQuantileValues())+2)
		for _, quantile := range point.GetQuantileValues() {
			series = append(series, seriesWithSample(
				appendLabel(labels, "quantile", formatFloat(quantile.GetQuantile())),
				quantile.GetValue(),
				timestamp,
			))
		}
		series = append(series, seriesWithSample(
			appendLabel(labels, "__name__", name+"_sum"),
			point.GetSum(),
			timestamp,
		))
		series = append(series, seriesWithSample(
			appendLabel(labels, "__name__", name+"_count"),
			float64(point.GetCount()),
			timestamp,
		))
		events = append(events, MetricEvent{EventID: eventIDs[index], Series: series})
	}
	return events, 0, nil
}

func seriesWithSample(labels []promrw.Label, value float64, timestamp time.Time) promrw.TimeSeries {
	return promrw.TimeSeries{
		Labels:  labels,
		Samples: []promrw.Sample{{Value: value, Timestamp: timestamp.UnixMilli()}},
	}
}

func numberValue(point *otlpmetricpb.NumberDataPoint) (float64, error) {
	switch value := point.GetValue().(type) {
	case *otlpmetricpb.NumberDataPoint_AsDouble:
		return value.AsDouble, nil
	case *otlpmetricpb.NumberDataPoint_AsInt:
		return float64(value.AsInt), nil
	default:
		return 0, fmt.Errorf("number data point has no value")
	}
}

func pointTime(value uint64, fallback time.Time) time.Time {
	if value == 0 {
		return fallback
	}
	return time.Unix(0, int64(value)).UTC()
}

func formatFloat(value float64) string {
	if math.IsInf(value, 1) {
		return "+Inf"
	}
	if math.IsInf(value, -1) {
		return "-Inf"
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

// metricLabels builds the final label set. Later sources override earlier
// ones: resource, data point, tenant/project, then extra labels. The result is
// sorted by label name, as the remote write protocol requires.
func metricLabels(
	name string,
	resourceLabels []otlputil.Label,
	pointAttributes []*commonpb.KeyValue,
	tenantID, projectID string,
	extra []promrw.Label,
) []promrw.Label {
	values := make(map[string]string, len(resourceLabels)+len(pointAttributes)+4)
	for _, label := range resourceLabels {
		if label.Name == "" || label.Name == "__name__" {
			continue
		}
		values[label.Name] = label.Value
	}
	for _, attribute := range pointAttributes {
		labelName := otlputil.SanitizeLabelName(attribute.GetKey())
		if labelName == "" || labelName == "__name__" {
			continue
		}
		values[labelName] = otlputil.ValueString(attribute.GetValue())
	}
	values["tenant_id"] = tenantID
	values["project_id"] = projectID
	for _, label := range extra {
		values[label.Name] = label.Value
	}

	names := make([]string, 0, len(values)+1)
	names = append(names, "__name__")
	for labelName := range values {
		names = append(names, labelName)
	}
	sort.Strings(names[1:])
	labels := make([]promrw.Label, 0, len(names))
	for _, labelName := range names {
		if labelName == "__name__" {
			labels = append(labels, promrw.Label{Name: "__name__", Value: name})
			continue
		}
		labels = append(labels, promrw.Label{Name: labelName, Value: values[labelName]})
	}
	return labels
}

// appendLabel returns a copy with one label replaced or appended. It keeps the
// label set sorted so callers can still use it as a canonical key.
func appendLabel(labels []promrw.Label, name, value string) []promrw.Label {
	out := make([]promrw.Label, len(labels), len(labels)+1)
	copy(out, labels)
	for index := range out {
		if out[index].Name == name {
			out[index].Value = value
			return out
		}
	}
	out = append(out, promrw.Label{Name: name, Value: value})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func countOtlpPoints(request *metricpb.ExportMetricsServiceRequest) int {
	total := 0
	for _, resourceMetrics := range request.GetResourceMetrics() {
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			for _, metric := range scopeMetrics.GetMetrics() {
				switch data := metric.GetData().(type) {
				case *otlpmetricpb.Metric_Gauge:
					total += len(data.Gauge.GetDataPoints())
				case *otlpmetricpb.Metric_Sum:
					total += len(data.Sum.GetDataPoints())
				case *otlpmetricpb.Metric_Histogram:
					total += len(data.Histogram.GetDataPoints())
				case *otlpmetricpb.Metric_ExponentialHistogram:
					total += len(data.ExponentialHistogram.GetDataPoints())
				case *otlpmetricpb.Metric_Summary:
					total += len(data.Summary.GetDataPoints())
				}
			}
		}
	}
	return total
}

// GroupEvents flattens events and merges series with identical labels so a
// request carries multiple samples per series. The order of first appearance
// is preserved for determinism.
func GroupEvents(events []MetricEvent) []promrw.TimeSeries {
	index := make(map[string]int, len(events))
	series := make([]promrw.TimeSeries, 0, len(events))
	for _, event := range events {
		for _, item := range event.Series {
			key := labelKey(item.Labels)
			position, ok := index[key]
			if !ok {
				index[key] = len(series)
				series = append(series, promrw.TimeSeries{
					Labels:  item.Labels,
					Samples: append([]promrw.Sample(nil), item.Samples...),
				})
				continue
			}
			series[position].Samples = append(series[position].Samples, item.Samples...)
		}
	}
	return series
}

func labelKey(labels []promrw.Label) string {
	size := 0
	for _, label := range labels {
		size += len(label.Name) + len(label.Value) + 2
	}
	buffer := make([]byte, 0, size)
	for _, label := range labels {
		buffer = append(buffer, label.Name...)
		buffer = append(buffer, 0xff)
		buffer = append(buffer, label.Value...)
		buffer = append(buffer, 0xfe)
	}
	return string(buffer)
}
