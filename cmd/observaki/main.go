package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/observaki/observaki/internal/adapters/clickhouse/writer"
	natsadapter "github.com/observaki/observaki/internal/adapters/nats"
	"github.com/observaki/observaki/internal/adapters/victoriametrics"
	"github.com/observaki/observaki/internal/adapters/walconsumer"
	"github.com/observaki/observaki/internal/config"
	"github.com/observaki/observaki/internal/httpserver"
	"github.com/observaki/observaki/internal/ingest"
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

	srv := httpserver.New(cfg.HTTPAddr, version.Version, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx)
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

	server := walconsumer.NewServer(cfg.WriterAddr, consumer, logger)
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

func usage(w io.Writer) error {
	_, err := fmt.Fprintf(w, `observaki — OTel-native observability platform

Usage:
  observaki <command>

Commands:
  version   print version, commit and build date
  serve     start the HTTP API (env %s, default %s)
  gateway   start the OTLP gateway (env %s/%s, default %s/%s)
  writer    consume the NATS WAL and write ClickHouse/VictoriaMetrics (env %s, default %s)
  help      show this help
`,
		config.EnvHTTPAddr, config.DefaultHTTPAddr,
		config.EnvOTLPGRPCAddr, config.EnvOTLPHTTPAddr,
		config.DefaultOTLPGRPCAddr, config.DefaultOTLPHTTPAddr,
		config.EnvWriterAddr, config.DefaultWriterAddr,
	)
	return err
}
