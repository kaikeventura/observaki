package report

import (
	"math"
	"time"

	"github.com/observaki/observaki/bench/internal/resources"
	"github.com/observaki/observaki/bench/internal/runner"
	"github.com/observaki/observaki/bench/loadgen"
)

// Metrics is the normalized result of one run. A nil pointer means the metric
// was not collected; MetricStatus explains why.
type Metrics struct {
	EventsPerSecond          *float64     `json:"events_per_second,omitempty"`
	TargetRate               *int         `json:"target_rate,omitempty"`
	RateDeltaPercent         *float64     `json:"rate_delta_percent,omitempty"`
	RateWithinTolerance      *bool        `json:"rate_within_tolerance,omitempty"`
	BytesPerSecond           *float64     `json:"bytes_per_second,omitempty"`
	CompressedBytesPerSecond *float64     `json:"compressed_bytes_per_second,omitempty"`
	SendLatencyMS            *Latency     `json:"send_latency_ms,omitempty"`
	IngestLagMS              *Latency     `json:"ingest_lag_ms,omitempty"`
	QueryLatencyMS           *Latency     `json:"query_latency_ms,omitempty"`
	CPU                      *CPU         `json:"cpu,omitempty"`
	RAM                      *RAM         `json:"ram,omitempty"`
	Disk                     *Disk        `json:"disk,omitempty"`
	Children                 *Children    `json:"children,omitempty"`
	Compression              *Compression `json:"compression,omitempty"`
	ErrorRate                *float64     `json:"error_rate,omitempty"`
	DroppedEvents            *int64       `json:"dropped_events,omitempty"`
	ConfirmedLostEvents      *int64       `json:"confirmed_lost_events,omitempty"`
	BackpressureEvents       *int64       `json:"backpressure_events,omitempty"`
	PacingLagEvents          *int64       `json:"pacing_lag_events,omitempty"`
	GeneratedEvents          *int64       `json:"generated_events,omitempty"`
	SentEvents               *int64       `json:"sent_events,omitempty"`
}

// Latency is a quantile summary in milliseconds.
type Latency struct {
	P50MS  float64 `json:"p50_ms"`
	P95MS  float64 `json:"p95_ms"`
	P99MS  float64 `json:"p99_ms"`
	Count  int64   `json:"count"`
	Shards int     `json:"shards"`
	Basis  string  `json:"basis,omitempty"`
}

// CPU is the machine CPU summary for a run.
type CPU struct {
	MeanPercent float64 `json:"mean_percent"`
	P95Percent  float64 `json:"p95_percent"`
	MaxPercent  float64 `json:"max_percent"`
}

// RAM is the machine memory summary for a run.
type RAM struct {
	TotalBytes      uint64  `json:"total_bytes"`
	UsedBytesMean   uint64  `json:"used_bytes_mean"`
	UsedPercentMean float64 `json:"used_percent_mean"`
	UsedPercentMax  float64 `json:"used_percent_max"`
}

// Disk is the filesystem summary for a run.
type Disk struct {
	TotalBytes     uint64 `json:"total_bytes"`
	FreeBytesStart uint64 `json:"free_bytes_start"`
	FreeBytesEnd   uint64 `json:"free_bytes_end"`
}

// Children is the accumulated resource usage of the load generator processes.
type Children struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MaxRSSBytes   uint64  `json:"max_rss_bytes"`
	UserSeconds   float64 `json:"user_seconds"`
	SystemSeconds float64 `json:"system_seconds"`
}

// Compression contains the two ratios the harness can measure today: wire
// compression and the deterministic payload ratio of the reference workload.
type Compression struct {
	Ratio   *float64                `json:"ratio,omitempty"`
	Source  string                  `json:"source,omitempty"`
	Wire    *CompressionMeasurement `json:"wire,omitempty"`
	Payload *CompressionMeasurement `json:"payload,omitempty"`
}

// CompressionMeasurement is one compression observation.
type CompressionMeasurement struct {
	Ratio           float64  `json:"ratio"`
	RawBytes        int64    `json:"raw_bytes,omitempty"`
	CompressedBytes int64    `json:"compressed_bytes,omitempty"`
	BytesPerEvent   float64  `json:"bytes_per_event,omitempty"`
	Signals         []string `json:"signals,omitempty"`
}

