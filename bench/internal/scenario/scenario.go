// Package scenario loads and validates the declarative benchmark scenarios
// that live in bench/scenarios.
package scenario

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Kind is the scenario family.
type Kind string

const (
	KindThroughput  Kind = "throughput"
	KindQuery       Kind = "query"
	KindCardinality Kind = "cardinality"
	KindChaos       Kind = "chaos"
)

// Duration is a time.Duration that renders as a human-readable string in JSON.
type Duration time.Duration

// String implements fmt.Stringer.
func (d Duration) String() string { return time.Duration(d).String() }

// MarshalJSON renders the duration as a Go duration string.
func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// Definition is one benchmark scenario. Fields that only apply to some kinds
// are zero for the others.
type Definition struct {
	Name           string   `json:"name"`
	Kind           Kind     `json:"kind"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Hardware       string   `json:"hardware"`
	Duration       Duration `json:"duration"`
	Requires       []string `json:"requires"`
	Metrics        []string `json:"metrics"`
	Expected       string   `json:"expected"`
	BlockedByStory string   `json:"blocked_by_story,omitempty"`

	// Throughput and chaos.
	Rate        int    `json:"rate,omitempty"`
	Shards      int    `json:"shards,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	Compression string `json:"compression,omitempty"`

	// Query.
	DatasetEvents    int64    `json:"dataset_events,omitempty"`
	DatasetWindow    Duration `json:"dataset_window,omitempty"`
	SeedRate         int      `json:"seed_rate,omitempty"`
	SeedShards       int      `json:"seed_shards,omitempty"`
	QuerySet         string   `json:"query_set,omitempty"`
	QueryRepetitions int      `json:"query_repetitions,omitempty"`
	P95MS            int      `json:"p95_ms,omitempty"`

	// Cardinality.
	Series          int64 `json:"series,omitempty"`
	Tenants         int   `json:"tenants,omitempty"`
	SampleRate      int   `json:"sample_rate,omitempty"`
	P95DriftPercent int   `json:"p95_drift_percent,omitempty"`

	// Chaos.
	Failure          string   `json:"failure,omitempty"`
	Outage           Duration `json:"outage,omitempty"`
	LossBudgetEvents int64    `json:"loss_budget_events,omitempty"`

	// Path is where the definition was loaded from.
	Path string `json:"-"`
	// Raw keeps the original file for the result folder.
	Raw []byte `json:"-"`
}

// EventTimeRate is the event-time rate needed to spread DatasetEvents over
// DatasetWindow. It is used to seed a dataset faster than real time while the
// timestamps still cover a realistic query window.
func (d Definition) EventTimeRate() int64 {
	window := int64(d.DatasetWindow) / int64(time.Second)
	if d.DatasetEvents <= 0 || window <= 0 {
		return 0
	}
	return d.DatasetEvents / window
}

// SeedDuration is how long the dataset takes at the configured seed rate.
func (d Definition) SeedDuration() time.Duration {
	if d.DatasetEvents <= 0 || d.SeedRate <= 0 {
		return 0
	}
	seconds := float64(d.DatasetEvents) / float64(d.SeedRate)
	return time.Duration(seconds * float64(time.Second))
}

// ShardRate returns the per-shard rate when the total is divisible.
func (d Definition) ShardRate() int {
	if d.Shards <= 0 {
		return 0
	}
	return d.Rate / d.Shards
}

// Has reports whether the scenario requires the named service.
func (d Definition) Has(service string) bool {
	for _, item := range d.Requires {
		if item == service {
			return true
		}
	}
	return false
}

// ParseKind validates a scenario kind.
func ParseKind(value string) (Kind, error) {
	switch Kind(strings.ToLower(strings.TrimSpace(value))) {
	case KindThroughput:
		return KindThroughput, nil
	case KindQuery:
		return KindQuery, nil
	case KindCardinality:
		return KindCardinality, nil
	case KindChaos:
		return KindChaos, nil
	default:
		return "", fmt.Errorf("unsupported scenario kind %q (want throughput, query, cardinality or chaos)", value)
	}
}

var commonKeys = map[string]struct{}{
	"SCENARIO_NAME":             {},
	"SCENARIO_KIND":             {},
	"SCENARIO_TITLE":            {},
	"SCENARIO_DESCRIPTION":      {},
	"SCENARIO_HARDWARE":         {},
	"SCENARIO_DURATION":         {},
	"SCENARIO_REQUIRES":         {},
	"SCENARIO_METRICS":          {},
	"SCENARIO_EXPECTED":         {},
	"SCENARIO_BLOCKED_BY_STORY": {},
}

