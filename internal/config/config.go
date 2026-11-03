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

	return nil
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

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
