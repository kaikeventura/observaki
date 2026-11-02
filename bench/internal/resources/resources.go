// Package resources samples machine and child-process resource usage while a
// benchmark workload runs. It is the collection side of the benchmark report:
// CPU, memory and disk are sampled once per second and summarized with the
// same distribution vocabulary used for latency (min/mean/p95/max).
package resources

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	schemaVersion   = 1
	DefaultInterval = time.Second
)

// Sample is one point-in-time resource reading.
type Sample struct {
	At               time.Time `json:"at"`
	CPUPercent       float64   `json:"cpu_percent"`
	CPUAvailable     bool      `json:"cpu_available"`
	MemoryUsedBytes  uint64    `json:"memory_used_bytes"`
	MemoryTotalBytes uint64    `json:"memory_total_bytes"`
	MemoryUsedPct    float64   `json:"memory_used_percent"`
	MemoryAvailable  bool      `json:"memory_available"`
	DiskFreeBytes    uint64    `json:"disk_free_bytes"`
	DiskTotalBytes   uint64    `json:"disk_total_bytes"`
	DiskAvailable    bool      `json:"disk_available"`
	Load1            float64   `json:"load1,omitempty"`
}

// Stats is the distribution of one sampled metric.
type Stats struct {
	Min  float64 `json:"min"`
	Mean float64 `json:"mean"`
	P95  float64 `json:"p95"`
	Max  float64 `json:"max"`
}

// Children is the accumulated resource usage of the load generator processes.
type Children struct {
	Available     bool    `json:"available"`
	UserSeconds   float64 `json:"user_seconds"`
	SystemSeconds float64 `json:"system_seconds"`
	CPUPercent    float64 `json:"cpu_percent"`
	MaxRSSBytes   uint64  `json:"max_rss_bytes"`
}

// Summary is the per-run resource digest written to resources.json and to
// run.json.
type Summary struct {
	SchemaVersion      int      `json:"schema_version"`
	Supported          bool     `json:"supported"`
	CPUAvailable       bool     `json:"cpu_available"`
	MemoryAvailable    bool     `json:"memory_available"`
	DiskAvailable      bool     `json:"disk_available"`
	Samples            int      `json:"samples"`
	IntervalSeconds    float64  `json:"interval_seconds"`
	DurationSeconds    float64  `json:"duration_seconds"`
	CPUPercent         Stats    `json:"cpu_percent"`
	MemoryUsedBytes    Stats    `json:"memory_used_bytes"`
	MemoryUsedPercent  Stats    `json:"memory_used_percent"`
	DiskFreeBytesStart uint64   `json:"disk_free_bytes_start"`
	DiskFreeBytesEnd   uint64   `json:"disk_free_bytes_end"`
	DiskTotalBytes     uint64   `json:"disk_total_bytes"`
	Load1              Stats    `json:"load1"`
	Children           Children `json:"children"`
	Warnings           []string `json:"warnings,omitempty"`
}

// File is the complete resources.json artifact.
type File struct {
	SchemaVersion int      `json:"schema_version"`
	Summary       Summary  `json:"summary"`
	Samples       []Sample `json:"samples"`
}

// Collector samples the host while a workload runs. A zero Collector is not
// usable; call NewCollector.
type Collector struct {
	path     string
	interval time.Duration

	mu       sync.Mutex
	samples  []Sample
	warnings []string
	started  time.Time
	stopped  time.Time
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	sampler  sampler
}

// NewCollector creates a collector. The path selects the filesystem used for
// the disk reading (normally the result directory).
func NewCollector(path string) *Collector {
	if path == "" {
		path = "."
	}
	return &Collector{
		path:     path,
		interval: DefaultInterval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start begins sampling. It primes the CPU counters so the first recorded
// sample already has a meaningful delta.
func (c *Collector) Start() {
	c.started = time.Now().UTC()
	c.sampler = newSampler(c.path)
	c.sampler.prime()
	go c.loop()
}

func (c *Collector) loop() {
	defer close(c.done)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.record()
		case <-c.stop:
			return
		}
	}
}

func (c *Collector) record() {
	sample, warnings := c.sampler.sample()
	c.mu.Lock()
	c.samples = append(c.samples, sample)
	c.warnings = append(c.warnings, warnings...)
	c.mu.Unlock()
}

// Stop ends sampling and returns the digest. It is safe to call more than once.
func (c *Collector) Stop() Summary {
	c.once.Do(func() {
		close(c.stop)
		<-c.done
		c.stopped = time.Now().UTC()
	})
	c.mu.Lock()
	samples := make([]Sample, len(c.samples))
	copy(samples, c.samples)
	warnings := append([]string(nil), c.warnings...)
	c.mu.Unlock()
	return Summarize(samples, c.interval, c.started, c.stopped, warnings)
}

// Samples returns a copy of the raw readings.
func (c *Collector) Samples() []Sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	samples := make([]Sample, len(c.samples))
	copy(samples, c.samples)
	return samples
}

