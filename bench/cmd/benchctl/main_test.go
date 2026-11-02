package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
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
