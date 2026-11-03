package ingest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type staticSnapshotSource struct {
	snapshot TokenSnapshot
	err      error
}

func (s staticSnapshotSource) Load(context.Context) (TokenSnapshot, error) {
	return s.snapshot, s.err
}

type auditRecorder struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (r *auditRecorder) Record(_ context.Context, event AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *auditRecorder) last(t *testing.T) AuditEvent {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) == 0 {
		t.Fatal("no audit events recorded")
	}
	return r.events[len(r.events)-1]
}

func (r *auditRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func testSnapshot(now time.Time, tenants ...TenantConfig) TokenSnapshot {
	return TokenSnapshot{
		ConfigVersion: "test-1",
		Generation:    1,
		IssuedAt:      now.Add(-time.Minute),
		ExpiresAt:     now.Add(time.Hour),
		Tenants:       tenants,
	}
}

func tenantWithToken(tenantID, projectID, tokenID, token string, quota QuotaConfig, scopes ...string) TenantConfig {
	if len(scopes) == 0 {
		scopes = []string{ScopeIngest}
	}
	return TenantConfig{
		TenantID:    tenantID,
		IngestQuota: quota,
		Projects: []ProjectConfig{{
			ProjectID: projectID,
			Tokens: []TokenConfig{{
				TokenID: tokenID,
				SHA256:  hashToken(token),
				Scopes:  scopes,
			}},
		}},
	}
}

func newTestAuth(t *testing.T, snapshot TokenSnapshot, audit AuditSink) *AuthConfig {
	t.Helper()
	return &AuthConfig{
		Source: staticSnapshotSource{snapshot: snapshot},
		Audit:  audit,
	}
}

func newAuthedTestGateway(t *testing.T, sink Sink, snapshot TokenSnapshot, audit AuditSink) *Gateway {
	t.Helper()
	return newTestGateway(t, sink, func(cfg *Config) {
		cfg.Auth = newTestAuth(t, snapshot, audit)
	})
}

func TestCompileSnapshotValidation(t *testing.T) {
	now := time.Now().UTC()
	valid := func() TenantConfig {
		return tenantWithToken("tenant-a", "project-a", "token-a", "secret", QuotaConfig{EventsPerSecond: 100})
	}
	tests := []struct {
		name   string
		mutate func(*TokenSnapshot)
	}{
		{"missing version", func(s *TokenSnapshot) { s.ConfigVersion = "" }},
		{"missing generation", func(s *TokenSnapshot) { s.Generation = 0 }},
		{"missing issued_at", func(s *TokenSnapshot) { s.IssuedAt = time.Time{} }},
		{"missing expires_at", func(s *TokenSnapshot) { s.ExpiresAt = time.Time{} }},
		{"expiry before issue", func(s *TokenSnapshot) { s.ExpiresAt = s.IssuedAt.Add(-time.Minute) }},
		{"no tenants", func(s *TokenSnapshot) { s.Tenants = nil }},
		{"duplicate tenant", func(s *TokenSnapshot) { s.Tenants = append(s.Tenants, valid()) }},
		{"zero quota", func(s *TokenSnapshot) { s.Tenants[0].IngestQuota = QuotaConfig{} }},
		{"no projects", func(s *TokenSnapshot) { s.Tenants[0].Projects = nil }},
		{"no tokens", func(s *TokenSnapshot) { s.Tenants[0].Projects[0].Tokens = nil }},
		{"bad hash", func(s *TokenSnapshot) { s.Tenants[0].Projects[0].Tokens[0].SHA256 = "abc" }},
		{"empty scopes", func(s *TokenSnapshot) {
			s.Tenants[0].Projects[0].Tokens[0].Scopes = nil
		}},
		{"invalid project id", func(s *TokenSnapshot) { s.Tenants[0].Projects[0].ProjectID = "bad project" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := testSnapshot(now, valid())
			test.mutate(&snapshot)
			if _, err := compileSnapshot(snapshot); err == nil {
				t.Fatal("compileSnapshot() error = nil, want an error")
			}
		})
	}

	t.Run("default burst", func(t *testing.T) {
		snapshot := testSnapshot(now, valid())
		compiled, err := compileSnapshot(snapshot)
		if err != nil {
			t.Fatalf("compileSnapshot() error = %v", err)
		}
		if got := compiled.quotas["tenant-a"].BurstEvents; got != 100 {
			t.Errorf("default burst = %d, want 100", got)
		}
	})
}

func TestAuthenticatorTokens(t *testing.T) {
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	validTenant := tenantWithToken("tenant-a", "project-a", "token-a", "valid-token", QuotaConfig{EventsPerSecond: 100})
	validTenant.Projects[0].Tokens = append(validTenant.Projects[0].Tokens, TokenConfig{
		TokenID:   "token-expired",
		SHA256:    hashToken("expired-token"),
		Scopes:    []string{ScopeIngest},
		ExpiresAt: &expired,
	})
	queryOnly := tenantWithToken("tenant-b", "project-b", "token-query", "query-token", QuotaConfig{EventsPerSecond: 100}, "query")

	auth, err := NewAuthenticator(AuthConfig{
		Source: staticSnapshotSource{snapshot: testSnapshot(now, validTenant, queryOnly)},
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}

	tests := []struct {
		name    string
		header  string
		status  int
		reason  string
		tenant  string
		project string
	}{
		{name: "missing", header: "", status: http.StatusUnauthorized, reason: "missing_token"},
		{name: "wrong scheme", header: "Basic abc", status: http.StatusUnauthorized, reason: "missing_token"},
		{name: "unknown", header: "Bearer nope", status: http.StatusUnauthorized, reason: "invalid_token"},
		{name: "expired", header: "Bearer expired-token", status: http.StatusUnauthorized, reason: "expired_token"},
		{
			name:    "no ingest scope",
			header:  "Bearer query-token",
			status:  http.StatusForbidden,
			reason:  "insufficient_scope",
			tenant:  "tenant-b",
			project: "project-b",
		},
		{
			name:    "valid",
			header:  "Bearer valid-token",
			status:  0,
			tenant:  "tenant-a",
			project: "project-a",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity, requestErr := auth.Authenticate("req-1", test.header)
			if test.status == 0 {
				if requestErr != nil {
					t.Fatalf("Authenticate() error = %v", requestErr)
				}
				if identity.TenantID != test.tenant || identity.ProjectID != test.project {
					t.Errorf("identity = %+v, want tenant %q project %q", identity, test.tenant, test.project)
				}
				return
			}
			if requestErr == nil {
				t.Fatal("Authenticate() error = nil, want an error")
			}
			if requestErr.httpStatus != test.status || requestErr.reason != test.reason {
				t.Errorf("error = %d %q, want %d %q", requestErr.httpStatus, requestErr.reason, test.status, test.reason)
			}
			if identity.TenantID != test.tenant || identity.ProjectID != test.project {
				t.Errorf("identity = %+v, want tenant %q project %q", identity, test.tenant, test.project)
			}
		})
	}
}

func TestAuthenticatorFailsClosedWhenSnapshotExpires(t *testing.T) {
	now := time.Now().UTC()
	clock := now
	auth, err := NewAuthenticator(AuthConfig{
		Source: staticSnapshotSource{snapshot: testSnapshot(now, tenantWithToken("tenant-a", "project-a", "token-a", "valid", QuotaConfig{EventsPerSecond: 100}))},
		Now:    func() time.Time { return clock },
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	if _, requestErr := auth.Authenticate("req", "Bearer valid"); requestErr != nil {
		t.Fatalf("Authenticate() before expiry error = %v", requestErr)
	}

	clock = now.Add(2 * time.Hour)
	_, requestErr := auth.Authenticate("req", "Bearer valid")
	if requestErr == nil || requestErr.reason != "config_stale" || requestErr.httpStatus != http.StatusServiceUnavailable {
		t.Fatalf("Authenticate() after expiry = %+v, want config_stale 503", requestErr)
	}
	if _, requestErr := auth.Admit("req", Identity{TenantID: "tenant-a"}, 1); requestErr == nil || requestErr.reason != "config_stale" {
		t.Fatalf("Admit() after expiry = %+v, want config_stale", requestErr)
	}
	if state := auth.State(); !state.Expired {
		t.Error("State().Expired = false, want true")
	}
}

func TestAuthenticatorReloadsFileSnapshot(t *testing.T) {
	now := time.Now().UTC()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	writeSnapshot := func(tenantID, projectID, tokenID, token string) {
		t.Helper()
		snapshot := testSnapshot(now, tenantWithToken(tenantID, projectID, tokenID, token, QuotaConfig{EventsPerSecond: 100}))
		raw, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}
	}
	writeSnapshot("tenant-a", "project-a", "token-a", "first")

	source, err := NewFileSnapshotSource(path, "")
	if err != nil {
		t.Fatalf("NewFileSnapshotSource() error = %v", err)
	}
	auth, err := NewAuthenticator(AuthConfig{Source: source, ReloadInterval: -1})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	if identity, requestErr := auth.Authenticate("req", "Bearer first"); requestErr != nil || identity.TenantID != "tenant-a" {
		t.Fatalf("Authenticate(first) = %+v, %v", identity, requestErr)
	}

	writeSnapshot("tenant-b", "project-b", "token-b", "second")
	if err := auth.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if _, requestErr := auth.Authenticate("req", "Bearer first"); requestErr == nil || requestErr.reason != "invalid_token" {
		t.Fatalf("Authenticate(first) after reload = %+v, want invalid_token", requestErr)
	}
	identity, requestErr := auth.Authenticate("req", "Bearer second")
	if requestErr != nil || identity.TenantID != "tenant-b" || identity.ProjectID != "project-b" {
		t.Fatalf("Authenticate(second) = %+v, %v", identity, requestErr)
	}
	if state := auth.State(); state.Tenants != 1 || state.Generation != 1 {
		t.Errorf("State() = %+v", state)
	}
}

func TestFileSnapshotSourceSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey() error = %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	raw, err := json.Marshal(testSnapshot(time.Now().UTC(),
		tenantWithToken("tenant-a", "project-a", "token-a", "secret", QuotaConfig{EventsPerSecond: 100})))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, raw))
	if err := os.WriteFile(path+".sig", []byte(signature), 0o600); err != nil {
		t.Fatalf("os.WriteFile(signature) error = %v", err)
	}

	source, err := NewFileSnapshotSource(path, base64.StdEncoding.EncodeToString(publicKey))
	if err != nil {
		t.Fatalf("NewFileSnapshotSource() error = %v", err)
	}
	if _, err := source.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := os.WriteFile(path+".sig", []byte(base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))), 0o600); err != nil {
		t.Fatalf("os.WriteFile(tampered signature) error = %v", err)
	}
	if _, err := source.Load(context.Background()); err == nil {
		t.Fatal("Load() with a bad signature error = nil")
	}

	if err := os.Remove(path + ".sig"); err != nil {
		t.Fatalf("os.Remove() error = %v", err)
	}
	if _, err := source.Load(context.Background()); err == nil {
		t.Fatal("Load() without a signature error = nil")
	}
}

