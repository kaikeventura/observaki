// Package report turns completed benchmark run folders into a reproducible
// report in Markdown and JSON. It reads the run record, the load generator
// summaries and the resource samples produced by the runner; it never invents
// metrics that the current Phase 0 stack cannot measure.
package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/observaki/observaki/bench/internal/benchversion"
	"github.com/observaki/observaki/bench/internal/resources"
	"github.com/observaki/observaki/bench/internal/runner"
	"github.com/observaki/observaki/bench/internal/scenario"
	"github.com/observaki/observaki/bench/loadgen"
)

// SchemaVersion is the report.json schema version.
const SchemaVersion = 1

// Options configures report generation.
type Options struct {
	// RunDirs selects explicit result folders. When empty, ResultsRoot is
	// scanned for folders that contain a run.json.
	RunDirs     []string
	ResultsRoot string
	// Scenario filters runs by scenario name.
	Scenario string
	// Now is injectable for deterministic tests.
	Now func() time.Time
}

func (o Options) normalized() Options {
	if o.ResultsRoot == "" {
		o.ResultsRoot = filepath.Join("bench", "results")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

// Report is the machine-readable report.
type Report struct {
	SchemaVersion int             `json:"schema_version"`
	GeneratedAt   time.Time       `json:"generated_at"`
	Generator     Generator       `json:"generator"`
	Methodology   Methodology     `json:"methodology"`
	Environment   runner.HostInfo `json:"environment"`
	Runs          []Run           `json:"runs"`
	Comparisons   []Comparison    `json:"comparisons,omitempty"`
	CostModel     CostModel       `json:"cost_model"`
	Warnings      []string        `json:"warnings,omitempty"`
}

// Generator identifies the report producer.
type Generator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Built   string `json:"built"`
	Go      string `json:"go"`
}

// Methodology documents the measurement rules and the documented variance.
type Methodology struct {
	Workload    string            `json:"workload"`
	Definitions map[string]string `json:"definitions"`
	Variance    []VarianceRule    `json:"variance"`
	Repetitions []string          `json:"repetitions"`
	Limitations []string          `json:"limitations"`
}

// VarianceRule is one documented tolerance for rerun comparisons.
type VarianceRule struct {
	Metric            string  `json:"metric"`
	Unit              string  `json:"unit"`
	TolerancePercent  float64 `json:"tolerance_percent,omitempty"`
	ToleranceAbsolute float64 `json:"tolerance_absolute,omitempty"`
	MinRuns           int     `json:"min_runs"`
	Note              string  `json:"note,omitempty"`
}

// Run is one benchmark result folder.
type Run struct {
	Scenario        string          `json:"scenario"`
	Kind            scenario.Kind   `json:"kind"`
	Mode            runner.Mode     `json:"mode"`
	Status          runner.Status   `json:"status"`
	ResultDir       string          `json:"result_dir"`
	ConfigHash      string          `json:"config_hash"`
	Commit          string          `json:"commit"`
	Dirty           bool            `json:"dirty"`
	StartedAt       time.Time       `json:"started_at"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty"`
	DurationSeconds float64         `json:"duration_seconds"`
	Hardware        Hardware        `json:"hardware"`
	Versions        Versions        `json:"versions"`
	Resolved        runner.Resolved `json:"resolved"`
	Metrics         *Metrics        `json:"metrics,omitempty"`
	MetricStatus    []MetricStatus  `json:"metric_status"`
	Warnings        []string        `json:"warnings,omitempty"`
	Raw             Raw             `json:"raw"`
}

// Hardware is the machine that produced a run.
type Hardware struct {
	Hostname       string `json:"hostname"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	Kernel         string `json:"kernel,omitempty"`
	CPUModel       string `json:"cpu_model,omitempty"`
	CPUs           int    `json:"cpus"`
	MemoryBytes    uint64 `json:"memory_bytes,omitempty"`
	DiskTotalBytes uint64 `json:"disk_total_bytes,omitempty"`
}

// Versions identifies the software that produced a run.
type Versions struct {
	Benchctl string `json:"benchctl"`
	Loadgen  string `json:"loadgen"`
	Go       string `json:"go"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Kernel   string `json:"kernel,omitempty"`
}

// MetricStatus states whether a metric declared by the scenario contract was
// collected, and why it was not when it is missing.
type MetricStatus struct {
	Metric string `json:"metric"`
	Status string `json:"status"` // collected | not_collected
	Detail string `json:"detail,omitempty"`
}

// Raw keeps the source artifacts of a run inside report.json.
type Raw struct {
	Run       json.RawMessage      `json:"run"`
	Shards    []loadgen.Summary    `json:"shards,omitempty"`
	Resources *resources.File      `json:"resources,omitempty"`
	Payload   []loadgen.SizeReport `json:"payload,omitempty"`
}

// CostModel publishes the volume inputs required by the TCO decision. Pricing
// assumptions remain a founder decision (ADR-0006 Q-07).
type CostModel struct {
	Status          string   `json:"status"`
	Reason          string   `json:"reason"`
	EventsPerDay    *float64 `json:"events_per_day,omitempty"`
	RawTBPerDay     *float64 `json:"raw_tb_per_day,omitempty"`
	WireTBPerDay    *float64 `json:"wire_tb_per_day,omitempty"`
	PayloadTBPerDay *float64 `json:"payload_tb_per_day,omitempty"`
	Inputs          []string `json:"inputs,omitempty"`
}

// Build reads the selected runs and assembles the report. It does not write
// anything; call Write after a successful build.
func Build(options Options) (*Report, error) {
	options = options.normalized()
	dirs, err := selectRunDirs(options)
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return nil, errors.New("no benchmark result folders found; run a scenario first")
	}

	now := options.Now().UTC()
	report := &Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   now,
		Generator: Generator{
			Name:    "benchctl",
			Version: benchversion.Version,
			Commit:  benchversion.Commit,
			Built:   benchversion.Date,
			Go:      runtime.Version(),
		},
		Methodology: DefaultMethodology(),
		Environment: runner.CurrentHost(),
		Warnings:    []string{},
	}

	for _, dir := range dirs {
		run, err := loadRun(dir)
		if err != nil {
			return nil, fmt.Errorf("load run %s: %w", dir, err)
		}
		if options.Scenario != "" && run.Scenario != options.Scenario {
			continue
		}
		report.Runs = append(report.Runs, run)
	}
	if len(report.Runs) == 0 {
		return nil, fmt.Errorf("no runs match scenario %q", options.Scenario)
	}
	sort.Slice(report.Runs, func(i, j int) bool {
		if !report.Runs[i].StartedAt.Equal(report.Runs[j].StartedAt) {
			return report.Runs[i].StartedAt.Before(report.Runs[j].StartedAt)
		}
		return report.Runs[i].Scenario < report.Runs[j].Scenario
	})

	comparisons, warnings := compareRuns(report.Runs, report.Methodology.Variance)
	report.Comparisons = comparisons
	report.Warnings = append(report.Warnings, warnings...)
	for _, run := range report.Runs {
		report.Warnings = append(report.Warnings, run.Warnings...)
	}
	report.CostModel = buildCostModel(report.Runs)
	return report, nil
}

