package ingest

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
)

func TestHTTPAcceptsAllSignals(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	tests := []struct {
		name   string
		path   string
		body   []byte
		signal Signal
		events int
	}{
		{
			name:   "traces",
			path:   "/v1/traces",
			signal: SignalTraces,
			events: 2,
		},
		{
			name:   "logs",
			path:   "/v1/logs",
			signal: SignalLogs,
			events: 2,
		},
		{
			name:   "metrics",
			path:   "/v1/metrics",
			signal: SignalMetrics,
			events: 2,
		},
	}

	requests := map[string]proto.Message{
		"traces": tracesRequest(&tracepb.Span{Name: "one"}, &tracepb.Span{Name: "two"}),
		"logs": logsRequest(
			&logspb.LogRecord{Body: stringValue("one")},
			&logspb.LogRecord{Body: stringValue("two")},
		),
		"metrics": metricsRequest(gaugeMetric("test.metric", &metricpb.NumberDataPoint{}, &metricpb.NumberDataPoint{})),
	}
	for i := range tests {
		body, err := proto.Marshal(requests[tests[i].name])
		if err != nil {
			t.Fatalf("proto.Marshal(%s) error = %v", tests[i].name, err)
		}
		tests[i].body = body
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := postOTLP(t, gw, test.path, test.body, "application/x-protobuf", nil)
			defer closeResponse(t, response)
			if response.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("status = %d, want 200; body = %s", response.StatusCode, body)
			}
			if got := response.Header.Get("X-Request-Id"); got == "" {
				t.Error("X-Request-Id is empty")
			}

			batch := sink.last(t)
			if batch.Signal != test.signal {
				t.Errorf("sink signal = %q, want %q", batch.Signal, test.signal)
			}
			if got := batch.Events(); got != test.events {
				t.Errorf("sink events = %d, want %d", got, test.events)
			}
		})
	}
}

func TestHTTPJSONWithHexIDs(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	body := []byte(`{
		"resourceLogs": [{
			"scopeLogs": [{
				"logRecords": [{
					"timeUnixNano": "1700000000000000000",
					"traceId": "5b8efff798038103d269b633813fc60c",
					"spanId": "ee4802b5b8c4c0b1",
					"body": {"stringValue": "hello"}
				}]
			}]
		}],
		"unknownField": true
	}`)
	response := postOTLP(t, gw, "/v1/logs", body, "application/json", nil)
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want 200; body = %s", response.StatusCode, responseBody)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	record := sink.last(t).Logs.GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()[0]
	wantTraceID := []byte{0x5b, 0x8e, 0xff, 0xf7, 0x98, 0x03, 0x81, 0x03, 0xd2, 0x69, 0xb6, 0x33, 0x81, 0x3f, 0xc6, 0x0c}
	if !bytes.Equal(record.GetTraceId(), wantTraceID) {
		t.Errorf("trace ID = %x, want %x", record.GetTraceId(), wantTraceID)
	}
	if got := record.GetBody().GetStringValue(); got != "hello" {
		t.Errorf("body = %q, want hello", got)
	}
}

func TestHTTPGzip(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue("compressed")}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(body); err != nil {
		t.Fatalf("gzip write error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close error = %v", err)
	}

	response := postOTLP(t, gw, "/v1/logs", compressed.Bytes(), "application/x-protobuf", map[string]string{
		"Content-Encoding": "gzip",
	})
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want 200; body = %s", response.StatusCode, responseBody)
	}
	if got := sink.last(t).Events(); got != 1 {
		t.Errorf("sink events = %d, want 1", got)
	}
}

func TestHTTPPartialSuccess(t *testing.T) {
	sink := &recordingSink{result: func(batch *Batch) (Result, error) {
		return Result{Accepted: batch.Events() - 2, Rejected: 2, Reason: "two spans were invalid"}, nil
	}}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(tracesRequest(
		&tracepb.Span{Name: "one"},
		&tracepb.Span{Name: "two"},
		&tracepb.Span{Name: "three"},
	))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response := postOTLP(t, gw, "/v1/traces", body, "application/x-protobuf", nil)
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response error = %v", err)
	}
	exportResponse := &collectortracepb.ExportTraceServiceResponse{}
	if err := proto.Unmarshal(responseBody, exportResponse); err != nil {
		t.Fatalf("unmarshal response error = %v", err)
	}
	if exportResponse.GetPartialSuccess() == nil {
		t.Fatal("PartialSuccess = nil, want rejected count and reason")
	}
	if got := exportResponse.GetPartialSuccess().GetRejectedSpans(); got != 2 {
		t.Errorf("RejectedSpans = %d, want 2", got)
	}
	if got := exportResponse.GetPartialSuccess().GetErrorMessage(); got != "two spans were invalid" {
		t.Errorf("ErrorMessage = %q", got)
	}
}

func TestHTTPEventLimit(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, func(cfg *Config) { cfg.MaxEvents = 2 })
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(
		&logspb.LogRecord{Body: stringValue("one")},
		&logspb.LogRecord{Body: stringValue("two")},
		&logspb.LogRecord{Body: stringValue("three")},
	))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response := postOTLP(t, gw, "/v1/logs", body, "application/x-protobuf", nil)
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.StatusCode)
	}
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response error = %v", err)
	}
	statusMessage := &statuspb.Status{}
	if err := proto.Unmarshal(responseBody, statusMessage); err != nil {
		t.Fatalf("unmarshal status error = %v", err)
	}
	if !strings.Contains(statusMessage.GetMessage(), "limit is 2") {
		t.Errorf("status message = %q, want the configured limit", statusMessage.GetMessage())
	}
	if got := sink.count(); got != 0 {
		t.Errorf("sink received %d batches, want 0", got)
	}
}

