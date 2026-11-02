package vmbench

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Query is one MetricsQL dashboard query.
type Query struct {
	ID          string
	Surface     string
	Description string
	Expr        string
	Window      time.Duration
	Step        time.Duration
}

// LoadQueries reads the TSV catalog. The header is required; comment lines
// start with '#'.
func LoadQueries(path string) ([]Query, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	line := 0
	headerSeen := false
	var queries []Query
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if !headerSeen {
			headerSeen = true
			if text != "id\tsurface\tdescription\texpr\twindow\tstep" {
				return nil, fmt.Errorf("vmbench: %s:%d: unexpected header %q", path, line, text)
			}
			continue
		}
		fields := strings.Split(text, "\t")
		if len(fields) != 6 {
			return nil, fmt.Errorf("vmbench: %s:%d: %d fields, want 6", path, line, len(fields))
		}
		window, err := time.ParseDuration(fields[4])
		if err != nil {
			return nil, fmt.Errorf("vmbench: %s:%d: window: %w", path, line, err)
		}
		step, err := time.ParseDuration(fields[5])
		if err != nil {
			return nil, fmt.Errorf("vmbench: %s:%d: step: %w", path, line, err)
		}
		queries = append(queries, Query{
			ID:          fields[0],
			Surface:     fields[1],
			Description: fields[2],
			Expr:        fields[3],
			Window:      window,
			Step:        step,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !headerSeen {
		return nil, fmt.Errorf("vmbench: %s: empty query catalog", path)
	}
	if len(queries) != 20 {
		return nil, fmt.Errorf("vmbench: %s: %d queries, want 20", path, len(queries))
	}
	return queries, nil
}

// ParseLimit parses a tenant=limit flag value.
func ParseLimit(value string) (tenant string, limit int, err error) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("vmbench: limit %q must be tenant=N", value)
	}
	tenant = strings.TrimSpace(parts[0])
	if tenant == "" {
		return "", 0, fmt.Errorf("vmbench: limit %q has an empty tenant", value)
	}
	limit, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || limit < 0 {
		return "", 0, fmt.Errorf("vmbench: limit %q must have a non-negative integer", value)
	}
	return tenant, limit, nil
}
