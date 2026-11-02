// Package runner executes benchmark scenarios and records their results.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/observaki/observaki/bench/internal/scenario"
	"github.com/observaki/observaki/bench/loadgen"
)

// ErrBlocked reports that a scenario is defined but its executor is not
// available yet (it lands with a later Phase 0 story).
var ErrBlocked = errors.New("scenario execution is not available yet")

const (
	schemaVersion        = 1
	rateTolerancePercent = 5.0
)

// Mode controls how much of a scenario is executed.
type Mode string

const (
	// ModeRun executes the scenario.
	ModeRun Mode = "run"
	// ModeDryRun runs the load generator without sending to a target.
	ModeDryRun Mode = "dry-run"
	// ModePlan validates and records the plan without starting a workload.
	ModePlan Mode = "plan"
)

// Status is the terminal state of a run record.
type Status string

const (
	StatusRunning Status = "running"
	StatusOK      Status = "ok"
	StatusFailed  Status = "failed"
	StatusBlocked Status = "blocked"
	StatusPlanned Status = "planned"
)

// Options configures a run.
type Options struct {
	RepoRoot         string
	ResultsRoot      string
	QueriesDir       string
	LoadgenPath      string
	Endpoint         string
	Mode             Mode
	DurationOverride time.Duration
	RateOverride     int
	StartTime        time.Time
	Now              func() time.Time
	Stdout           io.Writer
	Stderr           io.Writer
}

