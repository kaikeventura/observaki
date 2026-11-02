// Command benchctl lists, validates and runs the Observaki benchmark
// scenarios. bench/run.sh is the stable entry point.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/observaki/observaki/bench/internal/runner"
	"github.com/observaki/observaki/bench/internal/scenario"
)

func main() {
	if code := run(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func run(args []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("benchctl", flag.ContinueOnError)
	global.SetOutput(stderr)
	rootFlag := global.String("root", "", "repository root (default: auto-detect)")
	global.Usage = func() { usage(stderr) }
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	remaining := global.Args()
	if len(remaining) == 0 {
		usage(stderr)
		return 1
	}

	root, err := resolveRoot(*rootFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}

	switch remaining[0] {
	case "list":
		return list(root, stdout, stderr)
	case "check":
		return check(root, remaining[1:], stdout, stderr)
	case "queries":
		return queries(root, stdout, stderr)
	case "run":
		return runScenario(root, remaining[1:], stdout, stderr)
	case "version":
		_, _ = fmt.Fprintln(stdout, "benchctl 0.1.0 (phase 0)")
		return 0
	case "help":
		usage(stdout)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "benchctl: unknown command %q\n\n", remaining[0])
		usage(stderr)
		return 1
	}
}

func usage(writer io.Writer) {
	_, _ = fmt.Fprint(writer, `benchctl — Observaki benchmark scenario runner

Usage:
  bench/run.sh [--root DIR] list
  bench/run.sh [--root DIR] check [scenario...]
  bench/run.sh [--root DIR] queries
  bench/run.sh [--root DIR] run <scenario> [flags]

Run flags:
  --plan                   validate and record the plan without starting a workload
  --dry-run                run the load generator without sending to a target
  --duration DURATION      override the scenario duration (for example 10s)
  --rate N                 override the target events/s
  --endpoint URL           OTLP endpoint (default: $OBSERVAKI_BENCH_ENDPOINT or http://127.0.0.1:4318)
  --start-time RFC3339     fixed event start time for reproducible content
  --results-dir DIR        override the results root (default: bench/results)
  --loadgen PATH           override the load generator binary (default: bin/otlploadgen)

Exit codes: 0 ok/planned, 1 failure, 3 blocked (executor lands in a later story).
`)
}

func resolveRoot(flagValue string) (string, error) {
	if flagValue != "" {
		return filepath.Abs(flagValue)
	}
	if env := os.Getenv("OBSERVAKI_ROOT"); env != "" {
		return filepath.Abs(env)
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root not found; run from the repository or pass --root")
		}
		dir = parent
	}
}

func loadDefinitions(root string) ([]scenario.Definition, error) {
	return scenario.LoadDir(filepath.Join(root, "bench", "scenarios"))
}

func list(root string, stdout, stderr io.Writer) int {
	definitions, err := loadDefinitions(root)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tKIND\tDURATION\tKEY PARAMETER\tEXECUTION")
	for _, definition := range definitions {
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			definition.Name, definition.Kind, formatDuration(definition.Duration),
			keyParameter(definition), executionLabel(definition))
	}
	if err := writer.Flush(); err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	return 0
}

func keyParameter(definition scenario.Definition) string {
	switch definition.Kind {
	case scenario.KindThroughput, scenario.KindChaos:
		return fmt.Sprintf("%s events/s", formatCount(int64(definition.Rate)))
	case scenario.KindQuery:
		return fmt.Sprintf("%s events / %s", formatCount(definition.DatasetEvents), formatDuration(definition.DatasetWindow))
	case scenario.KindCardinality:
		return fmt.Sprintf("%s series", formatCount(definition.Series))
	default:
		return ""
	}
}

func executionLabel(definition scenario.Definition) string {
	if definition.BlockedByStory == "" {
		return "ready"
	}
	return "planned (Story " + definition.BlockedByStory + ")"
}

func check(root string, names []string, stdout, stderr io.Writer) int {
	definitions, err := loadDefinitions(root)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	selected, err := selectDefinitions(definitions, names)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	if len(names) == 0 {
		if missing := missingScenarios(definitions); len(missing) > 0 {
			_, _ = fmt.Fprintf(stderr, "check: missing required scenario(s): %v\n", missing)
			return 1
		}
	}

	failures := 0
	for _, definition := range selected {
		if definition.Kind == scenario.KindQuery {
			path := filepath.Join(root, "bench", "queries", definition.QuerySet+".tsv")
			queries, err := scenario.LoadQuerySet(path)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "fail %s: %v\n", definition.Name, err)
				failures++
				continue
			}
			expected := scenario.QueryCount(definition.QuerySet)
			if expected != len(queries) {
				_, _ = fmt.Fprintf(stderr, "fail %s: query set has %d queries, want %d\n", definition.Name, len(queries), expected)
				failures++
				continue
			}
			_, _ = fmt.Fprintf(stdout, "ok   %-22s %-11s %d queries\n", definition.Name, definition.Kind, len(queries))
			continue
		}
		_, _ = fmt.Fprintf(stdout, "ok   %-22s %-11s %s\n", definition.Name, definition.Kind, formatDuration(definition.Duration))
	}
	if failures > 0 {
		_, _ = fmt.Fprintf(stderr, "check: %d scenario(s) failed\n", failures)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "check: %d scenario(s) valid\n", len(selected))
	return 0
}

