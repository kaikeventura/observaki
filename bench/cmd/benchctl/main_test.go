package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/observaki/observaki/bench/internal/envprofile"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCheckRepositoryScenarios(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", repoRoot(t), "check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("check exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "check: 12 scenario(s) valid") {
		t.Fatalf("unexpected check output:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "20 queries") {
		t.Fatalf("check did not validate the dashboard query set:\n%s", stdout.String())
	}
}

func TestListShowsEveryFamily(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", repoRoot(t), "list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("list exit = %d, stderr = %s", code, stderr.String())
	}
	for _, name := range []string{"throughput-100k", "throughput-1m", "throughput-5m", "query-1b", "query-10b", "query-86b", "cardinality-100k", "cardinality-1m", "cardinality-10m", "chaos-clickhouse", "chaos-nats", "chaos-gateway"} {
		if !strings.Contains(stdout.String(), name) {
			t.Errorf("list output is missing %s", name)
		}
	}
	if !strings.Contains(stdout.String(), "planned (Story 3.5)") {
		t.Errorf("list output does not show the query executor status:\n%s", stdout.String())
	}
}

func TestQueriesCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", repoRoot(t), "queries"}, &stdout, &stderr); code != 0 {
		t.Fatalf("queries exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "q20") || !strings.Contains(stdout.String(), "p99 latency trend") {
		t.Fatalf("unexpected queries output:\n%s", stdout.String())
	}
}

func TestRunPlanThroughput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--root", repoRoot(t), "run", "throughput-100k", "--plan", "--results-dir", t.TempDir()}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run --plan exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status: planned") {
		t.Fatalf("unexpected run output:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--rate 100000") {
		t.Fatalf("plan does not contain the scenario rate:\n%s", stdout.String())
	}
}

func TestRunPlanQueryIsBlockedButPlanned(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--root", repoRoot(t), "run", "query-1b", "--plan", "--results-dir", t.TempDir()}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run --plan exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--event-time-rate 11574") {
		t.Fatalf("query plan does not contain the event-time rate:\n%s", stdout.String())
	}
}

func TestEnvShow(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", repoRoot(t), "env", "show"}, &stdout, &stderr); code != 0 {
		t.Fatalf("env show exit = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"profile", "t1", "clickhouse", "m6i.4xlarge", "1M events/s"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("env show output is missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestEnvCheck(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", repoRoot(t), "env", "check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("env check exit = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"ok", "6 clickhouse", "headroom", "events/s capacity", "cost"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("env check output is missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestEnvCostJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", repoRoot(t), "env", "cost", "--json", "--hours", "2"}, &stdout, &stderr); code != 0 {
		t.Fatalf("env cost exit = %d, stderr = %s", code, stderr.String())
	}
	var cost envprofile.Cost
	if err := json.Unmarshal(stdout.Bytes(), &cost); err != nil {
		t.Fatalf("decode cost JSON: %v\n%s", err, stdout.String())
	}
	if cost.HourlyTotal <= 0 || cost.RunTotal <= 0 || cost.MonthlyTotal <= 0 {
		t.Fatalf("unexpected cost totals: %+v", cost)
	}
	if len(cost.Roles) != 5 {
		t.Fatalf("role costs = %d, want 5", len(cost.Roles))
	}
}

func TestEnvTFVars(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", repoRoot(t), "env", "tfvars"}, &stdout, &stderr); code != 0 {
		t.Fatalf("env tfvars exit = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"clickhouse_count = 6", "gateway_count = 6", `region = "us-east-1"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("env tfvars output is missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestEnvCheckRejectsBadProfile(t *testing.T) {
	root := repoRoot(t)
	profile, err := envprofile.LoadProfile(filepath.Join(root, "bench", "env", "t1.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile.Roles[0].Count = 5
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bad-profile.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", root, "env", "check", "--profile", path}, &stdout, &stderr); code != 1 {
		t.Fatalf("env check exit = %d, want 1; stdout = %s; stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "clickhouse count = 5") {
		t.Fatalf("unexpected env check error:\n%s", stderr.String())
	}
}

func TestReportFromPlannedRun(t *testing.T) {
	results := t.TempDir()
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	args := []string{"--root", repoRoot(t), "run", "throughput-100k", "--plan", "--results-dir", results}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run --plan exit = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	args = []string{"--root", repoRoot(t), "report", "--results-dir", results, "--out", out}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("report exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "report: 1 run(s)") {
		t.Fatalf("unexpected report output:\n%s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(out, "report.json")); err != nil {
		t.Fatalf("report.json missing: %v", err)
	}
	markdown, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Observaki benchmark report", "throughput-100k", "planned run; no workload was executed"} {
		if !strings.Contains(string(markdown), want) {
			t.Errorf("report.md is missing %q", want)
		}
	}
}
