package report

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/observaki/observaki/bench/internal/runner"
	"github.com/observaki/observaki/bench/internal/scenario"
)

// Comparison groups runs with the same scenario and config hash so reruns can
// be judged against the documented variance.
type Comparison struct {
	Scenario        string           `json:"scenario"`
	Kind            scenario.Kind    `json:"kind"`
	ConfigHash      string           `json:"config_hash"`
	Commits         []string         `json:"commits"`
	SameCommit      bool             `json:"same_commit"`
	Runs            int              `json:"runs"`
	Detail          string           `json:"detail,omitempty"`
	WithinTolerance *bool            `json:"within_tolerance,omitempty"`
	Metrics         []VarianceResult `json:"metrics"`
}

// VarianceResult is the observed spread of one metric across reruns.
type VarianceResult struct {
	Metric            string   `json:"metric"`
	Unit              string   `json:"unit"`
	Runs              int      `json:"runs"`
	Mean              float64  `json:"mean"`
	Min               float64  `json:"min"`
	Max               float64  `json:"max"`
	StdDev            float64  `json:"stddev"`
	SpreadPercent     *float64 `json:"spread_percent,omitempty"`
	SpreadAbsolute    *float64 `json:"spread_absolute,omitempty"`
	TolerancePercent  *float64 `json:"tolerance_percent,omitempty"`
	ToleranceAbsolute *float64 `json:"tolerance_absolute,omitempty"`
	MinRuns           int      `json:"min_runs"`
	Assessed          bool     `json:"assessed"`
	WithinTolerance   *bool    `json:"within_tolerance,omitempty"`
	Detail            string   `json:"detail,omitempty"`
}

