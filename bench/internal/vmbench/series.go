// Package vmbench contains the deterministic metric series catalog and the
// MetricsQL client used by the VictoriaMetrics spike (Story 3.4). It is a
// benchmark harness, not product code.
package vmbench

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/observaki/observaki/internal/promrw"
)

// Kind is the sample generator for a metric family.
type Kind int

const (
	// Counter is a monotonically increasing counter.
	Counter Kind = iota
	// Gauge oscillates around a base value.
	Gauge
	// HistogramBucket is a cumulative histogram bucket.
	HistogramBucket
	// HistogramSum is the sum of observations for a histogram.
	HistogramSum
	// HistogramCount is the observation count for a histogram.
	HistogramCount
)

// Dimension is one label dimension of a metric family.
type Dimension struct {
	Name   string
	Values []string
}

// Family describes one metric family in the synthetic catalog.
type Family struct {
	Name  string
	Kind  Kind
	Dims  []Dimension
	Count int
	// Weight is the relative share of the tenant's series budget. The default
	// families sum to 500,000 series per tenant.
	Weight int
}

// Series is one pre-encoded time series.
type Series struct {
	LabelBytes  []byte
	TenantID    string
	ProjectID   string
	FamilyIndex int
	Index       int
	// Base, Rate, Phase and Latency drive SampleAt. Le is the bucket upper
	// bound for HistogramBucket series (math.Inf(1) for +Inf).
	Base    float64
	Rate    float64
	Phase   float64
	Latency float64
	Le      float64
}

// CatalogOptions configures the synthetic series catalog.
type CatalogOptions struct {
	Tenants []string
	// SeriesPerTenant is the number of active series per tenant. The default
	// is 500,000, which gives 1,000,000 series for two tenants.
	SeriesPerTenant int
	// Metric is the metric name used by probe catalogs.
	Metric string
}

// Catalog is a deterministic set of active series.
type Catalog struct {
	Series    []Series
	Families  []Family
	Tenants   []string
	PerTenant int
}

// DefaultSeriesPerTenant is the Story 3.4 target: 1M active series across two
// tenants.
const DefaultSeriesPerTenant = 500_000

var (
	methods       = []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	httpStatuses  = []string{"200", "201", "204", "301", "400", "401", "403", "404", "500", "503"}
	spanStatuses  = []string{"OK", "ERROR", "UNSET", "CANCELLED", "DEADLINE_EXCEEDED", "INTERNAL", "UNAVAILABLE", "RESOURCE_EXHAUSTED", "INVALID_ARGUMENT", "NOT_FOUND"}
	histogramLens = []string{"0.005", "0.01", "0.025", "0.05", "0.1", "0.25", "0.5", "1", "2.5", "5", "10", "+Inf"}
)

func numbered(prefix string, count int, width int) []string {
	values := make([]string, count)
	for i := range values {
		values[i] = fmt.Sprintf("%s-%0*d", prefix, width, i)
	}
	return values
}

