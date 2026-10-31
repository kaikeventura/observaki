package config

import (
	"strings"
	"testing"
)

func lookup(values map[string]string) Lookup {
	return func(key string) string { return values[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(lookup(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != DefaultHTTPAddr {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, DefaultHTTPAddr)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.Environment != DefaultEnvironment {
		t.Errorf("Environment = %q, want %q", cfg.Environment, DefaultEnvironment)
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
	if cfg.WebDir != DefaultWebDir {
		t.Errorf("WebDir = %q, want %q", cfg.WebDir, DefaultWebDir)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(lookup(map[string]string{
		EnvHTTPAddr:    "127.0.0.1:9000",
		EnvLogLevel:    "DEBUG",
		EnvEnvironment: "Staging",
		EnvDatabaseURL: "postgres://db.internal:5432/observaki",
		EnvWebDir:      "/srv/observaki/web",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != "127.0.0.1:9000" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.LogLevel != LogLevelDebug {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.Environment != EnvironmentStaging {
		t.Errorf("Environment = %q, want staging", cfg.Environment)
	}
	if cfg.DatabaseURL != "postgres://db.internal:5432/observaki" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.WebDir != "/srv/observaki/web" {
		t.Errorf("WebDir = %q", cfg.WebDir)
	}
}

func TestLoadEmptyValuesFallBackToDefaults(t *testing.T) {
	cfg, err := Load(lookup(map[string]string{
		EnvHTTPAddr: "   ",
		EnvLogLevel: "",
		EnvWebDir:   "",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != DefaultHTTPAddr || cfg.LogLevel != DefaultLogLevel || cfg.WebDir != DefaultWebDir {
		t.Errorf("empty values did not fall back to defaults: %+v", cfg)
	}
}

func TestLoadInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantVar string
		wantVal string
	}{
		{
			name:    "http address without port",
			env:     map[string]string{EnvHTTPAddr: "localhost"},
			wantVar: EnvHTTPAddr,
			wantVal: "localhost",
		},
		{
			name:    "http address with non numeric port",
			env:     map[string]string{EnvHTTPAddr: ":http"},
			wantVar: EnvHTTPAddr,
			wantVal: ":http",
		},
		{
			name:    "http address with out of range port",
			env:     map[string]string{EnvHTTPAddr: ":70000"},
			wantVar: EnvHTTPAddr,
			wantVal: ":70000",
		},
		{
			name:    "unknown log level",
			env:     map[string]string{EnvLogLevel: "verbose"},
			wantVar: EnvLogLevel,
			wantVal: "verbose",
		},
		{
			name:    "unknown environment",
			env:     map[string]string{EnvEnvironment: "prod"},
			wantVar: EnvEnvironment,
			wantVal: "prod",
		},
		{
			name:    "database url without scheme",
			env:     map[string]string{EnvDatabaseURL: "localhost:5432/observaki"},
			wantVar: EnvDatabaseURL,
		},
		{
			name:    "database url with unsupported scheme",
			env:     map[string]string{EnvDatabaseURL: "mysql://localhost:3306/observaki"},
			wantVar: EnvDatabaseURL,
			wantVal: "mysql://localhost:3306/observaki",
		},
		{
			name:    "database url without host",
			env:     map[string]string{EnvDatabaseURL: "postgres:///observaki"},
			wantVar: EnvDatabaseURL,
			wantVal: "postgres:///observaki",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(lookup(tt.env))
			if err == nil {
				t.Fatal("Load() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantVar) {
				t.Errorf("error %q does not name %q", err, tt.wantVar)
			}
			if tt.wantVal != "" && !strings.Contains(err.Error(), tt.wantVal) {
				t.Errorf("error %q does not include the rejected value %q", err, tt.wantVal)
			}
		})
	}
}

func TestValidateRejectsEmptyWebDir(t *testing.T) {
	cfg, err := Load(lookup(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.WebDir = "  "

	err = cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), EnvWebDir) {
		t.Errorf("error %q does not name %q", err, EnvWebDir)
	}
}

func TestLogLevelSlogLevel(t *testing.T) {
	if LogLevelDebug.SlogLevel().String() != "DEBUG" {
		t.Errorf("debug level = %v", LogLevelDebug.SlogLevel())
	}
	if LogLevelWarn.SlogLevel().String() != "WARN" {
		t.Errorf("warn level = %v", LogLevelWarn.SlogLevel())
	}
	if LogLevelError.SlogLevel().String() != "ERROR" {
		t.Errorf("error level = %v", LogLevelError.SlogLevel())
	}
	if LogLevelInfo.SlogLevel().String() != "INFO" {
		t.Errorf("info level = %v", LogLevelInfo.SlogLevel())
	}
}
