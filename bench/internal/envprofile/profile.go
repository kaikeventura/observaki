// Package envprofile defines the reproducible benchmark environment profile
// (topology, hardware, retention and cost inputs) and validates it against the
// Phase 0 T1 contract.
package envprofile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SchemaVersion is the profile schema version understood by this package.
const SchemaVersion = 1

const (
	gib = int64(1024 * 1024 * 1024)
	tib = 1024 * gib
)

var roleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// RequiredSignals are the signals that must be present in the workload mix.
var RequiredSignals = []string{"metrics", "logs", "traces"}

// RequiredRoles are the roles that make up the T1 reference cluster.
var RequiredRoles = []string{"clickhouse", "victoriametrics", "gateway", "nats", "loadgen"}

// Signal describes the share and compressed size of one telemetry signal.
type Signal struct {
	Share                   float64 `json:"share"`
	CompressedBytesPerEvent float64 `json:"compressed_bytes_per_event"`
}

// Workload pins the reference workload and the retention assumptions used to
// size the cluster.
type Workload struct {
	EventsPerSecond            int64             `json:"events_per_second"`
	RetentionHours             int               `json:"retention_hours"`
	Signals                    map[string]Signal `json:"signals"`
	ClickHouseReplication      int               `json:"clickhouse_replication"`
	VictoriaMetricsReplication int               `json:"victoriametrics_replication"`
	NATSReplication            int               `json:"nats_replication"`
	NATSBufferHours            int               `json:"nats_buffer_hours"`
	GatewayEventsPerVCPU       int               `json:"gateway_events_per_vcpu"`
	LoadgenEventsPerVCPU       int               `json:"loadgen_events_per_vcpu"`
}

// Role is one node role in the reference cluster.
type Role struct {
	Name                   string `json:"name"`
	Count                  int    `json:"count"`
	Min                    int    `json:"min"`
	Max                    int    `json:"max"`
	InstanceType           string `json:"instance_type"`
	VCPUs                  int    `json:"vcpus"`
	MemoryGiB              int    `json:"memory_gib"`
	DataDisks              int    `json:"data_disks"`
	DataDiskGiB            int    `json:"data_disk_gib"`
	DataDiskThroughputMBps int    `json:"data_disk_throughput_mbps"`
	DataDiskIOPS           int    `json:"data_disk_iops"`
	Purpose                string `json:"purpose,omitempty"`
}

