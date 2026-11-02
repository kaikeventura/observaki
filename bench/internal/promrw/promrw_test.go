package promrw

import (
	"math"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	req := &WriteRequest{Timeseries: []TimeSeries{
		{
			Labels: []Label{
				{Name: "__name__", Value: "http_requests_total"},
				{Name: "tenant_id", Value: "tenant-a"},
				{Name: "service", Value: "checkout"},
			},
			Samples: []Sample{
				{Value: 1.5, Timestamp: 1_700_000_000_000},
				{Value: 2.5, Timestamp: 1_700_000_001_000},
			},
		},
		{
			Labels:  []Label{{Name: "__name__", Value: "up"}},
			Samples: []Sample{{Value: 1, Timestamp: 1_700_000_000_000}},
		},
	}}

	raw, err := Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Unmarshal(raw)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got.Timeseries) != len(req.Timeseries) {
		t.Fatalf("series = %d, want %d", len(got.Timeseries), len(req.Timeseries))
	}
	for i := range req.Timeseries {
		if len(got.Timeseries[i].Labels) != len(req.Timeseries[i].Labels) {
			t.Fatalf("series %d labels = %d, want %d", i, len(got.Timeseries[i].Labels), len(req.Timeseries[i].Labels))
		}
		for j := range req.Timeseries[i].Labels {
			if got.Timeseries[i].Labels[j] != req.Timeseries[i].Labels[j] {
				t.Fatalf("series %d label %d = %+v, want %+v", i, j, got.Timeseries[i].Labels[j], req.Timeseries[i].Labels[j])
			}
		}
		if len(got.Timeseries[i].Samples) != len(req.Timeseries[i].Samples) {
			t.Fatalf("series %d samples = %d, want %d", i, len(got.Timeseries[i].Samples), len(req.Timeseries[i].Samples))
		}
		for j := range req.Timeseries[i].Samples {
			if got.Timeseries[i].Samples[j] != req.Timeseries[i].Samples[j] {
				t.Fatalf("series %d sample %d = %+v, want %+v", i, j, got.Timeseries[i].Samples[j], req.Timeseries[i].Samples[j])
			}
		}
	}
}

func TestCompressedRoundTrip(t *testing.T) {
	req := &WriteRequest{Timeseries: []TimeSeries{{
		Labels:  []Label{{Name: "__name__", Value: "metric"}, {Name: "tenant_id", Value: "t"}},
		Samples: []Sample{{Value: math.Inf(1), Timestamp: 42}},
	}}}
	body, err := EncodeCompressed(req)
	if err != nil {
		t.Fatalf("EncodeCompressed: %v", err)
	}
	got, err := DecodeCompressed(body)
	if err != nil {
		t.Fatalf("DecodeCompressed: %v", err)
	}
	if len(got.Timeseries) != 1 || got.Timeseries[0].Samples[0].Value != math.Inf(1) {
		t.Fatalf("decoded request = %+v", got)
	}
}

func TestAppendTimeSeriesMatchesMarshal(t *testing.T) {
	labels := []Label{
		{Name: "__name__", Value: "metric"},
		{Name: "tenant_id", Value: "tenant-a"},
	}
	SortLabels(labels)
	encoded := EncodeLabels(labels)

	one := AppendTimeSeries(nil, encoded, 12.5, 1_700_000_000_123)
	request := AppendWriteRequest(nil, [][]byte{one})
	got, err := Unmarshal(request)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := &WriteRequest{Timeseries: []TimeSeries{{
		Labels:  labels,
		Samples: []Sample{{Value: 12.5, Timestamp: 1_700_000_000_123}},
	}}}
	if len(got.Timeseries) != 1 {
		t.Fatalf("series = %d, want 1", len(got.Timeseries))
	}
	if got.Timeseries[0].Samples[0] != want.Timeseries[0].Samples[0] {
		t.Fatalf("sample = %+v, want %+v", got.Timeseries[0].Samples[0], want.Timeseries[0].Samples[0])
	}
	if len(got.Timeseries[0].Labels) != len(want.Timeseries[0].Labels) {
		t.Fatalf("labels = %d, want %d", len(got.Timeseries[0].Labels), len(want.Timeseries[0].Labels))
	}
	for i := range want.Timeseries[0].Labels {
		if got.Timeseries[0].Labels[i] != want.Timeseries[0].Labels[i] {
			t.Fatalf("label %d = %+v, want %+v", i, got.Timeseries[0].Labels[i], want.Timeseries[0].Labels[i])
		}
	}
}

func TestAppendTimeSeriesFieldMatchesMarshal(t *testing.T) {
	labels := []Label{{Name: "__name__", Value: "metric"}, {Name: "tenant_id", Value: "tenant-a"}}
	SortLabels(labels)
	encoded := EncodeLabels(labels)
	request := AppendTimeSeriesField(nil, encoded, 7.25, 1_700_000_000_456)
	got, err := Unmarshal(request)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got.Timeseries) != 1 || len(got.Timeseries[0].Samples) != 1 {
		t.Fatalf("decoded request = %+v", got)
	}
	if got.Timeseries[0].Samples[0].Value != 7.25 || got.Timeseries[0].Samples[0].Timestamp != 1_700_000_000_456 {
		t.Fatalf("sample = %+v", got.Timeseries[0].Samples[0])
	}
}

func TestSortLabels(t *testing.T) {
	labels := []Label{{Name: "z", Value: "1"}, {Name: "__name__", Value: "m"}, {Name: "a", Value: "2"}}
	SortLabels(labels)
	if labels[0].Name != "__name__" || labels[1].Name != "a" || labels[2].Name != "z" {
		t.Fatalf("labels not sorted: %+v", labels)
	}
}

func TestUnmarshalRejectsTruncatedBody(t *testing.T) {
	if _, err := Unmarshal([]byte{0x0a, 0x05, 0x00}); err == nil {
		t.Fatal("Unmarshal accepted a truncated body")
	}
	if _, err := DecodeCompressed([]byte("not snappy")); err == nil {
		t.Fatal("DecodeCompressed accepted invalid snappy")
	}
}

func TestMarshalRejectsEmptyLabels(t *testing.T) {
	if _, err := Marshal(&WriteRequest{Timeseries: []TimeSeries{{Samples: []Sample{{Value: 1}}}}}); err == nil {
		t.Fatal("Marshal accepted a series without labels")
	}
}
