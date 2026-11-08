package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/observaki/observaki/internal/adapters/clickhouse/writer"
	natsadapter "github.com/observaki/observaki/internal/adapters/nats"
	"github.com/observaki/observaki/internal/adapters/postgres"
	"github.com/observaki/observaki/internal/adapters/storageprobe"
	"github.com/observaki/observaki/internal/adapters/victoriametrics"
	"github.com/observaki/observaki/internal/adapters/walconsumer"
	"github.com/observaki/observaki/internal/alerting"
	"github.com/observaki/observaki/internal/api"
	"github.com/observaki/observaki/internal/config"
	"github.com/observaki/observaki/internal/healthcheck"
	"github.com/observaki/observaki/internal/httpserver"
	"github.com/observaki/observaki/internal/ingest"
	"github.com/observaki/observaki/internal/selfobs"
	"github.com/observaki/observaki/internal/version"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "observaki: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		_ = usage(stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "version":
		_, err := fmt.Fprintln(stdout, version.String())
		return err
	case "serve":
		return serve()
	case "gateway":
		return gateway()
	case "writer":
		return writerCommand()
	case "migrate":
		return migrateCommand(args[1:], stdout)
	case "backup":
		return backupCommand(args[1:], stdout)
	case "restore":
		return restoreCommand(args[1:], stdout)
	case "healthcheck":
		return healthcheckCommand(args[1:], stdout)
	case "help", "-h", "--help":
		return usage(stdout)
	default:
		_ = usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel.SlogLevel(),
	}))
	logger.Info("starting observaki", "env", cfg.Environment, "web_dir", cfg.WebDir)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var database *postgres.DB
	if cfg.DatabaseURL != "" {
		// The API must start even when PostgreSQL is still coming up (Helm
		// creates the Deployment before the Service DNS is ready). Retry for a
		// bounded window, then run degraded instead of crash-looping.
		deadline := time.Now().Add(30 * time.Second)
		for {
			connectContext, cancel := context.WithTimeout(ctx, 5*time.Second)
			database, err = postgres.Open(connectContext, cfg.DatabaseURL)
			cancel()
			if err == nil {
				defer database.Close()
				break
			}
			if time.Now().After(deadline) {
				logger.Warn("control-plane database is unavailable; metadata features are degraded", "error", err)
				database = nil
				break
			}
			logger.Warn("waiting for the control-plane database", "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}

	var pinger selfobs.PostgresPinger
	if database != nil {
		pinger = database
	}

	metrics := httpserver.NewRequestMetrics()
	alertingSource := &alertingProxy{}
	collector := selfobs.New(selfobs.Config{
		WriterURL:          cfg.SelfWriterURL,
		GatewayURL:         cfg.SelfGatewayURL,
		NATSMonitorURL:     cfg.SelfNATSMonitorURL,
		IngestLagThreshold: cfg.SelfIngestLag,
		Refresh:            cfg.SelfRefresh,
		Timeout:            cfg.SelfTimeout,
	}, metrics, alertingSource, pinger)

	if cfg.AlertingEnabled {
		if database == nil {
			alertingSource.set(staticAlerting{snapshot: selfobs.Alerting{
				Status:  selfobs.StatusCritical,
				Enabled: true,
				Detail:  "alerting is enabled but the control-plane database is unavailable",
			}})
		} else {
			store := database.NewAlertingStore()
			notifier := alerting.NewNotifier(store, cfg.AlertingTimeout, logger)
			service := alerting.NewService(
				store,
				func(ctx context.Context) alerting.Metrics {
					return alertingMetrics(collector.Overview(ctx))
				},
				notifier,
				cfg.AlertingInterval,
				cfg.AlertingDeliveryInterval,
				logger,
			)
			alertingSource.set(service)
			go service.Run(ctx)
			logger.Info("internal alerting enabled",
				"interval", cfg.AlertingInterval,
				"delivery_interval", cfg.AlertingDeliveryInterval,
			)
		}
	}

	srv := httpserver.New(cfg.HTTPAddr, version.Version, logger,
		httpserver.WithWebDir(cfg.WebDir),
		httpserver.WithHealthOverview(collector),
		httpserver.WithRequestMetrics(metrics),
		httpserver.WithControlPlane(controlPlaneStore(database)),
	)

	return srv.Run(ctx)
}

// controlPlaneStore returns the API projection of the control plane, or nil
// when the database is unavailable. The metadata endpoints then answer 503
// while health and the OpenAPI contract keep working.
func controlPlaneStore(database *postgres.DB) api.ControlPlane {
	if database == nil {
		return nil
	}
	return database.NewAPIStore()
}

// alertingProxy lets the collector and the alerting service reference each
// other: the collector needs the dead-man snapshot, the evaluator needs the
// overview metrics.
type alertingProxy struct {
	mu     sync.RWMutex
	source selfobs.AlertingSource
}

func (p *alertingProxy) set(source selfobs.AlertingSource) {
	p.mu.Lock()
	p.source = source
	p.mu.Unlock()
}

func (p *alertingProxy) AlertingSnapshot() selfobs.Alerting {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.source == nil {
		return selfobs.Alerting{Status: selfobs.StatusDisabled, Enabled: false, Detail: "internal alerting is disabled"}
	}
	return p.source.AlertingSnapshot()
}

type staticAlerting struct {
	snapshot selfobs.Alerting
}

func (s staticAlerting) AlertingSnapshot() selfobs.Alerting { return s.snapshot }

// alertingMetrics maps the health overview into the minimal evaluator inputs.
func alertingMetrics(overview selfobs.Overview) alerting.Metrics {
	metrics := alerting.Metrics{
		Components: make(map[string]string, len(overview.Components)),
	}
	if overview.Ingestion.LagP95Seconds != nil {
		metrics.IngestLagP95Seconds = *overview.Ingestion.LagP95Seconds
	}
	if overview.Ingestion.PendingMessages != nil {
		metrics.IngestPendingMessages = float64(*overview.Ingestion.PendingMessages)
	}
	if overview.Ingestion.DLQMessages != nil {
		metrics.IngestDLQMessages = float64(*overview.Ingestion.DLQMessages)
	}
	if overview.Ingestion.WriterConnected != nil {
		metrics.WriterConnected = *overview.Ingestion.WriterConnected
	}
	if overview.Ingestion.GatewayConnected != nil {
		metrics.GatewayConnected = *overview.Ingestion.GatewayConnected
	}
	metrics.QueryErrorRate = overview.Query.ErrorRate
	for _, component := range overview.Components {
		metrics.Components[component.Name] = component.Status
	}
	return metrics
}

func gateway() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	if cfg.IngestAuthFile == "" {
		return fmt.Errorf(
			"otlp gateway: %s is required; create a config snapshot with hashed ingest tokens (see docs/dev/local-setup.md)",
			config.EnvIngestAuthFile,
		)
	}
	signedSnapshot := cfg.Environment == config.EnvironmentStaging || cfg.Environment == config.EnvironmentProduction
	if signedSnapshot && cfg.IngestAuthPublicKey == "" {
		return fmt.Errorf("otlp gateway: %s is required outside development/test", config.EnvIngestAuthPublicKey)
	}
	if signedSnapshot && cfg.IngestAuditFile == "" {
		return fmt.Errorf("otlp gateway: %s is required outside development/test", config.EnvIngestAuditFile)
	}

	source, err := ingest.NewFileSnapshotSource(cfg.IngestAuthFile, cfg.IngestAuthPublicKey)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel.SlogLevel(),
	}))

	walCtx, cancelWAL := context.WithTimeout(context.Background(), cfg.NATSConnectTimeout)
	wal, err := natsadapter.Open(walCtx, natsadapter.Config{
		URL:             cfg.NATSURL,
		CredentialsFile: cfg.NATSCredentialsFile,
		SubjectPrefix:   cfg.NATSSubjectPrefix,
		StreamPrefix:    cfg.NATSStreamPrefix,
		Shards:          cfg.NATSShards,
		StreamReplicas:  cfg.NATSStreamReplicas,
		StreamStorage:   cfg.NATSStreamStorage,
		StreamMaxAge:    cfg.NATSStreamMaxAge,
		StreamMaxBytes:  cfg.NATSStreamMaxBytes,
		BatchMaxEvents:  cfg.NATSBatchMaxEvents,
		BatchMaxBytes:   cfg.NATSBatchMaxBytes,
		BatchLinger:     cfg.NATSBatchLinger,
		ConnectTimeout:  cfg.NATSConnectTimeout,
		PublishTimeout:  cfg.NATSPublishTimeout,
		MaxPendingBytes: cfg.NATSMaxPendingBytes,
		PublishWorkers:  cfg.NATSPublishWorkers,
		Logger:          logger,
	})
	cancelWAL()
	if err != nil {
		return fmt.Errorf("otlp gateway: open NATS WAL: %w", err)
	}
	logger.Info("otlp gateway uses the NATS JetStream WAL", "url", cfg.NATSURL, "shards", cfg.NATSShards)

	var audit ingest.AuditSink = ingest.LogAudit{Logger: logger}
	if cfg.IngestAuditFile != "" {
		fileAudit, err := ingest.OpenFileAudit(cfg.IngestAuditFile)
		if err != nil {
			_ = wal.Close()
			return err
		}
		defer func() { _ = fileAudit.Close() }()
		audit = fileAudit
	}
	if cfg.IngestAuthPublicKey == "" {
		logger.Warn("otlp gateway auth snapshot is not signed; signature verification is required in staging/production")
	}

	gw, err := ingest.New(ingest.Config{
		GRPCAddr:        cfg.OTLPGRPCAddr,
		HTTPAddr:        cfg.OTLPHTTPAddr,
		MaxRequestBytes: cfg.IngestMaxRequestBytes,
		MaxEvents:       cfg.IngestMaxEvents,
		Redaction: ingest.RedactionConfig{
			Keys: cfg.RedactKeys,
			Mask: cfg.RedactMask,
		},
		Auth: &ingest.AuthConfig{
			Source:         source,
			Audit:          audit,
			Logger:         logger,
			ReloadInterval: cfg.IngestAuthReload,
		},
		Sink:   wal,
		Logger: logger,
	})
	if err != nil {
		_ = wal.Close()
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return gw.Run(ctx)
}