var loadKeys = map[string]struct{}{
	"SCENARIO_RATE":        {},
	"SCENARIO_SHARDS":      {},
	"SCENARIO_PROTOCOL":    {},
	"SCENARIO_COMPRESSION": {},
}

var kindKeys = map[Kind]map[string]struct{}{
	KindThroughput: loadKeys,
	KindChaos: {
		"SCENARIO_RATE":               {},
		"SCENARIO_SHARDS":             {},
		"SCENARIO_PROTOCOL":           {},
		"SCENARIO_COMPRESSION":        {},
		"SCENARIO_FAILURE":            {},
		"SCENARIO_OUTAGE":             {},
		"SCENARIO_LOSS_BUDGET_EVENTS": {},
	},
	KindQuery: {
		"SCENARIO_DATASET_EVENTS":    {},
		"SCENARIO_DATASET_WINDOW":    {},
		"SCENARIO_SEED_RATE":         {},
		"SCENARIO_SEED_SHARDS":       {},
		"SCENARIO_QUERY_SET":         {},
		"SCENARIO_QUERY_REPETITIONS": {},
		"SCENARIO_P95_MS":            {},
	},
	KindCardinality: {
		"SCENARIO_SERIES":            {},
		"SCENARIO_TENANTS":           {},
		"SCENARIO_SAMPLE_RATE":       {},
		"SCENARIO_P95_DRIFT_PERCENT": {},
	},
}

var services = map[string]struct{}{
	"loadgen":         {},
	"otlp":            {},
	"gateway":         {},
	"clickhouse":      {},
	"victoriametrics": {},
	"nats":            {},
}

var metricNames = map[string]struct{}{
	"events_per_second":           {},
	"bytes_per_second":            {},
	"ingest_lag_p50_ms":           {},
	"ingest_lag_p95_ms":           {},
	"ingest_lag_p99_ms":           {},
	"error_rate":                  {},
	"dropped_events":              {},
	"confirmed_lost_events":       {},
	"cpu_percent":                 {},
	"ram_bytes":                   {},
	"disk_bytes":                  {},
	"compression_ratio":           {},
	"query_latency_p50_ms":        {},
	"query_latency_p95_ms":        {},
	"query_latency_p99_ms":        {},
	"rows_read":                   {},
	"bytes_read":                  {},
	"cache_hit_ratio":             {},
	"series_active":               {},
	"series_limit_errors":         {},
	"neighbor_p95_drift_percent":  {},
	"recovery_seconds":            {},
	"dlq_events":                  {},
	"loadgen_backpressure_events": {},
	"loadgen_pacing_lag_events":   {},
}

// LoadDir loads every .env scenario in dir, sorted by name.
func LoadDir(dir string) ([]Definition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read scenarios directory: %w", err)
	}
	definitions := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".env") {
			continue
		}
		definition, err := LoadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	if len(definitions) == 0 {
		return nil, fmt.Errorf("no .env scenarios found in %s", dir)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	return definitions, nil
}

// LoadFile loads and validates one scenario file. The scenario name must match
// the file name without the .env suffix.
func LoadFile(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read scenario: %w", err)
	}
	name := strings.TrimSuffix(filepath.Base(path), ".env")
	definition, err := Parse(name, string(data))
	if err != nil {
		return Definition{}, fmt.Errorf("%s: %w", path, err)
	}
	if definition.Name != name {
		return Definition{}, fmt.Errorf("%s: SCENARIO_NAME %q must match the file name %q", path, definition.Name, name)
	}
	definition.Path = path
	definition.Raw = data
	return definition, nil
}

// Parse parses and validates a scenario definition.
func Parse(name, data string) (Definition, error) {
	definition := Definition{Name: name}
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return definition, fmt.Errorf("line %d: expected KEY=value", lineNumber)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return definition, fmt.Errorf("line %d: empty key", lineNumber)
		}
		if _, duplicate := seen[key]; duplicate {
			return definition, fmt.Errorf("line %d: duplicate key %s", lineNumber, key)
		}
		seen[key] = struct{}{}
		if err := definition.set(key, unquote(strings.TrimSpace(value))); err != nil {
			return definition, fmt.Errorf("line %d: %w", lineNumber, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return definition, fmt.Errorf("read scenario: %w", err)
	}
	if definition.Name != name {
		return definition, fmt.Errorf("SCENARIO_NAME %q must match the file name %q", definition.Name, name)
	}
	if err := definition.validate(seen); err != nil {
		return definition, err
	}
	return definition, nil
}