func TestEventLimiter(t *testing.T) {
	limiter := newEventLimiter()
	now := time.Now()
	limiter.setQuotas(map[string]QuotaConfig{
		"tenant-a": {EventsPerSecond: 100, BurstEvents: 250},
	})

	if _, allowed, configured := limiter.admit("unknown", 1, now); allowed || configured {
		t.Errorf("unknown tenant = allowed %v configured %v, want false false", allowed, configured)
	}
	if _, allowed, _ := limiter.admit("tenant-a", 250, now); !allowed {
		t.Fatal("burst admit was rejected")
	}
	retry, allowed, _ := limiter.admit("tenant-a", 50, now)
	if allowed {
		t.Fatal("over-burst admit was allowed")
	}
	if retry < time.Second {
		t.Errorf("retry after = %v, want at least 1s", retry)
	}
	if _, allowed, _ := limiter.admit("tenant-a", 50, now.Add(500*time.Millisecond)); !allowed {
		t.Error("admit after refill was rejected")
	}
}

func TestAuditFileHashChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	audit, err := OpenFileAudit(path)
	if err != nil {
		t.Fatalf("OpenFileAudit() error = %v", err)
	}
	events := []AuditEvent{
		{Action: AuditActionCrossProjectWrite, Outcome: AuditOutcomeDenied, RequestID: "req-1"},
		{Action: AuditActionConfigLoaded, Outcome: AuditOutcomeSuccess, ConfigVersion: "v1", Generation: 1},
	}
	for _, event := range events {
		if err := audit.Record(context.Background(), event); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}
	if err := audit.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := VerifyAuditFile(path); err != nil {
		t.Fatalf("VerifyAuditFile() error = %v", err)
	}

	// Reopening resumes the chain instead of starting a new one.
	reopened, err := OpenFileAudit(path)
	if err != nil {
		t.Fatalf("OpenFileAudit() after restart error = %v", err)
	}
	if err := reopened.Record(context.Background(), AuditEvent{Action: "test", Outcome: "success"}); err != nil {
		t.Fatalf("Record() after restart error = %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := VerifyAuditFile(path); err != nil {
		t.Fatalf("VerifyAuditFile() after restart error = %v", err)
	}

	// Tampering with a stored hash is detected.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var tampered AuditEvent
	if err := json.Unmarshal([]byte(lines[1]), &tampered); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	tampered.Hash = strings.Repeat("0", 64)
	lines[1] = string(mustJSON(t, tampered))
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := VerifyAuditFile(path); err == nil {
		t.Fatal("VerifyAuditFile() after tampering error = nil")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

func TestHTTPAuthRejectsInvalidToken(t *testing.T) {
	sink := &recordingSink{}
	audit := &auditRecorder{}
	snapshot := testSnapshot(time.Now().UTC(),
		tenantWithToken("tenant-a", "project-a", "token-a", "secret", QuotaConfig{EventsPerSecond: 100}))
	gw := newAuthedTestGateway(t, sink, snapshot, audit)
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue("hello")}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response := postOTLP(t, gw, "/v1/logs", body, "application/x-protobuf", nil)
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.StatusCode)
	}
	if got := response.Header.Get("WWW-Authenticate"); !strings.Contains(got, "Bearer") {
		t.Errorf("WWW-Authenticate = %q, want Bearer", got)
	}
	if sink.count() != 0 {
		t.Errorf("sink received %d batches, want 0", sink.count())
	}
	if audit.count() != 1 {
		t.Errorf("audit recorded %d events for a missing token, want only the config load", audit.count())
	}
}

