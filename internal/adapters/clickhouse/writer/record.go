// Package writer implements the ClickHouse ingestion path: batched RowBinary
// writes, event_id deduplication, a durable local spool for replay and the
// counters that prove no acknowledged batch is lost.
//
// The writer is deliberately signal-specific at the edges (SubmitLogs and
// SubmitTraces) and signal-agnostic in the middle: one spool, one retry loop
// and one checkpoint mechanism. In the cluster profile the NATS JetStream WAL
// (Story 4.3) is the first durability boundary; the writer consumes it and
// keeps this local spool as the second boundary until ClickHouse confirms the
// insert (AD-4, Story 4.4).
package writer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// Signals accepted by the adapter.
const (
	SignalLogs   = "logs"
	SignalTraces = "traces"
)

// DefaultRetentionDays is applied when the record does not carry a per-row
// retention. The control plane reconciles the policy in Story 3.5.
const DefaultRetentionDays uint16 = 15

var (
	logColumns = []string{
		"tenant_id",
		"project_id",
		"timestamp",
		"ingested_at",
		"observed_timestamp",
		"severity",
		"severity_number",
		"service_name",
		"body",
		"attributes",
		"resource_attributes",
		"trace_id",
		"span_id",
		"event_id",
		"retention_days",
		"project_generation",
		"authz_epoch",
	}
	traceColumns = []string{
		"tenant_id",
		"project_id",
		"timestamp",
		"ingested_at",
		"trace_id",
		"span_id",
		"parent_span_id",
		"service_name",
		"operation_name",
		"span_kind",
		"duration_ns",
		"status_code",
		"status_message",
		"attributes",
		"resource_attributes",
		"events",
		"links",
		"event_id",
		"retention_days",
		"project_generation",
		"authz_epoch",
	}
)

// LogColumns returns the RowBinary column order of otel_logs.
func LogColumns() []string { return append([]string(nil), logColumns...) }

// TraceColumns returns the RowBinary column order of otel_traces.
func TraceColumns() []string { return append([]string(nil), traceColumns...) }

// LogRecord is one OTLP log record ready for storage. TenantID, ProjectID and
// EventID are mandatory (AD-2); the adapter rejects a batch without them.
type LogRecord struct {
	TenantID           string
	ProjectID          string
	Timestamp          time.Time
	ObservedTimestamp  time.Time
	Severity           string
	SeverityNumber     uint8
	ServiceName        string
	Body               string
	Attributes         map[string]string
	ResourceAttributes map[string]string
	TraceID            string
	SpanID             string
	EventID            string
	RetentionDays      uint16
	ProjectGeneration  uint64
	AuthzEpoch         uint64
}

// SpanRecord is one OTLP span ready for storage.
type SpanRecord struct {
	TenantID           string
	ProjectID          string
	Timestamp          time.Time
	TraceID            string
	SpanID             string
	ParentSpanID       string
	ServiceName        string
	OperationName      string
	SpanKind           string
	DurationNs         uint64
	StatusCode         string
	StatusMessage      string
	Attributes         map[string]string
	ResourceAttributes map[string]string
	Events             []SpanEvent
	Links              []SpanLink
	EventID            string
	RetentionDays      uint16
	ProjectGeneration  uint64
	AuthzEpoch         uint64
}

// SpanEvent is an OTLP span event.
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]string
}

// SpanLink is an OTLP span link.
type SpanLink struct {
	TraceID    string
	SpanID     string
	Attributes map[string]string
}

type identity struct {
	tenantID          string
	projectID         string
	eventID           string
	projectGeneration uint64
	authzEpoch        uint64
}

type record interface {
	identity() identity
	eventTime() time.Time
	validate() error
	appendRowBinary(dst []byte, eventTime, ingestedAt time.Time) []byte
}

func (r LogRecord) identity() identity {
	return identity{
		tenantID:          r.TenantID,
		projectID:         r.ProjectID,
		eventID:           r.EventID,
		projectGeneration: tenancyValue(r.ProjectGeneration),
		authzEpoch:        tenancyValue(r.AuthzEpoch),
	}
}

func (r LogRecord) eventTime() time.Time { return r.Timestamp }

func (r LogRecord) validate() error {
	if r.TenantID == "" {
		return errors.New("log record has no tenant_id")
	}
	if r.ProjectID == "" {
		return errors.New("log record has no project_id")
	}
	if r.EventID == "" {
		return errors.New("log record has no event_id")
	}
	if r.Timestamp.IsZero() {
		return errors.New("log record has no timestamp")
	}
	return nil
}

