package vmbench

import (
	"math"
	"testing"
	"time"
)

func TestDefaultFamilyWeights(t *testing.T) {
	total := 0
	for _, family := range defaultFamilies() {
		total += family.Weight
	}
	if total != DefaultSeriesPerTenant {
		t.Fatalf("default family weights = %d, want %d", total, DefaultSeriesPerTenant)
	}
}

func TestNewCatalogSeriesCountAndUniqueness(t *testing.T) {
	catalog, err := NewCatalog(CatalogOptions{
		Tenants:         []string{"tenant-a", "tenant-b"},
		SeriesPerTenant: 5_000,
	})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if len(catalog.Series) != 10_000 {
		t.Fatalf("series = %d, want 10000", len(catalog.Series))
	}
	seen := make(map[string]struct{}, len(catalog.Series))
	perTenant := map[string]int{}
	for i := range catalog.Series {
		labels := catalog.Labels(&catalog.Series[i])
		key := ""
		for _, label := range labels {
			key += label.Name + "=" + label.Value + ";"
		}
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate series %q", key)
		}
		seen[key] = struct{}{}
		perTenant[catalog.Series[i].TenantID]++
	}
	if perTenant["tenant-a"] != 5_000 || perTenant["tenant-b"] != 5_000 {
		t.Fatalf("per-tenant series = %+v, want 5000 each", perTenant)
	}
}

func TestNewCatalogDefaultSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1M-series allocation in -short mode")
	}
	catalog, err := NewCatalog(CatalogOptions{Tenants: []string{"tenant-a", "tenant-b"}})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if len(catalog.Series) != 1_000_000 {
		t.Fatalf("series = %d, want 1000000", len(catalog.Series))
	}
}

func TestSampleAtCounterIsMonotonic(t *testing.T) {
	catalog, err := NewCatalog(CatalogOptions{Tenants: []string{"tenant-a"}, SeriesPerTenant: 5_000})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	start := time.Unix(1_700_000_000, 0)
	for i := range catalog.Series {
		series := &catalog.Series[i]
		if catalog.Families[series.FamilyIndex].Kind != Counter {
			continue
		}
		first := catalog.SampleAt(series, start, start)
		second := catalog.SampleAt(series, start, start.Add(time.Hour))
		if second <= first {
			t.Fatalf("counter %s did not increase: %f -> %f", catalog.Families[series.FamilyIndex].Name, first, second)
		}
		return
	}
	t.Fatal("no counter series found")
}

func TestHistogramComponentsAreConsistent(t *testing.T) {
	catalog, err := NewCatalog(CatalogOptions{Tenants: []string{"tenant-a"}, SeriesPerTenant: 50_000})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	var bucketIndex, sumIndex, countIndex = -1, -1, -1
	for i := range catalog.Series {
		name := catalog.Families[catalog.Series[i].FamilyIndex].Name
		switch name {
		case "http_request_duration_seconds_bucket":
			if bucketIndex == -1 && catalog.Series[i].Le == math.Inf(1) {
				bucketIndex = i
			}
		case "http_request_duration_seconds_sum":
			if sumIndex == -1 {
				sumIndex = i
			}
		case "http_request_duration_seconds_count":
			if countIndex == -1 {
				countIndex = i
			}
		}
	}
	if bucketIndex < 0 || sumIndex < 0 || countIndex < 0 {
		t.Fatalf("missing histogram components: bucket=%d sum=%d count=%d", bucketIndex, sumIndex, countIndex)
	}
	bucket := &catalog.Series[bucketIndex]
	sum := &catalog.Series[sumIndex]
	count := &catalog.Series[countIndex]
	if bucket.Base != sum.Base || bucket.Rate != sum.Rate || bucket.Latency != sum.Latency {
		t.Fatalf("histogram components diverge: bucket=%+v sum=%+v", bucket, sum)
	}
	if sum.Base != count.Base || sum.Rate != count.Rate || sum.Latency != count.Latency {
		t.Fatalf("histogram components diverge: sum=%+v count=%+v", sum, count)
	}
	start := time.Unix(1_700_000_000, 0)
	at := start.Add(10 * time.Minute)
	if got, want := catalog.SampleAt(bucket, start, at), catalog.SampleAt(count, start, at); math.Abs(got-want) > 1e-9 {
		t.Fatalf("+Inf bucket = %f, want count %f", got, want)
	}
	if got, want := catalog.SampleAt(sum, start, at)/catalog.SampleAt(count, start, at), bucket.Latency; math.Abs(got-want) > 1e-9 {
		t.Fatalf("sum/count = %f, want latency %f", got, want)
	}
}

func TestProbeCatalog(t *testing.T) {
	catalog, err := NewProbeCatalog("tenant-a", "observaki_cardinality_probe_total", 1_000)
	if err != nil {
		t.Fatalf("NewProbeCatalog: %v", err)
	}
	if len(catalog.Series) != 1_000 {
		t.Fatalf("series = %d, want 1000", len(catalog.Series))
	}
	for i := range catalog.Series {
		if catalog.Series[i].TenantID != "tenant-a" {
			t.Fatalf("series %d tenant = %q", i, catalog.Series[i].TenantID)
		}
	}
}

func TestParseTenants(t *testing.T) {
	got := ParseTenants(" tenant-a, tenant-b ,,")
	if len(got) != 2 || got[0] != "tenant-a" || got[1] != "tenant-b" {
		t.Fatalf("ParseTenants = %v", got)
	}
}