func (o Options) normalized() Options {
	if o.RepoRoot == "" {
		o.RepoRoot, _ = os.Getwd()
	}
	if o.ResultsRoot == "" {
		o.ResultsRoot = filepath.Join(o.RepoRoot, "bench", "results")
	}
	if o.QueriesDir == "" {
		o.QueriesDir = filepath.Join(o.RepoRoot, "bench", "queries")
	}
	if o.LoadgenPath == "" {
		o.LoadgenPath = filepath.Join(o.RepoRoot, "bin", "otlploadgen")
	}
	if o.Endpoint == "" {
		o.Endpoint = os.Getenv("OBSERVAKI_BENCH_ENDPOINT")
	}
	if o.Endpoint == "" {
		o.Endpoint = "http://127.0.0.1:4318"
	}
	if o.Mode == "" {
		o.Mode = ModeRun
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	return o
}

// Record is the run.json written to the result folder.
type Record struct {
	SchemaVersion   int                 `json:"schema_version"`
	Scenario        scenario.Definition `json:"scenario"`
	Resolved        Resolved            `json:"resolved"`
	Mode            Mode                `json:"mode"`
	Status          Status              `json:"status"`
	StartedAt       time.Time           `json:"started_at"`
	FinishedAt      *time.Time          `json:"finished_at,omitempty"`
	DurationSeconds float64             `json:"duration_seconds"`
	Git             GitInfo             `json:"git"`
	Host            HostInfo            `json:"host"`
	Checks          []Check             `json:"checks"`
	Plan            []string            `json:"plan,omitempty"`
	Dataset         *DatasetPlan        `json:"dataset,omitempty"`
	Commands        []string            `json:"commands,omitempty"`
	Shards          []ShardResult       `json:"shards,omitempty"`
	Aggregate       *Aggregate          `json:"aggregate,omitempty"`
	ResultDir       string              `json:"result_dir"`
	Error           string              `json:"error,omitempty"`
}

// Resolved documents the effective options after CLI overrides.
type Resolved struct {
	Endpoint string            `json:"endpoint,omitempty"`
	Duration scenario.Duration `json:"duration,omitempty"`
	Rate     int               `json:"rate,omitempty"`
}

// GitInfo identifies the exact code used by a run.
type GitInfo struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

// HostInfo documents the machine that produced the numbers.
type HostInfo struct {
	Hostname    string `json:"hostname"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	CPUs        int    `json:"cpus"`
	MemoryBytes uint64 `json:"memory_bytes,omitempty"`
}

// Check is one prerequisite validation.
type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Skipped bool   `json:"skipped,omitempty"`
	Detail  string `json:"detail"`
}

// DatasetPlan documents how a query dataset is seeded.
type DatasetPlan struct {
	Events        int64             `json:"events"`
	Window        scenario.Duration `json:"window"`
	EventTimeRate int64             `json:"event_time_rate"`
	SeedRate      int               `json:"seed_rate"`
	SeedShards    int               `json:"seed_shards"`
	SeedDuration  scenario.Duration `json:"seed_duration"`
	QuerySet      string            `json:"query_set,omitempty"`
	Queries       int               `json:"queries,omitempty"`
	Repetitions   int               `json:"repetitions,omitempty"`
	P95MS         int               `json:"p95_ms,omitempty"`
}

// ShardResult is one load generator process.
type ShardResult struct {
	Index              int     `json:"index"`
	Rate               int     `json:"rate"`
	Seed               int64   `json:"seed"`
	ExitCode           int     `json:"exit_code"`
	PlannedEvents      int64   `json:"planned_events"`
	Generated          int64   `json:"generated"`
	Sent               int64   `json:"sent"`
	Failed             int64   `json:"failed"`
	Rejected           int64   `json:"rejected"`
	AchievedRate       float64 `json:"achieved_rate"`
	DurationSeconds    float64 `json:"duration_seconds"`
	BackpressureEvents int64   `json:"backpressure_events"`
	PacingLagEvents    int64   `json:"pacing_lag_events"`
	SummaryFile        string  `json:"summary_file,omitempty"`
	LogFile            string  `json:"log_file,omitempty"`
	Error              string  `json:"error,omitempty"`
}

// Aggregate is the cross-shard view used to decide the run status.
type Aggregate struct {
	TargetRate              int     `json:"target_rate"`
	AchievedRate            float64 `json:"achieved_rate"`
	RateDeltaPercent        float64 `json:"rate_delta_percent"`
	RateWithinTolerance     bool    `json:"rate_within_tolerance"`
	PlannedEvents           int64   `json:"planned_events"`
	Generated               int64   `json:"generated"`
	Sent                    int64   `json:"sent"`
	Failed                  int64   `json:"failed"`
	Rejected                int64   `json:"rejected"`
	BackpressureEvents      int64   `json:"backpressure_events"`
	PacingLagEvents         int64   `json:"pacing_lag_events"`
	MaxShardDurationSeconds float64 `json:"max_shard_duration_seconds"`
}

// Run executes (or plans) one scenario and always writes a result folder.
func Run(ctx context.Context, definition scenario.Definition, options Options) (*Record, error) {
	options = options.normalized()
	if options.DurationOverride < 0 {
		return nil, errors.New("duration override must be positive")
	}
	if options.RateOverride < 0 {
		return nil, errors.New("rate override must be positive")
	}

	started := options.Now().UTC()
	resultDir, err := prepareResultDir(options.ResultsRoot, definition.Name, started)
	if err != nil {
		return nil, err
	}
	record := &Record{
		SchemaVersion: schemaVersion,
		Scenario:      definition,
		Resolved: Resolved{
			Endpoint: options.Endpoint,
			Duration: scenario.Duration(scenarioDuration(definition, options)),
			Rate:     scenarioRate(definition, options),
		},
		Mode:      options.Mode,
		Status:    StatusRunning,
		StartedAt: started,
		Git:       gitInfo(options.RepoRoot),
		Host:      hostInfo(),
		ResultDir: resultDir,
	}

	if err := writeRecord(resultDir, record); err != nil {
		return record, err
	}
	if len(definition.Raw) > 0 {
		if err := os.WriteFile(filepath.Join(resultDir, "scenario.env"), definition.Raw, 0o644); err != nil {
			return record, fmt.Errorf("write scenario snapshot: %w", err)
		}
	}

	_, _ = fmt.Fprintf(options.Stdout, "bench: scenario %s (%s, %s)\n", definition.Name, definition.Kind, record.Mode)
	_, _ = fmt.Fprintf(options.Stdout, "bench: results %s\n", resultDir)

	checks := runChecks(definition, options)
	record.Checks = checks

	if definition.Kind != scenario.KindThroughput {
		record.Plan = buildPlan(definition, options)
		record.Dataset = datasetPlan(definition, options)
		if options.Mode == ModeRun {
			record.Status = StatusBlocked
			record.Error = blockedMessage(definition)
			finish(record, options)
			return record, writeFinal(resultDir, record, fmt.Errorf("%w: %s", ErrBlocked, record.Error))
		}
		record.Status = StatusPlanned
		finish(record, options)
		return record, writeFinal(resultDir, record, nil)
	}

	if options.Mode == ModePlan {
		record.Plan = buildPlan(definition, options)
		record.Status = StatusPlanned
		finish(record, options)
		return record, writeFinal(resultDir, record, nil)
	}

	if failures := failedChecks(checks); len(failures) > 0 {
		record.Status = StatusFailed
		record.Error = "prerequisites not met: " + strings.Join(failures, ", ")
		finish(record, options)
		return record, writeFinal(resultDir, record, errors.New(record.Error))
	}

	_, _ = fmt.Fprintln(options.Stdout, "bench: starting workload")
	record.Status = StatusRunning
	if err := writeRecord(resultDir, record); err != nil {
		return record, err
	}
	runErr := executeThroughput(ctx, definition, options, record, resultDir)
	finish(record, options)
	return record, writeFinal(resultDir, record, runErr)
}

func writeFinal(resultDir string, record *Record, runErr error) error {
	if err := writeRecord(resultDir, record); err != nil {
		return err
	}
	return runErr
}

func finish(record *Record, options Options) {
	finished := options.Now().UTC()
	record.FinishedAt = &finished
	record.DurationSeconds = finished.Sub(record.StartedAt).Seconds()
}

func prepareResultDir(root, name string, now time.Time) (string, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create results root: %w", err)
	}
	base := now.UTC().Format("20060102T150405Z") + "-" + name
	for attempt := 0; attempt < 100; attempt++ {
		dir := filepath.Join(root, base)
		if attempt > 0 {
			dir = filepath.Join(root, fmt.Sprintf("%s-%02d", base, attempt))
		}
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("create result directory: %w", err)
		}
	}
	return "", errors.New("could not allocate a unique result directory")
}

func writeRecord(resultDir string, record *Record) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run record: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(resultDir, "run.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write run record: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("publish run record: %w", err)
	}
	return nil
}

func runChecks(definition scenario.Definition, options Options) []Check {
	checks := []Check{checkLoadgen(options.LoadgenPath)}
	if definition.Has("gateway") || definition.Has("otlp") {
		checks = append(checks, checkEndpoint(options.Endpoint, options.Mode))
	}
	if definition.Has("clickhouse") {
		base := os.Getenv("OBSERVAKI_BENCH_CLICKHOUSE_URL")
		if base == "" {
			base = "http://127.0.0.1:8123"
		}
		checks = append(checks, checkHTTP("clickhouse", strings.TrimSuffix(base, "/")+"/ping", "Ok."))
	}
	if definition.Has("victoriametrics") {
		base := os.Getenv("OBSERVAKI_BENCH_VICTORIAMETRICS_URL")
		if base == "" {
			base = "http://127.0.0.1:8428"
		}
		checks = append(checks, checkHTTP("victoriametrics", strings.TrimSuffix(base, "/")+"/health", "OK"))
	}
	if definition.Has("nats") {
		address := os.Getenv("OBSERVAKI_BENCH_NATS_ADDR")
		if address == "" {
			address = "127.0.0.1:4222"
		}
		checks = append(checks, checkTCP("nats", address))
	}
	if definition.Kind == scenario.KindQuery {
		expected := scenario.QueryCount(definition.QuerySet)
		path := filepath.Join(options.QueriesDir, definition.QuerySet+".tsv")
		checks = append(checks, checkQuerySet(path, expected))
	}
	return checks
}

func failedChecks(checks []Check) []string {
	failed := make([]string, 0, len(checks))
	for _, check := range checks {
		if !check.OK && !check.Skipped {
			failed = append(failed, check.Name)
		}
	}
	return failed
}

func checkLoadgen(path string) Check {
	info, err := os.Stat(path)
	if err != nil {
		return Check{Name: "loadgen", Detail: fmt.Sprintf("%s not found; run make build", path)}
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return Check{Name: "loadgen", Detail: fmt.Sprintf("%s is not executable; run make build", path)}
	}
	return Check{Name: "loadgen", OK: true, Detail: path}
}

func checkEndpoint(endpoint string, mode Mode) Check {
	if mode != ModeRun {
		return Check{Name: "gateway", OK: true, Skipped: true, Detail: "skipped in " + string(mode) + " mode"}
	}
	address, err := endpointAddress(endpoint)
	if err != nil {
		return Check{Name: "gateway", Detail: err.Error()}
	}
	return checkTCP("gateway", address)
}

func checkTCP(name, address string) Check {
	connection, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return Check{Name: name, Detail: fmt.Sprintf("%s: %v", address, err)}
	}
	_ = connection.Close()
	return Check{Name: name, OK: true, Detail: address}
}

func checkHTTP(name, target, want string) Check {
	client := &http.Client{Timeout: 2 * time.Second}
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return Check{Name: name, Detail: err.Error()}
	}
	response, err := client.Do(request)
	if err != nil {
		return Check{Name: name, Detail: fmt.Sprintf("%s: %v", target, err)}
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 256))
	if response.StatusCode != http.StatusOK {
		return Check{Name: name, Detail: fmt.Sprintf("%s returned %s", target, response.Status)}
	}
	if want != "" && !strings.Contains(string(body), want) {
		return Check{Name: name, Detail: fmt.Sprintf("%s returned %q, want %q", target, strings.TrimSpace(string(body)), want)}
	}
	return Check{Name: name, OK: true, Detail: target}
}

func checkQuerySet(path string, expected int) Check {
	if expected <= 0 {
		return Check{Name: "queries", Detail: fmt.Sprintf("unknown query set %q", filepath.Base(path))}
	}
	queries, err := scenario.LoadQuerySet(path)
	if err != nil {
		return Check{Name: "queries", Detail: err.Error()}
	}
	if len(queries) != expected {
		return Check{Name: "queries", Detail: fmt.Sprintf("%s has %d queries, want %d", path, len(queries), expected)}
	}
	return Check{Name: "queries", OK: true, Detail: fmt.Sprintf("%s (%d queries)", path, len(queries))}
}

func endpointAddress(endpoint string) (string, error) {
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return "", fmt.Errorf("parse endpoint %q: %w", endpoint, err)
		}
		if parsed.Host == "" {
			return "", fmt.Errorf("endpoint %q has no host", endpoint)
		}
		return parsed.Host, nil
	}
	if endpoint == "" {
		return "", errors.New("endpoint is empty")
	}
	return endpoint, nil
}

func blockedMessage(definition scenario.Definition) string {
	if definition.BlockedByStory != "" {
		return fmt.Sprintf("execution lands in Story %s; the scenario contract is recorded and validated until then", definition.BlockedByStory)
	}
	return fmt.Sprintf("no executor for kind %q yet", definition.Kind)
}

func scenarioDuration(definition scenario.Definition, options Options) time.Duration {
	if options.DurationOverride > 0 {
		return options.DurationOverride
	}
	return time.Duration(definition.Duration)
}

func scenarioRate(definition scenario.Definition, options Options) int {
	if options.RateOverride > 0 {
		return options.RateOverride
	}
	return definition.Rate
}

func executeThroughput(ctx context.Context, definition scenario.Definition, options Options, record *Record, resultDir string) error {
	targetRate := scenarioRate(definition, options)
	duration := scenarioDuration(definition, options)
	rates := distribute(targetRate, definition.Shards)
	if len(rates) == 0 {
		return errors.New("scenario has no shards")
	}
	for index, rate := range rates {
		args := loadgenArgs(definition, options, rate, duration, int64(index+1), options.Mode == ModeDryRun, time.Time{})
		record.Commands = append(record.Commands, commandString(options.LoadgenPath, args))
	}

	results := make([]ShardResult, len(rates))
	var waitGroup sync.WaitGroup
	for index, rate := range rates {
		waitGroup.Add(1)
		go func(index, rate int) {
			defer waitGroup.Done()
			args := loadgenArgs(definition, options, rate, duration, int64(index+1), options.Mode == ModeDryRun, time.Time{})
			results[index] = runShard(ctx, options.LoadgenPath, args, index, rate, int64(index+1), resultDir)
		}(index, rate)
	}
	waitGroup.Wait()

	record.Shards = results
	aggregate := aggregateShards(results, targetRate)
	record.Aggregate = &aggregate
	_, _ = fmt.Fprintf(options.Stdout, "bench: achieved %.1f events/s (target %d/s, %+.2f%%), generated %d\n",
		aggregate.AchievedRate, aggregate.TargetRate, aggregate.RateDeltaPercent, aggregate.Generated)

	if err := validateAggregate(aggregate, results); err != nil {
		record.Status = StatusFailed
		record.Error = err.Error()
		return err
	}
	record.Status = StatusOK
	return nil
}

func loadgenArgs(definition scenario.Definition, options Options, rate int, duration time.Duration, seed int64, dryRun bool, startTime time.Time) []string {
	protocol := definition.Protocol
	if protocol == "" {
		protocol = "http"
	}
	compression := definition.Compression
	if compression == "" {
		compression = "none"
	}
	args := []string{
		"--endpoint", options.Endpoint,
		"--protocol", protocol,
		"--compression", compression,
		"--rate", strconv.Itoa(rate),
		"--duration", duration.String(),
		"--seed", strconv.FormatInt(seed, 10),
		"--metrics-addr", "",
		"--log-interval", "30s",
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if !startTime.IsZero() {
		args = append(args, "--start-time", startTime.UTC().Format(time.RFC3339))
	}
	return args
}

func runShard(ctx context.Context, binary string, args []string, index, rate int, seed int64, resultDir string) ShardResult {
	result := ShardResult{
		Index:       index,
		Rate:        rate,
		Seed:        seed,
		SummaryFile: fmt.Sprintf("loadgen-%02d.json", index),
		LogFile:     fmt.Sprintf("loadgen-%02d.log", index),
	}
	summaryPath := filepath.Join(resultDir, result.SummaryFile)
	logPath := filepath.Join(resultDir, result.LogFile)

	stdout, err := os.Create(summaryPath)
	if err != nil {
		result.ExitCode = -1
		result.Error = err.Error()
		return result
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		_ = stdout.Close()
		result.ExitCode = -1
		result.Error = err.Error()
		return result
	}

	command := exec.CommandContext(ctx, binary, args...)
	command.Stdout = stdout
	command.Stderr = logFile
	runErr := command.Run()
	if closeErr := stdout.Close(); closeErr != nil && result.Error == "" {
		result.Error = closeErr.Error()
	}
	if closeErr := logFile.Close(); closeErr != nil && result.Error == "" {
		result.Error = closeErr.Error()
	}

	if runErr != nil {
		result.ExitCode = -1
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			result.ExitCode = exitError.ExitCode()
		}
		result.Error = runErr.Error()
		return result
	}

	data, err := os.ReadFile(summaryPath)
	if err != nil {
		result.ExitCode = -1
		result.Error = fmt.Sprintf("read summary: %v", err)
		return result
	}
	var summary loadgen.Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		result.ExitCode = -1
		result.Error = fmt.Sprintf("decode summary: %v", err)
		return result
	}
	result.PlannedEvents = summary.PlannedEvents
	result.Generated = summary.Metrics.Generated
	result.Sent = summary.Metrics.Sent
	result.Failed = summary.Metrics.Failed
	result.Rejected = summary.Metrics.Rejected
	result.AchievedRate = summary.Metrics.AchievedRate
	result.DurationSeconds = summary.DurationSeconds
	result.BackpressureEvents = summary.Metrics.BackpressureEvents
	result.PacingLagEvents = summary.Metrics.PacingLagEvents
	return result
}

func aggregateShards(shards []ShardResult, targetRate int) Aggregate {
	aggregate := Aggregate{TargetRate: targetRate}
	for _, shard := range shards {
		aggregate.PlannedEvents += shard.PlannedEvents
		aggregate.Generated += shard.Generated
		aggregate.Sent += shard.Sent
		aggregate.Failed += shard.Failed
		aggregate.Rejected += shard.Rejected
		aggregate.BackpressureEvents += shard.BackpressureEvents
		aggregate.PacingLagEvents += shard.PacingLagEvents
		if shard.DurationSeconds > aggregate.MaxShardDurationSeconds {
			aggregate.MaxShardDurationSeconds = shard.DurationSeconds
		}
	}
	if aggregate.MaxShardDurationSeconds > 0 {
		aggregate.AchievedRate = float64(aggregate.Generated) / aggregate.MaxShardDurationSeconds
	}
	if targetRate > 0 {
		aggregate.RateDeltaPercent = (aggregate.AchievedRate - float64(targetRate)) / float64(targetRate) * 100
		aggregate.RateWithinTolerance = math.Abs(aggregate.RateDeltaPercent) <= rateTolerancePercent
	}
	return aggregate
}

func validateAggregate(aggregate Aggregate, shards []ShardResult) error {
	var problems []string
	for _, shard := range shards {
		if shard.ExitCode != 0 {
			problems = append(problems, fmt.Sprintf("shard %d exited with code %d", shard.Index, shard.ExitCode))
		}
		if shard.Error != "" {
			problems = append(problems, fmt.Sprintf("shard %d: %s", shard.Index, shard.Error))
		}
	}
	if aggregate.Failed > 0 {
		problems = append(problems, fmt.Sprintf("%d event(s) failed to send", aggregate.Failed))
	}
	if aggregate.Rejected > 0 {
		problems = append(problems, fmt.Sprintf("%d event(s) rejected by the receiver", aggregate.Rejected))
	}
	if !aggregate.RateWithinTolerance {
		problems = append(problems, fmt.Sprintf("achieved rate %.1f/s is outside ±%.0f%% of %d/s",
			aggregate.AchievedRate, rateTolerancePercent, aggregate.TargetRate))
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func distribute(total, shards int) []int {
	if shards <= 0 || total <= 0 {
		return nil
	}
	rates := make([]int, shards)
	base := total / shards
	remainder := total % shards
	for index := range rates {
		rates[index] = base
		if index < remainder {
			rates[index]++
		}
	}
	return rates
}

func datasetPlan(definition scenario.Definition, options Options) *DatasetPlan {
	if definition.Kind != scenario.KindQuery {
		return nil
	}
	return &DatasetPlan{
		Events:        definition.DatasetEvents,
		Window:        definition.DatasetWindow,
		EventTimeRate: definition.EventTimeRate(),
		SeedRate:      definition.SeedRate,
		SeedShards:    definition.SeedShards,
		SeedDuration:  scenario.Duration(definition.SeedDuration()),
		QuerySet:      definition.QuerySet,
		Queries:       scenario.QueryCount(definition.QuerySet),
		Repetitions:   definition.QueryRepetitions,
		P95MS:         definition.P95MS,
	}
}

func buildPlan(definition scenario.Definition, options Options) []string {
	var plan []string
	switch definition.Kind {
	case scenario.KindThroughput:
		duration := scenarioDuration(definition, options)
		for index, rate := range distribute(scenarioRate(definition, options), definition.Shards) {
			args := loadgenArgs(definition, options, rate, duration, int64(index+1), false, options.StartTime)
			plan = append(plan, commandString(options.LoadgenPath, args))
		}
	case scenario.KindQuery:
		startTime := options.Now().UTC().Add(-time.Duration(definition.DatasetWindow))
		for index, rate := range distribute(definition.SeedRate, definition.SeedShards) {
			args := loadgenArgs(definition, options, rate, definition.SeedDuration(), int64(index+1), false, startTime)
			args = append(args, "--event-time-rate", strconv.FormatInt(definition.EventTimeRate(), 10))
			plan = append(plan, commandString(options.LoadgenPath, args))
		}
		plan = append(plan, fmt.Sprintf("run %d queries from %s %d time(s)",
			scenario.QueryCount(definition.QuerySet),
			filepath.Join(options.QueriesDir, definition.QuerySet+".tsv"),
			definition.QueryRepetitions))
	case scenario.KindCardinality:
		plan = append(plan, fmt.Sprintf("generate %d active series at %d samples/s for %d tenants (series generator lands in Story %s)",
			definition.Series, definition.SampleRate, definition.Tenants, definition.BlockedByStory))
	case scenario.KindChaos:
		for index, rate := range distribute(definition.Rate, definition.Shards) {
			args := loadgenArgs(definition, options, rate, time.Duration(definition.Duration), int64(index+1), false, options.StartTime)
			plan = append(plan, commandString(options.LoadgenPath, args))
		}
		plan = append(plan, fmt.Sprintf("fault injection: stop %s for %s (automation lands in Story %s)",
			definition.Failure, definition.Outage, definition.BlockedByStory))
	}
	return plan
}

func commandString(binary string, args []string) string {
	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, quoteArg(binary))
	for _, arg := range args {
		quoted = append(quoted, quoteArg(arg))
	}
	return strings.Join(quoted, " ")
}

func quoteArg(value string) string {
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\"'") {
		return strconv.Quote(value)
	}
	return value
}

func gitInfo(root string) GitInfo {
	info := GitInfo{}
	if output, err := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD").Output(); err == nil {
		info.Commit = strings.TrimSpace(string(output))
	}
	if output, err := exec.Command("git", "-C", root, "status", "--porcelain").Output(); err == nil {
		info.Dirty = strings.TrimSpace(string(output)) != ""
	}
	return info
}

func hostInfo() HostInfo {
	info := HostInfo{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU()}
	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	}
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return info
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			break
		}
		kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			break
		}
		info.MemoryBytes = kilobytes * 1024
		break
	}
	return info
}