func writerCommand() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel.SlogLevel(),
	}))

	inserter, err := writer.NewHTTPInserter(writer.HTTPConfig{
		Endpoint:    cfg.ClickHouseURL,
		Database:    cfg.ClickHouseDatabase,
		User:        cfg.ClickHouseUser,
		Password:    cfg.ClickHousePassword,
		Compression: cfg.ClickHouseCompression,
		Timeout:     cfg.ClickHouseTimeout,
	})
	if err != nil {
		return fmt.Errorf("otlp writer: configure ClickHouse: %w", err)
	}
	vmClient, err := victoriametrics.Open(victoriametrics.Config{
		URL:                 cfg.VictoriaMetricsURL,
		Timeout:             cfg.VictoriaMetricsTimeout,
		MaxRetries:          cfg.VictoriaMetricsMaxRetries,
		RetryBackoff:        cfg.VictoriaMetricsRetryBackoff,
		MaxRetryBackoff:     cfg.WriterMaxRetryBackoff,
		MaxSeriesPerRequest: cfg.VictoriaMetricsMaxSeries,
		Logger:              logger,
	})
	if err != nil {
		return fmt.Errorf("otlp writer: configure VictoriaMetrics: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := walconsumer.NewStorage(ctx, walconsumer.StorageConfig{
		Inserter:      inserter,
		Metrics:       vmClient,
		SpoolDir:      cfg.WriterSpoolDir,
		DedupeWindow:  cfg.WriterDedupeWindow,
		RetentionDays: uint16(cfg.WriterRetentionDays),
		MaxRetries:    cfg.ClickHouseMaxRetries,
		RetryBackoff:  cfg.ClickHouseRetryBackoff,
		Logger:        logger,
	})
	if err != nil {
		vmClient.Close()
		return fmt.Errorf("otlp writer: open storage: %w", err)
	}

	consumer, err := walconsumer.Open(ctx, walconsumer.Config{
		URL:             cfg.NATSURL,
		CredentialsFile: cfg.NATSCredentialsFile,
		SubjectPrefix:   cfg.NATSSubjectPrefix,
		StreamPrefix:    cfg.NATSStreamPrefix,
		BatchMaxEvents:  cfg.WriterBatchMaxEvents,
		BatchLinger:     cfg.WriterBatchLinger,
		AckWait:         cfg.WriterAckWait,
		ConnectTimeout:  cfg.NATSConnectTimeout,
		RetryBackoff:    cfg.ClickHouseRetryBackoff,
		MaxRetryBackoff: cfg.WriterMaxRetryBackoff,
		Logger:          logger,
	}, store)
	if err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ClickHouseTimeout)
		_ = store.Close(shutdownCtx)
		cancel()
		return fmt.Errorf("otlp writer: open WAL consumer: %w", err)
	}

	server := walconsumer.NewServer(cfg.WriterAddr, consumer, logger,
		walconsumer.WithHandler("/storage", storageprobe.New(storageprobe.Config{
			ClickHouseURL:      cfg.ClickHouseURL,
			ClickHouseDatabase: cfg.ClickHouseDatabase,
			ClickHouseUser:     cfg.ClickHouseUser,
			ClickHousePassword: cfg.ClickHousePassword,
			VictoriaMetricsURL: cfg.VictoriaMetricsURL,
			Timeout:            cfg.SelfTimeout,
		}).Handler()),
	)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run(ctx)
	}()

	logger.Info("otlp writer running",
		"http_addr", cfg.WriterAddr,
		"spool_dir", cfg.WriterSpoolDir,
		"clickhouse", cfg.ClickHouseURL,
		"victoriametrics", cfg.VictoriaMetricsURL,
	)
	var runErr error
	select {
	case <-ctx.Done():
	case err := <-serverErr:
		runErr = err
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ClickHouseTimeout)
	defer cancel()
	if err := consumer.Close(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, err)
	}
	if err := store.Close(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, err)
	}
	return runErr
}

