package writer

import (
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/observaki/observaki/internal/adapters/clickhouse"
)

func TestLogRowBinaryLayout(t *testing.T) {
	eventTime := time.Date(2026, 11, 2, 12, 0, 0, 123456789, time.UTC)
	ingestedAt := eventTime.Add(2 * time.Second)
	record := LogRecord{
		TenantID:           "tenant-0",
		ProjectID:          "project-0",
		Timestamp:          eventTime,
		ObservedTimestamp:  eventTime.Add(-time.Millisecond),
		Severity:           "ERROR",
		SeverityNumber:     17,
		ServiceName:        "checkout",
		Body:               "payment failed",
		Attributes:         map[string]string{"http.status_code": "500"},
		ResourceAttributes: map[string]string{"deployment.environment": "prod"},
		TraceID:            "trace-1",
		SpanID:             "span-1",
		EventID:            "event-1",
		RetentionDays:      7,
	}

	decoder := newRowDecoder(t, record.appendRowBinary(nil, eventTime, ingestedAt))
	if got := decoder.string(); got != "tenant-0" {
		t.Fatalf("tenant_id = %q, want tenant-0", got)
	}
	if got := decoder.string(); got != "project-0" {
		t.Fatalf("project_id = %q, want project-0", got)
	}
	if got := decoder.timestamp(); !got.Equal(eventTime) {
		t.Fatalf("timestamp = %v, want %v", got, eventTime)
	}
	if got := decoder.timestamp(); !got.Equal(ingestedAt) {
		t.Fatalf("ingested_at = %v, want %v", got, ingestedAt)
	}
	if got := decoder.timestamp(); !got.Equal(record.ObservedTimestamp) {
		t.Fatalf("observed_timestamp = %v, want %v", got, record.ObservedTimestamp)
	}
	if got := decoder.string(); got != "ERROR" {
		t.Fatalf("severity = %q, want ERROR", got)
	}
	if got := decoder.uint8(); got != 17 {
		t.Fatalf("severity_number = %d, want 17", got)
	}
	if got := decoder.string(); got != "checkout" {
		t.Fatalf("service_name = %q, want checkout", got)
	}
	if got := decoder.string(); got != "payment failed" {
		t.Fatalf("body = %q, want payment failed", got)
	}
	if got := decoder.stringMap(); !equalMap(got, record.Attributes) {
		t.Fatalf("attributes = %v, want %v", got, record.Attributes)
	}
	if got := decoder.stringMap(); !equalMap(got, record.ResourceAttributes) {
		t.Fatalf("resource_attributes = %v, want %v", got, record.ResourceAttributes)
	}
	if got := decoder.string(); got != "trace-1" {
		t.Fatalf("trace_id = %q, want trace-1", got)
	}
	if got := decoder.string(); got != "span-1" {
		t.Fatalf("span_id = %q, want span-1", got)
	}
	if got := decoder.string(); got != "event-1" {
		t.Fatalf("event_id = %q, want event-1", got)
	}
	if got := decoder.uint16(); got != 7 {
		t.Fatalf("retention_days = %d, want 7", got)
	}
	if got := decoder.uint64(); got != 1 {
		t.Fatalf("project_generation = %d, want default 1", got)
	}
	if got := decoder.uint64(); got != 1 {
		t.Fatalf("authz_epoch = %d, want default 1", got)
	}
	decoder.done()
}

