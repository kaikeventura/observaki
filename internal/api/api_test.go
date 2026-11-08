package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	testToken      = "test-token-value"
	testProjectID  = "00000000-0000-4000-8000-000000000002"
	otherProjectID = "00000000-0000-4000-8000-0000000000ff"
)

type fakeControlPlane struct {
	principals map[string]Principal
	projects   map[string]Project
	tokens     map[string][]IngestToken
	authErr    error
}

func newFakeControlPlane(scopes ...string) *fakeControlPlane {
	hash := sha256.Sum256([]byte(testToken))
	return &fakeControlPlane{
		principals: map[string]Principal{
			hex.EncodeToString(hash[:]): {
				TokenID:   "token-1",
				OrgID:     "00000000-0000-4000-8000-000000000001",
				ProjectID: testProjectID,
				Scopes:    scopes,
			},
		},
		projects: map[string]Project{
			testProjectID: {
				ID:         testProjectID,
				OrgID:      "00000000-0000-4000-8000-000000000001",
				Name:       "Development",
				Slug:       "dev",
				Generation: 1,
				CreatedAt:  time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
			},
		},
		tokens: map[string][]IngestToken{
			testProjectID: {{
				ID:        "00000000-0000-4000-8000-000000000009",
				Name:      "Development ingest token",
				Scopes:    []string{"ingest"},
				CreatedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
			}},
		},
	}
}

func (f *fakeControlPlane) AuthenticateToken(_ context.Context, hash [sha256.Size]byte) (Principal, error) {
	if f.authErr != nil {
		return Principal{}, f.authErr
	}
	principal, ok := f.principals[hex.EncodeToString(hash[:])]
	if !ok {
		return Principal{}, ErrTokenNotFound
	}
	return principal, nil
}

func (f *fakeControlPlane) Project(_ context.Context, projectID string) (Project, error) {
	project, ok := f.projects[projectID]
	if !ok {
		return Project{}, ErrProjectNotFound
	}
	return project, nil
}

func (f *fakeControlPlane) IngestTokens(_ context.Context, projectID string) ([]IngestToken, error) {
	return f.tokens[projectID], nil
}

func newTestAPI(t *testing.T, controlPlane ControlPlane) *API {
	t.Helper()
	handler := New(Config{
		Version:      "v1.2.3-test",
		Logger:       slog.New(slog.DiscardHandler),
		ControlPlane: controlPlane,
	})
	mux := http.NewServeMux()
	handler.Register(mux)
	return handler
}

func serve(t *testing.T, handler *API, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	handler.Register(mux)
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	RequestID(Recover(handler.logger, mux)).ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) Error {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	var body Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (raw %q)", err, rec.Body.String())
	}
	if body.RequestID == "" || body.RequestID != rec.Header().Get(RequestIDHeader) {
		t.Fatalf("request_id = %q, header = %q", body.RequestID, rec.Header().Get(RequestIDHeader))
	}
	if body.Details == nil {
		t.Fatal("details is nil; AD-15 requires the field")
	}
	if strings.Contains(rec.Body.String(), "goroutine") || strings.Contains(rec.Body.String(), "panic") {
		t.Fatalf("error body leaks internal detail: %s", rec.Body.String())
	}
	return body
}

func TestHealthAndOpenAPIStayPublic(t *testing.T) {
	handler := newTestAPI(t, newFakeControlPlane("admin"))

	for _, path := range []string{"/healthz", "/api/v1/health", "/openapi.json"} {
		rec := serve(t, handler, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
		if rec.Header().Get(RequestIDHeader) == "" {
			t.Fatalf("%s did not set %s", path, RequestIDHeader)
		}
	}

	rec := serve(t, handler, http.MethodGet, "/openapi.json", "")
	var document map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}
	if document["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v, want 3.1.0", document["openapi"])
	}
}

