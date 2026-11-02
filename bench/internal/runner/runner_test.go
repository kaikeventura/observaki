package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/observaki/observaki/bench/internal/scenario"
)

const fakeLoadgen = `#!/usr/bin/env bash
set -euo pipefail
rate=1000
while [ $# -gt 0 ]; do
  case "$1" in
    --rate) rate="$2"; shift 2 ;;
    *) shift ;;
  esac
done
printf '{"planned_events":%d,"duration_seconds":1,"metrics":{"generated":%d,"sent":%d,"failed":0,"rejected":0}}\n' "$rate" "$rate" "$rate"
`

func writeFakeLoadgen(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "otlploadgen")
	if err := os.WriteFile(path, []byte(fakeLoadgen), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func fixedNow() time.Time {
	return time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
}

func TestDryRunThroughputRunsEveryShard(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "results")
	definition := scenario.Definition{
		Name:        "fake-throughput",
		Kind:        scenario.KindThroughput,
		Rate:        2000,
		Shards:      2,
		Protocol:    "http",
		Compression: "none",
		Duration:    scenario.Duration(time.Second),
		Requires:    []string{"loadgen", "otlp", "gateway"},
	}
	record, err := Run(context.Background(), definition, Options{
		RepoRoot:    dir,
		ResultsRoot: results,
		LoadgenPath: writeFakeLoadgen(t, dir),
		Mode:        ModeDryRun,
		Now:         fixedNow,
	})
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if record.Status != StatusOK {
		t.Fatalf("status = %s, want ok", record.Status)
	}
	if len(record.Shards) != 2 {
		t.Fatalf("shards = %d, want 2", len(record.Shards))
	}
	if record.Aggregate == nil {
		t.Fatal("aggregate is nil")
	}
	if got, want := record.Aggregate.Generated, int64(2000); got != want {
		t.Fatalf("generated = %d, want %d", got, want)
	}
	if !record.Aggregate.RateWithinTolerance {
		t.Fatalf("rate delta = %.2f%%, want within tolerance", record.Aggregate.RateDeltaPercent)
	}
	for _, shard := range record.Shards {
		for _, file := range []string{shard.SummaryFile, shard.LogFile} {
			if _, err := os.Stat(filepath.Join(record.ResultDir, file)); err != nil {
				t.Errorf("missing artifact %s: %v", file, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(record.ResultDir, "run.json")); err != nil {
		t.Fatalf("run.json missing: %v", err)
	}
	if record.ConfigHash == "" || !strings.HasPrefix(record.ConfigHash, "sha256:") {
		t.Fatalf("config hash = %q, want sha256", record.ConfigHash)
	}
	if record.Versions.Go == "" || record.Versions.Benchctl == "" {
		t.Fatalf("versions = %+v, want benchctl and Go versions", record.Versions)
	}
	if record.Host.CPUs <= 0 || record.Host.OS == "" {
		t.Fatalf("host = %+v, want hardware details", record.Host)
	}
	if record.Resources == nil {
		t.Fatal("resources summary is nil")
	}
	if _, err := os.Stat(filepath.Join(record.ResultDir, "resources.json")); err != nil {
		t.Fatalf("resources.json missing: %v", err)
	}
}

func TestConfigHashIgnoresEndpointAndChangesWithWorkload(t *testing.T) {
	definition := scenario.Definition{
		Name:        "fake-throughput",
		Kind:        scenario.KindThroughput,
		Rate:        2000,
		Shards:      2,
		Protocol:    "http",
		Compression: "none",
		Duration:    scenario.Duration(time.Minute),
		Requires:    []string{"loadgen"},
	}
	base := Options{Endpoint: "http://127.0.0.1:4318"}
	first := configHash(definition, base)
	second := configHash(definition, Options{Endpoint: "http://10.0.0.1:4318"})
	if first == "" || first != second {
		t.Fatalf("config hash changed with the endpoint: %q != %q", first, second)
	}
	changed := definition
	changed.Rate = 4000
	if third := configHash(changed, base); third == first {
		t.Fatal("config hash did not change with the workload rate")
	}
	changed = definition
	changed.Compression = "gzip"
	if fourth := configHash(changed, base); fourth == first {
		t.Fatal("config hash did not change with compression")
	}
}

func TestRunFailsWhenEndpointIsDown(t *testing.T) {
	dir := t.TempDir()
	definition := scenario.Definition{
		Name:        "fake-throughput",
		Kind:        scenario.KindThroughput,
		Rate:        1000,
		Shards:      1,
		Protocol:    "http",
		Compression: "none",
		Duration:    scenario.Duration(time.Second),
		Requires:    []string{"loadgen", "otlp", "gateway"},
	}
	record, err := Run(context.Background(), definition, Options{
		RepoRoot:    dir,
		ResultsRoot: filepath.Join(dir, "results"),
		LoadgenPath: writeFakeLoadgen(t, dir),
		Endpoint:    "127.0.0.1:1",
		Mode:        ModeRun,
		Now:         fixedNow,
	})
	if err == nil {
		t.Fatal("Run() = nil, want a prerequisite error")
	}
	if record.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", record.Status)
	}
	if !strings.Contains(record.Error, "gateway") {
		t.Fatalf("error = %q, want it to mention the gateway check", record.Error)
	}
	if len(record.Shards) != 0 {
		t.Fatalf("shards = %d, want none when prerequisites fail", len(record.Shards))
	}
}

func TestBlockedScenarioRecordsPlanAndDataset(t *testing.T) {
	dir := t.TempDir()
	definition := scenario.Definition{
		Name:             "fake-query",
		Kind:             scenario.KindQuery,
		Duration:         scenario.Duration(20 * time.Minute),
		Requires:         []string{"loadgen", "otlp", "gateway", "clickhouse", "victoriametrics"},
		DatasetEvents:    1_000_000_000,
		DatasetWindow:    scenario.Duration(24 * time.Hour),
		SeedRate:         1_000_000,
		SeedShards:       1,
		QuerySet:         "dashboard",
		QueryRepetitions: 3,
		P95MS:            1000,
		BlockedByStory:   "3.5",
	}
	record, err := Run(context.Background(), definition, Options{
		RepoRoot:    filepath.Join("..", "..", ".."),
		ResultsRoot: filepath.Join(dir, "results"),
		QueriesDir:  filepath.Join("..", "..", "queries"),
		LoadgenPath: writeFakeLoadgen(t, dir),
		Mode:        ModeRun,
		Now:         fixedNow,
	})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("Run() error = %v, want ErrBlocked", err)
	}
	if record.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked", record.Status)
	}
	if record.Dataset == nil || record.Dataset.Events != 1_000_000_000 {
		t.Fatalf("dataset plan = %+v, want 1B events", record.Dataset)
	}
	if len(record.Plan) < 2 {
		t.Fatalf("plan has %d steps, want seed and query steps", len(record.Plan))
	}
	if !strings.Contains(strings.Join(record.Plan, " "), "--event-time-rate 11574") {
		t.Fatalf("plan does not pin the event-time rate: %v", record.Plan)
	}
	if strings.Contains(strings.Join(record.Plan, " "), "--protocol ''") {
		t.Fatalf("plan is missing the query seed protocol defaults: %v", record.Plan)
	}
}

