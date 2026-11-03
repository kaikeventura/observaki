package ingest

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/observaki/observaki/internal/id"
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type encoding int

const (
	encodingProto encoding = iota
	encodingJSON
)

type httpExport struct {
	signal      Signal
	newRequest  func() proto.Message
	count       func(proto.Message) int
	newBatch    func(proto.Message) *Batch
	newResponse func(Result) proto.Message
}

func (g *Gateway) handleTraces(w http.ResponseWriter, r *http.Request) {
	g.serveHTTPExport(w, r, httpExport{
		signal:     SignalTraces,
		newRequest: func() proto.Message { return &tracepb.ExportTraceServiceRequest{} },
		count: func(message proto.Message) int {
			return countSpans(message.(*tracepb.ExportTraceServiceRequest))
		},
		newBatch: func(message proto.Message) *Batch {
			return &Batch{
				Signal:     SignalTraces,
				ReceivedAt: time.Now().UTC(),
				Traces:     message.(*tracepb.ExportTraceServiceRequest),
			}
		},
		newResponse: func(result Result) proto.Message {
			response := &tracepb.ExportTraceServiceResponse{}
			if result.Rejected > 0 || result.Reason != "" {
				response.PartialSuccess = &tracepb.ExportTracePartialSuccess{
					RejectedSpans: int64(result.Rejected),
					ErrorMessage:  result.Reason,
				}
			}
			return response
		},
	})
}

func (g *Gateway) handleLogs(w http.ResponseWriter, r *http.Request) {
	g.serveHTTPExport(w, r, httpExport{
		signal:     SignalLogs,
		newRequest: func() proto.Message { return &logspb.ExportLogsServiceRequest{} },
		count: func(message proto.Message) int {
			return countLogRecords(message.(*logspb.ExportLogsServiceRequest))
		},
		newBatch: func(message proto.Message) *Batch {
			return &Batch{
				Signal:     SignalLogs,
				ReceivedAt: time.Now().UTC(),
				Logs:       message.(*logspb.ExportLogsServiceRequest),
			}
		},
		newResponse: func(result Result) proto.Message {
			response := &logspb.ExportLogsServiceResponse{}
			if result.Rejected > 0 || result.Reason != "" {
				response.PartialSuccess = &logspb.ExportLogsPartialSuccess{
					RejectedLogRecords: int64(result.Rejected),
					ErrorMessage:       result.Reason,
				}
			}
			return response
		},
	})
}

func (g *Gateway) handleMetrics(w http.ResponseWriter, r *http.Request) {
	g.serveHTTPExport(w, r, httpExport{
		signal:     SignalMetrics,
		newRequest: func() proto.Message { return &metricpb.ExportMetricsServiceRequest{} },
		count: func(message proto.Message) int {
			return countDataPoints(message.(*metricpb.ExportMetricsServiceRequest))
		},
		newBatch: func(message proto.Message) *Batch {
			return &Batch{
				Signal:     SignalMetrics,
				ReceivedAt: time.Now().UTC(),
				Metrics:    message.(*metricpb.ExportMetricsServiceRequest),
			}
		},
		newResponse: func(result Result) proto.Message {
			response := &metricpb.ExportMetricsServiceResponse{}
			if result.Rejected > 0 || result.Reason != "" {
				response.PartialSuccess = &metricpb.ExportMetricsPartialSuccess{
					RejectedDataPoints: int64(result.Rejected),
					ErrorMessage:       result.Reason,
				}
			}
			return response
		},
	})
}

