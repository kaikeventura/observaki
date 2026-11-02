package scenario_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/observaki/observaki/bench/internal/scenario"
)

const throughputFixture = `SCENARIO_NAME=demo
SCENARIO_KIND=throughput
SCENARIO_TITLE=Demo
SCENARIO_DESCRIPTION=Demo scenario
SCENARIO_HARDWARE=Demo hardware
SCENARIO_DURATION=10m
SCENARIO_REQUIRES=loadgen,otlp,gateway
SCENARIO_METRICS=events_per_second
SCENARIO_EXPECTED=Demo expected output
SCENARIO_RATE=1000
SCENARIO_SHARDS=1
SCENARIO_PROTOCOL=http
SCENARIO_COMPRESSION=none
`

func TestRepositoryScenarios(t *testing.T) {
	definitions, err := scenario.LoadDir(filepath.Join("..", "..", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(definitions), 12; got != want {
		t.Fatalf("scenario count = %d, want %d", got, want)
	}

	expected := map[string]struct {
		kind scenario.Kind
		rate int
	}{
		"throughput-100k":  {kind: scenario.KindThroughput, rate: 100_000},
		"throughput-1m":    {kind: scenario.KindThroughput, rate: 1_000_000},
		"throughput-5m":    {kind: scenario.KindThroughput, rate: 5_000_000},
		"query-1b":         {kind: scenario.KindQuery},
		"query-10b":        {kind: scenario.KindQuery},
		"query-86b":        {kind: scenario.KindQuery},
		"cardinality-100k": {kind: scenario.KindCardinality},
		"cardinality-1m":   {kind: scenario.KindCardinality},
		"cardinality-10m":  {kind: scenario.KindCardinality},
		"chaos-clickhouse": {kind: scenario.KindChaos},
		"chaos-nats":       {kind: scenario.KindChaos},
		"chaos-gateway":    {kind: scenario.KindChaos},
	}
	for _, definition := range definitions {
		want, ok := expected[definition.Name]
		if !ok {
			t.Fatalf("unexpected scenario %q", definition.Name)
		}
		if definition.Kind != want.kind {
			t.Errorf("%s kind = %q, want %q", definition.Name, definition.Kind, want.kind)
		}
		if want.rate != 0 && definition.Rate != want.rate {
			t.Errorf("%s rate = %d, want %d", definition.Name, definition.Rate, want.rate)
		}
		if definition.Expected == "" || definition.Hardware == "" {
			t.Errorf("%s is missing documentation fields", definition.Name)
		}
	}
}

func TestQueryDatasetMath(t *testing.T) {
	definitions, err := scenario.LoadDir(filepath.Join("..", "..", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]scenario.Definition, len(definitions))
	for _, definition := range definitions {
		byName[definition.Name] = definition
	}

	tests := []struct {
		name          string
		events        int64
		window        time.Duration
		eventTimeRate int64
		seedDuration  time.Duration
	}{
		{name: "query-1b", events: 1_000_000_000, window: 24 * time.Hour, eventTimeRate: 11_574, seedDuration: 1000 * time.Second},
		{name: "query-10b", events: 10_000_000_000, window: 7 * 24 * time.Hour, eventTimeRate: 16_534, seedDuration: 2000 * time.Second},
		{name: "query-86b", events: 86_400_000_000, window: 24 * time.Hour, eventTimeRate: 1_000_000, seedDuration: 17_280 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition, ok := byName[test.name]
			if !ok {
				t.Fatalf("scenario %s not found", test.name)
			}
			if definition.DatasetEvents != test.events {
				t.Errorf("events = %d, want %d", definition.DatasetEvents, test.events)
			}
			if got := time.Duration(definition.DatasetWindow); got != test.window {
				t.Errorf("window = %s, want %s", got, test.window)
			}
			if got := definition.EventTimeRate(); got != test.eventTimeRate {
				t.Errorf("event time rate = %d, want %d", got, test.eventTimeRate)
			}
			if got := definition.SeedDuration(); got != test.seedDuration {
				t.Errorf("seed duration = %s, want %s", got, test.seedDuration)
			}
		})
	}
}

func TestQuerySets(t *testing.T) {
	definitions, err := scenario.LoadDir(filepath.Join("..", "..", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		if definition.Kind != scenario.KindQuery {
			continue
		}
		queries, err := scenario.LoadQuerySet(filepath.Join("..", "..", "queries", definition.QuerySet+".tsv"))
		if err != nil {
			t.Fatalf("%s: %v", definition.Name, err)
		}
		if len(queries) != scenario.DashboardQueryCount {
			t.Fatalf("%s has %d queries, want %d", definition.Name, len(queries), scenario.DashboardQueryCount)
		}
		if scenario.QueryCount(definition.QuerySet) != len(queries) {
			t.Fatalf("%s query count does not match the catalog", definition.Name)
		}
	}
}

func TestParseRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unknown key",
			content: throughputFixture + "SCENARIO_NOPE=1\n",
			want:    "unknown key",
		},
		{
			name:    "duplicate key",
			content: throughputFixture + "SCENARIO_RATE=2000\n",
			want:    "duplicate key",
		},
		{
			name:    "name mismatch",
			content: strings.Replace(throughputFixture, "SCENARIO_NAME=demo", "SCENARIO_NAME=other", 1),
			want:    "must match the file name",
		},
		{
			name:    "invalid kind",
			content: strings.Replace(throughputFixture, "SCENARIO_KIND=throughput", "SCENARIO_KIND=soak", 1),
			want:    "unsupported scenario kind",
		},
		{
			name:    "field from another kind",
			content: throughputFixture + "SCENARIO_SERIES=1000\n",
			want:    "not valid for kind",
		},
		{
			name:    "rate not divisible",
			content: strings.Replace(throughputFixture, "SCENARIO_SHARDS=1", "SCENARIO_SHARDS=3", 1),
			want:    "rate must be divisible",
		},
		{
			name:    "missing required key",
			content: strings.Replace(throughputFixture, "SCENARIO_RATE=1000\n", "", 1),
			want:    "missing required key",
		},
		{
			name:    "invalid duration",
			content: strings.Replace(throughputFixture, "SCENARIO_DURATION=10m", "SCENARIO_DURATION=10x", 1),
			want:    "invalid duration",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := scenario.Parse("demo", test.content)
			if err == nil {
				t.Fatal("Parse() = nil, want error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse() error = %q, want it to contain %q", err, test.want)
			}
		})
	}
}