// Write emits report.json and report.md atomically in dir.
func (r *Report) Write(dir string) (jsonPath, markdownPath string, err error) {
	if r == nil {
		return "", "", errors.New("nil report")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("create report directory: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("encode report: %w", err)
	}
	data = append(data, '\n')
	jsonPath = filepath.Join(dir, "report.json")
	if err := writeFileAtomic(jsonPath, data); err != nil {
		return "", "", err
	}
	markdownPath = filepath.Join(dir, "report.md")
	if err := writeFileAtomic(markdownPath, []byte(r.Markdown())); err != nil {
		return jsonPath, "", err
	}
	return jsonPath, markdownPath, nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("publish %s: %w", path, err)
	}
	return nil
}

func selectRunDirs(options Options) ([]string, error) {
	if len(options.RunDirs) > 0 {
		dirs := make([]string, 0, len(options.RunDirs))
		for _, dir := range options.RunDirs {
			absolute, err := filepath.Abs(dir)
			if err != nil {
				return nil, fmt.Errorf("resolve run directory %s: %w", dir, err)
			}
			if _, err := os.Stat(filepath.Join(absolute, "run.json")); err != nil {
				return nil, fmt.Errorf("%s: %w (a run folder must contain run.json)", dir, err)
			}
			dirs = append(dirs, absolute)
		}
		return dirs, nil
	}

	root, err := filepath.Abs(options.ResultsRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve results root: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("results root %s does not exist", root)
		}
		return nil, fmt.Errorf("read results root: %w", err)
	}
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "run.json")); err == nil {
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func loadRun(dir string) (Run, error) {
	runData, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		return Run{}, err
	}
	var record runner.Record
	if err := json.Unmarshal(runData, &record); err != nil {
		return Run{}, fmt.Errorf("decode run.json: %w", err)
	}
	if record.SchemaVersion != 1 {
		return Run{}, fmt.Errorf("unsupported run schema version %d", record.SchemaVersion)
	}

	run := Run{
		Scenario:        record.Scenario.Name,
		Kind:            record.Scenario.Kind,
		Mode:            record.Mode,
		Status:          record.Status,
		ResultDir:       dir,
		ConfigHash:      record.ConfigHash,
		Commit:          record.Git.Commit,
		Dirty:           record.Git.Dirty,
		StartedAt:       record.StartedAt,
		FinishedAt:      record.FinishedAt,
		DurationSeconds: record.DurationSeconds,
		Versions: Versions{
			Benchctl: record.Versions.Benchctl,
			Loadgen:  record.Versions.Benchctl,
			Go:       record.Versions.Go,
			OS:       record.Versions.OS,
			Arch:     record.Versions.Arch,
			Kernel:   record.Versions.Kernel,
		},
		Resolved: record.Resolved,
		Raw:      Raw{Run: json.RawMessage(runData)},
	}
	run.Hardware = Hardware{
		Hostname:    record.Host.Hostname,
		OS:          record.Host.OS,
		Arch:        record.Host.Arch,
		Kernel:      record.Host.Kernel,
		CPUModel:    record.Host.CPUModel,
		CPUs:        record.Host.CPUs,
		MemoryBytes: record.Host.MemoryBytes,
	}
	if run.Versions.Benchctl == "" {
		run.Versions.Benchctl = "unknown"
		run.Versions.Loadgen = "unknown"
	}
	if run.ConfigHash == "" {
		run.Warnings = append(run.Warnings, "run.json has no config_hash; report predates the reproducibility contract")
	}

	var shardSummaries []loadgen.Summary
	for _, shard := range record.Shards {
		if shard.SummaryFile == "" {
			continue
		}
		summaryData, err := os.ReadFile(filepath.Join(dir, shard.SummaryFile))
		if err != nil {
			run.Warnings = append(run.Warnings, fmt.Sprintf("shard %d summary: %v", shard.Index, err))
			continue
		}
		var summary loadgen.Summary
		if err := json.Unmarshal(summaryData, &summary); err != nil {
			run.Warnings = append(run.Warnings, fmt.Sprintf("shard %d summary: %v", shard.Index, err))
			continue
		}
		shardSummaries = append(shardSummaries, summary)
	}
	run.Raw.Shards = shardSummaries

	resourceFile, resourceErr := (*resources.File)(nil), error(nil)
	if record.Status != runner.StatusPlanned && record.Status != runner.StatusBlocked {
		resourceFile, resourceErr = loadResources(dir, record)
		if resourceErr != nil {
			run.Warnings = append(run.Warnings, resourceErr.Error())
		} else if resourceFile != nil {
			run.Raw.Resources = resourceFile
			if resourceFile.Summary.DiskTotalBytes > 0 {
				run.Hardware.DiskTotalBytes = resourceFile.Summary.DiskTotalBytes
			}
		}
	}

	run.Metrics, run.Raw.Payload = buildMetrics(&record, shardSummaries, resourceFile)
	run.MetricStatus = buildMetricStatus(record.Scenario.Metrics, run.Metrics, &record, resourceFile, resourceErr)
	return run, nil
}

