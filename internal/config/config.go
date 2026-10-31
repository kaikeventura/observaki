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
)

// Environment variable names.
const (
	EnvHTTPAddr    = "OBSERVAKI_HTTP_ADDR"
	EnvLogLevel    = "OBSERVAKI_LOG_LEVEL"
	EnvEnvironment = "OBSERVAKI_ENV"
	EnvDatabaseURL = "OBSERVAKI_DATABASE_URL"
	EnvWebDir      = "OBSERVAKI_WEB_DIR"
)

// Defaults applied when a variable is unset or empty.
const (
	DefaultHTTPAddr    = ":8080"
	DefaultLogLevel    = LogLevelInfo
	DefaultEnvironment = EnvironmentDevelopment
	DefaultWebDir      = "web/build"
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

	return nil
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
