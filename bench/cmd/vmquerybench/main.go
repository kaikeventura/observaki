// Command vmquerybench runs the 20 representative MetricsQL queries against
// VictoriaMetrics and verifies rate, histogram_quantile and label aggregation
// on a small deterministic dataset. It is the query harness behind Story 3.4.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/observaki/observaki/bench/internal/benchversion"
	"github.com/observaki/observaki/bench/internal/vmbench"
	"github.com/observaki/observaki/internal/promrw"
)

func main() {
	if code := run(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: vmquerybench run|query|verify [flags]")
		return 2
	}
	switch args[0] {
	case "run":
		return runQueries(args[1:], stdout, stderr)
	case "query":
		return runSingleQuery(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "--version", "version":
		_, _ = fmt.Fprintf(stdout, "vmquerybench %s (%s)\n", benchversion.Version, benchversion.Commit)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "vmquerybench: unknown command %q\n", args[0])
		return 2
	}
}

type singleQueryOptions struct {
	vmURL   string
	expr    string
	window  time.Duration
	step    time.Duration
	runs    int
	warmup  int
	out     string
	timeout time.Duration
	nocache bool
}

func runSingleQuery(args []string, stdout, stderr io.Writer) int {
	opts := singleQueryOptions{}
	flags := flag.NewFlagSet("vmquerybench query", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.vmURL, "vm-url", "http://127.0.0.1:18428", "VictoriaMetrics base URL")
	flags.StringVar(&opts.expr, "expr", "", "MetricsQL expression")
	flags.DurationVar(&opts.window, "window", 5*time.Minute, "query window")
	flags.DurationVar(&opts.step, "step", time.Minute, "query step")
	flags.IntVar(&opts.runs, "runs", 10, "measured runs")
	flags.IntVar(&opts.warmup, "warmup", 1, "warmup runs")
	flags.StringVar(&opts.out, "out", "", "report JSON path")
	flags.DurationVar(&opts.timeout, "timeout", 60*time.Second, "per-query timeout")
	flags.BoolVar(&opts.nocache, "nocache", false, "bypass the VictoriaMetrics rollup cache")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if opts.expr == "" || opts.runs <= 0 || opts.window <= 0 || opts.step <= 0 {
		_, _ = fmt.Fprintln(stderr, "vmquerybench: expr, runs, window and step are required")
		return 2
	}
	client := vmbench.NewClient(opts.vmURL)
	client.HTTP.Timeout = opts.timeout
	client.NoCache = opts.nocache
	now := time.Now().Truncate(time.Second)
	start := now.Add(-opts.window)
	for i := 0; i < opts.warmup; i++ {
		_, _ = client.QueryRange(context.Background(), opts.expr, start, now, opts.step)
	}
	var latencies []float64
	errors := 0
	resultSeries := 0
	for i := 0; i < opts.runs; i++ {
		begin := time.Now()
		series, err := client.QueryRange(context.Background(), opts.expr, start, now, opts.step)
		latencies = append(latencies, float64(time.Since(begin))/float64(time.Millisecond))
		if err != nil {
			errors++
			continue
		}
		if len(series) > resultSeries {
			resultSeries = len(series)
		}
	}
	summary := vmbench.Summarize(latencies)
	status := "pass"
	if errors > 0 || resultSeries == 0 {
		status = "fail"
	}
	report := map[string]any{
		"status":        status,
		"expr":          opts.expr,
		"window":        opts.window.String(),
		"step":          opts.step.String(),
		"runs":          opts.runs,
		"warmup":        opts.warmup,
		"result_series": resultSeries,
		"errors":        errors,
		"latency_ms":    summary,
	}
	if opts.out != "" {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "vmquerybench: encode report: %v\n", err)
			return 1
		}
		if err := os.WriteFile(opts.out, append(encoded, '\n'), 0o644); err != nil {
			_, _ = fmt.Fprintf(stderr, "vmquerybench: write report: %v\n", err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "vmquerybench: %s expr=%q series=%d p50=%.1fms p95=%.1fms errors=%d\n",
		status, opts.expr, resultSeries, summary.P50MS, summary.P95MS, errors)
	if status == "pass" {
		return 0
	}
	return 1
}

type queryOptions struct {
	vmURL   string
	catalog string
	runs    int
	warmup  int
	out     string
	timeout time.Duration
	nocache bool
}