func (r LogRecord) appendRowBinary(dst []byte, eventTime, ingestedAt time.Time) []byte {
	observed := r.ObservedTimestamp
	if observed.IsZero() {
		observed = eventTime
	}
	retention := r.RetentionDays
	if retention == 0 {
		retention = DefaultRetentionDays
	}

	dst = appendString(dst, r.TenantID)
	dst = appendString(dst, r.ProjectID)
	dst = appendTime(dst, eventTime)
	dst = appendTime(dst, ingestedAt)
	dst = appendTime(dst, observed)
	dst = appendString(dst, r.Severity)
	dst = append(dst, r.SeverityNumber)
	dst = appendString(dst, r.ServiceName)
	dst = appendString(dst, r.Body)
	dst = appendMap(dst, r.Attributes)
	dst = appendMap(dst, r.ResourceAttributes)
	dst = appendString(dst, r.TraceID)
	dst = appendString(dst, r.SpanID)
	dst = appendString(dst, r.EventID)
	dst = binary.LittleEndian.AppendUint16(dst, retention)
	dst = binary.LittleEndian.AppendUint64(dst, tenancyValue(r.ProjectGeneration))
	return binary.LittleEndian.AppendUint64(dst, tenancyValue(r.AuthzEpoch))
}

func (r SpanRecord) identity() identity {
	return identity{
		tenantID:          r.TenantID,
		projectID:         r.ProjectID,
		eventID:           r.EventID,
		projectGeneration: tenancyValue(r.ProjectGeneration),
		authzEpoch:        tenancyValue(r.AuthzEpoch),
	}
}

func (r SpanRecord) eventTime() time.Time { return r.Timestamp }

func (r SpanRecord) validate() error {
	if r.TenantID == "" {
		return errors.New("span record has no tenant_id")
	}
	if r.ProjectID == "" {
		return errors.New("span record has no project_id")
	}
	if r.EventID == "" {
		return errors.New("span record has no event_id")
	}
	if r.TraceID == "" {
		return errors.New("span record has no trace_id")
	}
	if r.SpanID == "" {
		return errors.New("span record has no span_id")
	}
	if r.Timestamp.IsZero() {
		return errors.New("span record has no timestamp")
	}
	return nil
}

func (r SpanRecord) appendRowBinary(dst []byte, eventTime, ingestedAt time.Time) []byte {
	retention := r.RetentionDays
	if retention == 0 {
		retention = DefaultRetentionDays
	}

	dst = appendString(dst, r.TenantID)
	dst = appendString(dst, r.ProjectID)
	dst = appendTime(dst, eventTime)
	dst = appendTime(dst, ingestedAt)
	dst = appendString(dst, r.TraceID)
	dst = appendString(dst, r.SpanID)
	dst = appendString(dst, r.ParentSpanID)
	dst = appendString(dst, r.ServiceName)
	dst = appendString(dst, r.OperationName)
	dst = appendString(dst, r.SpanKind)
	dst = binary.LittleEndian.AppendUint64(dst, r.DurationNs)
	dst = appendString(dst, r.StatusCode)
	dst = appendString(dst, r.StatusMessage)
	dst = appendMap(dst, r.Attributes)
	dst = appendMap(dst, r.ResourceAttributes)
	dst = appendSpanEvents(dst, r.Events)
	dst = appendSpanLinks(dst, r.Links)
	dst = appendString(dst, r.EventID)
	dst = binary.LittleEndian.AppendUint16(dst, retention)
	dst = binary.LittleEndian.AppendUint64(dst, tenancyValue(r.ProjectGeneration))
	return binary.LittleEndian.AppendUint64(dst, tenancyValue(r.AuthzEpoch))
}

func appendString(dst []byte, value string) []byte {
	dst = binary.AppendUvarint(dst, uint64(len(value)))
	return append(dst, value...)
}

func appendTime(dst []byte, value time.Time) []byte {
	return binary.LittleEndian.AppendUint64(dst, uint64(value.UnixNano()))
}

// tenancyValue defaults the authorization generation/epoch to 1 so records
// written before the tenancy migration keep the schema default and remain
// readable under the initial scope (AD-20).
func tenancyValue(value uint64) uint64 {
	if value == 0 {
		return 1
	}
	return value
}

func appendMap(dst []byte, values map[string]string) []byte {
	dst = binary.AppendUvarint(dst, uint64(len(values)))
	for key, value := range values {
		dst = appendString(dst, key)
		dst = appendString(dst, value)
	}
	return dst
}

func appendSpanEvents(dst []byte, events []SpanEvent) []byte {
	dst = binary.AppendUvarint(dst, uint64(len(events)))
	for _, event := range events {
		dst = appendString(dst, event.Name)
		dst = appendTime(dst, event.Timestamp)
		dst = appendMap(dst, event.Attributes)
	}
	return dst
}

func appendSpanLinks(dst []byte, links []SpanLink) []byte {
	dst = binary.AppendUvarint(dst, uint64(len(links)))
	for _, link := range links {
		dst = appendString(dst, link.TraceID)
		dst = appendString(dst, link.SpanID)
		dst = appendMap(dst, link.Attributes)
	}
	return dst
}

// encodeError wraps a record validation error with its position so the caller
// can identify the bad row without logging the payload.
type encodeError struct {
	index int
	err   error
}

func (e *encodeError) Error() string {
	return fmt.Sprintf("record %d: %v", e.index, e.err)
}

func (e *encodeError) Unwrap() error { return e.err }