func TestHTTPAuthCrossProjectIsForbiddenAndAudited(t *testing.T) {
	sink := &recordingSink{}
	audit := &auditRecorder{}
	snapshot := testSnapshot(time.Now().UTC(),
		tenantWithToken("tenant-a", "project-a", "token-a", "secret", QuotaConfig{EventsPerSecond: 100}))
	gw := newAuthedTestGateway(t, sink, snapshot, audit)
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue("hello")}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response := postOTLP(t, gw, "/v1/logs", body, "application/x-protobuf", map[string]string{
		"Authorization": "Bearer secret",
		HeaderProjectID: "project-b",
		HeaderTenantID:  "tenant-b",
	})
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
	if sink.count() != 0 {
		t.Errorf("sink received %d batches, want 0", sink.count())
	}
	event := audit.last(t)
	if event.Action != AuditActionCrossProjectWrite || event.Outcome != AuditOutcomeDenied {
		t.Errorf("audit event = %+v", event)
	}
	if event.TenantID != "tenant-a" || event.ProjectID != "project-a" {
		t.Errorf("audit source = %s/%s, want tenant-a/project-a", event.TenantID, event.ProjectID)
	}
	if event.TargetTenantID != "tenant-b" || event.TargetProjectID != "project-b" {
		t.Errorf("audit target = %s/%s, want tenant-b/project-b", event.TargetTenantID, event.TargetProjectID)
	}
	if event.RequestID == "" {
		t.Error("audit request_id is empty")
	}
}

