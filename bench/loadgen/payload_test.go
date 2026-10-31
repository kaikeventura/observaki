package loadgen

import (
	"bytes"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func TestMixPattern(t *testing.T) {
	tests := []struct {
		name string
		mix  Mix
		want [3]int
	}{
		{name: "default", mix: DefaultMix(), want: [3]int{60, 30, 10}},
		{name: "custom", mix: Mix{Metrics: 50, Logs: 40, Traces: 10}, want: [3]int{50, 40, 10}},
		{name: "traces heavy", mix: Mix{Metrics: 20, Logs: 30, Traces: 50}, want: [3]int{20, 30, 50}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pattern := test.mix.Pattern()
			if len(pattern) != 100 {
				t.Fatalf("Pattern() length = %d, want 100", len(pattern))
			}
			var counts [3]int
			for _, signal := range pattern {
				counts[signal]++
			}
			if counts != test.want {
				t.Fatalf("Pattern() counts = %v, want %v", counts, test.want)
			}
		})
	}

	if err := (Mix{Metrics: 60, Logs: 30, Traces: 20}).Validate(); err == nil {
		t.Fatal("Validate() = nil for a mix that does not sum to 100")
	}
}

func TestSignalForDistribution(t *testing.T) {
	cfg := DefaultConfig()
	builder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var counts [3]int
	for index := int64(0); index < 1000; index++ {
		counts[builder.SignalFor(index)]++
	}
	want := [3]int{600, 300, 100}
	if counts != want {
		t.Fatalf("signal distribution = %v, want %v", counts, want)
	}
}

func TestEventTimeIsMonotonic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rate = 1_000_000
	builder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	first := builder.EventTime(0)
	second := builder.EventTime(1)
	if second <= first {
		t.Fatalf("EventTime(1) = %d, want > %d", second, first)
	}
	if got, want := second-first, uint64(1000); got != want {
		t.Fatalf("EventTime step = %d ns, want %d ns", got, want)
	}
}

func TestEventTimeUsesConfiguredStart(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StartTime = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	builder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := builder.EventTime(0), uint64(cfg.StartTime.UnixNano()); got != want {
		t.Fatalf("EventTime(0) = %d, want %d", got, want)
	}
}

func TestNewBuilderDefaultsStartTime(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StartTime = time.Time{}
	builder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if builder.EventTime(0) == 0 {
		t.Fatal("EventTime(0) = 0, want a non-zero default start time")
	}
}

func TestBuildBatchIsDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	indices := []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	for _, signal := range AllSignals {
		t.Run(signal.String(), func(t *testing.T) {
			firstBuilder, err := NewBuilder(cfg)
			if err != nil {
				t.Fatal(err)
			}
			secondBuilder, err := NewBuilder(cfg)
			if err != nil {
				t.Fatal(err)
			}
			first, err := firstBuilder.BuildBatch(signal, indices)
			if err != nil {
				t.Fatal(err)
			}
			second, err := secondBuilder.BuildBatch(signal, indices)
			if err != nil {
				t.Fatal(err)
			}
			firstBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(first.Proto())
			if err != nil {
				t.Fatal(err)
			}
			secondBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(second.Proto())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(firstBytes, secondBytes) {
				t.Fatalf("%s batches are not deterministic", signal)
			}
		})
	}
}

func TestBuildBatchChangesWithSeed(t *testing.T) {
	cfg := DefaultConfig()
	indices := []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	firstBuilder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Seed = 2
	secondBuilder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstBuilder.BuildBatch(Logs, indices)
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondBuilder.BuildBatch(Logs, indices)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := proto.MarshalOptions{Deterministic: true}.Marshal(first.Proto())
	secondBytes, _ := proto.MarshalOptions{Deterministic: true}.Marshal(second.Proto())
	if bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("log batches are identical for different seeds")
	}
}

func TestPayloadSizesAreWithinTolerance(t *testing.T) {
	reports, weighted, err := MeasureSizes(referencePayloadConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !PayloadWithinTolerance(weighted) {
		t.Fatalf("weighted gzip size = %.1f B/event, outside the ±10%% tolerance", weighted)
	}

	targets := map[Signal]float64{
		Metrics: 40,
		Logs:    200,
		Traces:  500,
	}
	for _, report := range reports {
		target := targets[report.Signal]
		deviation := math.Abs(report.GzipBytesPerEvent-target) / target
		if deviation > PayloadTolerance {
			t.Fatalf("%s gzip size = %.1f B/event, %.1f%% off the %.0f B target",
				report.Signal, report.GzipBytesPerEvent, deviation*100, target)
		}
	}
}

func TestPayloadCompressionRatio(t *testing.T) {
	reports, _, err := MeasureSizes(referencePayloadConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, report := range reports {
		if report.Signal == Metrics {
			continue
		}
		ratio := report.ProtobufBytesPerEvent / report.GzipBytesPerEvent
		if ratio < 5 {
			t.Fatalf("%s compression ratio = %.2fx, want at least 5x", report.Signal, ratio)
		}
	}
}

func referencePayloadConfig() Config {
	cfg := DefaultConfig()
	cfg.StartTime = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	return cfg
}

func TestZeroPayloadSizesBuild(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Payload = PayloadSizes{}
	builder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, signal := range AllSignals {
		batch, err := builder.BuildBatch(signal, []int64{0, 1, 2})
		if err != nil {
			t.Fatal(err)
		}
		if batch.Events != 3 {
			t.Fatalf("%s batch events = %d, want 3", signal, batch.Events)
		}
	}
}

func TestNewBuilderRejectsZeroRate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rate = 0
	if _, err := NewBuilder(cfg); err == nil {
		t.Fatal("NewBuilder() = nil error for rate 0")
	}
}
