package envprofile

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func referencePaths(t *testing.T) (string, string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "bench", "env", "t1.json"), filepath.Join(root, "bench", "env", "prices.aws-us-east-1.json")
}

func loadReference(t *testing.T) (*Profile, *Prices) {
	t.Helper()
	profilePath, pricesPath := referencePaths(t)
	profile, err := LoadProfile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	prices, err := LoadPrices(pricesPath)
	if err != nil {
		t.Fatal(err)
	}
	return profile, prices
}

func TestReferenceProfile(t *testing.T) {
	profile, _ := loadReference(t)
	if err := profile.Validate(); err != nil {
		t.Fatalf("reference profile is invalid: %v", err)
	}
	if got, want := profile.Workload.EventsPerSecond, int64(1_000_000); got != want {
		t.Fatalf("events_per_second = %d, want %d", got, want)
	}
	if got, want := math.Round(profile.Workload.WeightedBytesPerEvent()), 134.0; got != want {
		t.Fatalf("weighted bytes/event = %.1f, want %.1f", got, want)
	}
	capacity, err := profile.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if capacity.ClickHouseHeadroomPercent < 10 {
		t.Errorf("clickhouse headroom = %.1f%%, want at least 10%%", capacity.ClickHouseHeadroomPercent)
	}
	if capacity.VictoriaMetricsHeadroomPercent < 10 {
		t.Errorf("victoriametrics headroom = %.1f%%, want at least 10%%", capacity.VictoriaMetricsHeadroomPercent)
	}
	if capacity.NATSHeadroomPercent < 10 {
		t.Errorf("nats headroom = %.1f%%, want at least 10%%", capacity.NATSHeadroomPercent)
	}
	if capacity.GatewayCapacity < profile.Workload.EventsPerSecond {
		t.Errorf("gateway capacity = %d, want at least %d", capacity.GatewayCapacity, profile.Workload.EventsPerSecond)
	}
	if capacity.LoadgenCapacity < profile.Workload.EventsPerSecond {
		t.Errorf("loadgen capacity = %d, want at least %d", capacity.LoadgenCapacity, profile.Workload.EventsPerSecond)
	}
	if got, want := math.Round(capacity.TerabytesPerDay*100)/100, 11.58; got != want {
		t.Errorf("terabytes/day = %.2f, want %.2f", got, want)
	}
}

func TestReferencePricesAndEstimate(t *testing.T) {
	profile, prices := loadReference(t)
	if err := prices.Validate(profile); err != nil {
		t.Fatalf("reference prices are invalid: %v", err)
	}
	cost, err := Estimate(profile, prices, 4)
	if err != nil {
		t.Fatal(err)
	}
	if cost.Currency != "USD" {
		t.Errorf("currency = %q, want USD", cost.Currency)
	}
	if cost.HourlyTotal <= 0 {
		t.Fatalf("hourly total = %f, want > 0", cost.HourlyTotal)
	}
	if got, want := cost.RunTotal, cost.HourlyTotal*4; math.Abs(got-want) > 1e-9 {
		t.Errorf("run total = %f, want %f", got, want)
	}
	if got, want := cost.MonthlyTotal, cost.HourlyTotal*prices.HoursPerMonth; math.Abs(got-want) > 1e-6 {
		t.Errorf("monthly total = %f, want %f", got, want)
	}
	if len(cost.Roles) != len(profile.Roles) {
		t.Fatalf("role costs = %d, want %d", len(cost.Roles), len(profile.Roles))
	}
	sum := 0.0
	for _, role := range cost.Roles {
		hourly := role.InstanceHourly + role.StorageHourly + role.ThroughputHourly + role.IOPSHourly + role.PublicIPv4Hourly
		if math.Abs(hourly-role.HourlyTotal) > 1e-9 {
			t.Errorf("role %s hourly breakdown = %f, want %f", role.Name, hourly, role.HourlyTotal)
		}
		sum += role.HourlyTotal
	}
	if math.Abs(sum-cost.HourlyTotal) > 1e-9 {
		t.Errorf("sum of role costs = %f, want %f", sum, cost.HourlyTotal)
	}
	if cost.TerabytesPerDay <= 0 {
		t.Fatalf("terabytes/day = %f, want > 0", cost.TerabytesPerDay)
	}
	if got, want := cost.CostPerTBDay, cost.HourlyTotal*24/cost.TerabytesPerDay; math.Abs(got-want) > 1e-9 {
		t.Errorf("cost per TB/day = %f, want %f", got, want)
	}
}

