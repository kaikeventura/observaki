package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Markdown renders the human-readable report.
func (r *Report) Markdown() string {
	var builder strings.Builder
	builder.WriteString("# Observaki benchmark report\n\n")
	fmt.Fprintf(&builder, "Generated at %s by %s %s (commit %s, built %s).\n\n",
		r.GeneratedAt.Format(time.RFC3339), r.Generator.Name, r.Generator.Version, r.Generator.Commit, r.Generator.Built)
	builder.WriteString("This report includes every run that was selected, including failed and blocked runs. Comparisons only use `status: ok`, `mode: run` runs with the same scenario and config hash.\n\n")

	if len(r.Warnings) > 0 {
		builder.WriteString("## Warnings\n\n")
		for _, warning := range r.Warnings {
			fmt.Fprintf(&builder, "- %s\n", warning)
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Environment\n\n")
	builder.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&builder, "| Host | %s |\n", valueOr(r.Environment.Hostname, "unknown"))
	fmt.Fprintf(&builder, "| OS / kernel | %s/%s %s |\n", valueOr(r.Environment.OS, "?"), valueOr(r.Environment.Arch, "?"), r.Environment.Kernel)
	fmt.Fprintf(&builder, "| CPU | %s (%d logical CPUs) |\n", valueOr(r.Environment.CPUModel, "unknown"), r.Environment.CPUs)
	fmt.Fprintf(&builder, "| Memory | %s |\n", formatBytes(r.Environment.MemoryBytes))
	fmt.Fprintf(&builder, "| Go | %s |\n", r.Generator.Go)
	builder.WriteString("\n")

	builder.WriteString("## Execuções\n\n")
	builder.WriteString("| Scenario | Status | Mode | Events/s | Bytes/s | Send p95 | CPU mean | RAM mean | Compression | Error rate | Dropped |\n")
	builder.WriteString("|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, run := range r.Runs {
		fmt.Fprintf(&builder, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			run.Scenario, run.Status, run.Mode,
			formatOptional(run.valueEventsPerSecond(), "%.1f"),
			formatBytesPerSecondOptional(run.valueBytesPerSecond()),
			formatLatencyOptional(run.valueSendLatency(), "p95"),
			formatOptional(run.valueCPUMean(), "%.1f%%"),
			formatOptional(run.valueRAMMean(), "%.1f%%"),
			formatCompressionOptional(run.valueCompression()),
			formatOptional(run.valueErrorRate(), "%.3f%%"),
			formatInt64Optional(run.valueDropped()),
		)
	}
	builder.WriteString("\n")

	builder.WriteString("## Detalhe por execução\n\n")
	for _, run := range r.Runs {
		run.writeMarkdown(&builder)
	}

	if len(r.Comparisons) > 0 {
		builder.WriteString("## Variância entre repetições\n\n")
		for _, comparison := range r.Comparisons {
			comparison.writeMarkdown(&builder)
		}
	}

	r.writeMethodology(&builder)
	r.writeCostModel(&builder)
	return builder.String()
}

func (r Run) writeMarkdown(builder *strings.Builder) {
	fmt.Fprintf(builder, "### %s — %s\n\n", r.Scenario, r.Status)
	fmt.Fprintf(builder, "- Result: `%s`\n", r.ResultDir)
	fmt.Fprintf(builder, "- Commit: `%s`%s; config hash: `%s`\n", valueOr(r.Commit, "unknown"), dirtyLabel(r.Dirty), shortHash(r.ConfigHash))
	fmt.Fprintf(builder, "- Started: %s; duration: %.2fs\n", r.StartedAt.Format(time.RFC3339), r.DurationSeconds)
	fmt.Fprintf(builder, "- Hardware: %s, %d CPUs, %s RAM", valueOr(r.Hardware.CPUModel, "unknown CPU"), r.Hardware.CPUs, formatBytes(r.Hardware.MemoryBytes))
	if r.Hardware.DiskTotalBytes > 0 {
		fmt.Fprintf(builder, ", %s disk", formatBytes(r.Hardware.DiskTotalBytes))
	}
	builder.WriteString("\n")
	fmt.Fprintf(builder, "- Versions: benchctl %s, loadgen %s, Go %s, %s/%s %s\n\n",
		valueOr(r.Versions.Benchctl, "unknown"), valueOr(r.Versions.Loadgen, "unknown"), valueOr(r.Versions.Go, "?"),
		valueOr(r.Versions.OS, "?"), valueOr(r.Versions.Arch, "?"), r.Versions.Kernel)
	if r.Resolved.Endpoint != "" {
		fmt.Fprintf(builder, "- Endpoint: `%s`; duration override: %s; rate override: %d\n\n", r.Resolved.Endpoint, r.Resolved.Duration, r.Resolved.Rate)
	}

	builder.WriteString("| Metric | Value |\n|---|---|\n")
	if r.Metrics == nil {
		builder.WriteString("| All | no metrics collected |\n\n")
	} else {
		fmt.Fprintf(builder, "| Events/s | %s (target %s, delta %s) |\n",
			formatOptional(r.Metrics.EventsPerSecond, "%.1f"),
			formatIntOptional(r.Metrics.TargetRate),
			formatOptional(r.Metrics.RateDeltaPercent, "%+.2f%%"))
		fmt.Fprintf(builder, "| Bytes/s | %s |\n", formatBytesPerSecondOptional(r.Metrics.BytesPerSecond))
		if r.Metrics.CompressedBytesPerSecond != nil {
			fmt.Fprintf(builder, "| Wire bytes/s | %s |\n", formatBytesPerSecondOptional(r.Metrics.CompressedBytesPerSecond))
		}
		fmt.Fprintf(builder, "| Send latency p50/p95/p99 | %s |\n", formatLatencyTriple(r.Metrics.SendLatencyMS))
		fmt.Fprintf(builder, "| Ingest lag p50/p95/p99 | %s |\n", latencyValue(r.Metrics.IngestLagMS, r.MetricStatus, "ingest_lag_p50_ms", "ingest_lag_p95_ms", "ingest_lag_p99_ms"))
		fmt.Fprintf(builder, "| Query latency p50/p95/p99 | %s |\n", latencyValue(r.Metrics.QueryLatencyMS, r.MetricStatus, "query_latency_p50_ms", "query_latency_p95_ms", "query_latency_p99_ms"))
		if r.Metrics.CPU != nil {
			fmt.Fprintf(builder, "| CPU mean/p95/max | %.2f%% / %.2f%% / %.2f%% |\n", r.Metrics.CPU.MeanPercent, r.Metrics.CPU.P95Percent, r.Metrics.CPU.MaxPercent)
		} else {
			builder.WriteString("| CPU | not collected |\n")
		}
		if r.Metrics.RAM != nil {
			fmt.Fprintf(builder, "| RAM mean/max | %s (%.2f%%) / %.2f%% |\n", formatBytes(r.Metrics.RAM.UsedBytesMean), r.Metrics.RAM.UsedPercentMean, r.Metrics.RAM.UsedPercentMax)
		} else {
			builder.WriteString("| RAM | not collected |\n")
		}
		if r.Metrics.Disk != nil {
			fmt.Fprintf(builder, "| Disk free start/end | %s / %s (total %s) |\n", formatBytes(r.Metrics.Disk.FreeBytesStart), formatBytes(r.Metrics.Disk.FreeBytesEnd), formatBytes(r.Metrics.Disk.TotalBytes))
		} else {
			builder.WriteString("| Disk | not collected |\n")
		}
		if r.Metrics.Children != nil {
			fmt.Fprintf(builder, "| Loadgen child CPU/RSS | %.2f%% / %s |\n", r.Metrics.Children.CPUPercent, formatBytes(r.Metrics.Children.MaxRSSBytes))
		}
		if r.Metrics.Compression != nil {
			fmt.Fprintf(builder, "| Compression ratio | %.2fx (%s) |\n", *r.Metrics.Compression.Ratio, r.Metrics.Compression.Source)
			if r.Metrics.Compression.Wire != nil {
				fmt.Fprintf(builder, "| Wire compression | %.2fx (%s) |\n", r.Metrics.Compression.Wire.Ratio, strings.Join(r.Metrics.Compression.Wire.Signals, ", "))
			}
			if r.Metrics.Compression.Payload != nil {
				fmt.Fprintf(builder, "| Payload compression | %.2fx, %.1f gzip B/event |\n", r.Metrics.Compression.Payload.Ratio, r.Metrics.Compression.Payload.BytesPerEvent)
			}
		} else {
			builder.WriteString("| Compression ratio | not collected |\n")
		}
		fmt.Fprintf(builder, "| Error rate | %s |\n", formatOptional(r.Metrics.ErrorRate, "%.3f%%"))
		fmt.Fprintf(builder, "| Dropped events | %s |\n", formatInt64Optional(r.Metrics.DroppedEvents))
		fmt.Fprintf(builder, "| Confirmed lost events | %s |\n", intStatus(r.MetricStatus, "confirmed_lost_events"))
		fmt.Fprintf(builder, "| Backpressure events | %s |\n", formatInt64Optional(r.Metrics.BackpressureEvents))
		fmt.Fprintf(builder, "| Pacing lag events | %s |\n", formatInt64Optional(r.Metrics.PacingLagEvents))
		builder.WriteString("\n")
	}

	if len(r.MetricStatus) > 0 {
		builder.WriteString("**Metric coverage declared by the scenario:**\n\n")
		builder.WriteString("| Metric | Status | Detail |\n|---|---|---|\n")
		for _, status := range r.MetricStatus {
			fmt.Fprintf(builder, "| %s | %s | %s |\n", status.Metric, status.Status, escapePipes(status.Detail))
		}
		builder.WriteString("\n")
	}
	if len(r.Warnings) > 0 {
		builder.WriteString("**Warnings:**\n\n")
		for _, warning := range r.Warnings {
			fmt.Fprintf(builder, "- %s\n", warning)
		}
		builder.WriteString("\n")
	}
}

func (c Comparison) writeMarkdown(builder *strings.Builder) {
	fmt.Fprintf(builder, "### %s — `%s`\n\n", c.Scenario, shortHash(c.ConfigHash))
	fmt.Fprintf(builder, "- Runs: %d; commit(s): `%s`; same commit: %t\n", c.Runs, strings.Join(c.Commits, "`, `"), c.SameCommit)
	if c.Detail != "" {
		fmt.Fprintf(builder, "- %s\n", c.Detail)
	}
	if c.WithinTolerance != nil {
		fmt.Fprintf(builder, "- Documented variance: **%s**\n", passLabel(*c.WithinTolerance))
	}
	builder.WriteString("\n")
	builder.WriteString("| Metric | Runs | Mean | Min | Max | Spread | Tolerance | Assessed | Result |\n")
	builder.WriteString("|---|---|---|---|---|---|---|---|---|\n")
	for _, metric := range c.Metrics {
		fmt.Fprintf(builder, "| %s | %d | %.4g | %.4g | %.4g | %s | %s | %t | %s |\n",
			metric.Metric, metric.Runs, metric.Mean, metric.Min, metric.Max,
			formatSpread(metric), formatTolerance(metric), metric.Assessed, varianceResultLabel(metric))
	}
	builder.WriteString("\n")
}

func (r *Report) writeMethodology(builder *strings.Builder) {
	builder.WriteString("## Methodology\n\n")
	fmt.Fprintf(builder, "**Workload:** %s\n\n", r.Methodology.Workload)

	keys := make([]string, 0, len(r.Methodology.Definitions))
	for key := range r.Methodology.Definitions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	builder.WriteString("### Definitions\n\n")
	builder.WriteString("| Metric | Definition |\n|---|---|\n")
	for _, key := range keys {
		fmt.Fprintf(builder, "| %s | %s |\n", key, escapePipes(r.Methodology.Definitions[key]))
	}
	builder.WriteString("\n")

	builder.WriteString("### Documented variance\n\n")
	builder.WriteString("| Metric | Unit | Tolerance | Minimum runs | Note |\n|---|---|---|---|---|\n")
	for _, rule := range r.Methodology.Variance {
		tolerance := fmt.Sprintf("%.1f%%", rule.TolerancePercent)
		if rule.ToleranceAbsolute > 0 {
			tolerance = fmt.Sprintf("%.1f pp", rule.ToleranceAbsolute)
		}
		fmt.Fprintf(builder, "| %s | %s | %s | %d | %s |\n", rule.Metric, rule.Unit, tolerance, rule.MinRuns, escapePipes(rule.Note))
	}
	builder.WriteString("\n")

	builder.WriteString("### Repetition rules\n\n")
	for _, rule := range r.Methodology.Repetitions {
		fmt.Fprintf(builder, "- %s\n", rule)
	}
	builder.WriteString("\n")

	builder.WriteString("### Limitations\n\n")
	for _, limitation := range r.Methodology.Limitations {
		fmt.Fprintf(builder, "- %s\n", limitation)
	}
	builder.WriteString("\n")
}

func (r *Report) writeCostModel(builder *strings.Builder) {
	builder.WriteString("## Cost inputs\n\n")
	fmt.Fprintf(builder, "- Status: `%s` — %s\n", r.CostModel.Status, r.CostModel.Reason)
	fmt.Fprintf(builder, "- Events/day: %s\n", formatOptional(r.CostModel.EventsPerDay, "%.0f"))
	fmt.Fprintf(builder, "- Raw payload: %s\n", optionalTBPerDay(r.CostModel.RawTBPerDay))
	fmt.Fprintf(builder, "- Wire bytes: %s\n", optionalTBPerDay(r.CostModel.WireTBPerDay))
	fmt.Fprintf(builder, "- Reference gzip payload: %s\n", optionalTBPerDay(r.CostModel.PayloadTBPerDay))
	if len(r.CostModel.Inputs) > 0 {
		builder.WriteString("- Inputs required for the TCO decision:\n")
		for _, input := range r.CostModel.Inputs {
			fmt.Fprintf(builder, "  - %s\n", input)
		}
	}
	builder.WriteString("\n")
}

func formatLatencyTriple(latency *Latency) string {
	if latency == nil {
		return "not collected"
	}
	return fmt.Sprintf("%.2f / %.2f / %.2f ms (%d samples, %d shard(s), %s)",
		latency.P50MS, latency.P95MS, latency.P99MS, latency.Count, latency.Shards, latency.Basis)
}

func latencyValue(latency *Latency, status []MetricStatus, names ...string) string {
	if latency != nil {
		return formatLatencyTriple(latency)
	}
	for _, item := range status {
		for _, name := range names {
			if item.Metric == name {
				return "not collected — " + item.Detail
			}
		}
	}
	return "not collected — not declared by this scenario"
}

func intStatus(status []MetricStatus, name string) string {
	for _, item := range status {
		if item.Metric == name {
			if item.Status == "collected" {
				return "collected"
			}
			return "not collected — " + item.Detail
		}
	}
	return "not declared"
}

func formatSpread(metric VarianceResult) string {
	switch {
	case metric.SpreadPercent != nil:
		return fmt.Sprintf("%.2f%%", *metric.SpreadPercent)
	case metric.SpreadAbsolute != nil:
		return fmt.Sprintf("%.2f pp", *metric.SpreadAbsolute)
	default:
		return "—"
	}
}

func formatTolerance(metric VarianceResult) string {
	switch {
	case metric.TolerancePercent != nil:
		return fmt.Sprintf("%.1f%%", *metric.TolerancePercent)
	case metric.ToleranceAbsolute != nil:
		return fmt.Sprintf("%.1f pp", *metric.ToleranceAbsolute)
	default:
		return "—"
	}
}

func varianceResultLabel(metric VarianceResult) string {
	if !metric.Assessed {
		return "not assessed"
	}
	if metric.WithinTolerance == nil {
		return "—"
	}
	return passLabel(*metric.WithinTolerance)
}

func passLabel(within bool) string {
	if within {
		return "pass"
	}
	return "FAIL"
}

func formatOptional(value *float64, format string) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf(format, *value)
}