func TestLogRowBinaryDefaults(t *testing.T) {
	eventTime := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	record := LogRecord{
		TenantID:  "tenant-0",
		ProjectID: "project-0",
		Timestamp: eventTime,
		EventID:   "event-1",
	}
	decoder := newRowDecoder(t, record.appendRowBinary(nil, eventTime, eventTime))
	if got := decoder.string(); got != "tenant-0" {
		t.Fatalf("tenant_id = %q, want tenant-0", got)
	}
	if got := decoder.string(); got != "project-0" {
		t.Fatalf("project_id = %q, want project-0", got)
	}
	if got := decoder.timestamp(); !got.Equal(eventTime) {
		t.Fatalf("timestamp = %v, want %v", got, eventTime)
	}
	if got := decoder.timestamp(); !got.Equal(eventTime) {
		t.Fatalf("ingested_at = %v, want %v", got, eventTime)
	}
	if got := decoder.timestamp(); !got.Equal(eventTime) {
		t.Fatalf("observed_timestamp = %v, want the event timestamp", got)
	}
	if got := decoder.string(); got != "" {
		t.Fatalf("severity = %q, want empty", got)
	}
	if got := decoder.uint8(); got != 0 {
		t.Fatalf("severity_number = %d, want 0", got)
	}
	if got := decoder.string(); got != "" {
		t.Fatalf("service_name = %q, want empty", got)
	}
	if got := decoder.string(); got != "" {
		t.Fatalf("body = %q, want empty", got)
	}
	decoder.stringMap()
	decoder.stringMap()
	if got := decoder.string(); got != "" {
		t.Fatalf("trace_id = %q, want empty", got)
	}
	if got := decoder.string(); got != "" {
		t.Fatalf("span_id = %q, want empty", got)
	}
	if got := decoder.string(); got != "event-1" {
		t.Fatalf("event_id = %q, want event-1", got)
	}
	if got := decoder.uint16(); got != DefaultRetentionDays {
		t.Fatalf("retention_days = %d, want %d", got, DefaultRetentionDays)
	}
	if got := decoder.uint64(); got != 1 {
		t.Fatalf("project_generation = %d, want default 1", got)
	}
	if got := decoder.uint64(); got != 1 {
		t.Fatalf("authz_epoch = %d, want default 1", got)
	}
	decoder.done()
}

func TestSpanRowBinaryLayout(t *testing.T) {
	eventTime := time.Date(2026, 11, 2, 12, 0, 0, 500, time.UTC)
	record := SpanRecord{
		TenantID:      "tenant-0",
		ProjectID:     "project-0",
		Timestamp:     eventTime,
		TraceID:       "trace-1",
		SpanID:        "span-1",
		ParentSpanID:  "span-0",
		ServiceName:   "checkout",
		OperationName: "GET /cart",
		SpanKind:      "server",
		DurationNs:    1200,
		StatusCode:    "ERROR",
		StatusMessage: "timeout",
		Attributes:    map[string]string{"http.method": "GET"},
		ResourceAttributes: map[string]string{
			"service.name": "checkout",
		},
		Events: []SpanEvent{{
			Name:       "exception",
			Timestamp:  eventTime.Add(time.Millisecond),
			Attributes: map[string]string{"exception.type": "timeout"},
		}},
		Links: []SpanLink{{
			TraceID:    "trace-0",
			SpanID:     "span-0",
			Attributes: map[string]string{"link.kind": "parent"},
		}},
		EventID:       "event-1",
		RetentionDays: 15,
	}

	decoder := newRowDecoder(t, record.appendRowBinary(nil, eventTime, eventTime))
	if got := decoder.string(); got != "tenant-0" {
		t.Fatalf("tenant_id = %q, want tenant-0", got)
	}
	if got := decoder.string(); got != "project-0" {
		t.Fatalf("project_id = %q, want project-0", got)
	}
	decoder.timestamp()
	decoder.timestamp()
	if got := decoder.string(); got != "trace-1" {
		t.Fatalf("trace_id = %q, want trace-1", got)
	}
	if got := decoder.string(); got != "span-1" {
		t.Fatalf("span_id = %q, want span-1", got)
	}
	if got := decoder.string(); got != "span-0" {
		t.Fatalf("parent_span_id = %q, want span-0", got)
	}
	if got := decoder.string(); got != "checkout" {
		t.Fatalf("service_name = %q, want checkout", got)
	}
	if got := decoder.string(); got != "GET /cart" {
		t.Fatalf("operation_name = %q, want GET /cart", got)
	}
	if got := decoder.string(); got != "server" {
		t.Fatalf("span_kind = %q, want server", got)
	}
	if got := decoder.uint64(); got != 1200 {
		t.Fatalf("duration_ns = %d, want 1200", got)
	}
	if got := decoder.string(); got != "ERROR" {
		t.Fatalf("status_code = %q, want ERROR", got)
	}
	if got := decoder.string(); got != "timeout" {
		t.Fatalf("status_message = %q, want timeout", got)
	}
	decoder.stringMap()
	decoder.stringMap()
	if got := decoder.uvarint(); got != 1 {
		t.Fatalf("events length = %d, want 1", got)
	}
	if got := decoder.string(); got != "exception" {
		t.Fatalf("event name = %q, want exception", got)
	}
	decoder.timestamp()
	decoder.stringMap()
	if got := decoder.uvarint(); got != 1 {
		t.Fatalf("links length = %d, want 1", got)
	}
	if got := decoder.string(); got != "trace-0" {
		t.Fatalf("link trace_id = %q, want trace-0", got)
	}
	if got := decoder.string(); got != "span-0" {
		t.Fatalf("link span_id = %q, want span-0", got)
	}
	decoder.stringMap()
	if got := decoder.string(); got != "event-1" {
		t.Fatalf("event_id = %q, want event-1", got)
	}
	if got := decoder.uint16(); got != 15 {
		t.Fatalf("retention_days = %d, want 15", got)
	}
	if got := decoder.uint64(); got != 1 {
		t.Fatalf("project_generation = %d, want default 1", got)
	}
	if got := decoder.uint64(); got != 1 {
		t.Fatalf("authz_epoch = %d, want default 1", got)
	}
	decoder.done()
}