func TestHTTPAuthScopesBatchIdentity(t *testing.T) {
	sink := &recordingSink{}
	snapshot := testSnapshot(time.Now().UTC(),
		tenantWithToken("tenant-a", "project-a", "token-a", "secret", QuotaConfig{EventsPerSecond: 100}))
	gw := newAuthedTestGateway(t, sink, snapshot, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue("hello")}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response := postOTLP(t, gw, "/v1/logs", body, "application/x-protobuf", map[string]string{
		"Authorization": "Bearer secret",
		HeaderProjectID: "project-a",
		HeaderTenantID:  "tenant-a",
	})
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	batch := sink.last(t)
	if batch.TenantID != "tenant-a" || batch.ProjectID != "project-a" {
		t.Errorf("batch identity = %s/%s, want tenant-a/project-a", batch.TenantID, batch.ProjectID)
	}
	if batch.ConfigVersion != "test-1" || batch.ConfigGeneration != 1 {
		t.Errorf("batch config = %s/%d, want test-1/1", batch.ConfigVersion, batch.ConfigGeneration)
	}
}

func TestHTTPQuotaReturns429WithRetryAfter(t *testing.T) {
	sink := &recordingSink{}
	snapshot := testSnapshot(time.Now().UTC(),
		tenantWithToken("tenant-a", "project-a", "token-a", "secret", QuotaConfig{
			EventsPerSecond: 1,
			BurstEvents:     5,
		}))
	gw := newAuthedTestGateway(t, sink, snapshot, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(
		&logspb.LogRecord{Body: stringValue("one")},
		&logspb.LogRecord{Body: stringValue("two")},
		&logspb.LogRecord{Body: stringValue("three")},
		&logspb.LogRecord{Body: stringValue("four")},
		&logspb.LogRecord{Body: stringValue("five")},
		&logspb.LogRecord{Body: stringValue("six")},
	))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response := postOTLP(t, gw, "/v1/logs", body, "application/x-protobuf", map[string]string{
		"Authorization": "Bearer secret",
	})
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", response.StatusCode)
	}
	if retryAfter := response.Header.Get("Retry-After"); retryAfter != "1" {
		t.Errorf("Retry-After = %q, want 1", retryAfter)
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	var statusProto statuspb.Status
	if err := proto.Unmarshal(payload, &statusProto); err != nil {
		t.Fatalf("proto.Unmarshal() error = %v", err)
	}
	if statusProto.GetCode() != int32(codes.ResourceExhausted) {
		t.Errorf("status code = %d, want %d", statusProto.GetCode(), codes.ResourceExhausted)
	}
	if sink.count() != 0 {
		t.Errorf("sink received %d batches, want 0", sink.count())
	}
}

