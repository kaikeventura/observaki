// Package config loads and validates the process configuration from the
// environment, with safe defaults for local development.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variable names.
const (
	EnvHTTPAddr              = "OBSERVAKI_HTTP_ADDR"
	EnvLogLevel              = "OBSERVAKI_LOG_LEVEL"
	EnvEnvironment           = "OBSERVAKI_ENV"
	EnvDatabaseURL           = "OBSERVAKI_DATABASE_URL"
	EnvWebDir                = "OBSERVAKI_WEB_DIR"
	EnvOTLPGRPCAddr          = "OBSERVAKI_OTLP_GRPC_ADDR"
	EnvOTLPHTTPAddr          = "OBSERVAKI_OTLP_HTTP_ADDR"
	EnvIngestMaxRequestBytes = "OBSERVAKI_INGEST_MAX_REQUEST_BYTES"
	EnvIngestMaxEvents       = "OBSERVAKI_INGEST_MAX_EVENTS"
	EnvIngestAuthFile        = "OBSERVAKI_INGEST_AUTH_FILE"
	EnvIngestAuthPublicKey   = "OBSERVAKI_INGEST_AUTH_PUBLIC_KEY"
	EnvIngestAuthReload      = "OBSERVAKI_INGEST_AUTH_RELOAD_INTERVAL"
	EnvIngestAuditFile       = "OBSERVAKI_INGEST_AUDIT_FILE"
	EnvRedactKeys            = "OBSERVAKI_REDACT_ATTRIBUTE_KEYS"
	EnvRedactMask            = "OBSERVAKI_REDACT_MASK"
	EnvNATSURL               = "OBSERVAKI_NATS_URL"
	EnvNATSCredentialsFile   = "OBSERVAKI_NATS_CREDENTIALS_FILE"
	EnvNATSSubjectPrefix     = "OBSERVAKI_NATS_SUBJECT_PREFIX"
	EnvNATSStreamPrefix      = "OBSERVAKI_NATS_STREAM_PREFIX"
	EnvNATSShards            = "OBSERVAKI_NATS_SHARDS"
	EnvNATSStreamReplicas    = "OBSERVAKI_NATS_STREAM_REPLICAS"
	EnvNATSStreamMaxAge      = "OBSERVAKI_NATS_STREAM_MAX_AGE"
	EnvNATSStreamMaxBytes    = "OBSERVAKI_NATS_STREAM_MAX_BYTES"
	EnvNATSStreamStorage     = "OBSERVAKI_NATS_STREAM_STORAGE"
	EnvNATSBatchMaxEvents    = "OBSERVAKI_NATS_BATCH_MAX_EVENTS"
	EnvNATSBatchMaxBytes     = "OBSERVAKI_NATS_BATCH_MAX_BYTES"
	EnvNATSBatchLinger       = "OBSERVAKI_NATS_BATCH_LINGER"
	EnvNATSConnectTimeout    = "OBSERVAKI_NATS_CONNECT_TIMEOUT"
	EnvNATSPublishTimeout    = "OBSERVAKI_NATS_PUBLISH_TIMEOUT"
	EnvNATSMaxPendingBytes   = "OBSERVAKI_NATS_MAX_PENDING_BYTES"
	EnvNATSPublishWorkers    = "OBSERVAKI_NATS_PUBLISH_WORKERS"

	EnvWriterAddr               = "OBSERVAKI_WRITER_ADDR"
	EnvWriterSpoolDir           = "OBSERVAKI_WRITER_SPOOL_DIR"
	EnvWriterBatchMaxEvents     = "OBSERVAKI_WRITER_BATCH_MAX_EVENTS"
	EnvWriterBatchLinger        = "OBSERVAKI_WRITER_BATCH_LINGER"
	EnvWriterDedupeWindow       = "OBSERVAKI_WRITER_DEDUPE_WINDOW"
	EnvWriterAckWait            = "OBSERVAKI_WRITER_ACK_WAIT"
	EnvWriterRetentionDays      = "OBSERVAKI_WRITER_RETENTION_DAYS"
	EnvWriterMaxRetryBackoff    = "OBSERVAKI_WRITER_MAX_RETRY_BACKOFF"
	EnvClickHouseURL            = "OBSERVAKI_CLICKHOUSE_URL"
	EnvClickHouseDatabase       = "OBSERVAKI_CLICKHOUSE_DATABASE"
	EnvClickHouseUser           = "OBSERVAKI_CLICKHOUSE_USER"
	EnvClickHousePassword       = "OBSERVAKI_CLICKHOUSE_PASSWORD"
	EnvClickHouseTimeout        = "OBSERVAKI_CLICKHOUSE_TIMEOUT"
	EnvClickHouseCompression    = "OBSERVAKI_CLICKHOUSE_COMPRESSION"
	EnvClickHouseMaxRetries     = "OBSERVAKI_CLICKHOUSE_MAX_RETRIES"
	EnvClickHouseRetryBackoff   = "OBSERVAKI_CLICKHOUSE_RETRY_BACKOFF"
	EnvVictoriaMetricsURL       = "OBSERVAKI_VICTORIAMETRICS_URL"
	EnvVictoriaMetricsTimeout   = "OBSERVAKI_VICTORIAMETRICS_TIMEOUT"
	EnvVictoriaMetricsRetries   = "OBSERVAKI_VICTORIAMETRICS_MAX_RETRIES"
	EnvVictoriaMetricsBackoff   = "OBSERVAKI_VICTORIAMETRICS_RETRY_BACKOFF"
	EnvVictoriaMetricsMaxSeries = "OBSERVAKI_VICTORIAMETRICS_MAX_SERIES_PER_REQUEST"
)