func buildMetrics(record *runner.Record, shards []loadgen.Summary, resourceFile *resources.File) (*Metrics, []loadgen.SizeReport) {
	metrics := &Metrics{}
	collected := false

	if record.Aggregate != nil {
		value := record.Aggregate.AchievedRate
		metrics.EventsPerSecond = &value
		target := record.Aggregate.TargetRate
		metrics.TargetRate = &target
		delta := record.Aggregate.RateDeltaPercent
		metrics.RateDeltaPercent = &delta
		within := record.Aggregate.RateWithinTolerance
		metrics.RateWithinTolerance = &within
		generated := record.Aggregate.Generated
		metrics.GeneratedEvents = &generated
		sent := record.Aggregate.Sent
		metrics.SentEvents = &sent
		failed := record.Aggregate.Failed
		rejected := record.Aggregate.Rejected
		dropped := failed + rejected
		metrics.DroppedEvents = &dropped
		backpressure := record.Aggregate.BackpressureEvents
		metrics.BackpressureEvents = &backpressure
		pacing := record.Aggregate.PacingLagEvents
		metrics.PacingLagEvents = &pacing
		if generated > 0 {
			errorRate := float64(dropped) / float64(generated) * 100
			metrics.ErrorRate = &errorRate
		}
		collected = true
	}

	payload, mix := payloadCompression(shards)
	duration := maxShardDuration(shards)
	if duration <= 0 && record.Aggregate != nil {
		duration = record.Aggregate.MaxShardDurationSeconds
	}
	if duration > 0 && len(shards) > 0 {
		rawBytes, compressedBytes, compressedSignals, latency := summarizeShards(shards)
		bytesPerSecond := float64(rawBytes) / duration
		metrics.BytesPerSecond = &bytesPerSecond
		if compressedBytes > 0 {
			compressedPerSecond := float64(compressedBytes) / duration
			metrics.CompressedBytesPerSecond = &compressedPerSecond
		}
		metrics.SendLatencyMS = latency
		metrics.Compression = buildCompression(rawBytes, compressedBytes, compressedSignals, payload, mix)
		collected = true
	}

	if resourceFile != nil && resourceFile.Summary.Samples > 0 {
		summary := resourceFile.Summary
		if summary.CPUAvailable {
			metrics.CPU = &CPU{
				MeanPercent: summary.CPUPercent.Mean,
				P95Percent:  summary.CPUPercent.P95,
				MaxPercent:  summary.CPUPercent.Max,
			}
		}
		if summary.MemoryAvailable && summary.MemoryUsedBytes.Max > 0 {
			total := uint64(0)
			for _, sample := range resourceFile.Samples {
				if sample.MemoryTotalBytes > 0 {
					total = sample.MemoryTotalBytes
					break
				}
			}
			metrics.RAM = &RAM{
				TotalBytes:      total,
				UsedBytesMean:   uint64(summary.MemoryUsedBytes.Mean),
				UsedPercentMean: summary.MemoryUsedPercent.Mean,
				UsedPercentMax:  summary.MemoryUsedPercent.Max,
			}
		}
		if summary.DiskAvailable && summary.DiskTotalBytes > 0 {
			metrics.Disk = &Disk{
				TotalBytes:     summary.DiskTotalBytes,
				FreeBytesStart: summary.DiskFreeBytesStart,
				FreeBytesEnd:   summary.DiskFreeBytesEnd,
			}
		}
		if summary.Children.Available {
			metrics.Children = &Children{
				CPUPercent:    summary.Children.CPUPercent,
				MaxRSSBytes:   summary.Children.MaxRSSBytes,
				UserSeconds:   summary.Children.UserSeconds,
				SystemSeconds: summary.Children.SystemSeconds,
			}
		}
		collected = true
	}

	if !collected {
		return nil, payload
	}
	return metrics, payload
}

func summarizeShards(shards []loadgen.Summary) (rawBytes, compressedBytes int64, signals []string, latency *Latency) {
	for _, summary := range shards {
		rawBytes += summary.Metrics.Bytes
		if summary.Metrics.CompressedBytes > 0 {
			compressedBytes += summary.Metrics.CompressedBytes
			for _, signal := range summary.Metrics.Signals {
				if signal.CompressedBytes > 0 {
					signals = appendUnique(signals, signal.Signal)
				}
			}
		}
		latency = mergeLatency(latency, summary.Metrics.Latency)
	}
	return rawBytes, compressedBytes, signals, latency
}

