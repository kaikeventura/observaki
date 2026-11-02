package writer

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPInserterSendsRowBinaryWithDedupeToken(t *testing.T) {
	var gotQuery string
	var gotBody []byte
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotPath = request.URL.Path
		gotQuery = request.URL.RawQuery
		gotBody, _ = io.ReadAll(request.Body)
		response.Header().Set("X-ClickHouse-Summary", `{"written_rows":"99","written_bytes":"1234"}`)
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	inserter, err := NewHTTPInserter(HTTPConfig{
		Endpoint: server.URL,
		Database: "observaki_bench",
		User:     "observaki",
	})
	if err != nil {
		t.Fatalf("NewHTTPInserter() error = %v", err)
	}
	result, err := inserter.Insert(context.Background(), InsertRequest{
		Table:       "otel_logs",
		Data:        []byte("binary-payload"),
		Rows:        100,
		DedupeToken: "logs-1-abc",
	})
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if gotPath != "/" {
		t.Fatalf("request path = %q, want /", gotPath)
	}
	for _, fragment := range []string{
		"database=observaki_bench",
		"async_insert=1",
		"wait_for_async_insert=1",
		"insert_deduplicate=1",
		"insert_deduplication_token=logs-1-abc",
		"max_insert_threads=1",
		"user=observaki",
	} {
		if !strings.Contains(gotQuery, fragment) {
			t.Errorf("query %q is missing %q", gotQuery, fragment)
		}
	}
	if !strings.Contains(gotQuery, "INSERT+INTO+otel_logs+FORMAT+RowBinary") {
		t.Errorf("query %q is missing the insert statement", gotQuery)
	}
	if string(gotBody) != "binary-payload" {
		t.Fatalf("body = %q, want binary-payload", gotBody)
	}
	if result.WrittenRows != 99 {
		t.Fatalf("result = %+v, want 99 written rows", result)
	}
}

func TestHTTPInserterGzip(t *testing.T) {
	var gotEncoding string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotEncoding = request.Header.Get("Content-Encoding")
		reader, err := gzip.NewReader(request.Body)
		if err != nil {
			t.Errorf("gzip.NewReader() error = %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		gotBody, _ = io.ReadAll(reader)
		_ = reader.Close()
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	inserter, err := NewHTTPInserter(HTTPConfig{
		Endpoint:    server.URL,
		Database:    "db",
		Compression: "gzip",
	})
	if err != nil {
		t.Fatalf("NewHTTPInserter() error = %v", err)
	}
	_, err = inserter.Insert(context.Background(), InsertRequest{
		Table:       "otel_traces",
		Data:        []byte(strings.Repeat("span", 100)),
		Rows:        1,
		DedupeToken: "traces-1-abc",
	})
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if gotEncoding != "gzip" || string(gotBody) != strings.Repeat("span", 100) {
		t.Fatalf("encoding = %q, body length = %d, want gzip and the original payload", gotEncoding, len(gotBody))
	}
}

func TestHTTPInserterClassifiesErrors(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		permanent bool
	}{
		{name: "bad request", status: http.StatusBadRequest, body: "Code: 62. DB::Exception: syntax error", permanent: true},
		{name: "forbidden", status: http.StatusForbidden, body: "Code: 497. access denied", permanent: true},
		{name: "too many requests", status: http.StatusTooManyRequests, body: "Code: 159. timeout", permanent: false},
		{name: "server error", status: http.StatusServiceUnavailable, body: "Code: 202. too many parts", permanent: false},
		{name: "gateway timeout", status: http.StatusGatewayTimeout, body: "", permanent: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.status)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()
			inserter, err := NewHTTPInserter(HTTPConfig{Endpoint: server.URL, Database: "db"})
			if err != nil {
				t.Fatalf("NewHTTPInserter() error = %v", err)
			}
			_, err = inserter.Insert(context.Background(), InsertRequest{Table: "otel_logs", Data: []byte("x"), Rows: 1, DedupeToken: "t"})
			if err == nil {
				t.Fatal("Insert() error = nil, want an error")
			}
			if got := IsPermanent(err); got != test.permanent {
				t.Fatalf("IsPermanent(%v) = %v, want %v", err, got, test.permanent)
			}
		})
	}
}

func TestHTTPInserterRejectsInvalidRequests(t *testing.T) {
	inserter, err := NewHTTPInserter(HTTPConfig{Endpoint: "http://127.0.0.1:1", Database: "db"})
	if err != nil {
		t.Fatalf("NewHTTPInserter() error = %v", err)
	}
	tests := []InsertRequest{
		{Table: "otel_logs", Data: []byte("x"), Rows: 1},
		{Table: "", Data: []byte("x"), Rows: 1, DedupeToken: "t"},
		{Table: "otel_logs; DROP", Data: []byte("x"), Rows: 1, DedupeToken: "t"},
		{Table: "otel_logs", Data: []byte("x"), Rows: 0, DedupeToken: "t"},
	}
	for _, request := range tests {
		if _, err := inserter.Insert(context.Background(), request); err == nil {
			t.Fatalf("Insert(%+v) error = nil, want an error", request)
		}
	}
}

func TestSummaryUint(t *testing.T) {
	summary := `{"written_rows":"42","written_bytes":"2048"}`
	if value, ok := summaryUint(summary, "written_rows"); !ok || value != 42 {
		t.Fatalf("written_rows = %d, %v, want 42", value, ok)
	}
	if value, ok := summaryUint(summary, "written_bytes"); !ok || value != 2048 {
		t.Fatalf("written_bytes = %d, %v, want 2048", value, ok)
	}
	if _, ok := summaryUint(summary, "missing"); ok {
		t.Fatal("missing field was reported as present")
	}
	if _, ok := summaryUint("not-json", "written_rows"); ok {
		t.Fatal("invalid JSON was accepted")
	}
}