// Profile is the versioned T1 reference environment.
type Profile struct {
	SchemaVersion    int               `json:"schema_version"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	Region           string            `json:"region"`
	AvailabilityZone string            `json:"availability_zone"`
	RootDiskGiB      int               `json:"root_disk_gib"`
	Workload         Workload          `json:"workload"`
	Roles            []Role            `json:"roles"`
	Software         map[string]string `json:"software,omitempty"`
	Notes            []string          `json:"notes,omitempty"`
}

// ValidationError lists every problem found in a profile or price table.
type ValidationError struct {
	Problems []string
}

// Error implements error.
func (e *ValidationError) Error() string {
	return strings.Join(e.Problems, "; ")
}

// LoadProfile reads and strictly decodes a profile.
func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", path, err)
	}
	var profile Profile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode profile %s: %w", path, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode profile %s: %w", path, err)
	}
	return &profile, nil
}

// Validate reports every contract violation in the profile.
func (p Profile) Validate() error {
	problems := p.Problems()
	if len(problems) == 0 {
		return nil
	}
	return &ValidationError{Problems: problems}
}

// Problems returns the list of contract violations. It is exported so the CLI
// can print one line per problem instead of a single concatenated string.
func (p Profile) Problems() []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if p.SchemaVersion != SchemaVersion {
		add("schema_version = %d, want %d", p.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(p.Name) == "" {
		add("name is required")
	}
	if strings.TrimSpace(p.Region) == "" {
		add("region is required")
	}
	if strings.TrimSpace(p.AvailabilityZone) == "" {
		add("availability_zone is required")
	}
	if p.RootDiskGiB < 20 {
		add("root_disk_gib = %d, want at least 20", p.RootDiskGiB)
	}

	workload := p.Workload
	if workload.EventsPerSecond <= 0 {
		add("workload.events_per_second must be greater than zero")
	}
	if workload.RetentionHours <= 0 {
		add("workload.retention_hours must be greater than zero")
	}
	if workload.ClickHouseReplication < 1 {
		add("workload.clickhouse_replication must be at least 1")
	}
	if workload.VictoriaMetricsReplication < 1 {
		add("workload.victoriametrics_replication must be at least 1")
	}
	if workload.NATSReplication < 1 {
		add("workload.nats_replication must be at least 1")
	}
	if workload.NATSBufferHours <= 0 {
		add("workload.nats_buffer_hours must be greater than zero")
	}
	if workload.GatewayEventsPerVCPU <= 0 {
		add("workload.gateway_events_per_vcpu must be greater than zero")
	}
	if workload.LoadgenEventsPerVCPU <= 0 {
		add("workload.loadgen_events_per_vcpu must be greater than zero")
	}

	shareTotal := 0.0
	for _, name := range RequiredSignals {
		signal, ok := workload.Signals[name]
		if !ok {
			add("workload.signals.%s is required", name)
			continue
		}
		if signal.Share <= 0 || signal.Share > 1 {
			add("workload.signals.%s.share = %g, want (0, 1]", name, signal.Share)
		}
		if signal.CompressedBytesPerEvent <= 0 {
			add("workload.signals.%s.compressed_bytes_per_event must be greater than zero", name)
		}
		shareTotal += signal.Share
	}
	if len(workload.Signals) > 0 && math.Abs(shareTotal-1) > 0.001 {
		add("workload signal shares sum to %.4f, want 1.0", shareTotal)
	}

	roles := make(map[string]Role, len(p.Roles))
	for _, role := range p.Roles {
		if !roleNamePattern.MatchString(role.Name) {
			add("role %q is not a valid Terraform identifier", role.Name)
			continue
		}
		if _, ok := roles[role.Name]; ok {
			add("role %q is duplicated", role.Name)
			continue
		}
		roles[role.Name] = role

		if role.Min < 1 || role.Max < role.Min {
			add("role %s has invalid range [%d, %d]", role.Name, role.Min, role.Max)
		}
		if role.Count < role.Min || role.Count > role.Max {
			add("role %s count = %d, outside [%d, %d]", role.Name, role.Count, role.Min, role.Max)
		}
		if strings.TrimSpace(role.InstanceType) == "" {
			add("role %s instance_type is required", role.Name)
		}
		if role.VCPUs < 1 {
			add("role %s vcpus must be greater than zero", role.Name)
		}
		if role.MemoryGiB < 1 {
			add("role %s memory_gib must be greater than zero", role.Name)
		}
		if role.DataDisks < 0 {
			add("role %s data_disks must not be negative", role.Name)
		}
		if role.DataDisks == 0 {
			if role.DataDiskGiB != 0 || role.DataDiskThroughputMBps != 0 || role.DataDiskIOPS != 0 {
				add("role %s has data disk settings but data_disks = 0", role.Name)
			}
		} else {
			if role.DataDiskGiB < 1 {
				add("role %s data_disk_gib must be greater than zero", role.Name)
			}
			if role.DataDiskThroughputMBps < 125 {
				add("role %s data_disk_throughput_mbps = %d, want at least 125", role.Name, role.DataDiskThroughputMBps)
			}
			if role.DataDiskIOPS < 3000 {
				add("role %s data_disk_iops = %d, want at least 3000", role.Name, role.DataDiskIOPS)
			}
		}
	}

	for _, name := range RequiredRoles {
		if _, ok := roles[name]; !ok {
			add("required role %q is missing", name)
		}
	}

	if ch, ok := roles["clickhouse"]; ok {
		if ch.Count < 6 || ch.Count > 12 {
			add("clickhouse count = %d, want 6-12", ch.Count)
		}
		checkMinimum(add, ch, 16, 64)
	}
	if vm, ok := roles["victoriametrics"]; ok {
		if vm.Count != 3 {
			add("victoriametrics count = %d, want exactly 3", vm.Count)
		}
		checkMinimum(add, vm, 16, 64)
	}
	if gw, ok := roles["gateway"]; ok {
		if gw.Count < 4 || gw.Count > 6 {
			add("gateway count = %d, want 4-6", gw.Count)
		}
		if gw.VCPUs < 16 {
			add("gateway vcpus = %d, want at least 16", gw.VCPUs)
		}
		if gw.MemoryGiB < 8 {
			add("gateway memory_gib = %d, want at least 8", gw.MemoryGiB)
		}
	}
	if nats, ok := roles["nats"]; ok {
		checkMinimum(add, nats, 8, 16)
	}
	if loadgen, ok := roles["loadgen"]; ok {
		checkMinimum(add, loadgen, 16, 16)
	}

	if len(problems) == 0 {
		capacity, err := p.Capacity()
		if err != nil {
			add("%v", err)
		} else {
			if capacity.ClickHouseRawBytes < int64(float64(capacity.ClickHouseRequiredBytes)*1.10) {
				add("clickhouse raw capacity %s is below the 7-day requirement %s with 10%% headroom",
					formatBytes(capacity.ClickHouseRawBytes), formatBytes(capacity.ClickHouseRequiredBytes))
			}
			if capacity.VictoriaMetricsRawBytes < int64(float64(capacity.VictoriaMetricsRequiredBytes)*1.10) {
				add("victoriametrics raw capacity %s is below the retention requirement %s with 10%% headroom",
					formatBytes(capacity.VictoriaMetricsRawBytes), formatBytes(capacity.VictoriaMetricsRequiredBytes))
			}
			if capacity.NATSRawBytes < int64(float64(capacity.NATSRequiredBytes)*1.10) {
				add("nats raw capacity %s is below the %dh WAL requirement %s with 10%% headroom",
					formatBytes(capacity.NATSRawBytes), p.Workload.NATSBufferHours, formatBytes(capacity.NATSRequiredBytes))
			}
			if capacity.GatewayCapacity < p.Workload.EventsPerSecond {
				add("gateway fleet capacity %d events/s is below the %d events/s workload",
					capacity.GatewayCapacity, p.Workload.EventsPerSecond)
			}
			if capacity.LoadgenCapacity < p.Workload.EventsPerSecond {
				add("loadgen fleet capacity %d events/s is below the %d events/s workload",
					capacity.LoadgenCapacity, p.Workload.EventsPerSecond)
			}
		}
	}

	return problems
}

func checkMinimum(add func(string, ...any), role Role, vcpus, memoryGiB int) {
	if role.VCPUs < vcpus {
		add("%s vcpus = %d, want at least %d", role.Name, role.VCPUs, vcpus)
	}
	if role.MemoryGiB < memoryGiB {
		add("%s memory_gib = %d, want at least %d", role.Name, role.MemoryGiB, memoryGiB)
	}
}

// Capacity is the storage and compute headroom of the profile.
type Capacity struct {
	ClickHouseRawBytes             int64   `json:"clickhouse_raw_bytes"`
	ClickHouseLogicalBytes         int64   `json:"clickhouse_logical_bytes"`
	ClickHouseRequiredBytes        int64   `json:"clickhouse_required_bytes"`
	ClickHouseHeadroomPercent      float64 `json:"clickhouse_headroom_percent"`
	VictoriaMetricsRawBytes        int64   `json:"victoriametrics_raw_bytes"`
	VictoriaMetricsLogicalBytes    int64   `json:"victoriametrics_logical_bytes"`
	VictoriaMetricsRequiredBytes   int64   `json:"victoriametrics_required_bytes"`
	VictoriaMetricsHeadroomPercent float64 `json:"victoriametrics_headroom_percent"`
	NATSRawBytes                   int64   `json:"nats_raw_bytes"`
	NATSLogicalBytes               int64   `json:"nats_logical_bytes"`
	NATSRequiredBytes              int64   `json:"nats_required_bytes"`
	NATSHeadroomPercent            float64 `json:"nats_headroom_percent"`
	GatewayCapacity                int64   `json:"gateway_capacity_events_per_second"`
	LoadgenCapacity                int64   `json:"loadgen_capacity_events_per_second"`
	BytesPerSecond                 float64 `json:"bytes_per_second"`
	TerabytesPerDay                float64 `json:"terabytes_per_day"`
}

// Capacity computes the storage and compute headroom for the profile.
func (p Profile) Capacity() (Capacity, error) {
	roles, err := p.roleMap()
	if err != nil {
		return Capacity{}, err
	}
	ch, ok := roles["clickhouse"]
	if !ok {
		return Capacity{}, errors.New("clickhouse role is missing")
	}
	vm, ok := roles["victoriametrics"]
	if !ok {
		return Capacity{}, errors.New("victoriametrics role is missing")
	}
	nats, ok := roles["nats"]
	if !ok {
		return Capacity{}, errors.New("nats role is missing")
	}
	gateway, ok := roles["gateway"]
	if !ok {
		return Capacity{}, errors.New("gateway role is missing")
	}
	loadgen, ok := roles["loadgen"]
	if !ok {
		return Capacity{}, errors.New("loadgen role is missing")
	}

	metrics, ok := p.Workload.Signals["metrics"]
	if !ok {
		return Capacity{}, errors.New("metrics signal is missing")
	}
	logs, ok := p.Workload.Signals["logs"]
	if !ok {
		return Capacity{}, errors.New("logs signal is missing")
	}
	traces, ok := p.Workload.Signals["traces"]
	if !ok {
		return Capacity{}, errors.New("traces signal is missing")
	}

	retentionSeconds := float64(p.Workload.RetentionHours * 3600)
	eventsPerSecond := float64(p.Workload.EventsPerSecond)
	metricsBytesPerSecond := eventsPerSecond * metrics.Share * metrics.CompressedBytesPerEvent
	chBytesPerSecond := eventsPerSecond * (logs.Share*logs.CompressedBytesPerEvent + traces.Share*traces.CompressedBytesPerEvent)
	totalBytesPerSecond := metricsBytesPerSecond + chBytesPerSecond

	capacity := Capacity{
		ClickHouseRawBytes:           int64(ch.Count*ch.DataDisks*ch.DataDiskGiB) * gib,
		ClickHouseRequiredBytes:      int64(chBytesPerSecond * retentionSeconds * float64(p.Workload.ClickHouseReplication)),
		VictoriaMetricsRawBytes:      int64(vm.Count*vm.DataDisks*vm.DataDiskGiB) * gib,
		VictoriaMetricsRequiredBytes: int64(metricsBytesPerSecond * retentionSeconds * float64(p.Workload.VictoriaMetricsReplication)),
		NATSRawBytes:                 int64(nats.Count*nats.DataDisks*nats.DataDiskGiB) * gib,
		NATSRequiredBytes:            int64(totalBytesPerSecond * float64(p.Workload.NATSBufferHours*3600) * float64(p.Workload.NATSReplication)),
		GatewayCapacity:              int64(gateway.Count * gateway.VCPUs * p.Workload.GatewayEventsPerVCPU),
		LoadgenCapacity:              int64(loadgen.Count * loadgen.VCPUs * p.Workload.LoadgenEventsPerVCPU),
		BytesPerSecond:               totalBytesPerSecond,
		TerabytesPerDay:              totalBytesPerSecond * 86400 / 1e12,
	}
	if p.Workload.ClickHouseReplication > 0 {
		capacity.ClickHouseLogicalBytes = capacity.ClickHouseRawBytes / int64(p.Workload.ClickHouseReplication)
	}
	if p.Workload.VictoriaMetricsReplication > 0 {
		capacity.VictoriaMetricsLogicalBytes = capacity.VictoriaMetricsRawBytes / int64(p.Workload.VictoriaMetricsReplication)
	}
	if p.Workload.NATSReplication > 0 {
		capacity.NATSLogicalBytes = capacity.NATSRawBytes / int64(p.Workload.NATSReplication)
	}
	capacity.ClickHouseHeadroomPercent = headroom(capacity.ClickHouseRawBytes, capacity.ClickHouseRequiredBytes)
	capacity.VictoriaMetricsHeadroomPercent = headroom(capacity.VictoriaMetricsRawBytes, capacity.VictoriaMetricsRequiredBytes)
	capacity.NATSHeadroomPercent = headroom(capacity.NATSRawBytes, capacity.NATSRequiredBytes)
	return capacity, nil
}

func headroom(raw, required int64) float64 {
	if required <= 0 {
		return 0
	}
	return (float64(raw)/float64(required) - 1) * 100
}

// WeightedBytesPerEvent is the compressed average size of the workload.
func (w Workload) WeightedBytesPerEvent() float64 {
	total := 0.0
	for _, signal := range w.Signals {
		total += signal.Share * signal.CompressedBytesPerEvent
	}
	return total
}

// TFVars renders the Terraform variables that are derived from the profile.
// Region, availability zone and the shape of every role are owned by the
// profile; operator-specific settings (admin CIDR, SSH key) stay in
// terraform.tfvars.
func (p Profile) TFVars(source string) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Generated by benchctl env tfvars from %s. Do not edit.\n", source)
	fmt.Fprintf(&builder, "region = %s\n", strconv.Quote(p.Region))
	fmt.Fprintf(&builder, "availability_zone = %s\n", strconv.Quote(p.AvailabilityZone))
	fmt.Fprintf(&builder, "root_disk_gib = %d\n", p.RootDiskGiB)
	for _, role := range p.Roles {
		fmt.Fprintf(&builder, "%s_count = %d\n", role.Name, role.Count)
		fmt.Fprintf(&builder, "%s_instance_type = %s\n", role.Name, strconv.Quote(role.InstanceType))
		fmt.Fprintf(&builder, "%s_data_disks = %d\n", role.Name, role.DataDisks)
		fmt.Fprintf(&builder, "%s_data_disk_gib = %d\n", role.Name, role.DataDiskGiB)
		fmt.Fprintf(&builder, "%s_data_disk_throughput_mbps = %d\n", role.Name, role.DataDiskThroughputMBps)
		fmt.Fprintf(&builder, "%s_data_disk_iops = %d\n", role.Name, role.DataDiskIOPS)
	}
	return builder.String(), nil
}

// RolesByName returns the roles indexed by name.
func (p Profile) roleMap() (map[string]Role, error) {
	roles := make(map[string]Role, len(p.Roles))
	for _, role := range p.Roles {
		if _, ok := roles[role.Name]; ok {
			return nil, fmt.Errorf("role %q is duplicated", role.Name)
		}
		roles[role.Name] = role
	}
	return roles, nil
}

// SortedRoleNames returns the role names in alphabetical order.
func (p Profile) SortedRoleNames() []string {
	names := make([]string, 0, len(p.Roles))
	for _, role := range p.Roles {
		names = append(names, role.Name)
	}
	sort.Strings(names)
	return names
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("unexpected trailing JSON value")
	}
	return err
}

func formatBytes(value int64) string {
	switch {
	case value >= tib:
		return fmt.Sprintf("%.1f TiB", float64(value)/float64(tib))
	case value >= gib:
		return fmt.Sprintf("%.1f GiB", float64(value)/float64(gib))
	default:
		return fmt.Sprintf("%d B", value)
	}
}