func buildCompression(rawBytes, compressedBytes int64, signals []string, payloadReports []loadgen.SizeReport, mix loadgen.Mix) *Compression {
	compression := &Compression{}
	if compressedBytes > 0 && rawBytes > 0 {
		ratio := float64(rawBytes) / float64(compressedBytes)
		compression.Wire = &CompressionMeasurement{
			Ratio:           ratio,
			RawBytes:        rawBytes,
			CompressedBytes: compressedBytes,
			Signals:         signals,
		}
		compression.Ratio = &ratio
		compression.Source = "wire"
	}
	if payload := payloadMeasurement(payloadReports, mix); payload != nil {
		compression.Payload = payload
		if compression.Ratio == nil {
			ratio := payload.Ratio
			compression.Ratio = &ratio
			compression.Source = "payload"
		}
	}
	if compression.Ratio == nil {
		return nil
	}
	return compression
}

func payloadCompression(shards []loadgen.Summary) ([]loadgen.SizeReport, loadgen.Mix) {
	if len(shards) == 0 {
		return nil, loadgen.Mix{}
	}
	config := shards[0].Config
	cfg := loadgen.DefaultConfig()
	cfg.Protocol = config.Protocol
	cfg.Compression = config.Compression
	cfg.Rate = config.Rate
	cfg.EventTimeRate = config.EventTimeRate
	cfg.Duration = time.Duration(config.DurationSeconds * float64(time.Second))
	cfg.StartTime = config.StartTime
	cfg.Seed = config.Seed
	cfg.BatchSize = config.BatchSize
	cfg.Workers = config.Workers
	cfg.Mix = config.Mix
	cfg.Payload = config.Payload
	reports, _, err := loadgen.MeasureSizes(cfg)
	if err != nil {
		return nil, config.Mix
	}
	return reports, config.Mix
}

func payloadMeasurement(reports []loadgen.SizeReport, mix loadgen.Mix) *CompressionMeasurement {
	if len(reports) == 0 {
		return nil
	}
	var rawWeighted, gzipWeighted float64
	var rawTotal, gzipTotal int64
	var signals []string
	for _, report := range reports {
		weight := mix.Percent(report.Signal) / 100
		rawWeighted += weight * report.ProtobufBytesPerEvent
		gzipWeighted += weight * report.GzipBytesPerEvent
		rawTotal += int64(report.ProtobufBytes)
		gzipTotal += int64(report.GzipBytes)
		signals = append(signals, string(report.Signal))
	}
	if gzipWeighted <= 0 {
		return nil
	}
	return &CompressionMeasurement{
		Ratio:           rawWeighted / gzipWeighted,
		RawBytes:        rawTotal,
		CompressedBytes: gzipTotal,
		BytesPerEvent:   gzipWeighted,
		Signals:         signals,
	}
}

func maxShardDuration(shards []loadgen.Summary) float64 {
	maximum := 0.0
	for _, summary := range shards {
		if summary.DurationSeconds > maximum {
			maximum = summary.DurationSeconds
		}
	}
	return maximum
}

func mergeLatency(current *Latency, summary loadgen.LatencySummary) *Latency {
	if summary.Count == 0 {
		return current
	}
	if current == nil {
		return &Latency{
			P50MS:  summary.P50Ms,
			P95MS:  summary.P95Ms,
			P99MS:  summary.P99Ms,
			Count:  summary.Count,
			Shards: 1,
			Basis:  "max across load generator shards",
		}
	}
	current.Count += summary.Count
	current.Shards++
	current.P50MS = math.Max(current.P50MS, summary.P50Ms)
	current.P95MS = math.Max(current.P95MS, summary.P95Ms)
	current.P99MS = math.Max(current.P99MS, summary.P99Ms)
	return current
}

func appendUnique(values []string, value string) []string {
	for _, item := range values {
		if item == value {
			return values
		}
	}
	return append(values, value)
}

func buildMetricStatus(names []string, metrics *Metrics, record *runner.Record, resourceFile *resources.File, resourceErr error) []MetricStatus {
	status := make([]MetricStatus, 0, len(names))
	for _, name := range names {
		status = append(status, metricStatus(name, metrics, record, resourceFile, resourceErr))
	}
	return status
}