func TestProfileProblems(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Profile)
		want   string
	}{
		{
			name:   "too few clickhouse nodes",
			mutate: func(p *Profile) { p.Roles[0].Count = 5 },
			want:   "clickhouse count = 5",
		},
		{
			name:   "wrong victoriametrics count",
			mutate: func(p *Profile) { p.Roles[1].Count = 2 },
			want:   "victoriametrics count = 2",
		},
		{
			name:   "too many gateways",
			mutate: func(p *Profile) { p.Roles[2].Count = 7 },
			want:   "gateway count = 7",
		},
		{
			name: "signal shares do not sum to one",
			mutate: func(p *Profile) {
				metrics := p.Workload.Signals["metrics"]
				metrics.Share = 0.5
				p.Workload.Signals["metrics"] = metrics
			},
			want: "signal shares sum to 0.9000",
		},
		{
			name: "clickhouse disk is too small for retention",
			mutate: func(p *Profile) {
				p.Roles[0].DataDisks = 1
				p.Roles[0].DataDiskGiB = 1024
			},
			want: "clickhouse raw capacity",
		},
		{
			name:   "missing loadgen",
			mutate: func(p *Profile) { p.Roles = p.Roles[:len(p.Roles)-1] },
			want:   "required role \"loadgen\" is missing",
		},
		{
			name: "duplicate role",
			mutate: func(p *Profile) {
				p.Roles = append(p.Roles, p.Roles[0])
			},
			want: "duplicated",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile, _ := loadReference(t)
			test.mutate(profile)
			err := profile.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %q, want it to contain %q", err.Error(), test.want)
			}
		})
	}
}

func TestPricesProblems(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Prices)
		want   string
	}{
		{
			name:   "missing instance type",
			mutate: func(p *Prices) { delete(p.InstanceHourly, "m6i.4xlarge") },
			want:   "m6i.4xlarge",
		},
		{
			name:   "volume over the provider limit",
			mutate: func(p *Prices) { p.MaxVolumeGiB = 1024 },
			want:   "exceeds max_volume_gib",
		},
		{
			name:   "zero hours per month",
			mutate: func(p *Prices) { p.HoursPerMonth = 0 },
			want:   "hours_per_month",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile, prices := loadReference(t)
			test.mutate(prices)
			err := prices.Validate(profile)
			if err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %q, want it to contain %q", err.Error(), test.want)
			}
		})
	}
}

func TestTFVars(t *testing.T) {
	profile, _ := loadReference(t)
	output, err := profile.TFVars("bench/env/t1.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"region = \"us-east-1\"",
		"availability_zone = \"us-east-1a\"",
		"root_disk_gib = 100",
		"clickhouse_count = 6",
		"clickhouse_data_disks = 2",
		"victoriametrics_count = 3",
		"gateway_count = 6",
		"nats_count = 4",
		"loadgen_count = 2",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("tfvars output is missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"admin_cidr", "ssh_public_key"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("tfvars output must not contain operator setting %q:\n%s", unwanted, output)
		}
	}
}

func TestTFVarsDeterministic(t *testing.T) {
	profile, _ := loadReference(t)
	first, err := profile.TFVars("bench/env/t1.json")
	if err != nil {
		t.Fatal(err)
	}
	second, err := profile.TFVars("bench/env/t1.json")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("TFVars output is not deterministic")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile(path); err == nil {
		t.Fatal("LoadProfile() = nil, want an unknown-field error")
	}
}

func TestEstimateRejectsBadInputs(t *testing.T) {
	profile, prices := loadReference(t)
	if _, err := Estimate(profile, prices, 0); err == nil {
		t.Fatal("Estimate(hours=0) = nil, want an error")
	}
	if _, err := Estimate(nil, prices, 1); err == nil {
		t.Fatal("Estimate(nil profile) = nil, want an error")
	}
}
