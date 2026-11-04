package writer

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/observaki/observaki/internal/otlputil"
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	otlplogspb "go.opentelemetry.io/proto/otlp/logs/v1"
	otlptracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// LogsFromOTLP converts one redacted OTLP logs request into storage records.
// eventIDs must contain one deterministic ID per log record, in OTLP traversal
// order, as produced by ingest.SynthesizeEventIDs. receivedAt is the gateway
// receive time and is used only when the producer timestamp is missing.
func LogsFromOTLP(
	tenantID, projectID string,
	generation, authzEpoch uint64,
	request *logspb.ExportLogsServiceRequest,
	eventIDs []string,
	receivedAt time.Time,
) ([]LogRecord, error) {
	if request == nil {
		return nil, fmt.Errorf("logs request is nil")
	}
	total := countOtlpLogs(request)
	if total != len(eventIDs) {
		return nil, fmt.Errorf("logs request has %d records but %d event ids", total, len(eventIDs))
	}
	if total == 0 {
		return nil, fmt.Errorf("logs request has no records")
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	records := make([]LogRecord, 0, total)
	position := 0
	for _, resourceLogs := range request.GetResourceLogs() {
		resource := resourceLogs.GetResource()
		serviceName := otlputil.ServiceName(resource)
		resourceAttributes := otlputil.AttributeMap(resource.GetAttributes())
		for _, scopeLogs := range resourceLogs.GetScopeLogs() {
			for _, record := range scopeLogs.GetLogRecords() {
				timestamp := unixNanoTime(record.GetTimeUnixNano())
				if timestamp.IsZero() {
					timestamp = unixNanoTime(record.GetObservedTimeUnixNano())
				}
				if timestamp.IsZero() {
					timestamp = receivedAt
				}
				observed := unixNanoTime(record.GetObservedTimeUnixNano())
				if observed.IsZero() {
					observed = timestamp
				}
				records = append(records, LogRecord{
					TenantID:           tenantID,
					ProjectID:          projectID,
					Timestamp:          timestamp.UTC(),
					ObservedTimestamp:  observed.UTC(),
					Severity:           severityText(record),
					SeverityNumber:     uint8(record.GetSeverityNumber()),
					ServiceName:        serviceName,
					Body:               otlputil.ValueString(record.GetBody()),
					Attributes:         otlputil.AttributeMap(record.GetAttributes()),
					ResourceAttributes: resourceAttributes,
					TraceID:            hexID(record.GetTraceId()),
					SpanID:             hexID(record.GetSpanId()),
					EventID:            eventIDs[position],
					ProjectGeneration:  generation,
					AuthzEpoch:         authzEpoch,
				})
				position++
			}
		}
	}
	return records, nil
}

// TracesFromOTLP converts one redacted OTLP traces request into span records.
// eventIDs follows the same contract as LogsFromOTLP.
func TracesFromOTLP(
	tenantID, projectID string,
	generation, authzEpoch uint64,
	request *tracepb.ExportTraceServiceRequest,
	eventIDs []string,
	receivedAt time.Time,
) ([]SpanRecord, error) {
	if request == nil {
		return nil, fmt.Errorf("traces request is nil")
	}
	total := countOtlpSpans(request)
	if total != len(eventIDs) {
		return nil, fmt.Errorf("traces request has %d spans but %d event ids", total, len(eventIDs))
	}
	if total == 0 {
		return nil, fmt.Errorf("traces request has no spans")
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	records := make([]SpanRecord, 0, total)
	position := 0
	for _, resourceSpans := range request.GetResourceSpans() {
		resource := resourceSpans.GetResource()
		serviceName := otlputil.ServiceName(resource)
		resourceAttributes := otlputil.AttributeMap(resource.GetAttributes())
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				start := unixNanoTime(span.GetStartTimeUnixNano())
				end := unixNanoTime(span.GetEndTimeUnixNano())
				if start.IsZero() {
					start = receivedAt
				}
				var duration uint64
				if !end.IsZero() && !end.Before(start) {
					duration = uint64(end.Sub(start))
				}
				records = append(records, SpanRecord{
					TenantID:           tenantID,
					ProjectID:          projectID,
					Timestamp:          start.UTC(),
					TraceID:            hexID(span.GetTraceId()),
					SpanID:             hexID(span.GetSpanId()),
					ParentSpanID:       hexID(span.GetParentSpanId()),
					ServiceName:        serviceName,
					OperationName:      span.GetName(),
					SpanKind:           spanKind(span.GetKind()),
					DurationNs:         duration,
					StatusCode:         statusCode(span.GetStatus().GetCode()),
					StatusMessage:      span.GetStatus().GetMessage(),
					Attributes:         otlputil.AttributeMap(span.GetAttributes()),
					ResourceAttributes: resourceAttributes,
					Events:             spanEvents(span.GetEvents(), start),
					Links:              spanLinks(span.GetLinks()),
					EventID:            eventIDs[position],
					ProjectGeneration:  generation,
					AuthzEpoch:         authzEpoch,
				})
				position++
			}
		}
	}
	return records, nil
}

