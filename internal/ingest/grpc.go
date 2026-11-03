package ingest

import (
	"context"
	"fmt"
	"time"

	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	// Register gzip so OTLP/gRPC clients may compress requests (FR-1).
	_ "google.golang.org/grpc/encoding/gzip"
)

type traceService struct {
	tracepb.UnimplementedTraceServiceServer
	gateway *Gateway
}

func (s *traceService) Export(ctx context.Context, req *tracepb.ExportTraceServiceRequest) (*tracepb.ExportTraceServiceResponse, error) {
	return s.gateway.exportTraces(ctx, req)
}

type logsService struct {
	logspb.UnimplementedLogsServiceServer
	gateway *Gateway
}

func (s *logsService) Export(ctx context.Context, req *logspb.ExportLogsServiceRequest) (*logspb.ExportLogsServiceResponse, error) {
	return s.gateway.exportLogs(ctx, req)
}

type metricsService struct {
	metricpb.UnimplementedMetricsServiceServer
	gateway *Gateway
}

func (s *metricsService) Export(ctx context.Context, req *metricpb.ExportMetricsServiceRequest) (*metricpb.ExportMetricsServiceResponse, error) {
	return s.gateway.exportMetrics(ctx, req)
}

func (g *Gateway) exportTraces(ctx context.Context, req *tracepb.ExportTraceServiceRequest) (*tracepb.ExportTraceServiceResponse, error) {
	started := time.Now()
	requestID := requestIDFromContext(ctx)
	identity, requestErr := g.authenticateGRPC(ctx, SignalTraces, requestID)
	if requestErr != nil {
		g.recordGRPC(SignalTraces, identity, 0, 0, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}
	events := countSpans(req)

	if requestErr := g.checkEventLimit(requestID, events); requestErr != nil {
		g.recordGRPC(SignalTraces, identity, events, events, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}
	if events == 0 {
		g.recordGRPC(SignalTraces, identity, 0, 0, nil, started, requestID)
		return &tracepb.ExportTraceServiceResponse{}, nil
	}
	if requestErr := g.checkQuota(requestID, identity, events); requestErr != nil {
		g.recordGRPC(SignalTraces, identity, events, events, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}

	batch := &Batch{
		Signal:           SignalTraces,
		TenantID:         identity.TenantID,
		ProjectID:        identity.ProjectID,
		RequestID:        requestID,
		ConfigVersion:    identity.ConfigVersion,
		ConfigGeneration: identity.Generation,
		ReceivedAt:       time.Now().UTC(),
		Traces:           req,
	}
	g.redactor.apply(batch)
	result, err := g.sink.Consume(ctx, batch)
	if err != nil {
		requestErr := errFromSink(requestID, err)
		g.recordGRPC(SignalTraces, identity, events, 0, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}
	if requestErr := validateSinkResult(requestID, events, result); requestErr != nil {
		g.recordGRPC(SignalTraces, identity, events, 0, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}

	response := &tracepb.ExportTraceServiceResponse{}
	if result.Rejected > 0 || result.Reason != "" {
		response.PartialSuccess = &tracepb.ExportTracePartialSuccess{
			RejectedSpans: int64(result.Rejected),
			ErrorMessage:  result.Reason,
		}
	}
	g.recordGRPC(SignalTraces, identity, events, result.Rejected, nil, started, requestID)
	return response, nil
}

func (g *Gateway) exportLogs(ctx context.Context, req *logspb.ExportLogsServiceRequest) (*logspb.ExportLogsServiceResponse, error) {
	started := time.Now()
	requestID := requestIDFromContext(ctx)
	identity, requestErr := g.authenticateGRPC(ctx, SignalLogs, requestID)
	if requestErr != nil {
		g.recordGRPC(SignalLogs, identity, 0, 0, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}
	events := countLogRecords(req)

	if requestErr := g.checkEventLimit(requestID, events); requestErr != nil {
		g.recordGRPC(SignalLogs, identity, events, events, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}
	if events == 0 {
		g.recordGRPC(SignalLogs, identity, 0, 0, nil, started, requestID)
		return &logspb.ExportLogsServiceResponse{}, nil
	}
	if requestErr := g.checkQuota(requestID, identity, events); requestErr != nil {
		g.recordGRPC(SignalLogs, identity, events, events, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}

	batch := &Batch{
		Signal:           SignalLogs,
		TenantID:         identity.TenantID,
		ProjectID:        identity.ProjectID,
		RequestID:        requestID,
		ConfigVersion:    identity.ConfigVersion,
		ConfigGeneration: identity.Generation,
		ReceivedAt:       time.Now().UTC(),
		Logs:             req,
	}
	g.redactor.apply(batch)
	result, err := g.sink.Consume(ctx, batch)
	if err != nil {
		requestErr := errFromSink(requestID, err)
		g.recordGRPC(SignalLogs, identity, events, 0, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}
	if requestErr := validateSinkResult(requestID, events, result); requestErr != nil {
		g.recordGRPC(SignalLogs, identity, events, 0, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}

	response := &logspb.ExportLogsServiceResponse{}
	if result.Rejected > 0 || result.Reason != "" {
		response.PartialSuccess = &logspb.ExportLogsPartialSuccess{
			RejectedLogRecords: int64(result.Rejected),
			ErrorMessage:       result.Reason,
		}
	}
	g.recordGRPC(SignalLogs, identity, events, result.Rejected, nil, started, requestID)
	return response, nil
}

func (g *Gateway) exportMetrics(ctx context.Context, req *metricpb.ExportMetricsServiceRequest) (*metricpb.ExportMetricsServiceResponse, error) {
	started := time.Now()
	requestID := requestIDFromContext(ctx)
	identity, requestErr := g.authenticateGRPC(ctx, SignalMetrics, requestID)
	if requestErr != nil {
		g.recordGRPC(SignalMetrics, identity, 0, 0, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}
	events := countDataPoints(req)

	if requestErr := g.checkEventLimit(requestID, events); requestErr != nil {
		g.recordGRPC(SignalMetrics, identity, events, events, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}
	if events == 0 {
		g.recordGRPC(SignalMetrics, identity, 0, 0, nil, started, requestID)
		return &metricpb.ExportMetricsServiceResponse{}, nil
	}
	if requestErr := g.checkQuota(requestID, identity, events); requestErr != nil {
		g.recordGRPC(SignalMetrics, identity, events, events, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}

	batch := &Batch{
		Signal:           SignalMetrics,
		TenantID:         identity.TenantID,
		ProjectID:        identity.ProjectID,
		RequestID:        requestID,
		ConfigVersion:    identity.ConfigVersion,
		ConfigGeneration: identity.Generation,
		ReceivedAt:       time.Now().UTC(),
		Metrics:          req,
	}
	g.redactor.apply(batch)
	result, err := g.sink.Consume(ctx, batch)
	if err != nil {
		requestErr := errFromSink(requestID, err)
		g.recordGRPC(SignalMetrics, identity, events, 0, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}
	if requestErr := validateSinkResult(requestID, events, result); requestErr != nil {
		g.recordGRPC(SignalMetrics, identity, events, 0, requestErr, started, requestID)
		return nil, requestErr.grpcStatus()
	}

	response := &metricpb.ExportMetricsServiceResponse{}
	if result.Rejected > 0 || result.Reason != "" {
		response.PartialSuccess = &metricpb.ExportMetricsPartialSuccess{
			RejectedDataPoints: int64(result.Rejected),
			ErrorMessage:       result.Reason,
		}
	}
	g.recordGRPC(SignalMetrics, identity, events, result.Rejected, nil, started, requestID)
	return response, nil
}

func (g *Gateway) checkEventLimit(requestID string, events int) *requestError {
	if events > g.cfg.MaxEvents {
		return errTooManyEvents(requestID, events, g.cfg.MaxEvents)
	}
	return nil
}

func validateSinkResult(requestID string, events int, result Result) *requestError {
	if result.Accepted < 0 || result.Rejected < 0 || result.Accepted+result.Rejected != events {
		return errInternal(
			requestID,
			fmt.Sprintf("ingest sink returned accepted=%d rejected=%d for %d events", result.Accepted, result.Rejected, events),
			nil,
		)
	}
	return nil
}

func (g *Gateway) recordGRPC(signal Signal, identity Identity, events, rejected int, requestErr *requestError, started time.Time, requestID string) {
	g.metrics.observeLatency(identity.TenantID, time.Since(started))
	g.metrics.observeRequest(signal, protocolGRPC)
	g.metrics.observeEvents(signal, protocolGRPC, events, rejected)
	if requestErr == nil {
		return
	}
	g.metrics.observeError(signal, protocolGRPC)
	if requestErr.reason == "wal_backpressure" {
		g.metrics.observeWALBackpressure()
	}
	g.logger.Warn(
		"otlp export rejected",
		"request_id", requestID,
		"tenant_id", identity.TenantID,
		"project_id", identity.ProjectID,
		"signal", signal.String(),
		"protocol", "grpc",
		"events", events,
		"rejected", rejected,
		"reason", requestErr.reason,
		"error", requestErr.message,
		"cause", errorString(requestErr.cause),
		"duration_ms", time.Since(started).Milliseconds(),
	)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