func (g *Gateway) serveHTTPExport(w http.ResponseWriter, r *http.Request, export httpExport) {
	started := time.Now()
	requestID := id.NewUUIDv7()
	w.Header().Set("X-Request-Id", requestID)

	if r.Method != http.MethodPost {
		requestErr := errMethodNotAllowed(requestID, r.Method)
		writeHTTPError(w, encodingJSON, requestErr)
		g.recordHTTP(export.signal, Identity{}, 0, 0, requestErr, started, requestID)
		return
	}

	identity, requestErr := g.authenticateHTTP(r, export.signal, requestID)
	if requestErr != nil {
		writeHTTPError(w, encodingJSON, requestErr)
		g.recordHTTP(export.signal, identity, 0, 0, requestErr, started, requestID)
		return
	}
	reservation, requestErr := g.reserveHTTP(r, identity, requestID)
	if requestErr != nil {
		writeHTTPError(w, encodingJSON, requestErr)
		g.recordHTTP(export.signal, identity, 0, 0, requestErr, started, requestID)
		return
	}
	settled := false
	if reservation != nil {
		defer func() {
			if !settled {
				g.auth.Release(reservation)
			}
		}()
	}

	body, enc, requestErr := g.readBody(w, r, requestID)
	if requestErr != nil {
		writeHTTPError(w, enc, requestErr)
		g.recordHTTP(export.signal, identity, 0, 0, requestErr, started, requestID)
		return
	}
	// The per-tenant latency window measures gateway processing (decode,
	// admission, redaction, sink), not the client's upload time.
	started = time.Now()

	request := export.newRequest()
	if requestErr := decodeOTLP(body, enc, request, requestID); requestErr != nil {
		writeHTTPError(w, enc, requestErr)
		g.recordHTTP(export.signal, identity, 0, 0, requestErr, started, requestID)
		return
	}

	events := export.count(request)
	if requestErr := g.checkEventLimit(requestID, events); requestErr != nil {
		writeHTTPError(w, enc, requestErr)
		g.recordHTTP(export.signal, identity, events, events, requestErr, started, requestID)
		return
	}
	if events == 0 {
		writeHTTPResponse(w, enc, export.newResponse(Result{}), requestID)
		g.recordHTTP(export.signal, identity, 0, 0, nil, started, requestID)
		return
	}
	if reservation != nil {
		requestErr := g.settleHTTP(requestID, reservation, events)
		settled = true
		if requestErr != nil {
			writeHTTPError(w, enc, requestErr)
			g.recordHTTP(export.signal, identity, events, events, requestErr, started, requestID)
			return
		}
	} else {
		if requestErr := g.checkQuota(requestID, identity, events); requestErr != nil {
			writeHTTPError(w, enc, requestErr)
			g.recordHTTP(export.signal, identity, events, events, requestErr, started, requestID)
			return
		}
		if g.auth != nil {
			g.auth.RecordPayload(identity, r.ContentLength, events)
		}
	}

	batch := export.newBatch(request)
	batch.TenantID = identity.TenantID
	batch.ProjectID = identity.ProjectID
	batch.ConfigVersion = identity.ConfigVersion
	batch.ConfigGeneration = identity.Generation
	g.redactor.apply(batch)
	result, err := g.sink.Consume(r.Context(), batch)
	if err != nil {
		requestErr := errSinkUnavailable(requestID, err)
		writeHTTPError(w, enc, requestErr)
		g.recordHTTP(export.signal, identity, events, 0, requestErr, started, requestID)
		return
	}
	if requestErr := validateSinkResult(requestID, events, result); requestErr != nil {
		writeHTTPError(w, enc, requestErr)
		g.recordHTTP(export.signal, identity, events, 0, requestErr, started, requestID)
		return
	}

	writeHTTPResponse(w, enc, export.newResponse(result), requestID)
	g.recordHTTP(export.signal, identity, events, result.Rejected, nil, started, requestID)
}

func (g *Gateway) readBody(w http.ResponseWriter, r *http.Request, requestID string) ([]byte, encoding, *requestError) {
	enc, ok := parseContentType(r.Header.Get("Content-Type"))
	if !ok {
		return nil, encodingJSON, errUnsupportedMediaType(requestID, r.Header.Get("Content-Type"))
	}
	if r.ContentLength > g.cfg.MaxRequestBytes {
		return nil, enc, errTooLarge(requestID, g.cfg.MaxRequestBytes)
	}

	var reader io.Reader = http.MaxBytesReader(w, r.Body, g.cfg.MaxRequestBytes)
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return nil, enc, errInvalidArgument(requestID, "invalid gzip body: "+err.Error())
		}
		defer func() { _ = gzipReader.Close() }()
		reader = io.LimitReader(gzipReader, g.cfg.MaxRequestBytes+1)
	default:
		return nil, enc, errUnsupportedContentEncoding(requestID, r.Header.Get("Content-Encoding"))
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, enc, errTooLarge(requestID, g.cfg.MaxRequestBytes)
		}
		return nil, enc, errInvalidArgument(requestID, "read request body: "+err.Error())
	}
	if int64(len(body)) > g.cfg.MaxRequestBytes {
		return nil, enc, errTooLarge(requestID, g.cfg.MaxRequestBytes)
	}
	return body, enc, nil
}

func parseContentType(value string) (encoding, bool) {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return encodingJSON, false
	}
	switch strings.ToLower(mediaType) {
	case "application/x-protobuf", "application/protobuf":
		return encodingProto, true
	case "application/json":
		return encodingJSON, true
	default:
		return encodingJSON, false
	}
}

