// Command otlploadgen generates the deterministic OTLP workload used by the
// Observaki benchmark harness.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/observaki/observaki/bench/loadgen"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "otlploadgen: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg := loadgen.DefaultConfig()
	var (
		protocol     string
		compression  string
		mix          string
		startTime    string
		checkPayload bool
		dryRun       bool
		showVersion  bool
	)

	flags := flag.NewFlagSet("otlploadgen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprint(stderr, `otlploadgen — deterministic OTLP workload generator

Usage:
  otlploadgen [flags]

Examples:
  otlploadgen --endpoint 127.0.0.1:4317 --rate 1000000 --duration 5m
  otlploadgen --protocol http --endpoint http://127.0.0.1:4318 --rate 100000
  otlploadgen --dry-run --rate 100000 --duration 10s
  otlploadgen --check-payload

Flags:
`)
		flags.PrintDefaults()
	}

	flags.StringVar(&cfg.Endpoint, "endpoint", cfg.Endpoint, "OTLP endpoint (host:port for gRPC, URL for HTTP)")
	flags.StringVar(&protocol, "protocol", string(cfg.Protocol), "OTLP transport: grpc or http")
	flags.StringVar(&compression, "compression", string(cfg.Compression), "request compression: gzip or none")
	flags.IntVar(&cfg.Rate, "rate", cfg.Rate, "target events per second")
	flags.DurationVar(&cfg.Duration, "duration", cfg.Duration, "run duration (for example 5m)")
	flags.StringVar(&startTime, "start-time", "", "event start time in RFC3339 (default: now)")
	flags.Int64Var(&cfg.Seed, "seed", cfg.Seed, "deterministic seed")
	flags.IntVar(&cfg.BatchSize, "batch-size", cfg.BatchSize, "events per OTLP request")
	flags.IntVar(&cfg.Workers, "workers", cfg.Workers, "concurrent send workers")
	flags.StringVar(&cfg.MetricsAddr, "metrics-addr", cfg.MetricsAddr, "metrics listen address (empty disables /metrics)")
	flags.StringVar(&mix, "mix", "60,30,10", "signal mix in events per 100: metrics,logs,traces")
	flags.DurationVar(&cfg.RequestTimeout, "request-timeout", cfg.RequestTimeout, "per-request timeout")
	flags.DurationVar(&cfg.LogInterval, "log-interval", cfg.LogInterval, "progress log interval")
	flags.BoolVar(&checkPayload, "check-payload", false, "measure compressed payload sizes and exit")
	flags.BoolVar(&dryRun, "dry-run", false, "generate without sending")
	flags.BoolVar(&showVersion, "version", false, "print the load generator version and exit")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if showVersion {
		_, err := fmt.Fprintln(stdout, "otlploadgen 0.1.0 (phase 0)")
		return err
	}

	parsedProtocol, err := loadgen.ParseProtocol(protocol)
	if err != nil {
		return err
	}
	cfg.Protocol = parsedProtocol
	parsedCompression, err := loadgen.ParseCompression(compression)
	if err != nil {
		return err
	}
	cfg.Compression = parsedCompression
	parsedMix, err := parseMix(mix)
	if err != nil {
		return err
	}
	cfg.Mix = parsedMix
	cfg.DryRun = dryRun
	if startTime != "" {
		parsedStart, err := time.Parse(time.RFC3339, startTime)
		if err != nil {
			return fmt.Errorf("invalid --start-time %q: %w", startTime, err)
		}
		cfg.StartTime = parsedStart.UTC()
	}

	if checkPayload {
		return printPayloadReport(stdout, stderr, cfg)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	metrics := loadgen.NewRunMetrics(cfg.Rate)

	var metricsServer *http.Server
	if cfg.MetricsAddr != "" {
		listener, err := net.Listen("tcp", cfg.MetricsAddr)
		if err != nil {
			return fmt.Errorf("listen on metrics address %s: %w", cfg.MetricsAddr, err)
		}
		metricsServer = &http.Server{Handler: metrics.Handler(), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := metricsServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("metrics server stopped", "error", err)
			}
		}()
		logger.Info("metrics endpoint ready", "addr", cfg.MetricsAddr, "path", "/metrics")
	}

	sender, err := loadgen.NewSender(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = sender.Close() }()

	generator, err := loadgen.NewGenerator(cfg, sender, metrics)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	progressCtx, progressCancel := context.WithCancel(ctx)
	defer progressCancel()
	go logProgress(progressCtx, logger, metrics, cfg.LogInterval)

	logger.Info("starting workload",
		"endpoint", cfg.Endpoint,
		"protocol", cfg.Protocol,
		"compression", cfg.Compression,
		"rate", cfg.Rate,
		"duration", cfg.Duration.String(),
		"batch_size", cfg.BatchSize,
		"workers", cfg.Workers,
		"seed", cfg.Seed,
	)

	summary, runErr := generator.Run(ctx)
	progressCancel()
	if err := printSummary(stdout, summary); err != nil {
		return err
	}
	if metricsServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = metricsServer.Shutdown(shutdownCtx)
		cancel()
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
	}
	if summary.Metrics.Failed > 0 {
		return fmt.Errorf("%d event(s) failed to send", summary.Metrics.Failed)
	}
	if summary.Metrics.Rejected > 0 {
		return fmt.Errorf("%d event(s) rejected by the receiver", summary.Metrics.Rejected)
	}
	if summary.Metrics.Generated < summary.PlannedEvents && ctx.Err() == nil {
		return fmt.Errorf("generated %d of %d planned events", summary.Metrics.Generated, summary.PlannedEvents)
	}
	return nil
}

func parseMix(value string) (loadgen.Mix, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return loadgen.Mix{}, fmt.Errorf("mix must be metrics,logs,traces (for example 60,30,10)")
	}
	numbers := make([]int, 3)
	for i, part := range parts {
		number, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return loadgen.Mix{}, fmt.Errorf("invalid mix component %q: %w", part, err)
		}
		numbers[i] = number
	}
	mix := loadgen.Mix{Metrics: numbers[0], Logs: numbers[1], Traces: numbers[2]}
	if err := mix.Validate(); err != nil {
		return loadgen.Mix{}, err
	}
	return mix, nil
}

func printPayloadReport(stdout, stderr io.Writer, cfg loadgen.Config) error {
	reports, weighted, err := loadgen.MeasureSizes(cfg)
	if err != nil {
		return err
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "SIGNAL\tEVENTS\tPROTOBUF_B\tGZIP_B\tPROTOBUF_B/EVENT\tGZIP_B/EVENT\tRATIO")
	for _, report := range reports {
		ratio := 0.0
		if report.GzipBytesPerEvent > 0 {
			ratio = report.ProtobufBytesPerEvent / report.GzipBytesPerEvent
		}
		_, _ = fmt.Fprintf(table, "%s\t%d\t%d\t%d\t%.1f\t%.1f\t%.2fx\n",
			report.Signal, report.Events, report.ProtobufBytes, report.GzipBytes,
			report.ProtobufBytesPerEvent, report.GzipBytesPerEvent, ratio)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "weighted gzip: %.1f B/event (target %.1f, tolerance ±%.0f%%)\n",
		weighted, loadgen.TargetCompressedBytesPerEvent, loadgen.PayloadTolerance*100)
	if !loadgen.PayloadWithinTolerance(weighted) {
		return fmt.Errorf("weighted compressed size %.1f B/event is outside the ±%.0f%% tolerance",
			weighted, loadgen.PayloadTolerance*100)
	}
	_, _ = fmt.Fprintln(stderr, "payload check: ok")
	return nil
}

func printSummary(stdout io.Writer, summary loadgen.Summary) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

func logProgress(ctx context.Context, logger *slog.Logger, metrics *loadgen.RunMetrics, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snapshot := metrics.Snapshot()
			logger.Info("workload progress",
				"generated", snapshot.Generated,
				"sent", snapshot.Sent,
				"failed", snapshot.Failed,
				"rejected", snapshot.Rejected,
				"achieved_rate", int64(snapshot.AchievedRate),
				"target_rate", snapshot.TargetRate,
				"backpressure_events", snapshot.BackpressureEvents,
				"pacing_lag_events", snapshot.PacingLagEvents,
				"latency_p95_ms", snapshot.Latency.P95Ms,
			)
		}
	}
}