func countOtlpLogs(request *logspb.ExportLogsServiceRequest) int {
	total := 0
	for _, resourceLogs := range request.GetResourceLogs() {
		for _, scopeLogs := range resourceLogs.GetScopeLogs() {
			total += len(scopeLogs.GetLogRecords())
		}
	}
	return total
}

func countOtlpSpans(request *tracepb.ExportTraceServiceRequest) int {
	total := 0
	for _, resourceSpans := range request.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			total += len(scopeSpans.GetSpans())
		}
	}
	return total
}

func unixNanoTime(value uint64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(value))
}

func hexID(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	return hex.EncodeToString(value)
}

func severityText(record *otlplogspb.LogRecord) string {
	if text := record.GetSeverityText(); text != "" {
		return text
	}
	switch record.GetSeverityNumber() {
	case otlplogspb.SeverityNumber_SEVERITY_NUMBER_TRACE,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_TRACE2,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_TRACE3,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_TRACE4:
		return "TRACE"
	case otlplogspb.SeverityNumber_SEVERITY_NUMBER_DEBUG,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_DEBUG2,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_DEBUG3,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_DEBUG4:
		return "DEBUG"
	case otlplogspb.SeverityNumber_SEVERITY_NUMBER_INFO,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_INFO2,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_INFO3,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_INFO4:
		return "INFO"
	case otlplogspb.SeverityNumber_SEVERITY_NUMBER_WARN,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_WARN2,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_WARN3,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_WARN4:
		return "WARN"
	case otlplogspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_ERROR2,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_ERROR3,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_ERROR4:
		return "ERROR"
	case otlplogspb.SeverityNumber_SEVERITY_NUMBER_FATAL,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_FATAL2,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_FATAL3,
		otlplogspb.SeverityNumber_SEVERITY_NUMBER_FATAL4:
		return "FATAL"
	default:
		return "UNSPECIFIED"
	}
}

func spanKind(kind otlptracepb.Span_SpanKind) string {
	switch kind {
	case otlptracepb.Span_SPAN_KIND_INTERNAL:
		return "INTERNAL"
	case otlptracepb.Span_SPAN_KIND_SERVER:
		return "SERVER"
	case otlptracepb.Span_SPAN_KIND_CLIENT:
		return "CLIENT"
	case otlptracepb.Span_SPAN_KIND_PRODUCER:
		return "PRODUCER"
	case otlptracepb.Span_SPAN_KIND_CONSUMER:
		return "CONSUMER"
	default:
		return "UNSPECIFIED"
	}
}

func statusCode(code otlptracepb.Status_StatusCode) string {
	switch code {
	case otlptracepb.Status_STATUS_CODE_OK:
		return "OK"
	case otlptracepb.Status_STATUS_CODE_ERROR:
		return "ERROR"
	default:
		return "UNSET"
	}
}

func spanEvents(events []*otlptracepb.Span_Event, fallback time.Time) []SpanEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]SpanEvent, 0, len(events))
	for _, event := range events {
		timestamp := unixNanoTime(event.GetTimeUnixNano())
		if timestamp.IsZero() {
			timestamp = fallback
		}
		out = append(out, SpanEvent{
			Name:       event.GetName(),
			Timestamp:  timestamp.UTC(),
			Attributes: otlputil.AttributeMap(event.GetAttributes()),
		})
	}
	return out
}

func spanLinks(links []*otlptracepb.Span_Link) []SpanLink {
	if len(links) == 0 {
		return nil
	}
	out := make([]SpanLink, 0, len(links))
	for _, link := range links {
		out = append(out, SpanLink{
			TraceID:    hexID(link.GetTraceId()),
			SpanID:     hexID(link.GetSpanId()),
			Attributes: otlputil.AttributeMap(link.GetAttributes()),
		})
	}
	return out
}