func TestHTTPQuotaIsolatesTenants(t *testing.T) {
	now := time.Now().UTC()
	noisy := tenantWithToken("tenant-noisy", "project-noisy", "token-noisy", "noisy", QuotaConfig{
		EventsPerSecond: 100,
		BurstEvents:     10,
	})
	quiet := tenantWithToken("tenant-quiet", "project-quiet", "token-quiet", "quiet", QuotaConfig{
		EventsPerSecond: 10_000,
		BurstEvents:     10_000,
	})
	sink := &recordingSink{}
	gw := newAuthedTestGateway(t, sink, testSnapshot(now, noisy, quiet), nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	makeBody := func(events int) []byte {
		records := make([]*logspb.LogRecord, events)
		for i := range records {
			records[i] = &logspb.LogRecord{Body: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_IntValue{IntValue: int64(i)},
			}}
		}
		body, err := proto.Marshal(logsRequest(records...))
		if err != nil {
			t.Fatalf("proto.Marshal() error = %v", err)
		}
		return body
	}
	noisyBody := makeBody(11)
	quietBody := makeBody(5)

	var wg sync.WaitGroup
	type result struct {
		tenant string
		status int
	}
	results := make(chan result, 20)
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			response := postOTLP(t, gw, "/v1/logs", noisyBody, "application/x-protobuf", map[string]string{
				"Authorization": "Bearer noisy",
			})
			results <- result{tenant: "noisy", status: response.StatusCode}
			closeResponse(t, response)
		}()
		go func() {
			defer wg.Done()
			response := postOTLP(t, gw, "/v1/logs", quietBody, "application/x-protobuf", map[string]string{
				"Authorization": "Bearer quiet",
			})
			results <- result{tenant: "quiet", status: response.StatusCode}
			closeResponse(t, response)
		}()
	}
	wg.Wait()
	close(results)

	noisyLimited, quietLimited, quietOK := 0, 0, 0
	// The first noisy request may pass while the burst lasts; the rest must be
	// limited. The quiet tenant must never be limited.
	for result := range results {
		switch {
		case result.tenant == "noisy" && result.status == http.StatusTooManyRequests:
			noisyLimited++
		case result.tenant == "quiet" && result.status == http.StatusTooManyRequests:
			quietLimited++
		case result.tenant == "quiet" && result.status == http.StatusOK:
			quietOK++
		case result.status == http.StatusOK:
		default:
			t.Errorf("%s tenant got unexpected status %d", result.tenant, result.status)
		}
	}
	if noisyLimited == 0 {
		t.Error("noisy tenant received no 429 responses")
	}
	if quietLimited != 0 {
		t.Errorf("quiet tenant received %d 429 responses, want 0", quietLimited)
	}
	if quietOK != 10 {
		t.Errorf("quiet tenant got %d successful responses, want 10", quietOK)
	}
}