func runQueries(args []string, stdout, stderr io.Writer) int {
	opts := queryOptions{}
	flags := flag.NewFlagSet("vmquerybench run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.vmURL, "vm-url", "http://127.0.0.1:18428", "VictoriaMetrics base URL")
	flags.StringVar(&opts.catalog, "catalog", "bench/victoriametrics/queries/catalog.tsv", "query catalog TSV")
	flags.IntVar(&opts.runs, "runs", 5, "measured runs per query")
	flags.IntVar(&opts.warmup, "warmup", 1, "warmup runs per query")
	flags.StringVar(&opts.out, "out", "", "report JSON path")
	flags.DurationVar(&opts.timeout, "timeout", 60*time.Second, "per-query timeout")
	flags.BoolVar(&opts.nocache, "nocache", false, "bypass the VictoriaMetrics rollup cache")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if opts.runs <= 0 {
		_, _ = fmt.Fprintln(stderr, "vmquerybench: runs must be positive")
		return 2
	}
	queries, err := vmbench.LoadQueries(opts.catalog)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vmquerybench: %v\n", err)
		return 2
	}
	client := vmbench.NewClient(opts.vmURL)
	client.HTTP.Timeout = opts.timeout
	client.NoCache = opts.nocache

	type queryResult struct {
		ID           string          `json:"id"`
		Surface      string          `json:"surface"`
		Description  string          `json:"description"`
		Expr         string          `json:"expr"`
		Window       string          `json:"window"`
		Step         string          `json:"step"`
		Runs         int             `json:"runs"`
		Errors       int             `json:"errors"`
		ResultSeries int             `json:"result_series"`
		ColdMS       float64         `json:"cold_latency_ms"`
		ColdError    string          `json:"cold_error,omitempty"`
		Latency      vmbench.Summary `json:"latency_ms"`
		Status       string          `json:"status"`
	}

	var results []queryResult
	var allLatencies []float64
	totalErrors := 0
	now := time.Now().Truncate(time.Second)
	for _, query := range queries {
		result := queryResult{
			ID:          query.ID,
			Surface:     query.Surface,
			Description: query.Description,
			Expr:        query.Expr,
			Window:      query.Window.String(),
			Step:        query.Step.String(),
			Runs:        opts.runs,
		}
		start := now.Add(-query.Window)
		coldStart := time.Now()
		series, err := client.QueryRange(context.Background(), query.Expr, start, now, query.Step)
		result.ColdMS = float64(time.Since(coldStart)) / float64(time.Millisecond)
		if err != nil {
			result.ColdError = err.Error()
			result.Errors++
			totalErrors++
		} else {
			result.ResultSeries = len(series)
		}
		for i := 0; i < opts.warmup; i++ {
			_, _ = client.QueryRange(context.Background(), query.Expr, start, now, query.Step)
		}
		var latencies []float64
		for i := 0; i < opts.runs; i++ {
			begin := time.Now()
			series, err := client.QueryRange(context.Background(), query.Expr, start, now, query.Step)
			latency := float64(time.Since(begin)) / float64(time.Millisecond)
			latencies = append(latencies, latency)
			allLatencies = append(allLatencies, latency)
			if err != nil {
				result.Errors++
				totalErrors++
				continue
			}
			if len(series) > result.ResultSeries {
				result.ResultSeries = len(series)
			}
		}
		result.Latency = vmbench.Summarize(latencies)
		result.Status = "pass"
		if result.Errors > 0 || result.ResultSeries == 0 {
			result.Status = "fail"
		}
		results = append(results, result)
		_, _ = fmt.Fprintf(stderr, "vmquerybench: %s %s series=%d p50=%.1fms p95=%.1fms errors=%d\n",
			result.ID, result.Status, result.ResultSeries, result.Latency.P50MS, result.Latency.P95MS, result.Errors)
	}

	overall := vmbench.Summarize(allLatencies)
	status := "pass"
	if overall.P95MS >= 1000 || totalErrors > 0 {
		status = "fail"
	}
	for _, result := range results {
		if result.Status != "pass" {
			status = "fail"
			break
		}
	}
	report := map[string]any{
		"status":             status,
		"window":             "24h",
		"step":               "5m",
		"runs":               opts.runs,
		"warmup":             opts.warmup,
		"overall_latency_ms": overall,
		"errors":             totalErrors,
		"queries":            results,
	}
	if opts.out != "" {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "vmquerybench: encode report: %v\n", err)
			return 1
		}
		if err := os.WriteFile(opts.out, append(encoded, '\n'), 0o644); err != nil {
			_, _ = fmt.Fprintf(stderr, "vmquerybench: write report: %v\n", err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "vmquerybench: %s queries=%d p50=%.1fms p95=%.1fms p99=%.1fms errors=%d\n",
		status, len(results), overall.P50MS, overall.P95MS, overall.P99MS, totalErrors)
	if status == "pass" {
		return 0
	}
	return 1
}