func formatIntOptional(value *int) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d", *value)
}

func formatBytesPerSecondOptional(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return formatBytes(uint64(*value)) + "/s"
}

func formatInt64Optional(value *int64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d", *value)
}

func formatLatencyOptional(latency *Latency, quantile string) string {
	if latency == nil {
		return "n/a"
	}
	switch quantile {
	case "p50":
		return fmt.Sprintf("%.2f ms", latency.P50MS)
	case "p95":
		return fmt.Sprintf("%.2f ms", latency.P95MS)
	case "p99":
		return fmt.Sprintf("%.2f ms", latency.P99MS)
	default:
		return "n/a"
	}
}

func formatCompressionOptional(compression *Compression) string {
	if compression == nil || compression.Ratio == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2fx (%s)", *compression.Ratio, compression.Source)
}

func formatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor := float64(unit)
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	for _, suffix := range units {
		if float64(value) < divisor*unit || suffix == units[len(units)-1] {
			return fmt.Sprintf("%.1f %s", float64(value)/divisor, suffix)
		}
		divisor *= unit
	}
	return fmt.Sprintf("%d B", value)
}

func optionalTBPerDay(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.3f TB/day", *value)
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func dirtyLabel(dirty bool) string {
	if dirty {
		return " (dirty tree)"
	}
	return ""
}

func shortHash(hash string) string {
	if hash == "" {
		return "none"
	}
	hash = strings.TrimPrefix(hash, "sha256:")
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func escapePipes(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func (r Run) valueEventsPerSecond() *float64 {
	if r.Metrics == nil {
		return nil
	}
	return r.Metrics.EventsPerSecond
}

func (r Run) valueBytesPerSecond() *float64 {
	if r.Metrics == nil {
		return nil
	}
	return r.Metrics.BytesPerSecond
}

func (r Run) valueSendLatency() *Latency {
	if r.Metrics == nil {
		return nil
	}
	return r.Metrics.SendLatencyMS
}

func (r Run) valueCPUMean() *float64 {
	if r.Metrics == nil || r.Metrics.CPU == nil {
		return nil
	}
	value := r.Metrics.CPU.MeanPercent
	return &value
}

func (r Run) valueRAMMean() *float64 {
	if r.Metrics == nil || r.Metrics.RAM == nil {
		return nil
	}
	value := r.Metrics.RAM.UsedPercentMean
	return &value
}

func (r Run) valueCompression() *Compression {
	if r.Metrics == nil {
		return nil
	}
	return r.Metrics.Compression
}

func (r Run) valueErrorRate() *float64 {
	if r.Metrics == nil {
		return nil
	}
	return r.Metrics.ErrorRate
}

func (r Run) valueDropped() *int64 {
	if r.Metrics == nil {
		return nil
	}
	return r.Metrics.DroppedEvents
}