func unquote(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

func (d *Definition) set(key, value string) error {
	var err error
	switch key {
	case "SCENARIO_NAME":
		d.Name = value
	case "SCENARIO_KIND":
		d.Kind, err = ParseKind(value)
	case "SCENARIO_TITLE":
		d.Title = value
	case "SCENARIO_DESCRIPTION":
		d.Description = value
	case "SCENARIO_HARDWARE":
		d.Hardware = value
	case "SCENARIO_DURATION":
		d.Duration, err = ParseDuration(value)
	case "SCENARIO_REQUIRES":
		d.Requires = splitList(value)
	case "SCENARIO_METRICS":
		d.Metrics = splitList(value)
	case "SCENARIO_EXPECTED":
		d.Expected = value
	case "SCENARIO_BLOCKED_BY_STORY":
		d.BlockedByStory = value
	case "SCENARIO_RATE":
		d.Rate, err = strconv.Atoi(value)
	case "SCENARIO_SHARDS":
		d.Shards, err = strconv.Atoi(value)
	case "SCENARIO_PROTOCOL":
		d.Protocol = value
	case "SCENARIO_COMPRESSION":
		d.Compression = value
	case "SCENARIO_DATASET_EVENTS":
		d.DatasetEvents, err = strconv.ParseInt(value, 10, 64)
	case "SCENARIO_DATASET_WINDOW":
		d.DatasetWindow, err = ParseDuration(value)
	case "SCENARIO_SEED_RATE":
		d.SeedRate, err = strconv.Atoi(value)
	case "SCENARIO_SEED_SHARDS":
		d.SeedShards, err = strconv.Atoi(value)
	case "SCENARIO_QUERY_SET":
		d.QuerySet = value
	case "SCENARIO_QUERY_REPETITIONS":
		d.QueryRepetitions, err = strconv.Atoi(value)
	case "SCENARIO_P95_MS":
		d.P95MS, err = strconv.Atoi(value)
	case "SCENARIO_SERIES":
		d.Series, err = strconv.ParseInt(value, 10, 64)
	case "SCENARIO_TENANTS":
		d.Tenants, err = strconv.Atoi(value)
	case "SCENARIO_SAMPLE_RATE":
		d.SampleRate, err = strconv.Atoi(value)
	case "SCENARIO_P95_DRIFT_PERCENT":
		d.P95DriftPercent, err = strconv.Atoi(value)
	case "SCENARIO_FAILURE":
		d.Failure = value
	case "SCENARIO_OUTAGE":
		d.Outage, err = ParseDuration(value)
	case "SCENARIO_LOSS_BUDGET_EVENTS":
		d.LossBudgetEvents, err = strconv.ParseInt(value, 10, 64)
	default:
		return fmt.Errorf("unknown key %s", key)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}

// ParseDuration parses Go durations plus a day suffix (for example 7d).
func ParseDuration(value string) (Duration, error) {
	if strings.HasSuffix(value, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(value, "d"), 64)
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("invalid duration %q", value)
		}
		return Duration(time.Duration(days * 24 * float64(time.Hour))), nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", value, err)
	}
	return Duration(parsed), nil
}

