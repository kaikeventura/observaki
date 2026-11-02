package vmbench

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadQueries(t *testing.T) {
	queries, err := LoadQueries(filepath.Join("..", "..", "victoriametrics", "queries", "catalog.tsv"))
	if err != nil {
		t.Fatalf("LoadQueries: %v", err)
	}
	if len(queries) != 20 {
		t.Fatalf("queries = %d, want 20", len(queries))
	}
	seen := map[string]bool{}
	for _, query := range queries {
		if query.ID == "" || query.Expr == "" {
			t.Fatalf("empty query: %+v", query)
		}
		if seen[query.ID] {
			t.Fatalf("duplicate query ID %q", query.ID)
		}
		seen[query.ID] = true
		if query.Window != 24*time.Hour {
			t.Fatalf("query %s window = %v, want 24h", query.ID, query.Window)
		}
	}
}

func TestLoadQueriesRejectsWrongHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.tsv")
	if err := os.WriteFile(path, []byte("id\tsurface\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadQueries(path); err == nil {
		t.Fatal("LoadQueries accepted a wrong header")
	}
}

func TestParseLimit(t *testing.T) {
	tenant, limit, err := ParseLimit("tenant-a=150000")
	if err != nil {
		t.Fatalf("ParseLimit: %v", err)
	}
	if tenant != "tenant-a" || limit != 150_000 {
		t.Fatalf("ParseLimit = %q, %d", tenant, limit)
	}
	if _, _, err := ParseLimit("tenant-a"); err == nil {
		t.Fatal("ParseLimit accepted a missing limit")
	}
	if _, _, err := ParseLimit("tenant-a=-1"); err == nil {
		t.Fatal("ParseLimit accepted a negative limit")
	}
}

func TestPercentileAndSummary(t *testing.T) {
	values := []float64{10, 30, 20, 40}
	if got := Percentile(values, 50); got != 20 {
		t.Fatalf("p50 = %v, want 20", got)
	}
	if got := Percentile(values, 95); got != 40 {
		t.Fatalf("p95 = %v, want 40", got)
	}
	summary := Summarize(values)
	if summary.Count != 4 || summary.P95MS != 40 || summary.MaxMS != 40 {
		t.Fatalf("summary = %+v", summary)
	}
}