// Defaults applied when a variable is unset or empty.
const (
	DefaultHTTPAddr    = ":8080"
	DefaultLogLevel    = LogLevelInfo
	DefaultEnvironment = EnvironmentDevelopment
	DefaultWebDir      = "web/build"

	DefaultOTLPGRPCAddr = ":4317"
	DefaultOTLPHTTPAddr = ":4318"

	// DefaultIngestMaxRequestBytes is the OTLP request limit after
	// decompression (10 MiB).
	DefaultIngestMaxRequestBytes int64 = 10 << 20
	// DefaultIngestMaxEvents is the OTLP batch event limit.
	DefaultIngestMaxEvents = 10_000
	// DefaultIngestAuthReload is how often the gateway re-reads the auth
	// snapshot.
	DefaultIngestAuthReload = 30 * time.Second

	DefaultRedactMask = "[REDACTED]"
	// DefaultRedactKeys masks credit-card fields in attributes.
	DefaultRedactKeys = "credit_card,card_number,card.number,cc_number,pan,cvv,cvc"

	DefaultNATSURL            = "nats://127.0.0.1:4222"
	DefaultNATSSubjectPrefix  = "observaki.ingest.v1"
	DefaultNATSStreamPrefix   = "OBSERVAKI"
	DefaultNATSShards         = 32
	DefaultNATSStreamReplicas = 1
	// DefaultNATSStreamMaxAge covers a declared outage window plus repair.
	DefaultNATSStreamMaxAge    = 24 * time.Hour
	DefaultNATSStreamMaxBytes  = int64(32) << 30
	DefaultNATSStreamStorage   = "file"
	DefaultNATSBatchMaxEvents  = 100_000
	DefaultNATSBatchMaxBytes   = 4 << 20
	DefaultNATSBatchLinger     = 5 * time.Millisecond
	DefaultNATSConnectTimeout  = 10 * time.Second
	DefaultNATSPublishTimeout  = 5 * time.Second
	DefaultNATSMaxPendingBytes = int64(256) << 20
	DefaultNATSPublishWorkers  = 8

	DefaultWriterAddr            = ":8082"
	DefaultWriterSpoolDir        = "var/writer-spool"
	DefaultWriterBatchMaxEvents  = 50_000
	DefaultWriterBatchLinger     = time.Second
	DefaultWriterDedupeWindow    = 500_000
	DefaultWriterAckWait         = 30 * time.Minute
	DefaultWriterRetentionDays   = 15
	DefaultWriterMaxRetryBackoff = 30 * time.Second

	DefaultClickHouseURL          = "http://127.0.0.1:8123"
	DefaultClickHouseDatabase     = "observaki"
	DefaultClickHouseTimeout      = 2 * time.Minute
	DefaultClickHouseCompression  = "none"
	DefaultClickHouseMaxRetries   = 0
	DefaultClickHouseRetryBackoff = 500 * time.Millisecond

	DefaultVictoriaMetricsURL       = "http://127.0.0.1:8428"
	DefaultVictoriaMetricsTimeout   = 30 * time.Second
	DefaultVictoriaMetricsRetries   = 0
	DefaultVictoriaMetricsBackoff   = 500 * time.Millisecond
	DefaultVictoriaMetricsMaxSeries = 5_000
)

