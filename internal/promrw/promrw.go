// Package promrw implements the subset of the Prometheus remote write
// protocol needed by the VictoriaMetrics spike: a WriteRequest protobuf with
// labels and samples, snappy-compressed on the wire.
//
// The encoder is intentionally small and allocation-conscious: the load
// generator pre-encodes label sets once and appends only the sample for each
// request. The decoder is used by the admission proxy, which needs to read the
// tenant label before forwarding the original payload.
package promrw

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/golang/snappy"
)

// Label is one Prometheus label pair.
type Label struct {
	Name  string
	Value string
}

// Sample is one raw sample.
type Sample struct {
	Value     float64
	Timestamp int64 // milliseconds since the Unix epoch
}

// TimeSeries is a label set plus its samples.
type TimeSeries struct {
	Labels  []Label
	Samples []Sample
}

// WriteRequest is the top-level Prometheus remote write message.
type WriteRequest struct {
	Timeseries []TimeSeries
}

// SortLabels sorts labels by name, as required by the remote write protocol.
func SortLabels(labels []Label) {
	sort.Slice(labels, func(i, j int) bool { return labels[i].Name < labels[j].Name })
}

// EncodeLabels appends the protobuf encoding of repeated Label messages. The
// result is the value of TimeSeries.labels, so callers can reuse it for every
// request that carries the same series.
func EncodeLabels(labels []Label) []byte {
	size := 0
	for _, label := range labels {
		body := 1 + varintLen(uint64(len(label.Name))) + len(label.Name)
		body += 1 + varintLen(uint64(len(label.Value))) + len(label.Value)
		size += 1 + varintLen(uint64(body)) + body
	}
	out := make([]byte, 0, size)
	for _, label := range labels {
		body := 1 + varintLen(uint64(len(label.Name))) + len(label.Name)
		body += 1 + varintLen(uint64(len(label.Value))) + len(label.Value)
		out = appendTag(out, 1, wireBytes)
		out = appendVarint(out, uint64(body))
		out = appendTag(out, 1, wireBytes)
		out = appendVarint(out, uint64(len(label.Name)))
		out = append(out, label.Name...)
		out = appendTag(out, 2, wireBytes)
		out = appendVarint(out, uint64(len(label.Value)))
		out = append(out, label.Value...)
	}
	return out
}

// AppendTimeSeries appends one TimeSeries message with the pre-encoded label
// fields and a single sample. The caller owns dst.
func AppendTimeSeries(dst []byte, encodedLabels []byte, value float64, timestamp int64) []byte {
	// TimeSeries.labels (field 1) is repeated; encodedLabels already contains
	// one field-1 entry per label.
	dst = append(dst, encodedLabels...)

	// TimeSeries.samples (field 2): one Sample.
	dst = appendTag(dst, 2, wireBytes)
	dst = appendVarint(dst, uint64(1+8+1+varintLen(uint64(timestamp))))
	dst = appendTag(dst, 1, wireFixed64)
	dst = binary.LittleEndian.AppendUint64(dst, math.Float64bits(value))
	dst = appendTag(dst, 2, wireVarint)
	dst = appendVarint(dst, uint64(timestamp))
	return dst
}

// AppendWriteRequest appends a WriteRequest message with the given TimeSeries
// payloads (each one already encoded as a TimeSeries message).
func AppendWriteRequest(dst []byte, encodedSeries [][]byte) []byte {
	for _, series := range encodedSeries {
		dst = appendTag(dst, 1, wireBytes)
		dst = appendVarint(dst, uint64(len(series)))
		dst = append(dst, series...)
	}
	return dst
}

// AppendTimeSeriesField appends one WriteRequest.timeseries entry (field 1)
// with the given labels and one sample, without an intermediate allocation.
func AppendTimeSeriesField(dst []byte, encodedLabels []byte, value float64, timestamp int64) []byte {
	sampleLen := 1 + 8 + 1 + varintLen(uint64(timestamp))
	messageLen := len(encodedLabels) + 1 + varintLen(uint64(sampleLen)) + sampleLen
	dst = appendTag(dst, 1, wireBytes)
	dst = appendVarint(dst, uint64(messageLen))
	return AppendTimeSeries(dst, encodedLabels, value, timestamp)
}

// Marshal returns the uncompressed protobuf encoding of req.
func Marshal(req *WriteRequest) ([]byte, error) {
	size := 0
	for _, series := range req.Timeseries {
		body, err := marshalTimeSeries(&series)
		if err != nil {
			return nil, err
		}
		size += 1 + varintLen(uint64(len(body))) + len(body)
	}
	out := make([]byte, 0, size)
	for i := range req.Timeseries {
		body, err := marshalTimeSeries(&req.Timeseries[i])
		if err != nil {
			return nil, err
		}
		out = appendTag(out, 1, wireBytes)
		out = appendVarint(out, uint64(len(body)))
		out = append(out, body...)
	}
	return out, nil
}

func marshalTimeSeries(series *TimeSeries) ([]byte, error) {
	if len(series.Labels) == 0 {
		return nil, errors.New("promrw: time series without labels")
	}
	labels := EncodeLabels(series.Labels)
	size := len(labels)
	for _, sample := range series.Samples {
		sampleLen := 1 + 8 + 1 + varintLen(uint64(sample.Timestamp))
		size += 1 + varintLen(uint64(sampleLen)) + sampleLen
	}
	out := make([]byte, 0, size)
	out = append(out, labels...)
	for _, sample := range series.Samples {
		sampleLen := 1 + 8 + 1 + varintLen(uint64(sample.Timestamp))
		out = appendTag(out, 2, wireBytes)
		out = appendVarint(out, uint64(sampleLen))
		out = appendTag(out, 1, wireFixed64)
		out = binary.LittleEndian.AppendUint64(out, math.Float64bits(sample.Value))
		out = appendTag(out, 2, wireVarint)
		out = appendVarint(out, uint64(sample.Timestamp))
	}
	return out, nil
}