// defaultFamilies returns the family layout for one tenant. The weights sum to
// 500,000 series: the exact Story 3.4 target for two tenants.
func defaultFamilies() []Family {
	service200 := numbered("svc", 200, 3)
	service150 := numbered("svc", 150, 3)
	service250 := numbered("svc", 250, 3)
	service300 := numbered("svc", 300, 3)
	service500 := numbered("svc", 500, 3)
	instances5 := numbered("inst", 5, 2)
	instances10 := numbered("inst", 10, 2)
	instances50 := numbered("inst", 50, 2)
	namespaces := numbered("ns", 30, 2)
	pods := numbered("pod", 50, 3)
	containers := numbered("container", 10, 2)
	operations15 := numbered("op", 15, 2)
	operations5 := numbered("op", 5, 2)
	queues := numbered("queue", 45, 2)

	return []Family{
		{Name: "http_requests_total", Kind: Counter, Weight: 50_000, Dims: []Dimension{
			{Name: "service", Values: service200},
			{Name: "method", Values: methods},
			{Name: "status", Values: httpStatuses},
			{Name: "instance", Values: instances5},
		}},
		{Name: "http_request_duration_seconds_bucket", Kind: HistogramBucket, Weight: 40_000, Dims: []Dimension{
			{Name: "service", Values: service150},
			{Name: "method", Values: methods},
			{Name: "status", Values: httpStatuses[:7]},
			{Name: "le", Values: histogramLens},
		}},
		{Name: "http_request_duration_seconds_sum", Kind: HistogramSum, Weight: 3_333, Dims: []Dimension{
			{Name: "service", Values: service150},
			{Name: "method", Values: methods},
			{Name: "status", Values: httpStatuses[:7]},
		}},
		{Name: "http_request_duration_seconds_count", Kind: HistogramCount, Weight: 3_333, Dims: []Dimension{
			{Name: "service", Values: service150},
			{Name: "method", Values: methods},
			{Name: "status", Values: httpStatuses[:7]},
		}},
		{Name: "span_requests_total", Kind: Counter, Weight: 50_000, Dims: []Dimension{
			{Name: "service", Values: service500},
			{Name: "operation", Values: operations15},
			{Name: "status", Values: spanStatuses},
		}},
		{Name: "span_duration_seconds_bucket", Kind: HistogramBucket, Weight: 10_000, Dims: []Dimension{
			{Name: "service", Values: service250},
			{Name: "operation", Values: operations5},
			{Name: "le", Values: histogramLens},
		}},
		{Name: "span_duration_seconds_sum", Kind: HistogramSum, Weight: 833, Dims: []Dimension{
			{Name: "service", Values: service250},
			{Name: "operation", Values: operations5},
		}},
		{Name: "span_duration_seconds_count", Kind: HistogramCount, Weight: 833, Dims: []Dimension{
			{Name: "service", Values: service250},
			{Name: "operation", Values: operations5},
		}},
		{Name: "process_cpu_seconds_total", Kind: Counter, Weight: 15_000, Dims: []Dimension{
			{Name: "service", Values: service300},
			{Name: "instance", Values: instances50},
		}},
		{Name: "process_resident_memory_bytes", Kind: Gauge, Weight: 15_000, Dims: []Dimension{
			{Name: "service", Values: service300},
			{Name: "instance", Values: instances50},
		}},
		{Name: "go_goroutines", Kind: Gauge, Weight: 15_000, Dims: []Dimension{
			{Name: "service", Values: service300},
			{Name: "instance", Values: instances50},
		}},
		{Name: "go_memstats_alloc_bytes", Kind: Gauge, Weight: 15_000, Dims: []Dimension{
			{Name: "service", Values: service300},
			{Name: "instance", Values: instances50},
		}},
		{Name: "process_open_fds", Kind: Gauge, Weight: 15_000, Dims: []Dimension{
			{Name: "service", Values: service300},
			{Name: "instance", Values: instances50},
		}},
		{Name: "grpc_server_handled_total", Kind: Counter, Weight: 50_000, Dims: []Dimension{
			{Name: "service", Values: service200},
			{Name: "method", Values: methods},
			{Name: "status", Values: []string{"OK", "ERROR", "CANCELLED", "DEADLINE_EXCEEDED", "UNAVAILABLE"}},
			{Name: "instance", Values: instances10},
		}},
		{Name: "queue_depth", Kind: Gauge, Weight: 9_000, Dims: []Dimension{
			{Name: "service", Values: service200},
			{Name: "queue", Values: queues},
		}},
		// Infrastructure metrics fill the 1M-series budget without inflating
		// the dashboard families above. They are active series that a real
		// cluster reports, but the 20 representative queries do not scan them.
		{Name: "container_cpu_usage_seconds_total", Kind: Counter, Weight: 69_223, Dims: []Dimension{
			{Name: "namespace", Values: namespaces},
			{Name: "pod", Values: pods},
			{Name: "container", Values: containers},
			{Name: "instance", Values: instances5},
		}},
		{Name: "container_memory_working_set_bytes", Kind: Gauge, Weight: 69_223, Dims: []Dimension{
			{Name: "namespace", Values: namespaces},
			{Name: "pod", Values: pods},
			{Name: "container", Values: containers},
			{Name: "instance", Values: instances5},
		}},
		{Name: "kube_pod_container_status_restarts_total", Kind: Counter, Weight: 69_222, Dims: []Dimension{
			{Name: "namespace", Values: namespaces},
			{Name: "pod", Values: pods},
			{Name: "container", Values: containers},
			{Name: "instance", Values: instances5},
		}},
	}
}

