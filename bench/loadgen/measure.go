package loadgen

import (
	"bytes"
	"compress/gzip"
	"fmt"

	"google.golang.org/protobuf/proto"
)

const (
	// TargetCompressedBytesPerEvent is the reference workload average after
	// gzip compression.
	TargetCompressedBytesPerEvent = 134.0
	// PayloadTolerance is the allowed relative deviation from the target.
	PayloadTolerance = 0.10
)

// SizeReport is the measured size of one signal batch.
type SizeReport struct {
	Signal                Signal  `json:"signal"`
	Events                int     `json:"events"`
	ProtobufBytes         int     `json:"protobuf_bytes"`
	GzipBytes             int     `json:"gzip_bytes"`
	ProtobufBytesPerEvent float64 `json:"protobuf_bytes_per_event"`
	GzipBytesPerEvent     float64 `json:"gzip_bytes_per_event"`
}

// MeasureSizes builds one batch per signal, compresses it and reports the
// measured bytes per event. The weighted average uses the configured mix.
func MeasureSizes(cfg Config) ([]SizeReport, float64, error) {
	if cfg.BatchSize <= 0 {
		return nil, 0, fmt.Errorf("batch size must be greater than zero")
	}
	if err := cfg.Mix.Validate(); err != nil {
		return nil, 0, err
	}

	builder, err := NewBuilder(cfg)
	if err != nil {
		return nil, 0, err
	}

	reports := make([]SizeReport, 0, len(AllSignals))
	weighted := 0.0
	for _, signal := range AllSignals {
		indices := make([]int64, cfg.BatchSize)
		for i := range indices {
			indices[i] = int64(i)
		}
		batch, err := builder.BuildBatch(signal, indices)
		if err != nil {
			return nil, 0, err
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(batch.Proto())
		if err != nil {
			return nil, 0, fmt.Errorf("marshal %s: %w", signal, err)
		}
		compressed, err := gzipBytes(raw)
		if err != nil {
			return nil, 0, fmt.Errorf("compress %s: %w", signal, err)
		}
		report := SizeReport{
			Signal:                signal,
			Events:                batch.Events,
			ProtobufBytes:         len(raw),
			GzipBytes:             len(compressed),
			ProtobufBytesPerEvent: float64(len(raw)) / float64(batch.Events),
			GzipBytesPerEvent:     float64(len(compressed)) / float64(batch.Events),
		}
		reports = append(reports, report)
		weighted += cfg.Mix.Percent(signal) / 100 * report.GzipBytesPerEvent
	}
	return reports, weighted, nil
}

// PayloadWithinTolerance reports whether the weighted compressed size is
// within 10% of the 134 B/event target.
func PayloadWithinTolerance(bytesPerEvent float64) bool {
	lower := TargetCompressedBytesPerEvent * (1 - PayloadTolerance)
	upper := TargetCompressedBytesPerEvent * (1 + PayloadTolerance)
	return bytesPerEvent >= lower && bytesPerEvent <= upper
}

func gzipBytes(data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