func metricStatus(name string, metrics *Metrics, record *runner.Record, resourceFile *resources.File, resourceErr error) MetricStatus {
	notCollected := func(detail string) MetricStatus {
		return MetricStatus{Metric: name, Status: "not_collected", Detail: detail}
	}
	collected := func() MetricStatus {
		return MetricStatus{Metric: name, Status: "collected"}
	}
	if record.Status == runner.StatusBlocked {
		return notCollected("execution is blocked: " + record.Error)
	}
	if record.Status == runner.StatusPlanned || record.Mode == runner.ModePlan {
		return notCollected("planned run; no workload was executed")
	}
	if metrics == nil {
		if record.Error != "" {
			return notCollected("run failed: " + record.Error)
		}
		return notCollected("no metrics were collected for this run")
	}

	switch name {
	case "events_per_second":
		if metrics.EventsPerSecond != nil {
			return collected()
		}
	case "bytes_per_second":
		if metrics.BytesPerSecond != nil {
			return collected()
		}
	case "ingest_lag_p50_ms", "ingest_lag_p95_ms", "ingest_lag_p99_ms":
		return notCollected("requires gateway/WAL ingest metrics (Story 4.3)")
	case "error_rate":
		if metrics.ErrorRate != nil {
			return collected()
		}
	case "dropped_events":
		if metrics.DroppedEvents != nil {
			return collected()
		}
	case "confirmed_lost_events":
		return notCollected("requires gateway/WAL durability accounting (Stories 4.3-4.5)")
	case "cpu_percent":
		if metrics.CPU != nil {
			return collected()
		}
		if resourceErr != nil {
			return notCollected(resourceErr.Error())
		}
		return notCollected("resource sampling did not produce CPU samples")
	case "ram_bytes":
		if metrics.RAM != nil {
			return collected()
		}
		if resourceErr != nil {
			return notCollected(resourceErr.Error())
		}
		return notCollected("resource sampling did not produce memory samples")
	case "disk_bytes":
		if metrics.Disk != nil {
			return collected()
		}
		if resourceErr != nil {
			return notCollected(resourceErr.Error())
		}
		return notCollected("resource sampling did not produce disk samples")
	case "compression_ratio":
		if metrics.Compression != nil && metrics.Compression.Ratio != nil {
			return MetricStatus{Metric: name, Status: "collected", Detail: "source: " + metrics.Compression.Source + " (storage compression lands with Story 3.1-3.3)"}
		}
	case "query_latency_p50_ms", "query_latency_p95_ms", "query_latency_p99_ms":
		return notCollected("requires Query Service (Story 3.5)")
	case "rows_read", "bytes_read", "cache_hit_ratio":
		return notCollected("requires Query Service (Story 3.5)")
	case "series_active", "series_limit_errors", "neighbor_p95_drift_percent":
		return notCollected("requires VictoriaMetrics series generator (Story 3.4)")
	case "recovery_seconds", "dlq_events":
		return notCollected("requires chaos automation (Story 4.5)")
	case "loadgen_backpressure_events":
		if metrics.BackpressureEvents != nil {
			return collected()
		}
	case "loadgen_pacing_lag_events":
		if metrics.PacingLagEvents != nil {
			return collected()
		}
	}
	if resourceFile != nil && resourceFile.Summary.Supported {
		return notCollected("metric is declared but the current run has no collector for it")
	}
	return notCollected("metric is declared but not collected by the Phase 0 harness yet")
}

func buildCostModel(runs []Run) CostModel {
	model := CostModel{
		Status: "not_configured",
		Reason: "Pricing assumptions are an open founder decision (ADR-0006 Q-07); this report publishes the volume inputs only.",
		Inputs: []string{
			"events per day from the highest sustained throughput run",
			"raw and wire bytes per day from the same run",
			"reference payload gzip bytes per event from the deterministic generator",
			"TCO per TB/day, replica factor and operator hours are not priced here",
		},
	}
	var best *Run
	for index := range runs {
		run := &runs[index]
		if run.Status != runner.StatusOK || run.Metrics == nil || run.Metrics.EventsPerSecond == nil {
			continue
		}
		if best == nil || *run.Metrics.EventsPerSecond > *best.Metrics.EventsPerSecond {
			best = run
		}
	}
	if best == nil {
		return model
	}
	eventsPerSecond := *best.Metrics.EventsPerSecond
	eventsPerDay := eventsPerSecond * 86400
	model.EventsPerDay = &eventsPerDay
	if best.Metrics.BytesPerSecond != nil {
		rawTB := *best.Metrics.BytesPerSecond * 86400 / 1e12
		model.RawTBPerDay = &rawTB
	}
	if best.Metrics.CompressedBytesPerSecond != nil {
		wireTB := *best.Metrics.CompressedBytesPerSecond * 86400 / 1e12
		model.WireTBPerDay = &wireTB
	}
	if best.Metrics.Compression != nil && best.Metrics.Compression.Payload != nil {
		payloadTB := eventsPerSecond * best.Metrics.Compression.Payload.BytesPerEvent * 86400 / 1e12
		model.PayloadTBPerDay = &payloadTB
	}
	return model
}