// EncodeCompressed marshals req and snappy-compresses it.
func EncodeCompressed(req *WriteRequest) ([]byte, error) {
	raw, err := Marshal(req)
	if err != nil {
		return nil, err
	}
	return snappy.Encode(nil, raw), nil
}

// DecodeCompressed decompresses and unmarshals a remote write body.
func DecodeCompressed(body []byte) (*WriteRequest, error) {
	raw, err := snappy.Decode(nil, body)
	if err != nil {
		return nil, fmt.Errorf("promrw: snappy decode: %w", err)
	}
	return Unmarshal(raw)
}

// Unmarshal parses an uncompressed WriteRequest.
func Unmarshal(data []byte) (*WriteRequest, error) {
	req := &WriteRequest{}
	for len(data) > 0 {
		field, wire, rest, err := readTag(data)
		if err != nil {
			return nil, err
		}
		data = rest
		if field != 1 || wire != wireBytes {
			data, err = skipField(data, wire)
			if err != nil {
				return nil, err
			}
			continue
		}
		body, rest, err := readBytes(data)
		if err != nil {
			return nil, err
		}
		data = rest
		series, err := unmarshalTimeSeries(body)
		if err != nil {
			return nil, err
		}
		req.Timeseries = append(req.Timeseries, *series)
	}
	return req, nil
}

func unmarshalTimeSeries(data []byte) (*TimeSeries, error) {
	series := &TimeSeries{}
	for len(data) > 0 {
		field, wire, rest, err := readTag(data)
		if err != nil {
			return nil, err
		}
		data = rest
		switch {
		case field == 1 && wire == wireBytes:
			body, rest, err := readBytes(data)
			if err != nil {
				return nil, err
			}
			data = rest
			label, err := unmarshalLabel(body)
			if err != nil {
				return nil, err
			}
			series.Labels = append(series.Labels, *label)
		case field == 2 && wire == wireBytes:
			body, rest, err := readBytes(data)
			if err != nil {
				return nil, err
			}
			data = rest
			sample, err := unmarshalSample(body)
			if err != nil {
				return nil, err
			}
			series.Samples = append(series.Samples, *sample)
		default:
			data, err = skipField(data, wire)
			if err != nil {
				return nil, err
			}
		}
	}
	return series, nil
}

func unmarshalLabel(data []byte) (*Label, error) {
	label := &Label{}
	for len(data) > 0 {
		field, wire, rest, err := readTag(data)
		if err != nil {
			return nil, err
		}
		data = rest
		if wire != wireBytes {
			data, err = skipField(data, wire)
			if err != nil {
				return nil, err
			}
			continue
		}
		body, rest, err := readBytes(data)
		if err != nil {
			return nil, err
		}
		data = rest
		switch field {
		case 1:
			label.Name = string(body)
		case 2:
			label.Value = string(body)
		}
	}
	return label, nil
}

func unmarshalSample(data []byte) (*Sample, error) {
	sample := &Sample{}
	for len(data) > 0 {
		field, wire, rest, err := readTag(data)
		if err != nil {
			return nil, err
		}
		data = rest
		switch {
		case field == 1 && wire == wireFixed64:
			if len(data) < 8 {
				return nil, errors.New("promrw: truncated sample value")
			}
			sample.Value = math.Float64frombits(binary.LittleEndian.Uint64(data))
			data = data[8:]
		case field == 2 && wire == wireVarint:
			value, rest, err := readVarint(data)
			if err != nil {
				return nil, err
			}
			sample.Timestamp = int64(value)
			data = rest
		default:
			data, err = skipField(data, wire)
			if err != nil {
				return nil, err
			}
		}
	}
	return sample, nil
}

const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
)

func appendTag(dst []byte, field int, wire int) []byte {
	return appendVarint(dst, uint64(field<<3|wire))
}

func appendVarint(dst []byte, value uint64) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

func varintLen(value uint64) int {
	n := 1
	for value >= 0x80 {
		value >>= 7
		n++
	}
	return n
}

func readTag(data []byte) (field int, wire int, rest []byte, err error) {
	value, rest, err := readVarint(data)
	if err != nil {
		return 0, 0, nil, err
	}
	return int(value >> 3), int(value & 7), rest, nil
}

func readVarint(data []byte) (uint64, []byte, error) {
	var value uint64
	for i := 0; i < len(data) && i < 10; i++ {
		b := data[i]
		value |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return value, data[i+1:], nil
		}
	}
	return 0, nil, errors.New("promrw: truncated or overlong varint")
}

func readBytes(data []byte) ([]byte, []byte, error) {
	length, rest, err := readVarint(data)
	if err != nil {
		return nil, nil, err
	}
	if length > uint64(len(rest)) {
		return nil, nil, errors.New("promrw: length-delimited field exceeds body")
	}
	return rest[:length], rest[length:], nil
}

func skipField(data []byte, wire int) ([]byte, error) {
	switch wire {
	case wireVarint:
		_, rest, err := readVarint(data)
		return rest, err
	case wireFixed64:
		if len(data) < 8 {
			return nil, errors.New("promrw: truncated fixed64 field")
		}
		return data[8:], nil
	case wireBytes:
		_, rest, err := readBytes(data)
		return rest, err
	default:
		return nil, fmt.Errorf("promrw: unsupported wire type %d", wire)
	}
}