type verifyOptions struct {
	vmURL   string
	out     string
	timeout time.Duration
}

func runVerify(args []string, stdout, stderr io.Writer) int {
	opts := verifyOptions{}
	flags := flag.NewFlagSet("vmquerybench verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.vmURL, "vm-url", "http://127.0.0.1:18428", "VictoriaMetrics base URL")
	flags.StringVar(&opts.out, "out", "", "verification JSON path")
	flags.DurationVar(&opts.timeout, "timeout", 60*time.Second, "verification timeout")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	end := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	start := end.Add(-5 * time.Minute)
	if err := seedVerification(ctx, opts.vmURL, start, end); err != nil {
		_, _ = fmt.Fprintf(stderr, "vmquerybench: seed verification dataset: %v\n", err)
		return 1
	}
	client := vmbench.NewClient(opts.vmURL)
	client.HTTP.Timeout = opts.timeout

	if err := waitForVerification(ctx, client, end, 300); err != nil {
		_, _ = fmt.Fprintf(stderr, "vmquerybench: verification data not visible: %v\n", err)
		return 1
	}

	type check struct {
		Name  string  `json:"name"`
		Query string  `json:"query"`
		Want  float64 `json:"want"`
		Got   float64 `json:"got"`
		Error string  `json:"error,omitempty"`
		Pass  bool    `json:"pass"`
	}
	var checks []check
	add := func(name, query string, want float64, got float64, err error) {
		item := check{Name: name, Query: query, Want: want, Got: got, Pass: err == nil && math.Abs(got-want) <= 1e-6}
		if err != nil {
			item.Error = err.Error()
		}
		checks = append(checks, item)
	}
	instant := func(query string) (float64, error) {
		samples, err := client.Query(ctx, query, end)
		if err != nil {
			return 0, err
		}
		if len(samples) == 0 {
			return 0, fmt.Errorf("no result")
		}
		return samples[0].Value, nil
	}
	byLabel := func(query, label, value string) (float64, error) {
		samples, err := client.Query(ctx, query, end)
		if err != nil {
			return 0, err
		}
		for _, sample := range samples {
			if sample.Metric[label] == value {
				return sample.Value, nil
			}
		}
		return 0, fmt.Errorf("label %s=%s not found", label, value)
	}

	const rateAlphaQuery = `sum(rate(observaki_verify_requests_total{service="alpha"}[5m]))`
	got, err := instant(rateAlphaQuery)
	add("rate_alpha", rateAlphaQuery, 200.0/300.0, got, err)

	const rateByServiceQuery = `sum by (service) (rate(observaki_verify_requests_total[5m]))`
	got, err = byLabel(rateByServiceQuery, "service", "beta")
	add("rate_beta", rateByServiceQuery, 100.0/300.0, got, err)

	const increaseQuery = `sum(increase(observaki_verify_requests_total{service="alpha"}[5m]))`
	got, err = instant(increaseQuery)
	add("increase_alpha", increaseQuery, 200, got, err)

	const labelsQuery = `sum by (status) (observaki_verify_requests_total)`
	got, err = byLabel(labelsQuery, "status", "200")
	add("label_aggregation_200", labelsQuery, 200, got, err)
	got, err = byLabel(labelsQuery, "status", "500")
	add("label_aggregation_500", labelsQuery, 100, got, err)

	const quantileQuery = `histogram_quantile(0.95, sum by (le) (rate(observaki_verify_duration_seconds_bucket[5m])))`
	got, err = instant(quantileQuery)
	add("histogram_quantile_p95", quantileQuery, 0.475, got, err)

	const quantileMedianQuery = `histogram_quantile(0.50, sum by (le) (rate(observaki_verify_duration_seconds_bucket[5m])))`
	got, err = instant(quantileMedianQuery)
	add("histogram_quantile_p50", quantileMedianQuery, 0.25, got, err)

	status := "pass"
	for _, item := range checks {
		if !item.Pass {
			status = "fail"
		}
		_, _ = fmt.Fprintf(stderr, "vmquerybench: verify %s %s want=%.9f got=%.9f\n", item.Name, map[bool]string{true: "pass", false: "fail"}[item.Pass], item.Want, item.Got)
	}
	report := map[string]any{
		"status": status,
		"checks": checks,
	}
	if opts.out != "" {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "vmquerybench: encode verification: %v\n", err)
			return 1
		}
		if err := os.WriteFile(opts.out, append(encoded, '\n'), 0o644); err != nil {
			_, _ = fmt.Fprintf(stderr, "vmquerybench: write verification: %v\n", err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "vmquerybench: verify %s checks=%d\n", status, len(checks))
	if status == "pass" {
		return 0
	}
	return 1
}

func seedVerification(ctx context.Context, vmURL string, start, end time.Time) error {
	const step = 15 * time.Second
	samples := int(end.Sub(start)/step) + 1
	type seriesSpec struct {
		name   string
		labels []promrw.Label
		value  func(i int) float64
	}
	specs := []seriesSpec{
		{
			name:   "observaki_verify_requests_total",
			labels: []promrw.Label{{Name: "service", Value: "alpha"}, {Name: "status", Value: "200"}, {Name: "tenant_id", Value: "verify"}},
			value:  func(i int) float64 { return 10 * float64(i) },
		},
		{
			name:   "observaki_verify_requests_total",
			labels: []promrw.Label{{Name: "service", Value: "beta"}, {Name: "status", Value: "500"}, {Name: "tenant_id", Value: "verify"}},
			value:  func(i int) float64 { return 5 * float64(i) },
		},
		{
			name:   "observaki_verify_duration_seconds_bucket",
			labels: []promrw.Label{{Name: "le", Value: "0.5"}, {Name: "service", Value: "alpha"}, {Name: "tenant_id", Value: "verify"}},
			value:  func(i int) float64 { return 100 * float64(i) },
		},
		{
			name:   "observaki_verify_duration_seconds_bucket",
			labels: []promrw.Label{{Name: "le", Value: "+Inf"}, {Name: "service", Value: "alpha"}, {Name: "tenant_id", Value: "verify"}},
			value:  func(i int) float64 { return 100 * float64(i) },
		},
		{
			name:   "observaki_verify_duration_seconds_sum",
			labels: []promrw.Label{{Name: "service", Value: "alpha"}, {Name: "tenant_id", Value: "verify"}},
			value:  func(i int) float64 { return 50 * float64(i) },
		},
		{
			name:   "observaki_verify_duration_seconds_count",
			labels: []promrw.Label{{Name: "service", Value: "alpha"}, {Name: "tenant_id", Value: "verify"}},
			value:  func(i int) float64 { return 100 * float64(i) },
		},
	}

	req := &promrw.WriteRequest{}
	for _, spec := range specs {
		labels := append([]promrw.Label{{Name: "__name__", Value: spec.name}}, spec.labels...)
		promrw.SortLabels(labels)
		ts := promrw.TimeSeries{Labels: labels}
		for i := 0; i < samples; i++ {
			ts.Samples = append(ts.Samples, promrw.Sample{
				Value:     spec.value(i),
				Timestamp: start.Add(time.Duration(i) * step).UnixMilli(),
			})
		}
		req.Timeseries = append(req.Timeseries, ts)
	}
	body, err := promrw.EncodeCompressed(req)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, vmURL+"/api/v1/write", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Content-Encoding", "snappy")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		return fmt.Errorf("remote write returned %d: %s", response.StatusCode, string(payload))
	}
	return nil
}

func waitForVerification(ctx context.Context, client *vmbench.Client, at time.Time, want float64) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		samples, err := client.Query(ctx, "sum(observaki_verify_requests_total)", at)
		if err == nil && len(samples) > 0 && math.Abs(samples[0].Value-want) <= 1e-6 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("sum did not reach %v", want)
}