func selectDefinitions(definitions []scenario.Definition, names []string) ([]scenario.Definition, error) {
	if len(names) == 0 {
		return definitions, nil
	}
	byName := make(map[string]scenario.Definition, len(definitions))
	for _, definition := range definitions {
		byName[definition.Name] = definition
	}
	selected := make([]scenario.Definition, 0, len(names))
	for _, name := range names {
		definition, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown scenario %q", name)
		}
		selected = append(selected, definition)
	}
	return selected, nil
}

func missingScenarios(definitions []scenario.Definition) []string {
	required := []string{
		"throughput-100k", "throughput-1m", "throughput-5m",
		"query-1b", "query-10b", "query-86b",
		"cardinality-100k", "cardinality-1m", "cardinality-10m",
		"chaos-clickhouse", "chaos-nats", "chaos-gateway",
	}
	present := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		present[definition.Name] = struct{}{}
	}
	var missing []string
	for _, name := range required {
		if _, ok := present[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func queries(root string, stdout, stderr io.Writer) int {
	path := filepath.Join(root, "bench", "queries", "dashboard.tsv")
	querySet, err := scenario.LoadQuerySet(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "ID\tSURFACE\tSIGNAL\tENGINE\tWINDOW\tDATASETS\tDESCRIPTION")
	for _, query := range querySet {
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			query.ID, query.Surface, query.Signal, query.Engine, query.Window,
			strings.Join(query.Datasets, ","), query.Description)
	}
	if err := writer.Flush(); err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	return 0
}

func runScenario(root string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "benchctl: run requires a scenario name")
		return 1
	}
	name := args[0]
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	plan := flags.Bool("plan", false, "validate and record the plan without starting a workload")
	dryRun := flags.Bool("dry-run", false, "run the load generator without sending")
	duration := flags.String("duration", "", "override the scenario duration")
	rate := flags.Int("rate", 0, "override the target events/s")
	endpoint := flags.String("endpoint", "", "OTLP endpoint")
	startTime := flags.String("start-time", "", "fixed event start time in RFC3339")
	resultsDir := flags.String("results-dir", "", "results root")
	loadgenPath := flags.String("loadgen", "", "load generator binary")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if *plan && *dryRun {
		_, _ = fmt.Fprintln(stderr, "benchctl: --plan and --dry-run are mutually exclusive")
		return 1
	}
	if *rate < 0 {
		_, _ = fmt.Fprintln(stderr, "benchctl: --rate must not be negative")
		return 1
	}

	definition, err := scenario.LoadFile(filepath.Join(root, "bench", "scenarios", name+".env"))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}

	options := runner.Options{
		RepoRoot:     root,
		ResultsRoot:  *resultsDir,
		LoadgenPath:  *loadgenPath,
		Endpoint:     *endpoint,
		RateOverride: *rate,
		Stdout:       stdout,
		Stderr:       stderr,
	}
	switch {
	case *plan:
		options.Mode = runner.ModePlan
	case *dryRun:
		options.Mode = runner.ModeDryRun
	default:
		options.Mode = runner.ModeRun
	}
	if *duration != "" {
		parsed, err := scenario.ParseDuration(*duration)
		if err != nil || parsed <= 0 {
			_, _ = fmt.Fprintf(stderr, "benchctl: invalid --duration %q\n", *duration)
			return 1
		}
		options.DurationOverride = time.Duration(parsed)
	}
	if *startTime != "" {
		parsed, err := time.Parse(time.RFC3339, *startTime)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "benchctl: invalid --start-time %q: %v\n", *startTime, err)
			return 1
		}
		options.StartTime = parsed.UTC()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	record, runErr := runner.Run(ctx, definition, options)
	if record != nil {
		_, _ = fmt.Fprintf(stdout, "status: %s\n", record.Status)
		_, _ = fmt.Fprintf(stdout, "result: %s\n", record.ResultDir)
		if record.Aggregate != nil {
			_, _ = fmt.Fprintf(stdout, "rate: %.1f events/s (target %d/s, %+.2f%%)\n",
				record.Aggregate.AchievedRate, record.Aggregate.TargetRate, record.Aggregate.RateDeltaPercent)
		}
		if record.Mode == runner.ModePlan || record.Status == runner.StatusBlocked {
			for _, step := range record.Plan {
				_, _ = fmt.Fprintf(stdout, "plan: %s\n", step)
			}
		}
	}
	if runErr != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", runErr)
		if errors.Is(runErr, runner.ErrBlocked) {
			return 3
		}
		return 1
	}
	return 0
}

func formatDuration(duration scenario.Duration) string {
	value := time.Duration(duration)
	if value >= 48*time.Hour && value%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", value/(24*time.Hour))
	}
	return duration.String()
}

func formatCount(value int64) string {
	switch {
	case value >= 1_000_000_000:
		if value%1_000_000_000 == 0 {
			return fmt.Sprintf("%dB", value/1_000_000_000)
		}
		return fmt.Sprintf("%.1fB", float64(value)/1_000_000_000)
	case value >= 1_000_000:
		if value%1_000_000 == 0 {
			return fmt.Sprintf("%dM", value/1_000_000)
		}
		return fmt.Sprintf("%.1fM", float64(value)/1_000_000)
	case value >= 1_000:
		if value%1_000 == 0 {
			return fmt.Sprintf("%dk", value/1_000)
		}
		return fmt.Sprintf("%.1fk", float64(value)/1_000)
	default:
		return strconv.FormatInt(value, 10)
	}
}