func decodeOTLP(body []byte, enc encoding, message proto.Message, requestID string) *requestError {
	switch enc {
	case encodingJSON:
		converted, err := otlpJSONToProtoJSON(body)
		if err != nil {
			return errInvalidArgument(requestID, "invalid OTLP/JSON body: "+err.Error())
		}
		options := protojson.UnmarshalOptions{DiscardUnknown: true}
		if err := options.Unmarshal(converted, message); err != nil {
			return errInvalidArgument(requestID, "invalid OTLP/JSON payload: "+err.Error())
		}
	default:
		if err := proto.Unmarshal(body, message); err != nil {
			return errInvalidArgument(requestID, "invalid OTLP/protobuf payload: "+err.Error())
		}
	}
	return nil
}

// otlpJSONToProtoJSON adapts the OTLP/JSON deviations from the standard
// Protobuf JSON mapping: trace and span IDs are hex strings, not base64.
func otlpJSONToProtoJSON(body []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data after the JSON body")
	}
	convertOTLPJSONIDs(root)
	return json.Marshal(root)
}

func convertOTLPJSONIDs(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if isIDField(key) {
				if text, ok := item.(string); ok {
					if decoded, err := hex.DecodeString(text); err == nil {
						typed[key] = base64.StdEncoding.EncodeToString(decoded)
						continue
					}
				}
			}
			convertOTLPJSONIDs(item)
		}
	case []any:
		for _, item := range typed {
			convertOTLPJSONIDs(item)
		}
	}
}

func isIDField(key string) bool {
	switch key {
	case "traceId", "spanId", "parentSpanId", "trace_id", "span_id", "parent_span_id":
		return true
	default:
		return false
	}
}

func writeHTTPResponse(w http.ResponseWriter, enc encoding, message proto.Message, requestID string) {
	var (
		body        []byte
		err         error
		contentType string
	)
	switch enc {
	case encodingJSON:
		contentType = "application/json"
		body, err = protojson.Marshal(message)
	default:
		contentType = "application/x-protobuf"
		body, err = proto.Marshal(message)
	}
	if err != nil {
		writeHTTPError(w, enc, errInternal(requestID, "failed to encode the OTLP response", err))
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func writeHTTPError(w http.ResponseWriter, enc encoding, requestErr *requestError) {
	w.Header().Set("X-Request-Id", requestErr.requestID)
	if requestErr.httpStatus == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Bearer realm="observaki"`)
	}
	if seconds := requestErr.retryAfterSeconds(); seconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
	}

	statusProto := requestErr.proto()
	if enc == encodingJSON {
		body, err := protojson.Marshal(statusProto)
		if err != nil {
			body = []byte(fmt.Sprintf(`{"code":%d,"message":%q,"request_id":%q}`, requestErr.code, requestErr.message, requestErr.requestID))
		} else {
			var payload map[string]json.RawMessage
			if json.Unmarshal(body, &payload) == nil {
				requestID, _ := json.Marshal(requestErr.requestID)
				payload["request_id"] = requestID
				if withID, err := json.Marshal(payload); err == nil {
					body = withID
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(requestErr.httpStatus)
		_, _ = w.Write(body)
		return
	}

	body, err := proto.Marshal(statusProto)
	if err != nil {
		body = nil
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(requestErr.httpStatus)
	_, _ = w.Write(body)
}

func (g *Gateway) recordHTTP(signal Signal, identity Identity, events, rejected int, requestErr *requestError, started time.Time, requestID string) {
	g.metrics.observeLatency(identity.TenantID, time.Since(started))
	g.metrics.observeRequest(signal, protocolHTTP)
	g.metrics.observeEvents(signal, protocolHTTP, events, rejected)
	if requestErr == nil {
		return
	}
	g.metrics.observeError(signal, protocolHTTP)
	g.logger.Warn(
		"otlp export rejected",
		"request_id", requestID,
		"tenant_id", identity.TenantID,
		"project_id", identity.ProjectID,
		"signal", signal.String(),
		"protocol", "http",
		"events", events,
		"rejected", rejected,
		"reason", requestErr.reason,
		"error", requestErr.message,
		"cause", errorString(requestErr.cause),
		"duration_ms", time.Since(started).Milliseconds(),
	)
}

func (g *Gateway) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *Gateway) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	if !g.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *Gateway) handleNotFound(w http.ResponseWriter, r *http.Request) {
	requestID := id.NewUUIDv7()
	writeHTTPError(w, encodingJSON, errNotFound(requestID, r.URL.Path))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