func loadResources(dir string, record runner.Record) (*resources.File, error) {
	name := record.ResourcesFile
	if name == "" {
		name = "resources.json"
	}
	path := filepath.Join(dir, name)
	file, err := resources.Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("resources.json not found; resource metrics are unavailable for this run")
		}
		return nil, fmt.Errorf("read resources: %w", err)
	}
	return &file, nil
}

// DefaultMethodology is the benchmark contract used to judge reruns.
func DefaultMethodology() Methodology {
	return Methodology{
		Workload: "60% metrics (~40 B), 30% logs (~200 B) and 10% traces (~500 B), weighted average ~134 B/event after gzip; 60/30/10 signal mix.",
		Definitions: map[string]string{
			"events_per_second":     "Events generated divided by the longest load generator shard duration (runner aggregate).",
			"bytes_per_second":      "Uncompressed protobuf bytes accepted by the receiver divided by the longest shard duration.",
			"send_latency_ms":       "OTLP export request latency measured by the load generator; with multiple shards the report keeps the worst shard's p50/p95/p99.",
			"ingest_lag_ms":         "Time between the event timestamp and the durable write. Requires gateway/WAL metrics (Story 4.3); not collected in Phase 0 throughput runs.",
			"query_latency_ms":      "Query Service latency for the 20-query dashboard set. Requires the Query Service (Story 3.5).",
			"cpu_percent":           "Machine-wide CPU busy percentage sampled once per second; 100 means all CPUs busy.",
			"ram_bytes":             "Machine memory in use (MemTotal minus MemAvailable), sampled once per second.",
			"disk_bytes":            "Filesystem total and free bytes on the result path, sampled once per second.",
			"compression_ratio":     "Raw protobuf bytes divided by wire bytes when transport compression is measurable; otherwise the deterministic payload ratio from the same seed/config. Wire and payload ratios can differ because the workload interleaves signal indices while the payload calibration uses isolated batches. Storage compression (ClickHouse) lands with Story 3.1-3.3.",
			"error_rate":            "Failed plus rejected events divided by generated events.",
			"dropped_events":        "Failed plus rejected events reported by the load generator.",
			"confirmed_lost_events": "Requires gateway/WAL durability accounting (Stories 4.3-4.5); not collected in Phase 0 throughput runs.",
		},
		Variance: []VarianceRule{
			{Metric: "events_per_second", Unit: "events/s", TolerancePercent: 5, MinRuns: 2, Note: "ADR-0006 allows one continuous 1h gate run; with fewer than two runs the variance is not assessed."},
			{Metric: "bytes_per_second", Unit: "B/s", TolerancePercent: 5, MinRuns: 2},
			{Metric: "send_latency_p50_ms", Unit: "ms", TolerancePercent: 10, MinRuns: 3},
			{Metric: "send_latency_p95_ms", Unit: "ms", TolerancePercent: 10, MinRuns: 3},
			{Metric: "send_latency_p99_ms", Unit: "ms", TolerancePercent: 10, MinRuns: 3},
			{Metric: "cpu_percent_mean", Unit: "%", TolerancePercent: 10, MinRuns: 2},
			{Metric: "memory_used_percent_mean", Unit: "%", TolerancePercent: 10, MinRuns: 2},
			{Metric: "compression_ratio", Unit: "x", TolerancePercent: 5, MinRuns: 2},
			{Metric: "error_rate", Unit: "percentage points", ToleranceAbsolute: 0.1, MinRuns: 2},
		},
		Repetitions: []string{
			"Latency and error-rate comparisons need at least 3 runs (ADR-0006 rule 10); the report states when the minimum is not met.",
			"A 1h throughput gate run may be a single continuous execution when stability metrics (backpressure, pacing lag, resource samples) are published.",
			"Variance is only assessed for runs with the same scenario and config hash. Runs spanning commits are listed but not judged as reruns.",
			"Every run is listed, including failed and blocked ones; comparisons use only status=ok, mode=run runs.",
		},
		Limitations: []string{
			"Ingest lag p50/p95/p99 and confirmed lost events require the gateway/WAL (Stories 4.3-4.5); Phase 0 throughput runs report generator send latency instead and mark those metrics not collected.",
			"Query and cardinality latency require the Query Service and the series generator (Stories 3.4-3.5); their scenarios are reported as plans/blocked until then.",
			"ClickHouse/VictoriaMetrics storage compression is not measured here; compression_ratio is the wire or deterministic payload ratio, labelled by source.",
			"Resource sampling is host-level on Linux; other platforms report the metrics as unavailable.",
			"Pricing assumptions are an open founder decision (ADR-0006 Q-07); the report publishes the volume inputs, not a TCO verdict.",
		},
	}
}
