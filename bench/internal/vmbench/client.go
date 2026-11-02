package vmbench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Client talks to the VictoriaMetrics Prometheus-compatible query API.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// NoCache sets nocache=1 on every query, bypassing the rollup cache.
	NoCache bool
}

// NewClient returns a client with a bounded HTTP timeout.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        32,
				MaxIdleConnsPerHost: 32,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// VectorSample is one instant-query result.
type VectorSample struct {
	Metric    map[string]string
	Value     float64
	Timestamp int64
}

// Point is one point of a range-query result.
type Point struct {
	Timestamp int64
	Value     float64
}

// RangeSeries is one series returned by a range query.
type RangeSeries struct {
	Metric map[string]string
	Values []Point
}

// Query runs an instant query at the given time.
func (c *Client) Query(ctx context.Context, query string, at time.Time) ([]VectorSample, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("time", strconv.FormatInt(at.Unix(), 10))
	body, err := c.do(ctx, "/api/v1/query", params)
	if err != nil {
		return nil, err
	}
	var response struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("vmbench: decode instant query: %w", err)
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("vmbench: instant query failed: %s", response.Error)
	}
	samples := make([]VectorSample, 0, len(response.Data.Result))
	for _, result := range response.Data.Result {
		sample, err := parseVectorSample(result.Metric, result.Value)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, nil
}

// QueryRange runs a range query from start to end with the given step.
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]RangeSeries, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(start.Unix(), 10))
	params.Set("end", strconv.FormatInt(end.Unix(), 10))
	params.Set("step", strconv.FormatInt(int64(step.Seconds()), 10))
	body, err := c.do(ctx, "/api/v1/query_range", params)
	if err != nil {
		return nil, err
	}
	var response struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string   `json:"metric"`
				Values [][]json.RawMessage `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("vmbench: decode range query: %w", err)
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("vmbench: range query failed: %s", response.Error)
	}
	series := make([]RangeSeries, 0, len(response.Data.Result))
	for _, result := range response.Data.Result {
		points := make([]Point, 0, len(result.Values))
		for _, value := range result.Values {
			point, err := parsePoint(value)
			if err != nil {
				return nil, err
			}
			points = append(points, point)
		}
		series = append(series, RangeSeries{Metric: result.Metric, Values: points})
	}
	return series, nil
}

func (c *Client) do(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if c.NoCache {
		params.Set("nocache", "1")
	}
	endpoint := c.BaseURL + path + "?" + params.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vmbench: %s returned %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func parseVectorSample(metric map[string]string, value []json.RawMessage) (VectorSample, error) {
	if len(value) != 2 {
		return VectorSample{}, fmt.Errorf("vmbench: instant result has %d values, want 2", len(value))
	}
	var timestamp float64
	if err := json.Unmarshal(value[0], &timestamp); err != nil {
		return VectorSample{}, fmt.Errorf("vmbench: decode timestamp: %w", err)
	}
	var raw string
	if err := json.Unmarshal(value[1], &raw); err != nil {
		return VectorSample{}, fmt.Errorf("vmbench: decode sample: %w", err)
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return VectorSample{}, fmt.Errorf("vmbench: parse sample %q: %w", raw, err)
	}
	return VectorSample{Metric: metric, Value: parsed, Timestamp: int64(timestamp)}, nil
}

func parsePoint(value []json.RawMessage) (Point, error) {
	if len(value) != 2 {
		return Point{}, fmt.Errorf("vmbench: range point has %d values, want 2", len(value))
	}
	var timestamp float64
	if err := json.Unmarshal(value[0], &timestamp); err != nil {
		return Point{}, fmt.Errorf("vmbench: decode timestamp: %w", err)
	}
	var raw string
	if err := json.Unmarshal(value[1], &raw); err != nil {
		return Point{}, fmt.Errorf("vmbench: decode point: %w", err)
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return Point{}, fmt.Errorf("vmbench: parse point %q: %w", raw, err)
	}
	return Point{Timestamp: int64(timestamp), Value: parsed}, nil
}

// Percentile returns the nearest-rank percentile of values. It matches the
// convention used by the ClickHouse ingestion spike.
func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// Summary is a latency summary in milliseconds.
type Summary struct {
	Count int     `json:"count"`
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
	MaxMS float64 `json:"max_ms"`
}

// Summarize builds a Summary from millisecond values.
func Summarize(values []float64) Summary {
	return Summary{
		Count: len(values),
		P50MS: Percentile(values, 50),
		P95MS: Percentile(values, 95),
		P99MS: Percentile(values, 99),
		MaxMS: Percentile(values, 100),
	}
}