func TestGRPCAuthAndQuota(t *testing.T) {
	now := time.Now().UTC()
	snapshot := testSnapshot(now, tenantWithToken("tenant-a", "project-a", "token-a", "secret", QuotaConfig{
		EventsPerSecond: 1,
		BurstEvents:     3,
	}))
	audit := &auditRecorder{}
	sink := &recordingSink{}
	gw := newAuthedTestGateway(t, sink, snapshot, audit)
	startTestGateway(t, gw)
	waitReady(t, gw)

	conn := dialGRPC(t, gw.GRPCAddr())
	client := collectorlogspb.NewLogsServiceClient(conn)

	_, err := client.Export(context.Background(), logsRequest(&logspb.LogRecord{Body: stringValue("x")}))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("Export() without token error = %v, want Unauthenticated", err)
	}
	if sink.count() != 0 {
		t.Errorf("sink received %d batches, want 0", sink.count())
	}

	_, err = client.Export(
		metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer secret", "x-observaki-project-id", "project-b"),
		logsRequest(&logspb.LogRecord{Body: stringValue("x")}),
	)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Export() cross-project error = %v, want PermissionDenied", err)
	}
	if event := audit.last(t); event.Action != AuditActionCrossProjectWrite {
		t.Errorf("audit action = %q, want %q", event.Action, AuditActionCrossProjectWrite)
	}

	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer secret")
	if _, err := client.Export(ctx, logsRequest(
		&logspb.LogRecord{Body: stringValue("1")},
		&logspb.LogRecord{Body: stringValue("2")},
		&logspb.LogRecord{Body: stringValue("3")},
	)); err != nil {
		t.Fatalf("Export() within burst error = %v", err)
	}

	_, err = client.Export(ctx, logsRequest(
		&logspb.LogRecord{Body: stringValue("1")},
		&logspb.LogRecord{Body: stringValue("2")},
		&logspb.LogRecord{Body: stringValue("3")},
		&logspb.LogRecord{Body: stringValue("4")},
	))
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("Export() over quota error = %v, want ResourceExhausted", err)
	}
	var retryInfo *errdetails.RetryInfo
	for _, detail := range status.Convert(err).Details() {
		if info, ok := detail.(*errdetails.RetryInfo); ok {
			retryInfo = info
		}
	}
	if retryInfo == nil || retryInfo.GetRetryDelay() == nil {
		t.Fatalf("Export() over quota error has no RetryInfo: %v", err)
	}
	if batch := sink.last(t); batch.TenantID != "tenant-a" || batch.ProjectID != "project-a" {
		t.Errorf("batch identity = %s/%s", batch.TenantID, batch.ProjectID)
	}
}

