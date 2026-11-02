package scenario

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Query is one representative dashboard query. It is an engine-independent
// contract: the SQL/MetricsQL binding lands with the storage schema.
type Query struct {
	ID          string
	Surface     string
	Signal      string
	Engine      string
	Window      string
	Datasets    []string
	Description string
}

// DashboardQueryCount is the number of representative queries required by the
// benchmark methodology (ADR-0006, Q-03).
const DashboardQueryCount = 20

var (
	querySignals = map[string]struct{}{"logs": {}, "traces": {}, "metrics": {}}
	queryEngines = map[string]struct{}{"sql": {}, "metricsql": {}}
	queryWindows = map[string]struct{}{
		"point": {}, "15m": {}, "1h": {}, "24h": {}, "7d": {}, "scenario": {},
	}
	queryDatasets = map[string]struct{}{"1b": {}, "10b": {}, "86b": {}}
)

// LoadQuerySet loads a tab-separated query catalog.
func LoadQuerySet(path string) ([]Query, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open query set: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	queries := make([]Query, 0, DashboardQueryCount)
	headerSeen := false
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		columns := strings.Split(line, "\t")
		if !headerSeen {
			headerSeen = true
			if len(columns) != 7 || columns[0] != "id" {
				return nil, fmt.Errorf("%s:%d: expected header id, surface, signal, engine, window, datasets, description", path, lineNumber)
			}
			continue
		}
		if len(columns) != 7 {
			return nil, fmt.Errorf("%s:%d: expected 7 columns, got %d", path, lineNumber, len(columns))
		}
		query := Query{
			ID:          strings.TrimSpace(columns[0]),
			Surface:     strings.TrimSpace(columns[1]),
			Signal:      strings.TrimSpace(columns[2]),
			Engine:      strings.TrimSpace(columns[3]),
			Window:      strings.TrimSpace(columns[4]),
			Datasets:    splitList(columns[5]),
			Description: strings.TrimSpace(columns[6]),
		}
		if err := query.validate(); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
		queries = append(queries, query)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read query set: %w", err)
	}
	if !headerSeen {
		return nil, fmt.Errorf("%s: missing header", path)
	}
	if err := validateQueries(queries); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return queries, nil
}

func (q Query) validate() error {
	if q.ID == "" || q.Surface == "" || q.Description == "" {
		return fmt.Errorf("id, surface and description are required")
	}
	if _, ok := querySignals[q.Signal]; !ok {
		return fmt.Errorf("unsupported signal %q", q.Signal)
	}
	if _, ok := queryEngines[q.Engine]; !ok {
		return fmt.Errorf("unsupported engine %q", q.Engine)
	}
	if _, ok := queryWindows[q.Window]; !ok {
		return fmt.Errorf("unsupported window %q", q.Window)
	}
	if len(q.Datasets) == 0 {
		return fmt.Errorf("at least one dataset is required")
	}
	for _, dataset := range q.Datasets {
		if _, ok := queryDatasets[dataset]; !ok {
			return fmt.Errorf("unsupported dataset %q", dataset)
		}
	}
	return nil
}

func validateQueries(queries []Query) error {
	if len(queries) != DashboardQueryCount {
		return fmt.Errorf("expected %d queries, got %d", DashboardQueryCount, len(queries))
	}
	seen := make(map[string]struct{}, len(queries))
	for _, query := range queries {
		if _, duplicate := seen[query.ID]; duplicate {
			return fmt.Errorf("duplicate query id %q", query.ID)
		}
		seen[query.ID] = struct{}{}
	}
	return nil
}

// QueryCount returns the number of queries a known query set must contain.
func QueryCount(set string) int {
	if set == "dashboard" {
		return DashboardQueryCount
	}
	return 0
}