func migrateCommand(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	seed := flags.Bool("seed-dev", false, "insert the development metadata (development/test only)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, cfg, err := openControlPlane(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	applied, err := db.Migrate(ctx)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		_, _ = fmt.Fprintln(stdout, "schema is up to date")
	}
	for _, migration := range applied {
		_, _ = fmt.Fprintf(stdout, "applied %s (%s)\n", migration.Version, migration.Checksum[:12])
	}
	if *seed {
		if err := db.SeedDev(ctx, string(cfg.Environment)); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(stdout, "development metadata seeded")
	}
	schemaVersion, err := db.SchemaVersion(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "schema version: %s\n", schemaVersion)
	return err
}

func backupCommand(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", "-", "destination file (\"-\" writes JSON to stdout)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, _, err := openControlPlane(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	snapshot, err := db.Export(ctx, version.Version)
	if err != nil {
		return err
	}
	if *output == "-" {
		return snapshot.Encode(stdout)
	}
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("backup: open %s: %w", *output, err)
	}
	if err := snapshot.Encode(file); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("backup: close %s: %w", *output, err)
	}
	total := 0
	for _, count := range snapshot.Counts {
		total += count
	}
	_, err = fmt.Fprintf(stdout, "backup written to %s (%d rows, %d tables, schema %s)\n",
		*output, total, len(snapshot.Tables), snapshot.SchemaVersion)
	return err
}