func TestLogAuditFallback(t *testing.T) {
	var out strings.Builder
	audit := LogAudit{Logger: slog.New(slog.NewTextHandler(&out, nil))}
	if err := audit.Record(context.Background(), AuditEvent{
		Action:    AuditActionCrossProjectWrite,
		Outcome:   AuditOutcomeDenied,
		RequestID: "req",
	}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if !strings.Contains(out.String(), AuditActionCrossProjectWrite) {
		t.Errorf("log audit output = %q", out.String())
	}
}

func TestVerifyAuditFileMissingIsOK(t *testing.T) {
	if err := VerifyAuditFile(filepath.Join(t.TempDir(), "missing.jsonl")); err != nil {
		t.Errorf("VerifyAuditFile(missing) error = %v, want nil", err)
	}
}

func TestSnapshotSourceErrors(t *testing.T) {
	if _, err := NewFileSnapshotSource("", ""); err == nil {
		t.Error("empty snapshot path error = nil")
	}
	if _, err := NewFileSnapshotSource("x", "not-base64!"); err == nil {
		t.Error("invalid public key error = nil")
	}
	source := SnapshotSourceFunc(func(context.Context) (TokenSnapshot, error) {
		return TokenSnapshot{}, errors.New("boom")
	})
	if _, err := NewAuthenticator(AuthConfig{Source: source}); err == nil {
		t.Error("NewAuthenticator() with a failing source error = nil")
	}
}

func TestEventLimiterPreDecodeReservation(t *testing.T) {
	limiter := newEventLimiter()
	now := time.Now()
	limiter.setQuotas(map[string]QuotaConfig{
		"tenant-a": {EventsPerSecond: 1000, BurstEvents: 1000},
	})

	if _, warm, configured := limiter.estimate("tenant-a", 100_000); !warm || !configured {
		t.Fatalf("initial estimate warm=%v configured=%v, want true true", warm, configured)
	}
	limiter.recordPayload("tenant-a", 100_000, 1000)
	estimated, warm, configured := limiter.estimate("tenant-a", 100_000)
	if warm || !configured || estimated != 1000 {
		t.Fatalf("estimate = %d warm=%v configured=%v, want 1000 false true", estimated, warm, configured)
	}

	reservation, _, allowed, configured := limiter.reserve("tenant-a", estimated, 100_000, now)
	if !allowed || !configured {
		t.Fatalf("reserve allowed=%v configured=%v, want true true", allowed, configured)
	}
	if _, _, allowed, _ := limiter.reserve("tenant-a", 1, 100_000, now); allowed {
		t.Fatal("second reserve was allowed with an empty bucket")
	}

	if _, allowed, configured := limiter.settle(reservation, 500, now); !allowed || !configured {
		t.Fatalf("settle allowed=%v configured=%v, want true true", allowed, configured)
	}
	if reservation, _, allowed, _ := limiter.reserve("tenant-a", 500, 100_000, now); !allowed {
		t.Fatal("reserve after refund was rejected")
	} else {
		limiter.release(reservation)
	}
}

func TestHTTPPreDecodeQuotaRejection(t *testing.T) {
	sink := &recordingSink{}
	snapshot := testSnapshot(time.Now().UTC(),
		tenantWithToken("tenant-a", "project-a", "token-a", "secret", QuotaConfig{
			EventsPerSecond: 100,
			BurstEvents:     100,
		}))
	gw := newTestGateway(t, sink, func(cfg *Config) {
		cfg.MaxEvents = 1000
		cfg.Auth = newTestAuth(t, snapshot, nil)
	})
	startTestGateway(t, gw)
	waitReady(t, gw)

	makeBody := func(records int) []byte {
		items := make([]*logspb.LogRecord, records)
		for i := range items {
			items[i] = &logspb.LogRecord{Body: stringValue(strings.Repeat("x", 100))}
		}
		body, err := proto.Marshal(logsRequest(items...))
		if err != nil {
			t.Fatalf("proto.Marshal() error = %v", err)
		}
		return body
	}

	// The first request warms the payload estimate and consumes 10 events.
	response := postOTLP(t, gw, "/v1/logs", makeBody(10), "application/x-protobuf", map[string]string{
		"Authorization": "Bearer secret",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("warm-up status = %d, want 200", response.StatusCode)
	}
	closeResponse(t, response)

	// The second request is estimated above the remaining burst and must be
	// rejected before decoding, without reaching the sink.
	response = postOTLP(t, gw, "/v1/logs", makeBody(200), "application/x-protobuf", map[string]string{
		"Authorization": "Bearer secret",
	})
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("pre-decode status = %d, want 429", response.StatusCode)
	}
	if sink.count() != 1 {
		t.Errorf("sink received %d batches, want 1 (pre-decode rejection must not reach the sink)", sink.count())
	}
}