func (d Definition) validate(seen map[string]struct{}) error {
	for key := range seen {
		if _, ok := commonKeys[key]; ok {
			continue
		}
		if _, ok := kindKeys[d.Kind][key]; ok {
			continue
		}
		return fmt.Errorf("key %s is not valid for kind %q", key, d.Kind)
	}

	required := []string{
		"SCENARIO_NAME", "SCENARIO_KIND", "SCENARIO_TITLE", "SCENARIO_DESCRIPTION",
		"SCENARIO_HARDWARE", "SCENARIO_DURATION", "SCENARIO_REQUIRES",
		"SCENARIO_METRICS", "SCENARIO_EXPECTED",
	}
	switch d.Kind {
	case KindThroughput:
		required = append(required, "SCENARIO_RATE", "SCENARIO_SHARDS", "SCENARIO_PROTOCOL", "SCENARIO_COMPRESSION")
	case KindChaos:
		required = append(required, "SCENARIO_RATE", "SCENARIO_SHARDS", "SCENARIO_PROTOCOL", "SCENARIO_COMPRESSION",
			"SCENARIO_FAILURE", "SCENARIO_OUTAGE", "SCENARIO_LOSS_BUDGET_EVENTS")
	case KindQuery:
		required = append(required, "SCENARIO_DATASET_EVENTS", "SCENARIO_DATASET_WINDOW", "SCENARIO_SEED_RATE",
			"SCENARIO_SEED_SHARDS", "SCENARIO_QUERY_SET", "SCENARIO_QUERY_REPETITIONS", "SCENARIO_P95_MS")
	case KindCardinality:
		required = append(required, "SCENARIO_SERIES", "SCENARIO_TENANTS", "SCENARIO_SAMPLE_RATE",
			"SCENARIO_P95_DRIFT_PERCENT")
	}
	for _, key := range required {
		if _, ok := seen[key]; !ok {
			return fmt.Errorf("missing required key %s", key)
		}
	}
	if d.Duration <= 0 {
		return fmt.Errorf("duration must be greater than zero")
	}
	if len(d.Requires) == 0 {
		return fmt.Errorf("requires must list at least one service")
	}
	for _, service := range d.Requires {
		if _, ok := services[service]; !ok {
			return fmt.Errorf("unsupported required service %q", service)
		}
	}
	if len(d.Metrics) == 0 {
		return fmt.Errorf("metrics must list at least one metric")
	}
	for _, metric := range d.Metrics {
		if _, ok := metricNames[metric]; !ok {
			return fmt.Errorf("unsupported metric %q", metric)
		}
	}

	switch d.Kind {
	case KindThroughput:
		return d.validateLoad()
	case KindChaos:
		if err := d.validateLoad(); err != nil {
			return err
		}
		switch d.Failure {
		case "clickhouse", "nats", "gateway":
		default:
			return fmt.Errorf("unsupported failure %q (want clickhouse, nats or gateway)", d.Failure)
		}
		if d.Outage <= 0 || time.Duration(d.Outage) >= time.Duration(d.Duration) {
			return fmt.Errorf("outage must be greater than zero and shorter than the scenario duration")
		}
		if d.LossBudgetEvents != 0 {
			return fmt.Errorf("loss budget must stay at zero for the go/no-go gate")
		}
	case KindQuery:
		if d.DatasetEvents <= 0 {
			return fmt.Errorf("dataset events must be greater than zero")
		}
		if d.DatasetWindow <= 0 {
			return fmt.Errorf("dataset window must be greater than zero")
		}
		if d.SeedRate <= 0 || d.SeedShards <= 0 {
			return fmt.Errorf("seed rate and shards must be greater than zero")
		}
		if d.SeedRate%d.SeedShards != 0 {
			return fmt.Errorf("seed rate must be divisible by seed shards")
		}
		if d.EventTimeRate() <= 0 {
			return fmt.Errorf("dataset window is too small for the dataset size")
		}
		if d.QuerySet == "" || d.QueryRepetitions <= 0 || d.P95MS <= 0 {
			return fmt.Errorf("query set, repetitions and p95 target are required")
		}
		if d.SeedDuration() >= time.Duration(d.Duration) {
			return fmt.Errorf("scenario duration %s does not cover the %s seed phase", d.Duration, d.SeedDuration())
		}
	case KindCardinality:
		if d.Series <= 0 || d.SampleRate <= 0 {
			return fmt.Errorf("series and sample rate must be greater than zero")
		}
		if d.Tenants < 2 {
			return fmt.Errorf("at least two tenants are required for the noisy-neighbor check")
		}
		if d.P95DriftPercent <= 0 {
			return fmt.Errorf("p95 drift target must be greater than zero")
		}
	}
	return nil
}

func (d Definition) validateLoad() error {
	if d.Rate <= 0 || d.Shards <= 0 {
		return fmt.Errorf("rate and shards must be greater than zero")
	}
	if d.Rate%d.Shards != 0 {
		return fmt.Errorf("rate must be divisible by shards")
	}
	switch d.Protocol {
	case "grpc", "http":
	default:
		return fmt.Errorf("unsupported protocol %q (want grpc or http)", d.Protocol)
	}
	switch d.Compression {
	case "gzip", "none":
	default:
		return fmt.Errorf("unsupported compression %q (want gzip or none)", d.Compression)
	}
	return nil
}
