package envprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

// Prices is a versioned snapshot of the unit prices used by the estimator.
// It is deliberately a file, not a live price API: a benchmark estimate must
// be reproducible and reviewable.
type Prices struct {
	SchemaVersion             int                `json:"schema_version"`
	Currency                  string             `json:"currency"`
	AsOf                      string             `json:"as_of"`
	Source                    string             `json:"source"`
	HoursPerMonth             float64            `json:"hours_per_month"`
	InstanceHourly            map[string]float64 `json:"instance_hourly"`
	EBSGiBMonth               float64            `json:"ebs_gib_month"`
	EBSIncludedThroughputMBps float64            `json:"ebs_included_throughput_mbps"`
	EBSThroughputMBpsMonth    float64            `json:"ebs_throughput_mbps_month"`
	EBSIncludedIOPS           float64            `json:"ebs_included_iops"`
	EBSIOPSMonth              float64            `json:"ebs_iops_month"`
	PublicIPv4Hourly          float64            `json:"public_ipv4_hourly"`
	MaxVolumeGiB              int                `json:"max_volume_gib"`
	Notes                     []string           `json:"notes,omitempty"`
}

// LoadPrices reads and strictly decodes a price table.
func LoadPrices(path string) (*Prices, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read prices %s: %w", path, err)
	}
	var prices Prices
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&prices); err != nil {
		return nil, fmt.Errorf("decode prices %s: %w", path, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode prices %s: %w", path, err)
	}
	return &prices, nil
}

