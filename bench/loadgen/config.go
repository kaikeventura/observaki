// Package loadgen implements the deterministic OTLP workload generator used by
// the Observaki benchmark harness.
package loadgen

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Protocol is the OTLP transport used by the generator.
type Protocol string

const (
	ProtocolGRPC Protocol = "grpc"
	ProtocolHTTP Protocol = "http"
)

// ParseProtocol validates a protocol name.
func ParseProtocol(value string) (Protocol, error) {
	switch Protocol(strings.ToLower(strings.TrimSpace(value))) {
	case ProtocolGRPC:
		return ProtocolGRPC, nil
	case ProtocolHTTP:
		return ProtocolHTTP, nil
	default:
		return "", fmt.Errorf("unsupported protocol %q (want grpc or http)", value)
	}
}

// Compression is the request compression used by the generator.
type Compression string

const (
	CompressionGzip Compression = "gzip"
	CompressionNone Compression = "none"
)

// ParseCompression validates a compression name.
func ParseCompression(value string) (Compression, error) {
	switch Compression(strings.ToLower(strings.TrimSpace(value))) {
	case CompressionGzip:
		return CompressionGzip, nil
	case CompressionNone:
		return CompressionNone, nil
	default:
		return "", fmt.Errorf("unsupported compression %q (want gzip or none)", value)
	}
}

// Mix is the signal mix in events per 100 events.
type Mix struct {
	Metrics int `json:"metrics"`
	Logs    int `json:"logs"`
	Traces  int `json:"traces"`
}

// DefaultMix is the Phase 0 reference workload: 60% metrics, 30% logs and 10%
// traces.
func DefaultMix() Mix {
	return Mix{Metrics: 60, Logs: 30, Traces: 10}
}

// Validate checks that the mix sums to 100 and has no negative component.
func (m Mix) Validate() error {
	if m.Metrics < 0 || m.Logs < 0 || m.Traces < 0 {
		return errors.New("signal mix components must be non-negative")
	}
	if total := m.Metrics + m.Logs + m.Traces; total != 100 {
		return fmt.Errorf("signal mix must sum to 100, got %d", total)
	}
	return nil
}

// Percent returns the percentage of a signal in the mix.
func (m Mix) Percent(signal Signal) float64 {
	switch signal {
	case Metrics:
		return float64(m.Metrics)
	case Logs:
		return float64(m.Logs)
	case Traces:
		return float64(m.Traces)
	default:
		return 0
	}
}

// Pattern returns a deterministic 100-slot schedule with the exact mix. The
// deficit scheduler interleaves signals instead of emitting one large block.
func (m Mix) Pattern() []Signal {
	counts := [3]int{m.Metrics, m.Logs, m.Traces}
	emitted := [3]int{}
	pattern := make([]Signal, 0, 100)
	for slot := 1; slot <= 100; slot++ {
		best := 0
		bestDeficit := slot*counts[0] - emitted[0]*100
		for signal := 1; signal < 3; signal++ {
			deficit := slot*counts[signal] - emitted[signal]*100
			if deficit > bestDeficit {
				best = signal
				bestDeficit = deficit
			}
		}
		emitted[best]++
		pattern = append(pattern, Signal(best))
	}
	return pattern
}

// PayloadSizes controls the synthetic payload sizes. The defaults are tuned so
// that the gzip-compressed weighted average stays within 10% of 134 B/event.
type PayloadSizes struct {
	// MetricPadding is the length of the bench.payload attribute on each
	// metric data point.
	MetricPadding int `json:"metric_padding"`
	// LogBody is the length of the log record body.
	LogBody int `json:"log_body"`
	// TracePadding is the length of the bench.payload attribute on each span.
	TracePadding int `json:"trace_padding"`
}

// DefaultPayloadSizes returns the reference workload payload sizes.
func DefaultPayloadSizes() PayloadSizes {
	return PayloadSizes{
		MetricPadding: 832,
		LogBody:       820,
		TracePadding:  2400,
	}
}

// Config controls a generator run.
type Config struct {
	Endpoint       string
	Protocol       Protocol
	Compression    Compression
	Rate           int
	Duration       time.Duration
	StartTime      time.Time
	Seed           int64
	BatchSize      int
	Workers        int
	MetricsAddr    string
	Mix            Mix
	Payload        PayloadSizes
	RequestTimeout time.Duration
	LogInterval    time.Duration
	DryRun         bool
}

// DefaultConfig returns a runnable reference configuration.
func DefaultConfig() Config {
	return Config{
		Endpoint:       "127.0.0.1:4317",
		Protocol:       ProtocolGRPC,
		Compression:    CompressionGzip,
		Rate:           100_000,
		Duration:       5 * time.Minute,
		StartTime:      time.Now().UTC(),
		Seed:           1,
		BatchSize:      1000,
		Workers:        4,
		MetricsAddr:    "127.0.0.1:9090",
		Mix:            DefaultMix(),
		Payload:        DefaultPayloadSizes(),
		RequestTimeout: 10 * time.Second,
		LogInterval:    5 * time.Second,
	}
}

// TotalEvents returns the number of events a full run should generate.
func (c Config) TotalEvents() int64 {
	return int64(c.Rate) * int64(c.Duration) / int64(time.Second)
}

// Validate checks a configuration before a run.
func (c Config) Validate() error {
	if c.Protocol != ProtocolGRPC && c.Protocol != ProtocolHTTP {
		return fmt.Errorf("unsupported protocol %q", c.Protocol)
	}
	if c.Compression != CompressionGzip && c.Compression != CompressionNone {
		return fmt.Errorf("unsupported compression %q", c.Compression)
	}
	if c.Rate <= 0 {
		return errors.New("rate must be greater than zero")
	}
	if c.Duration <= 0 {
		return errors.New("duration must be greater than zero")
	}
	if c.BatchSize <= 0 {
		return errors.New("batch size must be greater than zero")
	}
	if c.Workers <= 0 {
		return errors.New("workers must be greater than zero")
	}
	if c.RequestTimeout <= 0 {
		return errors.New("request timeout must be greater than zero")
	}
	if c.LogInterval <= 0 {
		return errors.New("log interval must be greater than zero")
	}
	if err := c.Mix.Validate(); err != nil {
		return err
	}
	if c.Payload.MetricPadding < 0 || c.Payload.LogBody < 0 || c.Payload.TracePadding < 0 {
		return errors.New("payload sizes must be non-negative")
	}
	if c.DryRun {
		return nil
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return errors.New("endpoint is required unless dry-run is set")
	}
	if c.Protocol == ProtocolHTTP {
		parsed, err := url.Parse(c.Endpoint)
		if err != nil {
			return fmt.Errorf("invalid HTTP endpoint: %w", err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("HTTP endpoint must use http or https, got %q", parsed.Scheme)
		}
		if parsed.Host == "" {
			return errors.New("HTTP endpoint must include a host")
		}
	}
	return nil
}