func compareRuns(runs []Run, rules []VarianceRule) ([]Comparison, []string) {
	type group struct {
		scenario scenario.Kind
		runs     []Run
	}
	groups := make(map[string]*group)
	for _, run := range runs {
		if run.Status != runner.StatusOK || run.Mode != runner.ModeRun || run.Metrics == nil {
			continue
		}
		key := run.Scenario + "\x00" + run.ConfigHash
		current, ok := groups[key]
		if !ok {
			current = &group{scenario: run.Kind}
			groups[key] = current
		}
		current.runs = append(current.runs, run)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	comparisons := make([]Comparison, 0, len(keys))
	var warnings []string
	for _, key := range keys {
		current := groups[key]
		parts := strings.SplitN(key, "\x00", 2)
		comparison := Comparison{
			Scenario:   parts[0],
			Kind:       current.scenario,
			ConfigHash: parts[1],
			Runs:       len(current.runs),
			Commits:    uniqueCommits(current.runs),
		}
		comparison.SameCommit = len(comparison.Commits) <= 1
		if !comparison.SameCommit {
			comparison.Detail = "runs span multiple commits; variance is listed but not assessed as a rerun"
		}
		assessedAny := false
		withinAll := true
		for _, rule := range rules {
			result := varianceFor(current.runs, rule)
			if result.Assessed {
				assessedAny = true
				if result.WithinTolerance != nil && !*result.WithinTolerance {
					withinAll = false
					warnings = append(warnings, fmt.Sprintf("%s: %s spread exceeds the documented tolerance (%s)", parts[0], rule.Metric, result.Detail))
				}
			}
			comparison.Metrics = append(comparison.Metrics, result)
		}
		if assessedAny {
			within := withinAll
			comparison.WithinTolerance = &within
		}
		comparisons = append(comparisons, comparison)
	}
	return comparisons, warnings
}

func varianceFor(runs []Run, rule VarianceRule) VarianceResult {
	result := VarianceResult{
		Metric:            rule.Metric,
		Unit:              rule.Unit,
		MinRuns:           rule.MinRuns,
		TolerancePercent:  percentPointer(rule.TolerancePercent),
		ToleranceAbsolute: percentPointer(rule.ToleranceAbsolute),
	}
	var values []float64
	for _, run := range runs {
		if value, ok := metricValue(run, rule.Metric); ok {
			values = append(values, value)
		}
	}
	result.Runs = len(values)
	if len(values) == 0 {
		result.Detail = "no run provides this metric"
		return result
	}
	sort.Float64s(values)
	result.Min = values[0]
	result.Max = values[len(values)-1]
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	result.Mean = sum / float64(len(values))
	variance := 0.0
	for _, value := range values {
		variance += (value - result.Mean) * (value - result.Mean)
	}
	result.StdDev = math.Sqrt(variance / float64(len(values)))

	if rule.TolerancePercent > 0 {
		spread := 0.0
		if result.Mean != 0 {
			spread = (result.Max - result.Min) / result.Mean * 100
		}
		result.SpreadPercent = &spread
	}
	if rule.ToleranceAbsolute > 0 {
		spread := result.Max - result.Min
		result.SpreadAbsolute = &spread
	}

	sameCommit := len(uniqueCommits(runs)) <= 1
	switch {
	case !sameCommit:
		result.Detail = "runs span multiple commits; variance not assessed"
		return result
	case len(values) < rule.MinRuns:
		result.Detail = fmt.Sprintf("%d run(s) with this metric; %d required for a variance assessment", len(values), rule.MinRuns)
		return result
	}
	result.Assessed = true
	within := true
	if rule.TolerancePercent > 0 && result.SpreadPercent != nil {
		within = *result.SpreadPercent <= rule.TolerancePercent
		if !within {
			result.Detail = fmt.Sprintf("%.2f%% > %.2f%%", *result.SpreadPercent, rule.TolerancePercent)
		} else {
			result.Detail = fmt.Sprintf("%.2f%% within %.2f%%", *result.SpreadPercent, rule.TolerancePercent)
		}
	}
	if rule.ToleranceAbsolute > 0 && result.SpreadAbsolute != nil {
		within = within && *result.SpreadAbsolute <= rule.ToleranceAbsolute
		if !within {
			result.Detail = fmt.Sprintf("%.2fpp > %.2fpp", *result.SpreadAbsolute, rule.ToleranceAbsolute)
		} else {
			result.Detail = fmt.Sprintf("%.2fpp within %.2fpp", *result.SpreadAbsolute, rule.ToleranceAbsolute)
		}
	}
	result.WithinTolerance = &within
	return result
}

func metricValue(run Run, metric string) (float64, bool) {
	metrics := run.Metrics
	if metrics == nil {
		return 0, false
	}
	switch metric {
	case "events_per_second":
		return deref(metrics.EventsPerSecond)
	case "bytes_per_second":
		return deref(metrics.BytesPerSecond)
	case "send_latency_p50_ms":
		if metrics.SendLatencyMS != nil {
			return metrics.SendLatencyMS.P50MS, true
		}
	case "send_latency_p95_ms":
		if metrics.SendLatencyMS != nil {
			return metrics.SendLatencyMS.P95MS, true
		}
	case "send_latency_p99_ms":
		if metrics.SendLatencyMS != nil {
			return metrics.SendLatencyMS.P99MS, true
		}
	case "cpu_percent_mean":
		if metrics.CPU != nil {
			return metrics.CPU.MeanPercent, true
		}
	case "memory_used_percent_mean":
		if metrics.RAM != nil {
			return metrics.RAM.UsedPercentMean, true
		}
	case "compression_ratio":
		if metrics.Compression != nil && metrics.Compression.Ratio != nil {
			return *metrics.Compression.Ratio, true
		}
	case "error_rate":
		return deref(metrics.ErrorRate)
	}
	return 0, false
}

func deref(value *float64) (float64, bool) {
	if value == nil {
		return 0, false
	}
	return *value, true
}

func percentPointer(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

func uniqueCommits(runs []Run) []string {
	seen := make(map[string]struct{}, len(runs))
	var commits []string
	for _, run := range runs {
		commit := run.Commit
		if commit == "" {
			commit = "unknown"
		}
		if _, ok := seen[commit]; ok {
			continue
		}
		seen[commit] = struct{}{}
		commits = append(commits, commit)
	}
	sort.Strings(commits)
	return commits
}
