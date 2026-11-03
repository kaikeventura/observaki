package loadgen

import (
	"testing"
	"time"
)

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate() = %v", err)
	}
	if got, want := cfg.TotalEvents(), int64(100_000*300); got != want {
		t.Fatalf("TotalEvents() = %d, want %d", got, want)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Config) {}},
		{name: "zero rate", mutate: func(c *Config) { c.Rate = 0 }, wantErr: true},
		{name: "negative event-time rate", mutate: func(c *Config) { c.EventTimeRate = -1 }, wantErr: true},
		{name: "zero duration", mutate: func(c *Config) { c.Duration = 0 }, wantErr: true},
		{name: "zero batch size", mutate: func(c *Config) { c.BatchSize = 0 }, wantErr: true},
		{name: "zero workers", mutate: func(c *Config) { c.Workers = 0 }, wantErr: true},
		{name: "zero timeout", mutate: func(c *Config) { c.RequestTimeout = 0 }, wantErr: true},
		{name: "zero log interval", mutate: func(c *Config) { c.LogInterval = 0 }, wantErr: true},
		{name: "invalid protocol", mutate: func(c *Config) { c.Protocol = "carrier-pigeon" }, wantErr: true},
		{name: "invalid compression", mutate: func(c *Config) { c.Compression = "brotli" }, wantErr: true},
		{name: "mix does not sum to 100", mutate: func(c *Config) { c.Mix = Mix{Metrics: 60, Logs: 30, Traces: 20} }, wantErr: true},
		{name: "negative mix", mutate: func(c *Config) { c.Mix = Mix{Metrics: 110, Logs: -10, Traces: 0} }, wantErr: true},
		{name: "negative payload", mutate: func(c *Config) { c.Payload.LogBody = -1 }, wantErr: true},
		{name: "empty endpoint", mutate: func(c *Config) { c.Endpoint = "" }, wantErr: true},
		{name: "empty endpoint dry run", mutate: func(c *Config) { c.Endpoint = ""; c.DryRun = true }},
		{name: "http endpoint without scheme", mutate: func(c *Config) { c.Protocol = ProtocolHTTP; c.Endpoint = "127.0.0.1:4318" }, wantErr: true},
		{name: "http endpoint with scheme", mutate: func(c *Config) { c.Protocol = ProtocolHTTP; c.Endpoint = "http://127.0.0.1:4318" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := DefaultConfig()
			test.mutate(&cfg)
			err := cfg.Validate()
			if test.wantErr && err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestTotalEventsTruncatesSubSecondDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rate = 10_000
	cfg.Duration = 150 * time.Millisecond
	if got, want := cfg.TotalEvents(), int64(1500); got != want {
		t.Fatalf("TotalEvents() = %d, want %d", got, want)
	}
}

func TestParseProtocolAndCompression(t *testing.T) {
	if got, err := ParseProtocol(" GRPC "); err != nil || got != ProtocolGRPC {
		t.Fatalf("ParseProtocol() = %q, %v", got, err)
	}
	if got, err := ParseCompression("gzip"); err != nil || got != CompressionGzip {
		t.Fatalf("ParseCompression() = %q, %v", got, err)
	}
	if _, err := ParseProtocol("udp"); err == nil {
		t.Fatal("ParseProtocol(udp) = nil error")
	}
	if _, err := ParseCompression("zstd"); err == nil {
		t.Fatal("ParseCompression(zstd) = nil error")
	}
}

func TestParseHeaders(t *testing.T) {
	headers, err := ParseHeaders([]string{"Authorization=Bearer abc", "X-Observaki-Project-Id=project-a"})
	if err != nil {
		t.Fatalf("ParseHeaders() = %v", err)
	}
	if headers["Authorization"] != "Bearer abc" || headers["X-Observaki-Project-Id"] != "project-a" {
		t.Fatalf("ParseHeaders() = %v", headers)
	}
	if _, err := ParseHeaders([]string{"missing-equals"}); err == nil {
		t.Fatal("ParseHeaders(missing-equals) = nil error")
	}
	if _, err := ParseHeaders([]string{"=value"}); err == nil {
		t.Fatal("ParseHeaders(empty key) = nil error")
	}
	if _, err := ParseHeaders([]string{"key=line\nbreak"}); err == nil {
		t.Fatal("ParseHeaders(newline) = nil error")
	}
	cfg := DefaultConfig()
	cfg.Headers = map[string]string{"bad key": "value"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() with a header key containing whitespace = nil")
	}
}