func restoreCommand(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "-", "backup file (\"-\" reads JSON from stdin)")
	force := flags.Bool("force", false, "replace existing metadata")
	if err := flags.Parse(args); err != nil {
		return err
	}

	var data []byte
	var err error
	if *input == "-" {
		data, err = io.ReadAll(io.LimitReader(os.Stdin, 512<<20))
	} else {
		data, err = os.ReadFile(*input)
	}
	if err != nil {
		return fmt.Errorf("restore: read input: %w", err)
	}
	snapshot, err := postgres.DecodeSnapshot(data)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	db, _, err := openControlPlane(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	result, err := db.Restore(ctx, snapshot, postgres.RestoreOptions{Force: *force})
	if err != nil {
		return err
	}
	total := int64(0)
	for _, count := range result.Tables {
		total += count
	}
	_, err = fmt.Fprintf(stdout, "restore complete: %d rows in %.3fs (format %d)\n",
		total, result.Elapsed.Seconds(), result.FormatVersion)
	return err
}

func openControlPlane(ctx context.Context) (*postgres.DB, config.Config, error) {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return nil, config.Config{}, err
	}
	if cfg.DatabaseURL == "" {
		return nil, cfg, fmt.Errorf("%s is required for the control-plane commands", config.EnvDatabaseURL)
	}
	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, cfg, err
	}
	return db, cfg, nil
}

func usage(w io.Writer) error {
	_, err := fmt.Fprintf(w, `observaki — OTel-native observability platform

Usage:
  observaki <command>

Commands:
  version      print version, commit and build date
  serve        start the HTTP API and UI (env %s, default %s)
  gateway      start the OTLP gateway (env %s/%s, default %s/%s)
  writer       consume the NATS WAL and write ClickHouse/VictoriaMetrics (env %s, default %s)
  migrate      apply the PostgreSQL control-plane migrations (env %s)
  backup       export the control-plane metadata as versioned JSON (env %s)
  restore      restore the control-plane metadata from a backup (env %s)
  healthcheck  probe an HTTP health endpoint (used by container healthchecks)
  help         show this help
`,
		config.EnvHTTPAddr, config.DefaultHTTPAddr,
		config.EnvOTLPGRPCAddr, config.EnvOTLPHTTPAddr,
		config.DefaultOTLPGRPCAddr, config.DefaultOTLPHTTPAddr,
		config.EnvWriterAddr, config.DefaultWriterAddr,
		config.EnvDatabaseURL,
		config.EnvDatabaseURL,
		config.EnvDatabaseURL,
	)
	return err
}

// healthcheckCommand probes an HTTP health endpoint and exits non-zero when it
// is not healthy. It deliberately does not load the process configuration: it
// runs in a distroless container that may not have the data plane settings.
func healthcheckCommand(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	url := flags.String("url", healthcheck.DefaultURL, "health endpoint URL")
	timeout := flags.Duration("timeout", healthcheck.DefaultTimeout, "probe timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := healthcheck.Probe(ctx, healthcheck.DefaultClient, *url); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "ok: %s\n", *url)
	return err
}