func TestAuthErrorsUseTheAD15Envelope(t *testing.T) {
	handler := newTestAPI(t, newFakeControlPlane("admin"))

	cases := []struct {
		name       string
		token      string
		header     string
		wantStatus int
		wantCode   string
		wantReason string
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized, wantCode: CodeUnauthorized, wantReason: "missing_credentials"},
		{name: "wrong scheme", header: "Basic abc", wantStatus: http.StatusUnauthorized, wantCode: CodeUnauthorized, wantReason: "invalid_credentials"},
		{name: "unknown token", token: "not-a-real-token", wantStatus: http.StatusUnauthorized, wantCode: CodeUnauthorized, wantReason: "invalid_token"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			handler.Register(mux)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			RequestID(Recover(handler.logger, mux)).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			body := decodeError(t, rec)
			if body.Code != tc.wantCode || body.Details["reason"] != tc.wantReason {
				t.Fatalf("body = %+v", body)
			}
		})
	}
}

func TestScopeAndProjectIsolation(t *testing.T) {
	t.Run("missing scope", func(t *testing.T) {
		handler := newTestAPI(t, newFakeControlPlane("query"))
		rec := serve(t, handler, http.MethodGet,
			"/api/v1/projects/"+testProjectID+"/ingest-tokens", testToken)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		body := decodeError(t, rec)
		if body.Details["required_scope"] != "admin" {
			t.Fatalf("details = %+v", body.Details)
		}
	})

	t.Run("cross-project token", func(t *testing.T) {
		handler := newTestAPI(t, newFakeControlPlane("admin"))
		rec := serve(t, handler, http.MethodGet,
			"/api/v1/projects/"+otherProjectID+"/ingest-tokens", testToken)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		if body := decodeError(t, rec); body.Details["reason"] != "project_scope" {
			t.Fatalf("details = %+v", body.Details)
		}
	})

	t.Run("invalid project id", func(t *testing.T) {
		handler := newTestAPI(t, newFakeControlPlane("admin"))
		rec := serve(t, handler, http.MethodGet, "/api/v1/projects/not-a-uuid/ingest-tokens", testToken)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if body := decodeError(t, rec); body.Details["field"] != "projectID" {
			t.Fatalf("details = %+v", body.Details)
		}
	})
}

func TestProjectsAndIngestTokens(t *testing.T) {
	handler := newTestAPI(t, newFakeControlPlane("admin"))

	rec := serve(t, handler, http.MethodGet, "/api/v1/projects", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("projects status = %d, body %s", rec.Code, rec.Body.String())
	}
	var projects ProjectList
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("decode projects: %v", err)
	}
	if len(projects.Projects) != 1 || projects.Projects[0].ID != testProjectID {
		t.Fatalf("projects = %+v", projects)
	}

	rec = serve(t, handler, http.MethodGet,
		"/api/v1/projects/"+testProjectID+"/ingest-tokens", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("tokens status = %d, body %s", rec.Code, rec.Body.String())
	}
	var tokens IngestTokenList
	if err := json.Unmarshal(rec.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("decode tokens: %v", err)
	}
	if len(tokens.Tokens) != 1 || tokens.Tokens[0].Name != "Development ingest token" {
		t.Fatalf("tokens = %+v", tokens)
	}
	// The digest must never appear in the payload (AD-19).
	if strings.Contains(strings.ToLower(rec.Body.String()), "sha256") ||
		strings.Contains(rec.Body.String(), hex.EncodeToString(make([]byte, 32))) {
		t.Fatalf("token response leaked a digest: %s", rec.Body.String())
	}
}

func TestSampleQueryContract(t *testing.T) {
	handler := newTestAPI(t, newFakeControlPlane("query"))

	rec := serve(t, handler, http.MethodGet, "/api/v1/query/sample?signal=logs", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var sample SampleQuery
	if err := json.Unmarshal(rec.Body.Bytes(), &sample); err != nil {
		t.Fatalf("decode sample: %v", err)
	}
	if !sample.Sample || sample.Signal != "logs" || sample.ProjectID != testProjectID ||
		sample.OrgID != "00000000-0000-4000-8000-000000000001" || sample.RequestID == "" ||
		len(sample.Series) != 1 || len(sample.Series[0].Points) != 5 {
		t.Fatalf("sample = %+v", sample)
	}

	rec = serve(t, handler, http.MethodGet, "/api/v1/query/sample?signal=unknown", testToken)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid signal status = %d, want 400", rec.Code)
	}
	if body := decodeError(t, rec); body.Details["field"] != "signal" {
		t.Fatalf("details = %+v", body.Details)
	}
}