func TestParseAcceptsDayDurations(t *testing.T) {
	content := strings.Replace(throughputFixture, "SCENARIO_DURATION=10m", "SCENARIO_DURATION=2d", 1)
	definition, err := scenario.Parse("demo", content)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := time.Duration(definition.Duration), 48*time.Hour; got != want {
		t.Fatalf("duration = %s, want %s", got, want)
	}
}

func TestLoadFileValidatesFileAndScenarioName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.env")
	if err := os.WriteFile(path, []byte(throughputFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	definition, err := scenario.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Name != "demo" {
		t.Fatalf("name = %q, want demo", definition.Name)
	}
	if len(definition.Raw) == 0 {
		t.Fatal("Raw snapshot is empty")
	}

	other := filepath.Join(dir, "other.env")
	if err := os.WriteFile(other, []byte(throughputFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := scenario.LoadFile(other); err == nil || !strings.Contains(err.Error(), "must match the file name") {
		t.Fatalf("LoadFile() error = %v, want name mismatch", err)
	}
}

func TestLoadQuerySetRejectsWrongCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.tsv")
	content := "id\tsurface\tsignal\tengine\twindow\tdatasets\tdescription\n" +
		"q01\tExplore\tlogs\tsql\t1h\t1b\tvolume\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := scenario.LoadQuerySet(path); err == nil || !strings.Contains(err.Error(), "expected 20 queries") {
		t.Fatalf("LoadQuerySet() error = %v, want count failure", err)
	}
}