func TestRecordValidation(t *testing.T) {
	validLog := testLog("e1")
	tests := []struct {
		name   string
		record interface{ validate() error }
		want   bool
	}{
		{name: "valid log", record: validLog, want: false},
		{name: "log without tenant", record: LogRecord{ProjectID: "p", EventID: "e", Timestamp: time.Now()}, want: true},
		{name: "log without project", record: LogRecord{TenantID: "t", EventID: "e", Timestamp: time.Now()}, want: true},
		{name: "log without event", record: LogRecord{TenantID: "t", ProjectID: "p", Timestamp: time.Now()}, want: true},
		{name: "log without time", record: LogRecord{TenantID: "t", ProjectID: "p", EventID: "e"}, want: true},
		{name: "valid span", record: SpanRecord{TenantID: "t", ProjectID: "p", EventID: "e", TraceID: "tr", SpanID: "sp", Timestamp: time.Now()}, want: false},
		{name: "span without trace", record: SpanRecord{TenantID: "t", ProjectID: "p", EventID: "e", SpanID: "sp", Timestamp: time.Now()}, want: true},
		{name: "span without span id", record: SpanRecord{TenantID: "t", ProjectID: "p", EventID: "e", TraceID: "tr", Timestamp: time.Now()}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.record.validate()
			if test.want && err == nil {
				t.Fatal("validate() error = nil, want an error")
			}
			if !test.want && err != nil {
				t.Fatalf("validate() error = %v, want nil", err)
			}
		})
	}
}

func TestClampTimestamp(t *testing.T) {
	ingestedAt := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	if got, clamped := clampTimestamp(ingestedAt.Add(-time.Hour), ingestedAt, 24*time.Hour); clamped || !got.Equal(ingestedAt.Add(-time.Hour)) {
		t.Fatalf("recent timestamp was clamped: %v, %v", got, clamped)
	}
	if got, clamped := clampTimestamp(ingestedAt.Add(48*time.Hour), ingestedAt, 24*time.Hour); !clamped || !got.Equal(ingestedAt) {
		t.Fatalf("future timestamp = %v, clamped = %v, want ingested_at", got, clamped)
	}
	if got, clamped := clampTimestamp(ingestedAt.Add(-48*time.Hour), ingestedAt, 24*time.Hour); !clamped || !got.Equal(ingestedAt) {
		t.Fatalf("stale timestamp = %v, clamped = %v, want ingested_at", got, clamped)
	}
}

func TestColumnOrderMatchesMigrations(t *testing.T) {
	migrations, err := clickhouse.Migrations()
	if err != nil {
		t.Fatalf("clickhouse.Migrations() error = %v", err)
	}
	want := map[string][]string{
		"otel_logs":   LogColumns(),
		"otel_traces": TraceColumns(),
	}
	for _, migration := range migrations {
		expected, ok := want[migration.Name]
		if !ok {
			continue
		}
		got := migrationColumns(migration.SQL)
		// 0004 adds project_generation and authz_epoch after the CREATE
		// columns; the writer always inserts them explicitly.
		if !slices.Equal(got, expected[:len(got)]) {
			t.Fatalf("%s columns = %v, want prefix %v", migration.Name, got, expected[:len(got)])
		}
	}
	tenancy := []string{"project_generation", "authz_epoch"}
	for table, columns := range want {
		if !slices.Equal(columns[len(columns)-len(tenancy):], tenancy) {
			t.Fatalf("%s tenancy columns = %v, want %v", table, columns[len(columns)-len(tenancy):], tenancy)
		}
	}
}