// NewCatalog builds a deterministic catalog for the given tenants. Each
// tenant gets the same metric families with different tenant_id labels.
func NewCatalog(opts CatalogOptions) (*Catalog, error) {
	if len(opts.Tenants) == 0 {
		return nil, fmt.Errorf("vmbench: at least one tenant is required")
	}
	perTenant := opts.SeriesPerTenant
	if perTenant <= 0 {
		perTenant = DefaultSeriesPerTenant
	}
	families, err := scaleFamilies(defaultFamilies(), perTenant)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{Families: families, Tenants: append([]string(nil), opts.Tenants...), PerTenant: perTenant}
	for _, tenant := range opts.Tenants {
		globalIndex := 0
		for familyIndex, family := range families {
			for index := 0; index < family.Count; index++ {
				project := projectFor(globalIndex, perTenant)
				series := buildSeries(tenant, familyIndex, family, index, project)
				catalog.Series = append(catalog.Series, series)
				globalIndex++
			}
		}
	}
	return catalog, nil
}

// NewProbeCatalog builds a simple one-family catalog for the cardinality test.
// It is intentionally independent from the main 1M-series catalog so the
// admission proxy can start with a known per-tenant series count.
func NewProbeCatalog(tenant, metric string, series int) (*Catalog, error) {
	if tenant == "" {
		return nil, fmt.Errorf("vmbench: probe tenant is required")
	}
	if metric == "" {
		metric = "observaki_cardinality_probe_total"
	}
	if series <= 0 {
		return nil, fmt.Errorf("vmbench: probe series must be positive")
	}
	family := Family{
		Name:   metric,
		Kind:   Counter,
		Weight: series,
		Count:  series,
		Dims:   []Dimension{{Name: "series_id", Values: numbered("s", series, 6)}},
	}
	catalog := &Catalog{Families: []Family{family}, Tenants: []string{tenant}, PerTenant: series}
	for index := 0; index < series; index++ {
		catalog.Series = append(catalog.Series, buildSeries(tenant, 0, family, index, projectFor(index, series)))
	}
	return catalog, nil
}

func scaleFamilies(base []Family, perTenant int) ([]Family, error) {
	weightSum := 0
	for _, family := range base {
		weightSum += family.Weight
	}
	if weightSum != DefaultSeriesPerTenant {
		return nil, fmt.Errorf("vmbench: default family weights sum to %d, want %d", weightSum, DefaultSeriesPerTenant)
	}
	families := make([]Family, 0, len(base))
	assigned := 0
	for i, family := range base {
		count := perTenant * family.Weight / weightSum
		if i == len(base)-1 {
			count = perTenant - assigned
		}
		if count < 0 {
			return nil, fmt.Errorf("vmbench: negative count for family %s", family.Name)
		}
		if count == 0 {
			continue
		}
		capacity := 1
		for _, dim := range family.Dims {
			capacity *= len(dim.Values)
			if capacity > 1_000_000_000 {
				break
			}
		}
		if count > capacity {
			return nil, fmt.Errorf("vmbench: family %s count %d exceeds dimension capacity %d", family.Name, count, capacity)
		}
		family.Count = count
		assigned += count
		families = append(families, family)
	}
	if assigned != perTenant {
		return nil, fmt.Errorf("vmbench: assigned %d series, want %d", assigned, perTenant)
	}
	return families, nil
}

func projectFor(globalIndex, perTenant int) string {
	if perTenant > 0 && globalIndex >= perTenant/2 {
		return "project-1"
	}
	return "project-0"
}

func buildSeries(tenant string, familyIndex int, family Family, index int, project string) Series {
	values := dimensionValues(family.Dims, index)
	labels := make([]promrw.Label, 0, len(values)+3)
	labels = append(labels, promrw.Label{Name: "__name__", Value: family.Name})
	for i, dim := range family.Dims {
		labels = append(labels, promrw.Label{Name: dim.Name, Value: values[i]})
	}
	labels = append(labels,
		promrw.Label{Name: "tenant_id", Value: tenant},
		promrw.Label{Name: "project_id", Value: project},
	)
	promrw.SortLabels(labels)

	seedValues := values
	if family.Kind == HistogramBucket {
		// The histogram components share the same base/rate/latency for the
		// same non-le labels, so _sum/_count stay consistent with the buckets.
		seedValues = values[:len(values)-1]
	}
	seed := seriesSeed(familyGroup(family.Name), seedValues, tenant)
	unit := float64(seed%1_000_000) / 1_000_000
	series := Series{
		LabelBytes:  promrw.EncodeLabels(labels),
		TenantID:    tenant,
		ProjectID:   project,
		FamilyIndex: familyIndex,
		Index:       index,
		Base:        100 + unit*1_000,
		Rate:        0.05 + unit*0.5,
		Phase:       unit * 2 * math.Pi,
		Latency:     0.005 + unit*0.2,
	}
	if family.Kind == HistogramBucket {
		series.Le = parseLe(values[len(values)-1])
	}
	return series
}

