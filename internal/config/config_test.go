package config

import (
	"strings"
	"testing"
	"time"
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
	if cfg.OTLPGRPCAddr != DefaultOTLPGRPCAddr {
		t.Errorf("OTLPGRPCAddr = %q, want %q", cfg.OTLPGRPCAddr, DefaultOTLPGRPCAddr)
	}
	if cfg.OTLPHTTPAddr != DefaultOTLPHTTPAddr {
		t.Errorf("OTLPHTTPAddr = %q, want %q", cfg.OTLPHTTPAddr, DefaultOTLPHTTPAddr)
	}
	if cfg.IngestMaxRequestBytes != DefaultIngestMaxRequestBytes {
		t.Errorf("IngestMaxRequestBytes = %d, want %d", cfg.IngestMaxRequestBytes, DefaultIngestMaxRequestBytes)
	}
	if cfg.IngestMaxEvents != DefaultIngestMaxEvents {
		t.Errorf("IngestMaxEvents = %d, want %d", cfg.IngestMaxEvents, DefaultIngestMaxEvents)
	}
	if cfg.IngestAuthReload != DefaultIngestAuthReload {
		t.Errorf("IngestAuthReload = %s, want %s", cfg.IngestAuthReload, DefaultIngestAuthReload)
	}
	if cfg.IngestAuthFile != "" || cfg.IngestAuthPublicKey != "" || cfg.IngestAuditFile != "" {
		t.Errorf("auth paths = %q %q %q, want empty", cfg.IngestAuthFile, cfg.IngestAuthPublicKey, cfg.IngestAuditFile)
	}
	if cfg.RedactMask != DefaultRedactMask {
		t.Errorf("RedactMask = %q, want %q", cfg.RedactMask, DefaultRedactMask)
	}
	if got := strings.Join(cfg.RedactKeys, ","); got != DefaultRedactKeys {
		t.Errorf("RedactKeys = %q, want %q", got, DefaultRedactKeys)
	}
	if cfg.NATSURL != DefaultNATSURL {
		t.Errorf("NATSURL = %q, want %q", cfg.NATSURL, DefaultNATSURL)
	}
	if cfg.NATSSubjectPrefix != DefaultNATSSubjectPrefix || cfg.NATSStreamPrefix != DefaultNATSStreamPrefix {
		t.Errorf("NATS prefixes = %q/%q", cfg.NATSSubjectPrefix, cfg.NATSStreamPrefix)
	}
	if cfg.NATSShards != DefaultNATSShards || cfg.NATSStreamReplicas != DefaultNATSStreamReplicas {
		t.Errorf("NATS shards/replicas = %d/%d", cfg.NATSShards, cfg.NATSStreamReplicas)
	}
	if cfg.NATSStreamMaxAge != DefaultNATSStreamMaxAge || cfg.NATSStreamMaxBytes != DefaultNATSStreamMaxBytes {
		t.Errorf("NATS retention = %s/%d", cfg.NATSStreamMaxAge, cfg.NATSStreamMaxBytes)
	}
	if cfg.NATSStreamStorage != DefaultNATSStreamStorage {
		t.Errorf("NATSStreamStorage = %q", cfg.NATSStreamStorage)
	}
	if cfg.NATSBatchMaxEvents != DefaultNATSBatchMaxEvents || cfg.NATSBatchMaxBytes != DefaultNATSBatchMaxBytes {
		t.Errorf("NATS batch = %d/%d", cfg.NATSBatchMaxEvents, cfg.NATSBatchMaxBytes)
	}
	if cfg.NATSBatchLinger != DefaultNATSBatchLinger {
		t.Errorf("NATSBatchLinger = %s", cfg.NATSBatchLinger)
	}
	if cfg.NATSPublishTimeout != DefaultNATSPublishTimeout || cfg.NATSConnectTimeout != DefaultNATSConnectTimeout {
		t.Errorf("NATS timeouts = %s/%s", cfg.NATSPublishTimeout, cfg.NATSConnectTimeout)
	}
	if cfg.NATSMaxPendingBytes != DefaultNATSMaxPendingBytes || cfg.NATSPublishWorkers != DefaultNATSPublishWorkers {
		t.Errorf("NATS pending/workers = %d/%d", cfg.NATSMaxPendingBytes, cfg.NATSPublishWorkers)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(lookup(map[string]string{
		EnvHTTPAddr:    "127.0.0.1:9000",
		EnvLogLevel:    "DEBUG",
		EnvEnvironment: "Staging",
		EnvDatabaseURL: "postgres://db.internal:5432/observaki",
		EnvWebDir:      "/srv/observaki/web",

		EnvOTLPGRPCAddr:          "127.0.0.1:14317",
		EnvOTLPHTTPAddr:          "127.0.0.1:14318",
		EnvIngestMaxRequestBytes: "2048",
		EnvIngestMaxEvents:       "42",
		EnvIngestAuthFile:        "/etc/observaki/ingest-auth.json",
		EnvIngestAuthPublicKey:   "cHVibGljLWtleQ==",
		EnvIngestAuthReload:      "45s",
		EnvIngestAuditFile:       "/var/log/observaki/audit.jsonl",
		EnvRedactKeys:            "secret, token",
		EnvRedactMask:            "***",
		EnvNATSURL:               "tls://nats.internal:4222",
		EnvNATSCredentialsFile:   "/etc/observaki/nats.creds",
		EnvNATSSubjectPrefix:     "obs.ingest.v1",
		EnvNATSStreamPrefix:      "OBS_TEST",
		EnvNATSShards:            "64",
		EnvNATSStreamReplicas:    "3",
		EnvNATSStreamMaxAge:      "12h",
		EnvNATSStreamMaxBytes:    "10737418240",
		EnvNATSStreamStorage:     "memory",
		EnvNATSBatchMaxEvents:    "5000",
		EnvNATSBatchMaxBytes:     "1048576",
		EnvNATSBatchLinger:       "10ms",
		EnvNATSConnectTimeout:    "3s",
		EnvNATSPublishTimeout:    "2s",
		EnvNATSMaxPendingBytes:   "67108864",
		EnvNATSPublishWorkers:    "4",
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
	if cfg.OTLPGRPCAddr != "127.0.0.1:14317" {
		t.Errorf("OTLPGRPCAddr = %q", cfg.OTLPGRPCAddr)
	}
	if cfg.OTLPHTTPAddr != "127.0.0.1:14318" {
		t.Errorf("OTLPHTTPAddr = %q", cfg.OTLPHTTPAddr)
	}
	if cfg.IngestMaxRequestBytes != 2048 {
		t.Errorf("IngestMaxRequestBytes = %d", cfg.IngestMaxRequestBytes)
	}
	if cfg.IngestMaxEvents != 42 {
		t.Errorf("IngestMaxEvents = %d", cfg.IngestMaxEvents)
	}
	if cfg.IngestAuthFile != "/etc/observaki/ingest-auth.json" {
		t.Errorf("IngestAuthFile = %q", cfg.IngestAuthFile)
	}
	if cfg.IngestAuthPublicKey != "cHVibGljLWtleQ==" {
		t.Errorf("IngestAuthPublicKey = %q", cfg.IngestAuthPublicKey)
	}
	if cfg.IngestAuthReload != 45*time.Second {
		t.Errorf("IngestAuthReload = %s", cfg.IngestAuthReload)
	}
	if cfg.IngestAuditFile != "/var/log/observaki/audit.jsonl" {
		t.Errorf("IngestAuditFile = %q", cfg.IngestAuditFile)
	}
	if got := strings.Join(cfg.RedactKeys, ","); got != "secret,token" {
		t.Errorf("RedactKeys = %q", got)
	}
	if cfg.RedactMask != "***" {
		t.Errorf("RedactMask = %q", cfg.RedactMask)
	}
	if cfg.NATSURL != "tls://nats.internal:4222" || cfg.NATSCredentialsFile != "/etc/observaki/nats.creds" {
		t.Errorf("NATS connection = %q/%q", cfg.NATSURL, cfg.NATSCredentialsFile)
	}
	if cfg.NATSSubjectPrefix != "obs.ingest.v1" || cfg.NATSStreamPrefix != "OBS_TEST" {
		t.Errorf("NATS prefixes = %q/%q", cfg.NATSSubjectPrefix, cfg.NATSStreamPrefix)
	}
	if cfg.NATSShards != 64 || cfg.NATSStreamReplicas != 3 {
		t.Errorf("NATS shards/replicas = %d/%d", cfg.NATSShards, cfg.NATSStreamReplicas)
	}
	if cfg.NATSStreamMaxAge != 12*time.Hour || cfg.NATSStreamMaxBytes != 10<<30 {
		t.Errorf("NATS retention = %s/%d", cfg.NATSStreamMaxAge, cfg.NATSStreamMaxBytes)
	}
	if cfg.NATSStreamStorage != "memory" {
		t.Errorf("NATSStreamStorage = %q", cfg.NATSStreamStorage)
	}
	if cfg.NATSBatchMaxEvents != 5000 || cfg.NATSBatchMaxBytes != 1<<20 || cfg.NATSBatchLinger != 10*time.Millisecond {
		t.Errorf("NATS batch = %d/%d/%s", cfg.NATSBatchMaxEvents, cfg.NATSBatchMaxBytes, cfg.NATSBatchLinger)
	}
	if cfg.NATSConnectTimeout != 3*time.Second || cfg.NATSPublishTimeout != 2*time.Second {
		t.Errorf("NATS timeouts = %s/%s", cfg.NATSConnectTimeout, cfg.NATSPublishTimeout)
	}
	if cfg.NATSMaxPendingBytes != 64<<20 || cfg.NATSPublishWorkers != 4 {
		t.Errorf("NATS pending/workers = %d/%d", cfg.NATSMaxPendingBytes, cfg.NATSPublishWorkers)
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
		{
			name:    "otlp grpc address without port",
			env:     map[string]string{EnvOTLPGRPCAddr: "localhost"},
			wantVar: EnvOTLPGRPCAddr,
			wantVal: "localhost",
		},
		{
			name:    "otlp addresses collide",
			env:     map[string]string{EnvOTLPGRPCAddr: ":4317", EnvOTLPHTTPAddr: ":4317"},
			wantVar: EnvOTLPGRPCAddr,
		},
		{
			name:    "max request bytes not a number",
			env:     map[string]string{EnvIngestMaxRequestBytes: "ten"},
			wantVar: EnvIngestMaxRequestBytes,
			wantVal: "ten",
		},
		{
			name:    "max request bytes too small",
			env:     map[string]string{EnvIngestMaxRequestBytes: "0"},
			wantVar: EnvIngestMaxRequestBytes,
			wantVal: "0",
		},
		{
			name:    "max events not a number",
			env:     map[string]string{EnvIngestMaxEvents: "many"},
			wantVar: EnvIngestMaxEvents,
			wantVal: "many",
		},
		{
			name:    "max events zero",
			env:     map[string]string{EnvIngestMaxEvents: "0"},
			wantVar: EnvIngestMaxEvents,
			wantVal: "0",
		},
		{
			name:    "auth reload not a duration",
			env:     map[string]string{EnvIngestAuthReload: "often"},
			wantVar: EnvIngestAuthReload,
			wantVal: "often",
		},
		{
			name:    "auth reload below one second",
			env:     map[string]string{EnvIngestAuthReload: "100ms"},
			wantVar: EnvIngestAuthReload,
			wantVal: "100ms",
		},
		{
			name:    "redact keys empty",
			env:     map[string]string{EnvRedactKeys: " , , "},
			wantVar: EnvRedactKeys,
		},
		{
			name:    "nats url invalid scheme",
			env:     map[string]string{EnvNATSURL: "http://nats:4222"},
			wantVar: EnvNATSURL,
			wantVal: "http://nats:4222",
		},
		{
			name:    "nats subject prefix has wildcard",
			env:     map[string]string{EnvNATSSubjectPrefix: "observaki.*"},
			wantVar: EnvNATSSubjectPrefix,
			wantVal: "observaki.*",
		},
		{
			name:    "nats shards zero",
			env:     map[string]string{EnvNATSShards: "0"},
			wantVar: EnvNATSShards,
			wantVal: "0",
		},
		{
			name:    "nats replicas too high",
			env:     map[string]string{EnvNATSStreamReplicas: "6"},
			wantVar: EnvNATSStreamReplicas,
			wantVal: "6",
		},
		{
			name:    "nats storage unknown",
			env:     map[string]string{EnvNATSStreamStorage: "ssd"},
			wantVar: EnvNATSStreamStorage,
			wantVal: "ssd",
		},
		{
			name:    "nats batch linger too small",
			env:     map[string]string{EnvNATSBatchLinger: "1us"},
			wantVar: EnvNATSBatchLinger,
			wantVal: "1µs",
		},
		{
			name:    "nats publish workers zero",
			env:     map[string]string{EnvNATSPublishWorkers: "0"},
			wantVar: EnvNATSPublishWorkers,
			wantVal: "0",
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