// LogLevel is a validated logging level.
type LogLevel string

// Supported log levels.
const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// SlogLevel maps the configured level to the standard library logger.
func (l LogLevel) SlogLevel() slog.Level {
	switch l {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelWarn:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Environment is a validated runtime environment.
type Environment string

// Supported runtime environments.
const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentTest        Environment = "test"
	EnvironmentStaging     Environment = "staging"
	EnvironmentProduction  Environment = "production"
)

// Config is the validated process configuration.
type Config struct {
	HTTPAddr    string
	LogLevel    LogLevel
	Environment Environment
	DatabaseURL string
	WebDir      string

	OTLPGRPCAddr          string
	OTLPHTTPAddr          string
	IngestMaxRequestBytes int64
	IngestMaxEvents       int
	IngestAuthFile        string
	IngestAuthPublicKey   string
	IngestAuthReload      time.Duration
	IngestAuditFile       string
	RedactKeys            []string
	RedactMask            string

	NATSURL             string
	NATSCredentialsFile string
	NATSSubjectPrefix   string
	NATSStreamPrefix    string
	NATSShards          int
	NATSStreamReplicas  int
	NATSStreamMaxAge    time.Duration
	NATSStreamMaxBytes  int64
	NATSStreamStorage   string
	NATSBatchMaxEvents  int
	NATSBatchMaxBytes   int
	NATSBatchLinger     time.Duration
	NATSConnectTimeout  time.Duration
	NATSPublishTimeout  time.Duration
	NATSMaxPendingBytes int64
	NATSPublishWorkers  int

	WriterAddr            string
	WriterSpoolDir        string
	WriterBatchMaxEvents  int
	WriterBatchLinger     time.Duration
	WriterDedupeWindow    int
	WriterAckWait         time.Duration
	WriterRetentionDays   int
	WriterMaxRetryBackoff time.Duration

	ClickHouseURL          string
	ClickHouseDatabase     string
	ClickHouseUser         string
	ClickHousePassword     string
	ClickHouseTimeout      time.Duration
	ClickHouseCompression  string
	ClickHouseMaxRetries   int
	ClickHouseRetryBackoff time.Duration

	VictoriaMetricsURL          string
	VictoriaMetricsTimeout      time.Duration
	VictoriaMetricsMaxRetries   int
	VictoriaMetricsRetryBackoff time.Duration
	VictoriaMetricsMaxSeries    int
}

// Lookup returns the value of an environment variable (os.Getenv compatible).
type Lookup func(string) string

// LoadFromEnv loads the configuration from the process environment.
func LoadFromEnv() (Config, error) {
	return Load(os.Getenv)
}

// Load builds a configuration from lookup and validates it. Empty values fall
// back to the documented defaults; invalid values return an error naming the
// variable and the rejected value.
func Load(lookup Lookup) (Config, error) {
	if lookup == nil {
		lookup = func(string) string { return "" }
	}

	cfg := Config{
		HTTPAddr:    valueOr(lookup(EnvHTTPAddr), DefaultHTTPAddr),
		LogLevel:    LogLevel(strings.ToLower(valueOr(lookup(EnvLogLevel), string(DefaultLogLevel)))),
		Environment: Environment(strings.ToLower(valueOr(lookup(EnvEnvironment), string(DefaultEnvironment)))),
		DatabaseURL: strings.TrimSpace(lookup(EnvDatabaseURL)),
		WebDir:      strings.TrimSpace(valueOr(lookup(EnvWebDir), DefaultWebDir)),

		OTLPGRPCAddr:        valueOr(lookup(EnvOTLPGRPCAddr), DefaultOTLPGRPCAddr),
		OTLPHTTPAddr:        valueOr(lookup(EnvOTLPHTTPAddr), DefaultOTLPHTTPAddr),
		IngestAuthFile:      strings.TrimSpace(lookup(EnvIngestAuthFile)),
		IngestAuthPublicKey: strings.TrimSpace(lookup(EnvIngestAuthPublicKey)),
		IngestAuditFile:     strings.TrimSpace(lookup(EnvIngestAuditFile)),
		RedactMask:          valueOr(lookup(EnvRedactMask), DefaultRedactMask),

		NATSURL:             valueOr(lookup(EnvNATSURL), DefaultNATSURL),
		NATSCredentialsFile: strings.TrimSpace(lookup(EnvNATSCredentialsFile)),
		NATSSubjectPrefix:   valueOr(lookup(EnvNATSSubjectPrefix), DefaultNATSSubjectPrefix),
		NATSStreamPrefix:    valueOr(lookup(EnvNATSStreamPrefix), DefaultNATSStreamPrefix),
		NATSStreamStorage:   strings.ToLower(valueOr(lookup(EnvNATSStreamStorage), DefaultNATSStreamStorage)),

		WriterAddr:            valueOr(lookup(EnvWriterAddr), DefaultWriterAddr),
		WriterSpoolDir:        valueOr(lookup(EnvWriterSpoolDir), DefaultWriterSpoolDir),
		ClickHouseURL:         valueOr(lookup(EnvClickHouseURL), DefaultClickHouseURL),
		ClickHouseDatabase:    valueOr(lookup(EnvClickHouseDatabase), DefaultClickHouseDatabase),
		ClickHouseUser:        strings.TrimSpace(lookup(EnvClickHouseUser)),
		ClickHousePassword:    lookup(EnvClickHousePassword),
		ClickHouseCompression: strings.ToLower(valueOr(lookup(EnvClickHouseCompression), DefaultClickHouseCompression)),
		VictoriaMetricsURL:    valueOr(lookup(EnvVictoriaMetricsURL), DefaultVictoriaMetricsURL),
	}

	var err error
	cfg.IngestMaxRequestBytes, err = int64Value(lookup, EnvIngestMaxRequestBytes, DefaultIngestMaxRequestBytes)
	if err != nil {
		return Config{}, err
	}
	cfg.IngestMaxEvents, err = intValue(lookup, EnvIngestMaxEvents, DefaultIngestMaxEvents)
	if err != nil {
		return Config{}, err
	}
	cfg.IngestAuthReload, err = durationValue(lookup, EnvIngestAuthReload, DefaultIngestAuthReload)
	if err != nil {
		return Config{}, err
	}
	cfg.RedactKeys, err = redactKeysValue(lookup)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSShards, err = intValue(lookup, EnvNATSShards, DefaultNATSShards)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSStreamReplicas, err = intValue(lookup, EnvNATSStreamReplicas, DefaultNATSStreamReplicas)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSStreamMaxAge, err = durationValue(lookup, EnvNATSStreamMaxAge, DefaultNATSStreamMaxAge)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSStreamMaxBytes, err = int64Value(lookup, EnvNATSStreamMaxBytes, DefaultNATSStreamMaxBytes)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSBatchMaxEvents, err = intValue(lookup, EnvNATSBatchMaxEvents, DefaultNATSBatchMaxEvents)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSBatchMaxBytes, err = intValue(lookup, EnvNATSBatchMaxBytes, DefaultNATSBatchMaxBytes)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSBatchLinger, err = durationValue(lookup, EnvNATSBatchLinger, DefaultNATSBatchLinger)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSConnectTimeout, err = durationValue(lookup, EnvNATSConnectTimeout, DefaultNATSConnectTimeout)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSPublishTimeout, err = durationValue(lookup, EnvNATSPublishTimeout, DefaultNATSPublishTimeout)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSMaxPendingBytes, err = int64Value(lookup, EnvNATSMaxPendingBytes, DefaultNATSMaxPendingBytes)
	if err != nil {
		return Config{}, err
	}
	cfg.NATSPublishWorkers, err = intValue(lookup, EnvNATSPublishWorkers, DefaultNATSPublishWorkers)
	if err != nil {
		return Config{}, err
	}
	cfg.WriterBatchMaxEvents, err = intValue(lookup, EnvWriterBatchMaxEvents, DefaultWriterBatchMaxEvents)
	if err != nil {
		return Config{}, err
	}
	cfg.WriterBatchLinger, err = durationValue(lookup, EnvWriterBatchLinger, DefaultWriterBatchLinger)
	if err != nil {
		return Config{}, err
	}
	cfg.WriterDedupeWindow, err = intValue(lookup, EnvWriterDedupeWindow, DefaultWriterDedupeWindow)
	if err != nil {
		return Config{}, err
	}
	cfg.WriterAckWait, err = durationValue(lookup, EnvWriterAckWait, DefaultWriterAckWait)
	if err != nil {
		return Config{}, err
	}
	cfg.WriterRetentionDays, err = intValue(lookup, EnvWriterRetentionDays, DefaultWriterRetentionDays)
	if err != nil {
		return Config{}, err
	}
	cfg.WriterMaxRetryBackoff, err = durationValue(lookup, EnvWriterMaxRetryBackoff, DefaultWriterMaxRetryBackoff)
	if err != nil {
		return Config{}, err
	}
	cfg.ClickHouseTimeout, err = durationValue(lookup, EnvClickHouseTimeout, DefaultClickHouseTimeout)
	if err != nil {
		return Config{}, err
	}
	cfg.ClickHouseMaxRetries, err = intValue(lookup, EnvClickHouseMaxRetries, DefaultClickHouseMaxRetries)
	if err != nil {
		return Config{}, err
	}
	cfg.ClickHouseRetryBackoff, err = durationValue(lookup, EnvClickHouseRetryBackoff, DefaultClickHouseRetryBackoff)
	if err != nil {
		return Config{}, err
	}
	cfg.VictoriaMetricsTimeout, err = durationValue(lookup, EnvVictoriaMetricsTimeout, DefaultVictoriaMetricsTimeout)
	if err != nil {
		return Config{}, err
	}
	cfg.VictoriaMetricsMaxRetries, err = intValue(lookup, EnvVictoriaMetricsRetries, DefaultVictoriaMetricsRetries)
	if err != nil {
		return Config{}, err
	}
	cfg.VictoriaMetricsRetryBackoff, err = durationValue(lookup, EnvVictoriaMetricsBackoff, DefaultVictoriaMetricsBackoff)
	if err != nil {
		return Config{}, err
	}
	cfg.VictoriaMetricsMaxSeries, err = intValue(lookup, EnvVictoriaMetricsMaxSeries, DefaultVictoriaMetricsMaxSeries)
	if err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks the configuration and reports every problem found.
func (c Config) Validate() error {
	if err := validateAddr(c.HTTPAddr); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvHTTPAddr, c.HTTPAddr, err)
	}

	switch c.LogLevel {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
	default:
		return fmt.Errorf("invalid %s %q: must be one of debug, info, warn, error", EnvLogLevel, c.LogLevel)
	}

	switch c.Environment {
	case EnvironmentDevelopment, EnvironmentTest, EnvironmentStaging, EnvironmentProduction:
	default:
		return fmt.Errorf(
			"invalid %s %q: must be one of development, test, staging, production",
			EnvEnvironment,
			c.Environment,
		)
	}

	if c.DatabaseURL != "" {
		if err := validateDatabaseURL(c.DatabaseURL); err != nil {
			return fmt.Errorf("invalid %s %q: %w", EnvDatabaseURL, c.DatabaseURL, err)
		}
	}

	if strings.TrimSpace(c.WebDir) == "" {
		return fmt.Errorf("invalid %s: must not be empty", EnvWebDir)
	}

	if err := validateAddr(c.OTLPGRPCAddr); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvOTLPGRPCAddr, c.OTLPGRPCAddr, err)
	}
	if err := validateAddr(c.OTLPHTTPAddr); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvOTLPHTTPAddr, c.OTLPHTTPAddr, err)
	}
	if c.OTLPGRPCAddr == c.OTLPHTTPAddr {
		return fmt.Errorf("%s and %s must differ, both are %q", EnvOTLPGRPCAddr, EnvOTLPHTTPAddr, c.OTLPGRPCAddr)
	}

	if c.IngestMaxRequestBytes < 1024 || c.IngestMaxRequestBytes > 1<<30 {
		return fmt.Errorf("invalid %s %d: must be between 1024 and 1073741824 bytes", EnvIngestMaxRequestBytes, c.IngestMaxRequestBytes)
	}
	if c.IngestMaxEvents < 1 || c.IngestMaxEvents > 10_000_000 {
		return fmt.Errorf("invalid %s %d: must be between 1 and 10000000", EnvIngestMaxEvents, c.IngestMaxEvents)
	}
	if c.IngestAuthReload < time.Second || c.IngestAuthReload > time.Hour {
		return fmt.Errorf("invalid %s %s: must be between 1s and 1h", EnvIngestAuthReload, c.IngestAuthReload)
	}
	if strings.TrimSpace(c.RedactMask) == "" {
		return fmt.Errorf("invalid %s: must not be empty", EnvRedactMask)
	}
	if len(c.RedactKeys) == 0 {
		return fmt.Errorf("invalid %s: must contain at least one key", EnvRedactKeys)
	}

	if err := validateNATSURL(c.NATSURL); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvNATSURL, c.NATSURL, err)
	}
	if err := validateSubjectPrefix(c.NATSSubjectPrefix); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvNATSSubjectPrefix, c.NATSSubjectPrefix, err)
	}
	if err := validateStreamPrefix(c.NATSStreamPrefix); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvNATSStreamPrefix, c.NATSStreamPrefix, err)
	}
	if c.NATSShards < 1 || c.NATSShards > 1024 {
		return fmt.Errorf("invalid %s %d: must be between 1 and 1024", EnvNATSShards, c.NATSShards)
	}
	if c.NATSStreamReplicas < 1 || c.NATSStreamReplicas > 5 {
		return fmt.Errorf("invalid %s %d: must be between 1 and 5", EnvNATSStreamReplicas, c.NATSStreamReplicas)
	}
	if c.NATSStreamMaxAge < time.Minute || c.NATSStreamMaxAge > 30*24*time.Hour {
		return fmt.Errorf("invalid %s %s: must be between 1m and 720h", EnvNATSStreamMaxAge, c.NATSStreamMaxAge)
	}
	if c.NATSStreamMaxBytes < 1<<20 || c.NATSStreamMaxBytes > 1<<40 {
		return fmt.Errorf("invalid %s %d: must be between 1048576 and 1099511627776 bytes", EnvNATSStreamMaxBytes, c.NATSStreamMaxBytes)
	}
	switch c.NATSStreamStorage {
	case "file", "memory":
	default:
		return fmt.Errorf("invalid %s %q: must be file or memory", EnvNATSStreamStorage, c.NATSStreamStorage)
	}
	if c.NATSBatchMaxEvents < 1 || c.NATSBatchMaxEvents > 1_000_000 {
		return fmt.Errorf("invalid %s %d: must be between 1 and 1000000", EnvNATSBatchMaxEvents, c.NATSBatchMaxEvents)
	}
	if c.NATSBatchMaxBytes < 1024 || c.NATSBatchMaxBytes > 16<<20 {
		return fmt.Errorf("invalid %s %d: must be between 1024 and 16777216 bytes", EnvNATSBatchMaxBytes, c.NATSBatchMaxBytes)
	}
	if c.NATSBatchLinger < time.Millisecond || c.NATSBatchLinger > 5*time.Second {
		return fmt.Errorf("invalid %s %s: must be between 1ms and 5s", EnvNATSBatchLinger, c.NATSBatchLinger)
	}
	if c.NATSConnectTimeout < 500*time.Millisecond || c.NATSConnectTimeout > time.Minute {
		return fmt.Errorf("invalid %s %s: must be between 500ms and 1m", EnvNATSConnectTimeout, c.NATSConnectTimeout)
	}
	if c.NATSPublishTimeout < 100*time.Millisecond || c.NATSPublishTimeout > time.Minute {
		return fmt.Errorf("invalid %s %s: must be between 100ms and 1m", EnvNATSPublishTimeout, c.NATSPublishTimeout)
	}
	if c.NATSMaxPendingBytes < 1<<20 || c.NATSMaxPendingBytes > 8<<30 {
		return fmt.Errorf("invalid %s %d: must be between 1048576 and 8589934592 bytes", EnvNATSMaxPendingBytes, c.NATSMaxPendingBytes)
	}
	if c.NATSPublishWorkers < 1 || c.NATSPublishWorkers > 64 {
		return fmt.Errorf("invalid %s %d: must be between 1 and 64", EnvNATSPublishWorkers, c.NATSPublishWorkers)
	}

	if err := validateAddr(c.WriterAddr); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvWriterAddr, c.WriterAddr, err)
	}
	if strings.TrimSpace(c.WriterSpoolDir) == "" {
		return fmt.Errorf("invalid %s: must not be empty", EnvWriterSpoolDir)
	}
	if c.WriterBatchMaxEvents < 1 || c.WriterBatchMaxEvents > 1_000_000 {
		return fmt.Errorf("invalid %s %d: must be between 1 and 1000000", EnvWriterBatchMaxEvents, c.WriterBatchMaxEvents)
	}
	if c.WriterBatchLinger < 10*time.Millisecond || c.WriterBatchLinger > 30*time.Second {
		return fmt.Errorf("invalid %s %s: must be between 10ms and 30s", EnvWriterBatchLinger, c.WriterBatchLinger)
	}
	if c.WriterDedupeWindow < 0 || c.WriterDedupeWindow > 10_000_000 {
		return fmt.Errorf("invalid %s %d: must be between 0 and 10000000", EnvWriterDedupeWindow, c.WriterDedupeWindow)
	}
	if c.WriterAckWait < time.Second || c.WriterAckWait > 24*time.Hour {
		return fmt.Errorf("invalid %s %s: must be between 1s and 24h", EnvWriterAckWait, c.WriterAckWait)
	}
	if c.WriterRetentionDays < 1 || c.WriterRetentionDays > 3650 {
		return fmt.Errorf("invalid %s %d: must be between 1 and 3650", EnvWriterRetentionDays, c.WriterRetentionDays)
	}
	if c.WriterMaxRetryBackoff < 100*time.Millisecond || c.WriterMaxRetryBackoff > 10*time.Minute {
		return fmt.Errorf("invalid %s %s: must be between 100ms and 10m", EnvWriterMaxRetryBackoff, c.WriterMaxRetryBackoff)
	}

	if err := validateHTTPURL(c.ClickHouseURL); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvClickHouseURL, c.ClickHouseURL, err)
	}
	if err := validateIdentifier(c.ClickHouseDatabase); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvClickHouseDatabase, c.ClickHouseDatabase, err)
	}
	if c.ClickHouseTimeout < time.Second || c.ClickHouseTimeout > 30*time.Minute {
		return fmt.Errorf("invalid %s %s: must be between 1s and 30m", EnvClickHouseTimeout, c.ClickHouseTimeout)
	}
	switch c.ClickHouseCompression {
	case "none", "gzip":
	default:
		return fmt.Errorf("invalid %s %q: must be none or gzip", EnvClickHouseCompression, c.ClickHouseCompression)
	}
	if c.ClickHouseMaxRetries < 0 || c.ClickHouseMaxRetries > 1000 {
		return fmt.Errorf("invalid %s %d: must be between 0 and 1000", EnvClickHouseMaxRetries, c.ClickHouseMaxRetries)
	}
	if c.ClickHouseRetryBackoff < 10*time.Millisecond || c.ClickHouseRetryBackoff > time.Minute {
		return fmt.Errorf("invalid %s %s: must be between 10ms and 1m", EnvClickHouseRetryBackoff, c.ClickHouseRetryBackoff)
	}

	if err := validateHTTPURL(c.VictoriaMetricsURL); err != nil {
		return fmt.Errorf("invalid %s %q: %w", EnvVictoriaMetricsURL, c.VictoriaMetricsURL, err)
	}
	if c.VictoriaMetricsTimeout < time.Second || c.VictoriaMetricsTimeout > 30*time.Minute {
		return fmt.Errorf("invalid %s %s: must be between 1s and 30m", EnvVictoriaMetricsTimeout, c.VictoriaMetricsTimeout)
	}
	if c.VictoriaMetricsMaxRetries < 0 || c.VictoriaMetricsMaxRetries > 1000 {
		return fmt.Errorf("invalid %s %d: must be between 0 and 1000", EnvVictoriaMetricsRetries, c.VictoriaMetricsMaxRetries)
	}
	if c.VictoriaMetricsRetryBackoff < 10*time.Millisecond || c.VictoriaMetricsRetryBackoff > time.Minute {
		return fmt.Errorf("invalid %s %s: must be between 10ms and 1m", EnvVictoriaMetricsBackoff, c.VictoriaMetricsRetryBackoff)
	}
	if c.VictoriaMetricsMaxSeries < 1 || c.VictoriaMetricsMaxSeries > 1_000_000 {
		return fmt.Errorf("invalid %s %d: must be between 1 and 1000000", EnvVictoriaMetricsMaxSeries, c.VictoriaMetricsMaxSeries)
	}

	return nil
}

func validateNATSURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	switch parsed.Scheme {
	case "nats", "tls", "ws", "wss":
	default:
		return fmt.Errorf("scheme %q is not nats, tls, ws or wss", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("host is missing")
	}
	return nil
}

func validateSubjectPrefix(value string) error {
	tokens := strings.Split(value, ".")
	if len(tokens) == 0 {
		return errors.New("must not be empty")
	}
	for _, token := range tokens {
		if token == "" {
			return errors.New("must not contain empty tokens")
		}
		for _, r := range token {
			if !isNATSSafeRune(r) {
				return fmt.Errorf("token %q contains %q; use letters, digits, _ or -", token, r)
			}
		}
	}
	return nil
}

func validateStreamPrefix(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("must not be empty")
	}
	for _, r := range value {
		if !isNATSSafeRune(r) {
			return fmt.Errorf("contains %q; use letters, digits, _ or -", r)
		}
	}
	return nil
}

func isNATSSafeRune(r rune) bool {
	return r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == '_' || r == '-'
}

func int64Value(lookup Lookup, key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(lookup(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, raw, err)
	}
	return value, nil
}

func intValue(lookup Lookup, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(lookup(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, raw, err)
	}
	return value, nil
}

func durationValue(lookup Lookup, key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(lookup(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, raw, err)
	}
	return value, nil
}

func redactKeysValue(lookup Lookup) ([]string, error) {
	raw := strings.TrimSpace(lookup(EnvRedactKeys))
	if raw == "" {
		raw = DefaultRedactKeys
	}
	parts := strings.Split(raw, ",")
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		if key := strings.TrimSpace(part); key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("invalid %s %q: must contain at least one key", EnvRedactKeys, raw)
	}
	return keys, nil
}

func validateAddr(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("port %q is not a number", port)
	}
	if number < 1 || number > 65535 {
		return fmt.Errorf("port %d is out of range 1-65535", number)
	}
	return nil
}

func validateDatabaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("scheme %q is not postgres or postgresql", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("host is missing")
	}
	return nil
}

func validateHTTPURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("scheme %q is not http or https", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("host is missing")
	}
	return nil
}

func validateIdentifier(value string) error {
	if value == "" {
		return errors.New("must not be empty")
	}
	for index, r := range value {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || index > 0 && r >= '0' && r <= '9' {
			continue
		}
		return fmt.Errorf("contains %q; use letters, digits or _", r)
	}
	return nil
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