func TestPlanSucceedsWithoutLoadgen(t *testing.T) {
	dir := t.TempDir()
	definition := scenario.Definition{
		Name:        "fake-throughput",
		Kind:        scenario.KindThroughput,
		Rate:        1000,
		Shards:      1,
		Protocol:    "http",
		Compression: "none",
		Duration:    scenario.Duration(time.Second),
		Requires:    []string{"loadgen"},
	}
	record, err := Run(context.Background(), definition, Options{
		RepoRoot:    dir,
		ResultsRoot: filepath.Join(dir, "results"),
		LoadgenPath: filepath.Join(dir, "missing"),
		Mode:        ModePlan,
		Now:         fixedNow,
	})
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if record.Status != StatusPlanned {
		t.Fatalf("status = %s, want planned", record.Status)
	}
	if len(record.Plan) != 1 {
		t.Fatalf("plan = %v, want one command", record.Plan)
	}
	var decoded map[string]any
	data, err := os.ReadFile(filepath.Join(record.ResultDir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if got, want := decoded["schema_version"], float64(schemaVersion); got != want {
		t.Fatalf("schema_version = %v, want %v", got, want)
	}
}

func TestResultDirectoriesAreUnique(t *testing.T) {
	results := t.TempDir()
	first, err := prepareResultDir(results, "demo", fixedNow())
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareResultDir(results, "demo", fixedNow())
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("result dirs are equal: %s", first)
	}
}

func TestDistribute(t *testing.T) {
	tests := []struct {
		total  int
		shards int
		want   []int
	}{
		{total: 5_000_000, shards: 5, want: []int{1_000_000, 1_000_000, 1_000_000, 1_000_000, 1_000_000}},
		{total: 10, shards: 3, want: []int{4, 3, 3}},
		{total: 0, shards: 3, want: nil},
	}
	for _, test := range tests {
		got := distribute(test.total, test.shards)
		if len(got) != len(test.want) {
			t.Fatalf("distribute(%d, %d) = %v, want %v", test.total, test.shards, got, test.want)
		}
		for index := range got {
			if got[index] != test.want[index] {
				t.Fatalf("distribute(%d, %d) = %v, want %v", test.total, test.shards, got, test.want)
			}
		}
	}
}