func dimensionValues(dims []Dimension, index int) []string {
	values := make([]string, len(dims))
	remaining := index
	for i := len(dims) - 1; i >= 0; i-- {
		n := len(dims[i].Values)
		values[i] = dims[i].Values[remaining%n]
		remaining /= n
	}
	return values
}

func familyGroup(name string) string {
	for _, suffix := range []string{"_bucket", "_sum", "_count"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

// seriesSeed is a small deterministic FNV-1a hash. It only needs to be stable
// across runs, not cryptographic.
func seriesSeed(name string, values []string, tenant string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	hash := uint64(offset64)
	for _, value := range append([]string{name, tenant}, values...) {
		for i := 0; i < len(value); i++ {
			hash ^= uint64(value[i])
			hash *= prime64
		}
	}
	return hash
}

func parseLe(value string) float64 {
	if value == "+Inf" {
		return math.Inf(1)
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return math.Inf(1)
	}
	return parsed
}

// SampleAt returns the deterministic value of series at time t. start is the
// beginning of the load; counters and histograms are monotonic from it.
func (c *Catalog) SampleAt(series *Series, start time.Time, t time.Time) float64 {
	elapsed := t.Sub(start).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	family := c.Families[series.FamilyIndex]
	switch family.Kind {
	case Counter:
		return series.Base + series.Rate*elapsed
	case Gauge:
		return series.Base + 20*math.Sin(2*math.Pi*elapsed/300+series.Phase)
	case HistogramBucket:
		total := series.Base + series.Rate*elapsed
		return total * exponentialCDF(series.Le, series.Latency)
	case HistogramSum:
		total := series.Base + series.Rate*elapsed
		return total * series.Latency
	case HistogramCount:
		return series.Base + series.Rate*elapsed
	default:
		return 0
	}
}

// exponentialCDF is the cumulative distribution used by the synthetic
// histograms. It keeps bucket values cumulative and consistent with _sum and
// _count.
func exponentialCDF(le float64, mean float64) float64 {
	if math.IsInf(le, 1) {
		return 1
	}
	if le <= 0 || mean <= 0 {
		return 0
	}
	return 1 - math.Exp(-le/mean)
}

// FamilyByName returns the family with the given metric name.
func (c *Catalog) FamilyByName(name string) (Family, bool) {
	for _, family := range c.Families {
		if family.Name == name {
			return family, true
		}
	}
	return Family{}, false
}

// Labels reconstructs the labels of a series. It is used by tests and by the
// correctness verifier; the hot load path uses LabelBytes directly.
func (c *Catalog) Labels(series *Series) []promrw.Label {
	family := c.Families[series.FamilyIndex]
	values := dimensionValues(family.Dims, series.Index)
	labels := make([]promrw.Label, 0, len(values)+3)
	labels = append(labels, promrw.Label{Name: "__name__", Value: family.Name})
	for i, dim := range family.Dims {
		labels = append(labels, promrw.Label{Name: dim.Name, Value: values[i]})
	}
	labels = append(labels,
		promrw.Label{Name: "tenant_id", Value: series.TenantID},
		promrw.Label{Name: "project_id", Value: series.ProjectID},
	)
	promrw.SortLabels(labels)
	return labels
}

// SortedTenants returns the catalog tenants sorted for stable reporting.
func (c *Catalog) SortedTenants() []string {
	tenants := append([]string(nil), c.Tenants...)
	sort.Strings(tenants)
	return tenants
}

// ParseTenants parses a comma-separated tenant list.
func ParseTenants(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	tenants := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			tenants = append(tenants, part)
		}
	}
	return tenants
}