func migrationColumns(sql string) []string {
	var columns []string
	inCreate := false
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CREATE TABLE") {
			inCreate = true
			continue
		}
		if !inCreate {
			continue
		}
		if strings.HasPrefix(trimmed, ")") {
			break
		}
		if trimmed == "" || trimmed == "(" || strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "INDEX") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) > 0 {
			columns = append(columns, fields[0])
		}
	}
	return columns
}

func equalMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

type rowDecoder struct {
	t      *testing.T
	data   []byte
	offset int
}

func newRowDecoder(t *testing.T, data []byte) *rowDecoder {
	t.Helper()
	return &rowDecoder{t: t, data: data}
}

func (d *rowDecoder) remaining() int { return len(d.data) - d.offset }

func (d *rowDecoder) done() {
	d.t.Helper()
	if d.remaining() != 0 {
		d.t.Fatalf("%d bytes left after decoding row", d.remaining())
	}
}

func (d *rowDecoder) uvarint() uint64 {
	d.t.Helper()
	value, read := binary.Uvarint(d.data[d.offset:])
	if read <= 0 {
		d.t.Fatal("invalid uvarint in row")
	}
	d.offset += read
	return value
}

func (d *rowDecoder) uint8() uint8 {
	d.t.Helper()
	if d.remaining() < 1 {
		d.t.Fatal("short row for uint8")
	}
	value := d.data[d.offset]
	d.offset++
	return value
}

func (d *rowDecoder) uint16() uint16 {
	d.t.Helper()
	if d.remaining() < 2 {
		d.t.Fatal("short row for uint16")
	}
	value := binary.LittleEndian.Uint16(d.data[d.offset:])
	d.offset += 2
	return value
}

func (d *rowDecoder) uint64() uint64 {
	d.t.Helper()
	if d.remaining() < 8 {
		d.t.Fatal("short row for uint64")
	}
	value := binary.LittleEndian.Uint64(d.data[d.offset:])
	d.offset += 8
	return value
}

func (d *rowDecoder) string() string {
	d.t.Helper()
	length := d.uvarint()
	if length > uint64(d.remaining()) {
		d.t.Fatalf("string length %d exceeds %d remaining bytes", length, d.remaining())
	}
	value := string(d.data[d.offset : d.offset+int(length)])
	d.offset += int(length)
	return value
}

func (d *rowDecoder) timestamp() time.Time {
	d.t.Helper()
	return time.Unix(0, int64(d.uint64())).UTC()
}

func (d *rowDecoder) stringMap() map[string]string {
	d.t.Helper()
	length := d.uvarint()
	if length > uint64(math.MaxInt) {
		d.t.Fatalf("map length %d overflows int", length)
	}
	values := make(map[string]string, int(length))
	for index := uint64(0); index < length; index++ {
		values[d.string()] = d.string()
	}
	return values
}

func TestEncodeErrorUnwrap(t *testing.T) {
	cause := errors.New("bad")
	err := &encodeError{index: 3, err: cause}
	if !errors.Is(err, cause) {
		t.Fatal("encodeError does not unwrap its cause")
	}
	if !strings.Contains(err.Error(), "record 3") {
		t.Fatalf("encodeError = %q, want the record index", err.Error())
	}
}

func TestParallelEncodingKeepsEveryRow(t *testing.T) {
	const count = 5000
	records := make([]LogRecord, count)
	accepted := make([]int, count)
	times := make([]time.Time, count)
	eventTime := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for index := range records {
		records[index] = LogRecord{
			TenantID:  "tenant-0",
			ProjectID: "project-0",
			Timestamp: eventTime,
			EventID:   "event-" + strconv.Itoa(index),
		}
		accepted[index] = index
		times[index] = eventTime
	}
	buffer := encodeAccepted(records, accepted, times, eventTime, 4)
	decoder := newRowDecoder(t, buffer)
	seen := make(map[string]struct{}, count)
	for index := 0; index < count; index++ {
		decoder.string()
		decoder.string()
		decoder.timestamp()
		decoder.timestamp()
		decoder.timestamp()
		decoder.string()
		decoder.uint8()
		decoder.string()
		decoder.string()
		decoder.stringMap()
		decoder.stringMap()
		decoder.string()
		decoder.string()
		eventID := decoder.string()
		decoder.uint16()
		decoder.uint64()
		decoder.uint64()
		if _, ok := seen[eventID]; ok {
			t.Fatalf("parallel encoding duplicated event_id %q", eventID)
		}
		seen[eventID] = struct{}{}
	}
	decoder.done()
	if len(seen) != count {
		t.Fatalf("decoded %d unique events, want %d", len(seen), count)
	}
}