// Summarize builds the digest for a set of samples.
func Summarize(samples []Sample, interval time.Duration, started, stopped time.Time, warnings []string) Summary {
	summary := Summary{
		SchemaVersion:   schemaVersion,
		Samples:         len(samples),
		IntervalSeconds: interval.Seconds(),
		Warnings:        warnings,
	}
	if !started.IsZero() && !stopped.IsZero() {
		summary.DurationSeconds = stopped.Sub(started).Seconds()
	}
	if len(samples) == 0 {
		return summary
	}
	summary.Supported = true

	cpu := make([]float64, 0, len(samples))
	memoryBytes := make([]float64, 0, len(samples))
	memoryPercent := make([]float64, 0, len(samples))
	load := make([]float64, 0, len(samples))
	for _, sample := range samples {
		cpu = append(cpu, sample.CPUPercent)
		memoryBytes = append(memoryBytes, float64(sample.MemoryUsedBytes))
		memoryPercent = append(memoryPercent, sample.MemoryUsedPct)
		load = append(load, sample.Load1)
		if sample.CPUAvailable {
			summary.CPUAvailable = true
		}
		if sample.MemoryAvailable {
			summary.MemoryAvailable = true
		}
		if sample.DiskAvailable {
			summary.DiskAvailable = true
		}
	}
	summary.CPUPercent = StatsFor(cpu)
	summary.MemoryUsedBytes = StatsFor(memoryBytes)
	summary.MemoryUsedPercent = StatsFor(memoryPercent)
	summary.Load1 = StatsFor(load)
	summary.DiskFreeBytesStart = samples[0].DiskFreeBytes
	summary.DiskFreeBytesEnd = samples[len(samples)-1].DiskFreeBytes
	summary.DiskTotalBytes = samples[len(samples)-1].DiskTotalBytes
	return summary
}

// StatsFor returns the distribution of values.
func StatsFor(values []float64) Stats {
	if len(values) == 0 {
		return Stats{}
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	sum := 0.0
	for _, value := range sorted {
		sum += value
	}
	return Stats{
		Min:  sorted[0],
		Mean: sum / float64(len(sorted)),
		P95:  quantile(sorted, 0.95),
		Max:  sorted[len(sorted)-1],
	}
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	// Nearest-rank percentile: with few samples p95 is the largest value,
	// which is the conservative reading for a resource ceiling.
	index := int(math.Ceil(q*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// Write stores the resources artifact atomically in dir.
func Write(dir string, summary Summary, samples []Sample) (string, error) {
	file := File{SchemaVersion: schemaVersion, Summary: summary, Samples: samples}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode resources: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, "resources.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", fmt.Errorf("write resources: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("publish resources: %w", err)
	}
	return path, nil
}

// Read loads a resources.json artifact.
func Read(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("decode resources: %w", err)
	}
	if file.SchemaVersion != schemaVersion {
		return File{}, fmt.Errorf("unsupported resources schema version %d", file.SchemaVersion)
	}
	return file, nil
}

// sampler keeps the previous CPU counters and reads one sample.
type sampler struct {
	path      string
	lastTotal uint64
	lastIdle  uint64
	primed    bool
}

func newSampler(path string) sampler {
	return sampler{path: path}
}

func (s *sampler) prime() {
	total, idle, err := readCPUTimes()
	if err != nil {
		return
	}
	s.lastTotal = total
	s.lastIdle = idle
	s.primed = true
}

func (s *sampler) sample() (Sample, []string) {
	sample := Sample{At: time.Now().UTC()}
	var warnings []string

	if total, idle, err := readCPUTimes(); err != nil {
		warnings = append(warnings, "cpu: "+err.Error())
	} else {
		if s.primed && total > s.lastTotal {
			deltaTotal := total - s.lastTotal
			deltaIdle := idle - s.lastIdle
			if deltaIdle > deltaTotal {
				deltaIdle = deltaTotal
			}
			sample.CPUPercent = float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
			sample.CPUAvailable = true
		}
		s.lastTotal = total
		s.lastIdle = idle
		s.primed = true
	}

	if total, available, err := readMemory(); err != nil {
		warnings = append(warnings, "memory: "+err.Error())
	} else {
		sample.MemoryTotalBytes = total
		if available < total {
			sample.MemoryUsedBytes = total - available
		}
		if total > 0 {
			sample.MemoryUsedPct = float64(sample.MemoryUsedBytes) / float64(total) * 100
			sample.MemoryAvailable = true
		}
	}

	if free, total, err := diskUsage(s.path); err != nil {
		warnings = append(warnings, "disk: "+err.Error())
	} else {
		sample.DiskFreeBytes = free
		sample.DiskTotalBytes = total
		sample.DiskAvailable = true
	}

	if load, err := readLoad1(); err == nil {
		sample.Load1 = load
	}
	return sample, warnings
}