func TestHTTPRequestSizeLimit(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, func(cfg *Config) { cfg.MaxRequestBytes = 512 })
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue(strings.Repeat("x", 2048))}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response := postOTLP(t, gw, "/v1/logs", body, "application/x-protobuf", nil)
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.StatusCode)
	}
	if got := sink.count(); got != 0 {
		t.Errorf("sink received %d batches, want 0", got)
	}
}

func TestHTTPGzipBombIsLimitedAfterDecompression(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, func(cfg *Config) { cfg.MaxRequestBytes = 1024 })
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue(strings.Repeat("x", 64*1024))}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(body); err != nil {
		t.Fatalf("gzip write error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close error = %v", err)
	}
	if compressed.Len() > 1024 {
		t.Fatalf("compressed body is %d bytes; the test needs it under the limit", compressed.Len())
	}

	response := postOTLP(t, gw, "/v1/logs", compressed.Bytes(), "application/x-protobuf", map[string]string{
		"Content-Encoding": "gzip",
	})
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.StatusCode)
	}
}

func TestHTTPErrors(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	tests := []struct {
		name        string
		method      string
		path        string
		body        []byte
		contentType string
		wantStatus  int
	}{
		{name: "method not allowed", method: http.MethodGet, path: "/v1/logs", wantStatus: http.StatusMethodNotAllowed},
		{name: "unsupported media type", method: http.MethodPost, path: "/v1/logs", body: []byte("{}"), contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType},
		{name: "malformed protobuf", method: http.MethodPost, path: "/v1/logs", body: []byte{0xff, 0xff}, contentType: "application/x-protobuf", wantStatus: http.StatusBadRequest},
		{name: "malformed json", method: http.MethodPost, path: "/v1/logs", body: []byte("{"), contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "unknown path", method: http.MethodPost, path: "/v1/unknown", body: []byte("{}"), contentType: "application/json", wantStatus: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(test.method, "http://"+gw.HTTPAddr()+test.path, bytes.NewReader(test.body))
			if err != nil {
				t.Fatalf("http.NewRequest() error = %v", err)
			}
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response, err := testHTTPClient.Do(request)
			if err != nil {
				t.Fatalf("request error = %v", err)
			}
			defer closeResponse(t, response)
			if response.StatusCode != test.wantStatus {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, test.wantStatus, body)
			}
			if got := response.Header.Get("X-Request-Id"); got == "" {
				t.Error("X-Request-Id is empty")
			}
		})
	}
}

func TestHTTPJSONErrorShape(t *testing.T) {
	gw := newTestGateway(t, &recordingSink{}, func(cfg *Config) { cfg.MaxEvents = 1 })
	startTestGateway(t, gw)
	waitReady(t, gw)

	body := []byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"one"}},{"body":{"stringValue":"two"}}]}]}]}`)
	response := postOTLP(t, gw, "/v1/logs", body, "application/json", nil)
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var payload struct {
		Code      int32  `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response error = %v", err)
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		t.Fatalf("unmarshal error = %v; body = %s", err, responseBody)
	}
	if payload.Code == 0 || payload.Message == "" || payload.RequestID == "" {
		t.Fatalf("error envelope = %+v, want code, message and request_id", payload)
	}
	if !strings.Contains(payload.Message, "limit is 1") {
		t.Errorf("message = %q, want the configured limit", payload.Message)
	}
}

func TestHTTPEmptyRequestSucceeds(t *testing.T) {
	gw := newTestGateway(t, &recordingSink{}, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	response := postOTLP(t, gw, "/v1/traces", nil, "application/x-protobuf", nil)
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
}

func TestHTTPHealthReadyAndMetrics(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	health := getBody(t, "http://"+gw.HTTPAddr()+"/healthz")
	if health.status != http.StatusOK || !strings.Contains(health.body, `"status":"ok"`) {
		t.Fatalf("healthz = %d %s", health.status, health.body)
	}
	ready := getBody(t, "http://"+gw.HTTPAddr()+"/readyz")
	if ready.status != http.StatusOK || !strings.Contains(ready.body, `"status":"ok"`) {
		t.Fatalf("readyz = %d %s", ready.status, ready.body)
	}

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue("hello")}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response := postOTLP(t, gw, "/v1/logs", body, "application/x-protobuf", nil)
	_ = response.Body.Close()

	metrics := getBody(t, "http://"+gw.HTTPAddr()+"/metrics")
	if metrics.status != http.StatusOK {
		t.Fatalf("metrics status = %d", metrics.status)
	}
	for _, want := range []string{
		"observaki_gateway_requests_total",
		`observaki_gateway_requests_total{signal="logs",protocol="http"} 1`,
		`observaki_gateway_events_total{signal="logs",protocol="http"} 1`,
		"observaki_gateway_up 1",
	} {
		if !strings.Contains(metrics.body, want) {
			t.Errorf("metrics body does not contain %q:\n%s", want, metrics.body)
		}
	}
}

type httpBody struct {
	status int
	body   string
}

func getBody(t *testing.T, url string) httpBody {
	t.Helper()
	response, err := testHTTPClient.Get(url)
	if err != nil {
		t.Fatalf("GET %s error = %v", url, err)
	}
	defer closeResponse(t, response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s error = %v", url, err)
	}
	return httpBody{status: response.StatusCode, body: string(body)}
}

func postOTLP(t *testing.T, gw *Gateway, path string, body []byte, contentType string, headers map[string]string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://"+gw.HTTPAddr()+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", contentType)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := testHTTPClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s error = %v", path, err)
	}
	return response
}

func closeResponse(t *testing.T, response *http.Response) {
	t.Helper()
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}
