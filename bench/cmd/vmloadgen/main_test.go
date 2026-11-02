package main

import (
	"testing"
	"time"

	"github.com/observaki/observaki/bench/internal/vmbench"
)

func TestBuildCatalogLoad(t *testing.T) {
	catalog, err := buildCatalog(options{mode: "load", tenants: "tenant-a,tenant-b", series: 1_000})
	if err != nil {
		t.Fatalf("buildCatalog: %v", err)
	}
	if len(catalog.Series) != 1_000 {
		t.Fatalf("series = %d, want 1000", len(catalog.Series))
	}
	if len(catalog.Tenants) != 2 {
		t.Fatalf("tenants = %d, want 2", len(catalog.Tenants))
	}
}

func TestBuildCatalogProbe(t *testing.T) {
	catalog, err := buildCatalog(options{mode: "probe", probeTenant: "tenant-a", series: 500, metric: "probe_total"})
	if err != nil {
		t.Fatalf("buildCatalog: %v", err)
	}
	if len(catalog.Series) != 500 || catalog.Series[0].TenantID != "tenant-a" {
		t.Fatalf("probe catalog = %d series, tenant %q", len(catalog.Series), catalog.Series[0].TenantID)
	}
}

func TestReportStatus(t *testing.T) {
	run := &loadRun{
		opts:    options{rate: 1_000, duration: time.Minute, workers: 1, batchSeries: 100},
		catalog: &vmbench.Catalog{},
		start:   time.Now().Add(-time.Minute),
	}
	run.sent.Store(60_000)
	report := run.report(false)
	if report.Status != "pass" {
		t.Fatalf("status = %q, want pass: %+v", report.Status, report)
	}
	run.errors.Store(1)
	report = run.report(false)
	if report.Status != "fail" {
		t.Fatalf("status = %q, want fail", report.Status)
	}
}

func TestReportAllowsRejections(t *testing.T) {
	run := &loadRun{
		opts:    options{rate: 1_000, duration: time.Minute, workers: 1, allowRejections: true},
		catalog: &vmbench.Catalog{},
		start:   time.Now().Add(-time.Minute),
	}
	run.sent.Store(60_000)
	run.rejected.Store(10)
	if got := run.report(false).Status; got != "pass" {
		t.Fatalf("status = %q, want pass when rejections are expected", got)
	}
}

func TestReportInterrupted(t *testing.T) {
	run := &loadRun{
		opts:    options{rate: 1_000, duration: time.Minute, workers: 1},
		catalog: &vmbench.Catalog{},
		start:   time.Now().Add(-time.Second),
	}
	run.sent.Store(1_000)
	if got := run.report(true).Status; got != "interrupted" {
		t.Fatalf("status = %q, want interrupted", got)
	}
}