// Validate checks the price table against a profile.
func (p Prices) Validate(profile *Profile) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if p.SchemaVersion != SchemaVersion {
		add("schema_version = %d, want %d", p.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(p.Currency) == "" {
		add("currency is required")
	}
	if strings.TrimSpace(p.AsOf) == "" {
		add("as_of is required")
	}
	if strings.TrimSpace(p.Source) == "" {
		add("source is required")
	}
	if p.HoursPerMonth <= 0 {
		add("hours_per_month must be greater than zero")
	}
	if p.EBSGiBMonth < 0 || p.EBSIncludedThroughputMBps < 0 || p.EBSThroughputMBpsMonth < 0 ||
		p.EBSIncludedIOPS < 0 || p.EBSIOPSMonth < 0 || p.PublicIPv4Hourly < 0 {
		add("price values must not be negative")
	}
	if p.MaxVolumeGiB < 1 {
		add("max_volume_gib must be greater than zero")
	}
	if profile == nil {
		add("profile is required")
	} else {
		for _, role := range profile.Roles {
			price, ok := p.InstanceHourly[role.InstanceType]
			if !ok {
				add("instance_hourly[%q] is missing for role %s", role.InstanceType, role.Name)
				continue
			}
			if price <= 0 {
				add("instance_hourly[%q] must be greater than zero", role.InstanceType)
			}
			if role.DataDisks > 0 && role.DataDiskGiB > p.MaxVolumeGiB {
				add("role %s data_disk_gib = %d exceeds max_volume_gib = %d", role.Name, role.DataDiskGiB, p.MaxVolumeGiB)
			}
		}
	}
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// RoleCost is the estimated cost of one role.
type RoleCost struct {
	Name             string  `json:"name"`
	Count            int     `json:"count"`
	InstanceType     string  `json:"instance_type"`
	InstanceHourly   float64 `json:"instance_hourly"`
	StorageHourly    float64 `json:"storage_hourly"`
	ThroughputHourly float64 `json:"throughput_hourly"`
	IOPSHourly       float64 `json:"iops_hourly"`
	PublicIPv4Hourly float64 `json:"public_ipv4_hourly"`
	HourlyTotal      float64 `json:"hourly_total"`
	MonthlyTotal     float64 `json:"monthly_total"`
}

// Cost is the estimated cost of the reference cluster.
type Cost struct {
	Currency        string     `json:"currency"`
	Hours           float64    `json:"hours"`
	HourlyTotal     float64    `json:"hourly_total"`
	RunTotal        float64    `json:"run_total"`
	MonthlyTotal    float64    `json:"monthly_total"`
	TerabytesPerDay float64    `json:"terabytes_per_day"`
	CostPerTBDay    float64    `json:"cost_per_tb_day"`
	Roles           []RoleCost `json:"roles"`
	Assumptions     []string   `json:"assumptions"`
}

// Estimate computes the on-demand cost of the profile for a run of hours.
// It never invents a price: a missing instance type is an error.
func Estimate(profile *Profile, prices *Prices, hours float64) (Cost, error) {
	if profile == nil {
		return Cost{}, fmt.Errorf("profile is required")
	}
	if prices == nil {
		return Cost{}, fmt.Errorf("prices are required")
	}
	if hours <= 0 {
		return Cost{}, fmt.Errorf("hours must be greater than zero")
	}
	if err := profile.Validate(); err != nil {
		return Cost{}, err
	}
	if err := prices.Validate(profile); err != nil {
		return Cost{}, err
	}

	capacity, err := profile.Capacity()
	if err != nil {
		return Cost{}, err
	}

	cost := Cost{
		Currency:        prices.Currency,
		Hours:           hours,
		TerabytesPerDay: capacity.TerabytesPerDay,
		Roles:           make([]RoleCost, 0, len(profile.Roles)),
		Assumptions: []string{
			fmt.Sprintf("on-demand prices: %s (as of %s)", prices.Source, prices.AsOf),
			"single availability zone; no cross-AZ or internet egress is included",
			"one public IPv4 address per node and gp3 root/data volumes",
			"no backups, object storage, support plan or taxes",
			fmt.Sprintf("%.0f hours per month", prices.HoursPerMonth),
		},
	}
	if len(prices.Notes) > 0 {
		cost.Assumptions = append(cost.Assumptions, prices.Notes...)
	}

	for _, role := range profile.Roles {
		instancePrice, ok := prices.InstanceHourly[role.InstanceType]
		if !ok {
			return Cost{}, fmt.Errorf("instance_hourly[%q] is missing for role %s", role.InstanceType, role.Name)
		}
		storageGiB := float64(profile.RootDiskGiB+role.DataDisks*role.DataDiskGiB) * float64(role.Count)
		storageHourly := storageGiB * prices.EBSGiBMonth / prices.HoursPerMonth

		throughputPerVolume := math.Max(0, float64(role.DataDiskThroughputMBps)-prices.EBSIncludedThroughputMBps)
		throughputHourly := float64(role.Count*role.DataDisks) * throughputPerVolume * prices.EBSThroughputMBpsMonth / prices.HoursPerMonth

		iopsPerVolume := math.Max(0, float64(role.DataDiskIOPS)-prices.EBSIncludedIOPS)
		iopsHourly := float64(role.Count*role.DataDisks) * iopsPerVolume * prices.EBSIOPSMonth / prices.HoursPerMonth

		roleCost := RoleCost{
			Name:             role.Name,
			Count:            role.Count,
			InstanceType:     role.InstanceType,
			InstanceHourly:   instancePrice * float64(role.Count),
			StorageHourly:    storageHourly,
			ThroughputHourly: throughputHourly,
			IOPSHourly:       iopsHourly,
			PublicIPv4Hourly: prices.PublicIPv4Hourly * float64(role.Count),
		}
		roleCost.HourlyTotal = roleCost.InstanceHourly + roleCost.StorageHourly + roleCost.ThroughputHourly + roleCost.IOPSHourly + roleCost.PublicIPv4Hourly
		roleCost.MonthlyTotal = roleCost.HourlyTotal * prices.HoursPerMonth
		cost.HourlyTotal += roleCost.HourlyTotal
		cost.Roles = append(cost.Roles, roleCost)
	}
	cost.RunTotal = cost.HourlyTotal * hours
	cost.MonthlyTotal = cost.HourlyTotal * prices.HoursPerMonth
	if cost.TerabytesPerDay > 0 {
		cost.CostPerTBDay = cost.HourlyTotal * 24 / cost.TerabytesPerDay
	}
	return cost, nil
}

// SortedInstanceTypes returns the price table keys in alphabetical order.
func (p Prices) SortedInstanceTypes() []string {
	types := make([]string, 0, len(p.InstanceHourly))
	for instanceType := range p.InstanceHourly {
		types = append(types, instanceType)
	}
	sort.Strings(types)
	return types
}