func TestReservedQueryAndUnknownRoutes(t *testing.T) {
	handler := newTestAPI(t, newFakeControlPlane("admin"))

	rec := serve(t, handler, http.MethodPost, "/api/v1/query", testToken)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("query status = %d, want 501", rec.Code)
	}
	if body := decodeError(t, rec); body.Code != CodeQueryNotAvailable {
		t.Fatalf("code = %q", body.Code)
	}

	rec = serve(t, handler, http.MethodGet, "/api/v1/does-not-exist", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d, want 404", rec.Code)
	}
	if body := decodeError(t, rec); body.Code != CodeNotFound {
		t.Fatalf("code = %q", body.Code)
	}

	rec = serve(t, handler, http.MethodDelete, "/api/v1/projects", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d, want 405", rec.Code)
	}
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q", rec.Header().Get("Allow"))
	}
}

func TestControlPlaneUnavailable(t *testing.T) {
	handler := newTestAPI(t, nil)
	rec := serve(t, handler, http.MethodGet, "/api/v1/projects", testToken)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body := decodeError(t, rec); body.Code != CodeMetadataUnavailable {
		t.Fatalf("code = %q", body.Code)
	}
}

func TestRecoverNeverLeaksStack(t *testing.T) {
	handler := New(Config{Version: "test", Logger: slog.New(slog.DiscardHandler)})
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom with secret")
	})
	chain := RequestID(Recover(handler.logger, panicking))

	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := decodeError(t, rec)
	if body.Code != CodeInternalError {
		t.Fatalf("code = %q", body.Code)
	}
	if strings.Contains(rec.Body.String(), "boom with secret") {
		t.Fatalf("panic value leaked: %s", rec.Body.String())
	}
}

func TestOpenAPIDocumentMatchesOperations(t *testing.T) {
	document := OpenAPIDocument()
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths has type %T", document["paths"])
	}

	documented := 0
	for _, operation := range Operations() {
		item, ok := paths[operation.Path].(map[string]any)
		if !ok {
			t.Fatalf("path %s is not documented", operation.Path)
		}
		if _, ok := item[strings.ToLower(operation.Method)]; !ok {
			t.Fatalf("%s %s is not documented", operation.Method, operation.Path)
		}
		documented++
	}
	for path, raw := range paths {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("path %s has type %T", path, raw)
		}
		documented -= len(item)
	}
	if documented != 0 {
		t.Fatalf("the document has %d operations that the router does not register", -documented)
	}

	for _, path := range []string{
		"/api/v1/health", "/api/v1/health/overview", "/api/v1/projects",
		"/api/v1/projects/{projectID}/ingest-tokens", "/api/v1/query/sample",
	} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("AC1 endpoint %s is missing from the contract", path)
		}
	}
}

func TestOpenAPIGoldenAndChangelog(t *testing.T) {
	generated := OpenAPIJSON()

	golden, err := os.ReadFile("../../" + OpenAPIGoldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (run `make api-gen`)", err)
	}
	if !bytes.Equal(golden, generated) {
		t.Fatal("api/openapi/openapi.json is stale; run `make api-gen` and commit the result")
	}

	changelog, err := os.ReadFile("../../api/openapi/CHANGELOG.md")
	if err != nil {
		t.Fatalf("read changelog: %v", err)
	}
	if !strings.Contains(string(changelog), "## "+OpenAPIVersion) {
		t.Fatalf("the OpenAPI changelog has no entry for %s", OpenAPIVersion)
	}
}
