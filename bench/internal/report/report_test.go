package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/observaki/observaki/bench/internal/resources"
	"github.com/observaki/observaki/bench/internal/runner"
	"github.com/observaki/observaki/bench/internal/scenario"
	"github.com/observaki/observaki/bench/loadgen"
)

type fixture struct {
	configHash      string
	commit          string
	eventsPerSecond float64
	latencyP95      float64
	cpuMean         float64
	ramPercent      float64
	status          runner.Status
	mode            runner.Mode
	withResources   bool
	withShards      bool
	error           string
}

func defaultFixture() fixture {
	return fixture{
		configHash:      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		commit:          "abc1234",
		eventsPerSecond: 100_000,
		latencyP95:      5,
		cpuMean:         40,
		ramPercent:      25,
		status:          runner.StatusOK,
		mode:            runner.ModeRun,
		withResources:   true,
		withShards:      true,
	}
}

func writeFixtureRun(t *testing.T, root, name string, values fixture) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).Add(time.Duration(len(name)) * time.Minute)
	finished := started.Add(6 * time.Second)
	record := runner.Record{
		SchemaVersion: 1,
		Scenario: scenario.Definition{
			Name:        "throughput-demo",
			Kind:        scenario.KindThroughput,
			Title:       "Throughput demo",
			Hardware:    "test host",
			Duration:    scenario.Duration(time.Hour),
			Requires:    []string{"loadgen", "gateway"},
			Metrics:     []string{"events_per_second", "bytes_per_second", "ingest_lag_p50_ms", "ingest_lag_p95_ms", "ingest_lag_p99_ms", "error_rate", "dropped_events", "confirmed_lost_events", "cpu_percent", "ram_bytes", "disk_bytes", "compression_ratio", "loadgen_backpressure_events", "loadgen_pacing_lag_events", "query_latency_p50_ms"},
			Expected:    "test expectation",
			Rate:        100_000,
			Shards:      1,
			Protocol:    "http",
			Compression: "gzip",
		},
		Resolved: runner.Resolved{
			Endpoint: "http://127.0.0.1:4318",
			Duration: scenario.Duration(5 * time.Second),
			Rate:     100_000,
		},
		ConfigHash:      values.configHash,
		Mode:            values.mode,
		Status:          values.status,
		StartedAt:       started,
		FinishedAt:      &finished,
		DurationSeconds: 6,
		Git:             runner.GitInfo{Commit: values.commit},
		Host: runner.HostInfo{
			Hostname:    "bench-01",
			OS:          "linux",
			Arch:        "amd64",
			Kernel:      "6.12.0-test",
			CPUModel:    "Test CPU",
			CPUs:        16,
			MemoryBytes: 64 << 30,
		},
		Versions: runner.Versions{
			Benchctl: "v-test",
			Go:       "go1.27",
			OS:       "linux",
			Arch:     "amd64",
			Kernel:   "6.12.0-test",
		},
		ResultDir: dir,
		Error:     values.error,
	}
	if values.status == runner.StatusPlanned || values.status == runner.StatusBlocked {
		record.Plan = []string{"loadgen --rate 100000"}
		record.FinishedAt = &finished
	} else if values.withShards {
		generated := int64(values.eventsPerSecond * 5)
		rawBytes := generated * 400
		compressedBytes := generated * 134
		record.Shards = []runner.ShardResult{{
			Index:         0,
			Rate:          100_000,
			Seed:          1,
			SummaryFile:   "loadgen-00.json",
			PlannedEvents: generated,
			Generated:     generated,
			Sent:          generated,
			AchievedRate:  values.eventsPerSecond,
		}}
		record.Aggregate = &runner.Aggregate{
			TargetRate:              100_000,
			AchievedRate:            values.eventsPerSecond,
			RateDeltaPercent:        (values.eventsPerSecond - 100_000) / 100_000 * 100,
			RateWithinTolerance:     true,
			PlannedEvents:           generated,
			Generated:               generated,
			Sent:                    generated,
			MaxShardDurationSeconds: 5,
		}
		summary := loadgen.Summary{
			Config: loadgen.ConfigSummary{
				Protocol:        loadgen.ProtocolHTTP,
				Compression:     loadgen.CompressionGzip,
				Rate:            100_000,
				EventTimeRate:   100_000,
				DurationSeconds: 5,
				StartTime:       started,
				Seed:            1,
				BatchSize:       1000,
				Workers:         4,
				Mix:             loadgen.DefaultMix(),
				Payload:         loadgen.DefaultPayloadSizes(),
			},
			PlannedEvents:   generated,
			FinishedAt:      finished,
			DurationSeconds: 5,
			Metrics: loadgen.MetricsSnapshot{
				Generated:       generated,
				Sent:            generated,
				Bytes:           rawBytes,
				CompressedBytes: compressedBytes,
				Latency: loadgen.LatencySummary{
					Count:  500,
					P50Ms:  values.latencyP95 * 0.4,
					P95Ms:  values.latencyP95,
					P99Ms:  values.latencyP95 * 1.8,
					MinMs:  0.1,
					MaxMs:  values.latencyP95 * 2,
					MeanMs: values.latencyP95 * 0.7,
				},
				Signals: []loadgen.SignalMetrics{
					{Signal: "metrics", Generated: generated * 6 / 10, Sent: generated * 6 / 10, Bytes: rawBytes * 6 / 10, CompressedBytes: compressedBytes * 6 / 10},
					{Signal: "logs", Generated: generated * 3 / 10, Sent: generated * 3 / 10, Bytes: rawBytes * 3 / 10, CompressedBytes: compressedBytes * 3 / 10},
					{Signal: "traces", Generated: generated / 10, Sent: generated / 10, Bytes: rawBytes / 10, CompressedBytes: compressedBytes / 10},
				},
			},
		}
		summaryData, err := json.Marshal(summary)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "loadgen-00.json"), summaryData, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if values.withResources {
		total := uint64(64 << 30)
		usedMean := uint64(float64(total) * values.ramPercent / 100)
		summary := resources.Summary{
			SchemaVersion:      1,
			Supported:          true,
			CPUAvailable:       true,
			MemoryAvailable:    true,
			DiskAvailable:      true,
			Samples:            2,
			IntervalSeconds:    1,
			DurationSeconds:    5,
			CPUPercent:         resources.Stats{Min: values.cpuMean - 5, Mean: values.cpuMean, P95: values.cpuMean + 5, Max: values.cpuMean + 10},
			MemoryUsedBytes:    resources.Stats{Min: float64(usedMean), Mean: float64(usedMean), P95: float64(usedMean), Max: float64(usedMean)},
			MemoryUsedPercent:  resources.Stats{Min: values.ramPercent - 1, Mean: values.ramPercent, P95: values.ramPercent + 1, Max: values.ramPercent + 2},
			DiskFreeBytesStart: 500 << 30,
			DiskFreeBytesEnd:   499 << 30,
			DiskTotalBytes:     1 << 40,
		}
		samples := []resources.Sample{
			{At: started, CPUPercent: values.cpuMean, CPUAvailable: true, MemoryUsedBytes: usedMean, MemoryTotalBytes: total, MemoryUsedPct: values.ramPercent, MemoryAvailable: true, DiskFreeBytes: 500 << 30, DiskTotalBytes: 1 << 40, DiskAvailable: true},
			{At: finished, CPUPercent: values.cpuMean + 1, CPUAvailable: true, MemoryUsedBytes: usedMean, MemoryTotalBytes: total, MemoryUsedPct: values.ramPercent, MemoryAvailable: true, DiskFreeBytes: 499 << 30, DiskTotalBytes: 1 << 40, DiskAvailable: true},
		}
		if _, err := resources.Write(dir, summary, samples); err != nil {
			t.Fatal(err)
		}
		record.ResourcesFile = "resources.json"
		record.Resources = &summary
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func fixedNow() time.Time {
	return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
}

func TestBuildReportWithRepetitions(t *testing.T) {
	root := t.TempDir()
	first := defaultFixture()
	second := first
	second.eventsPerSecond = 99_500
	second.latencyP95 = 5.1
	third := first
	third.eventsPerSecond = 99_250
	third.latencyP95 = 4.9
	dirs := []string{
		writeFixtureRun(t, root, "20261101T100001Z-throughput-demo", first),
		writeFixtureRun(t, root, "20261101T100002Z-throughput-demo", second),
		writeFixtureRun(t, root, "20261101T100003Z-throughput-demo", third),
	}

	report, err := Build(Options{RunDirs: dirs, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(report.Runs), 3; got != want {
		t.Fatalf("runs = %d, want %d", got, want)
	}
	run := report.Runs[0]
	if run.Metrics == nil || run.Metrics.EventsPerSecond == nil {
		t.Fatalf("metrics = %+v, want events/s", run.Metrics)
	}
	if *run.Metrics.EventsPerSecond != 100_000 {
		t.Fatalf("events/s = %f, want 100000", *run.Metrics.EventsPerSecond)
	}
	if run.Metrics.SendLatencyMS == nil || run.Metrics.SendLatencyMS.P95MS != 5 {
		t.Fatalf("send latency = %+v, want p95 5", run.Metrics.SendLatencyMS)
	}
	if run.Metrics.CPU == nil || run.Metrics.CPU.MeanPercent != 40 {
		t.Fatalf("cpu = %+v, want mean 40", run.Metrics.CPU)
	}
	if run.Metrics.RAM == nil || run.Metrics.RAM.TotalBytes != 64<<30 {
		t.Fatalf("ram = %+v, want 64 GiB total", run.Metrics.RAM)
	}
	if run.Metrics.Disk == nil || run.Metrics.Disk.TotalBytes != 1<<40 {
		t.Fatalf("disk = %+v, want 1 TiB", run.Metrics.Disk)
	}
	if run.Metrics.Compression == nil || run.Metrics.Compression.Source != "wire" {
		t.Fatalf("compression = %+v, want wire", run.Metrics.Compression)
	}
	if run.Metrics.Compression.Payload == nil || run.Metrics.Compression.Payload.BytesPerEvent < 100 {
		t.Fatalf("payload compression = %+v, want the reference ~134 B/event", run.Metrics.Compression.Payload)
	}
	if run.Metrics.ErrorRate == nil || *run.Metrics.ErrorRate != 0 {
		t.Fatalf("error rate = %+v, want 0", run.Metrics.ErrorRate)
	}
	if run.Metrics.DroppedEvents == nil || *run.Metrics.DroppedEvents != 0 {
		t.Fatalf("dropped = %+v, want 0", run.Metrics.DroppedEvents)
	}

	status := metricStatusByName(run.MetricStatus, "ingest_lag_p95_ms")
	if status == nil || status.Status != "not_collected" || !strings.Contains(status.Detail, "gateway") {
		t.Fatalf("ingest lag status = %+v, want not_collected with gateway reason", status)
	}
	status = metricStatusByName(run.MetricStatus, "cpu_percent")
	if status == nil || status.Status != "collected" {
		t.Fatalf("cpu status = %+v, want collected", status)
	}

	if len(report.Comparisons) != 1 {
		t.Fatalf("comparisons = %d, want 1", len(report.Comparisons))
	}
	comparison := report.Comparisons[0]
	if comparison.Runs != 3 || !comparison.SameCommit {
		t.Fatalf("comparison = %+v", comparison)
	}
	events := varianceByName(comparison.Metrics, "events_per_second")
	if events == nil || !events.Assessed || events.WithinTolerance == nil || !*events.WithinTolerance {
		t.Fatalf("events variance = %+v, want assessed within tolerance", events)
	}
	latency := varianceByName(comparison.Metrics, "send_latency_p95_ms")
	if latency == nil || !latency.Assessed {
		t.Fatalf("latency variance = %+v, want assessed with 3 runs", latency)
	}
	if comparison.WithinTolerance == nil || !*comparison.WithinTolerance {
		t.Fatalf("comparison within tolerance = %+v, want pass", comparison.WithinTolerance)
	}
	if report.CostModel.RawTBPerDay == nil || *report.CostModel.RawTBPerDay <= 0 {
		t.Fatalf("cost model = %+v, want raw TB/day", report.CostModel)
	}

	out := t.TempDir()
	jsonPath, markdownPath, err := report.Write(out)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	runs, ok := decoded["runs"].([]any)
	if !ok || len(runs) != 3 {
		t.Fatalf("report.json runs = %v", decoded["runs"])
	}
	firstRun := runs[0].(map[string]any)
	if _, ok := firstRun["config_hash"]; !ok {
		t.Fatal("report.json is missing config_hash")
	}
	if _, ok := firstRun["raw"]; !ok {
		t.Fatal("report.json is missing raw results")
	}
	hardware := firstRun["hardware"].(map[string]any)
	if hardware["cpu_model"] != "Test CPU" {
		t.Fatalf("hardware = %v", hardware)
	}
	versions := firstRun["versions"].(map[string]any)
	if versions["benchctl"] != "v-test" {
		t.Fatalf("versions = %v", versions)
	}

	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(markdown)
	for _, want := range []string{
		"# Observaki benchmark report",
		"## Variância entre repetições",
		"## Methodology",
		"throughput-demo",
		"requires gateway/WAL",
		"Documented variance: **pass**",
		"Cost inputs",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("report.md is missing %q", want)
		}
	}
}

func TestVarianceExceededIsReported(t *testing.T) {
	root := t.TempDir()
	first := defaultFixture()
	first.eventsPerSecond = 100_000
	second := first
	second.eventsPerSecond = 120_000
	second.latencyP95 = 6
	dirs := []string{
		writeFixtureRun(t, root, "run-a", first),
		writeFixtureRun(t, root, "run-b", second),
	}
	report, err := Build(Options{RunDirs: dirs, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Comparisons) != 1 {
		t.Fatalf("comparisons = %d, want 1", len(report.Comparisons))
	}
	events := varianceByName(report.Comparisons[0].Metrics, "events_per_second")
	if events == nil || !events.Assessed || events.WithinTolerance == nil || *events.WithinTolerance {
		t.Fatalf("events variance = %+v, want assessed outside tolerance", events)
	}
	if report.Comparisons[0].WithinTolerance == nil || *report.Comparisons[0].WithinTolerance {
		t.Fatalf("comparison = %+v, want fail", report.Comparisons[0])
	}
	if len(report.Warnings) == 0 || !strings.Contains(strings.Join(report.Warnings, " "), "exceeds") {
		t.Fatalf("warnings = %v, want a variance warning", report.Warnings)
	}
	markdown := report.Markdown()
	if !strings.Contains(markdown, "FAIL") || !strings.Contains(markdown, "Documented variance: **FAIL**") {
		t.Fatalf("markdown does not report the failed variance:\n%s", markdown)
	}
}

func TestPlannedRunIsIncludedWithoutMetrics(t *testing.T) {
	root := t.TempDir()
	planned := defaultFixture()
	planned.status = runner.StatusPlanned
	planned.mode = runner.ModePlan
	planned.withResources = false
	planned.withShards = false
	dir := writeFixtureRun(t, root, "planned-run", planned)
	report, err := Build(Options{RunDirs: []string{dir}, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 1 || report.Runs[0].Status != runner.StatusPlanned {
		t.Fatalf("runs = %+v, want one planned run", report.Runs)
	}
	if report.Runs[0].Metrics != nil {
		t.Fatalf("metrics = %+v, want none for a planned run", report.Runs[0].Metrics)
	}
	for _, status := range report.Runs[0].MetricStatus {
		if status.Status != "not_collected" || !strings.Contains(status.Detail, "planned") {
			t.Fatalf("status = %+v, want planned not_collected", status)
		}
	}
	if len(report.Comparisons) != 0 {
		t.Fatalf("comparisons = %+v, want none", report.Comparisons)
	}
	if !strings.Contains(report.Markdown(), "planned run; no workload was executed") {
		t.Fatal("markdown does not explain the planned run")
	}
}

func TestMissingResourcesIsAWarning(t *testing.T) {
	root := t.TempDir()
	values := defaultFixture()
	values.withResources = false
	dir := writeFixtureRun(t, root, "no-resources", values)
	report, err := Build(Options{RunDirs: []string{dir}, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(report.Runs))
	}
	run := report.Runs[0]
	if run.Metrics == nil || run.Metrics.CPU != nil || run.Metrics.RAM != nil || run.Metrics.Disk != nil {
		t.Fatalf("resource metrics = %+v, want nil", run.Metrics)
	}
	if len(run.Warnings) == 0 || !strings.Contains(strings.Join(run.Warnings, " "), "resources.json") {
		t.Fatalf("warnings = %v, want a resources warning", run.Warnings)
	}
	status := metricStatusByName(run.MetricStatus, "cpu_percent")
	if status == nil || status.Status != "not_collected" || !strings.Contains(status.Detail, "resources.json") {
		t.Fatalf("cpu status = %+v, want missing resources detail", status)
	}
}

func TestDryRunIsListedButNotCompared(t *testing.T) {
	root := t.TempDir()
	first := defaultFixture()
	second := defaultFixture()
	second.eventsPerSecond = 99_000
	dry := defaultFixture()
	dry.mode = runner.ModeDryRun
	dirs := []string{
		writeFixtureRun(t, root, "run-real-a", first),
		writeFixtureRun(t, root, "run-real-b", second),
		writeFixtureRun(t, root, "run-dry", dry),
	}
	report, err := Build(Options{RunDirs: dirs, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 3 {
		t.Fatalf("runs = %d, want 3 (the dry-run must still be listed)", len(report.Runs))
	}
	if len(report.Comparisons) != 1 {
		t.Fatalf("comparisons = %d, want 1", len(report.Comparisons))
	}
	if report.Comparisons[0].Runs != 2 {
		t.Fatalf("comparison runs = %d, want 2 (dry-run excluded)", report.Comparisons[0].Runs)
	}
}

func TestRunsWithoutConfigHashAreNotCompared(t *testing.T) {
	root := t.TempDir()
	first := defaultFixture()
	first.configHash = ""
	second := defaultFixture()
	second.configHash = ""
	dirs := []string{
		writeFixtureRun(t, root, "legacy-a", first),
		writeFixtureRun(t, root, "legacy-b", second),
	}
	report, err := Build(Options{RunDirs: dirs, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(report.Runs))
	}
	if len(report.Comparisons) != 0 {
		t.Fatalf("comparisons = %+v, want none without config hashes", report.Comparisons)
	}
}

func TestBuildRejectsEmptyResults(t *testing.T) {
	if _, err := Build(Options{ResultsRoot: t.TempDir(), Now: fixedNow}); err == nil {
		t.Fatal("Build() = nil, want an empty-results error")
	}
}

func TestBuildRejectsUnsupportedRunSchema(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(`{"schema_version": 99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Options{RunDirs: []string{dir}, Now: fixedNow}); err == nil {
		t.Fatal("Build() = nil, want a schema error")
	}
}

func metricStatusByName(status []MetricStatus, name string) *MetricStatus {
	for index := range status {
		if status[index].Metric == name {
			return &status[index]
		}
	}
	return nil
}

func varianceByName(results []VarianceResult, name string) *VarianceResult {
	for index := range results {
		if results[index].Metric == name {
			return &results[index]
		}
	}
	return nil
}
