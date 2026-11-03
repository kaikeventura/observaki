package ingest

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ScopeIngest is the token scope required to write telemetry through the
// gateway. Tokens are project-scoped (AD-2/AD-3).
const ScopeIngest = "ingest"

// DefaultAuthReloadInterval is how often the gateway re-reads its config
// snapshot (AD-19).
const DefaultAuthReloadInterval = 30 * time.Second

const (
	maxAuthSnapshotBytes = 16 << 20
	maxTokenBytes        = 4096
	maxIdentifierBytes   = 128
)

// Identity is the authenticated tenant/project behind an OTLP request. It is
// always resolved from the token; client-provided routing hints are only
// assertions checked against it (AD-3).
type Identity struct {
	TenantID      string
	ProjectID     string
	TokenID       string
	ConfigVersion string
	Generation    int64
}

// QuotaConfig is the per-tenant ingest quota, measured in events per second
// (spans, log records or data points).
type QuotaConfig struct {
	EventsPerSecond float64 `json:"events_per_second"`
	BurstEvents     int     `json:"burst_events"`
}

// TenantConfig is one tenant in the config snapshot.
type TenantConfig struct {
	TenantID    string          `json:"tenant_id"`
	IngestQuota QuotaConfig     `json:"ingest_quota"`
	Projects    []ProjectConfig `json:"projects"`
}

// ProjectConfig groups the tokens that may write to a project.
type ProjectConfig struct {
	ProjectID string        `json:"project_id"`
	Tokens    []TokenConfig `json:"tokens"`
}

// TokenConfig is one hashed API token. Only the SHA-256 digest is stored; the
// plaintext token is never written to the snapshot (AD-13).
type TokenConfig struct {
	TokenID   string     `json:"token_id"`
	SHA256    string     `json:"sha256"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// TokenSnapshot is the versioned configuration snapshot consumed by the
// gateway (AD-19). The file transport used in Phase 0 is replaced by the
// signed NATS/HTTP snapshot from the reconciler in Phase 1.
type TokenSnapshot struct {
	ConfigVersion string         `json:"config_version"`
	Generation    int64          `json:"generation"`
	IssuedAt      time.Time      `json:"issued_at"`
	ExpiresAt     time.Time      `json:"expires_at"`
	Tenants       []TenantConfig `json:"tenants"`
}

// SnapshotSource loads the current config snapshot.
type SnapshotSource interface {
	Load(ctx context.Context) (TokenSnapshot, error)
}

// SnapshotSourceFunc adapts a function to SnapshotSource.
type SnapshotSourceFunc func(ctx context.Context) (TokenSnapshot, error)

// Load implements SnapshotSource.
func (f SnapshotSourceFunc) Load(ctx context.Context) (TokenSnapshot, error) {
	return f(ctx)
}

// FileSnapshotSource reads a JSON snapshot and, when a public key is
// configured, verifies the detached Ed25519 signature in "<path>.sig". The
// signature is mandatory outside development/test (AD-19).
type FileSnapshotSource struct {
	path      string
	publicKey ed25519.PublicKey
}

// NewFileSnapshotSource builds a file-backed snapshot source. publicKey is the
// base64 (standard or raw) Ed25519 public key; empty disables verification.
func NewFileSnapshotSource(path, publicKey string) (*FileSnapshotSource, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("ingest: auth snapshot path is required")
	}
	source := &FileSnapshotSource{path: path}
	if strings.TrimSpace(publicKey) == "" {
		return source, nil
	}
	decoded, err := decodeBase64(publicKey)
	if err != nil {
		return nil, fmt.Errorf("ingest: decode auth snapshot public key: %w", err)
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("ingest: auth snapshot public key is %d bytes, want %d", len(decoded), ed25519.PublicKeySize)
	}
	source.publicKey = ed25519.PublicKey(decoded)
	return source, nil
}

// Load implements SnapshotSource.
func (s *FileSnapshotSource) Load(ctx context.Context) (TokenSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return TokenSnapshot{}, err
	}
	info, err := os.Stat(s.path)
	if err != nil {
		return TokenSnapshot{}, err
	}
	if info.Size() > maxAuthSnapshotBytes {
		return TokenSnapshot{}, fmt.Errorf("auth snapshot %s is %d bytes, limit is %d", s.path, info.Size(), maxAuthSnapshotBytes)
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return TokenSnapshot{}, err
	}
	if len(s.publicKey) > 0 {
		signature, err := os.ReadFile(s.path + ".sig")
		if err != nil {
			return TokenSnapshot{}, fmt.Errorf("read auth snapshot signature: %w", err)
		}
		if len(signature) > 4096 {
			return TokenSnapshot{}, errors.New("auth snapshot signature file is too large")
		}
		decoded, err := decodeBase64(string(signature))
		if err != nil {
			return TokenSnapshot{}, fmt.Errorf("decode auth snapshot signature: %w", err)
		}
		if !ed25519.Verify(s.publicKey, raw, decoded) {
			return TokenSnapshot{}, errors.New("auth snapshot signature is invalid")
		}
	}
	var snapshot TokenSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return TokenSnapshot{}, fmt.Errorf("parse auth snapshot: %w", err)
	}
	return snapshot, nil
}

func decodeBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(value)
}

// AuthConfig configures token authentication and per-tenant quotas.
type AuthConfig struct {
	// Source loads the versioned config snapshot. Required.
	Source SnapshotSource
	// Audit receives security events (cross-project writes, scope denials and
	// snapshot loads). Nil falls back to structured logs.
	Audit AuditSink
	// Logger receives operational logs. Nil discards logs.
	Logger *slog.Logger
	// ReloadInterval is how often the snapshot is re-read. Zero uses
	// DefaultAuthReloadInterval; negative disables the reload loop.
	ReloadInterval time.Duration
	// Now overrides the clock in tests.
	Now func() time.Time
}

type compiledToken struct {
	identity  Identity
	scopes    map[string]struct{}
	expiresAt time.Time
}

type compiledSnapshot struct {
	version    string
	generation int64
	issuedAt   time.Time
	expiresAt  time.Time
	tokens     map[[sha256.Size]byte]compiledToken
	quotas     map[string]QuotaConfig
	tenants    int
}

func (s *compiledSnapshot) expired(now time.Time) bool {
	return s == nil || !now.Before(s.expiresAt)
}

func compileSnapshot(snapshot TokenSnapshot) (*compiledSnapshot, error) {
	if strings.TrimSpace(snapshot.ConfigVersion) == "" {
		return nil, errors.New("auth snapshot: config_version is required")
	}
	if snapshot.Generation < 1 {
		return nil, errors.New("auth snapshot: generation must be at least 1")
	}
	if snapshot.IssuedAt.IsZero() {
		return nil, errors.New("auth snapshot: issued_at is required")
	}
	if snapshot.ExpiresAt.IsZero() {
		return nil, errors.New("auth snapshot: expires_at is required")
	}
	if !snapshot.ExpiresAt.After(snapshot.IssuedAt) {
		return nil, errors.New("auth snapshot: expires_at must be after issued_at")
	}
	if len(snapshot.Tenants) == 0 {
		return nil, errors.New("auth snapshot: at least one tenant is required")
	}

	compiled := &compiledSnapshot{
		version:    snapshot.ConfigVersion,
		generation: snapshot.Generation,
		issuedAt:   snapshot.IssuedAt.UTC(),
		expiresAt:  snapshot.ExpiresAt.UTC(),
		tokens:     make(map[[sha256.Size]byte]compiledToken),
		quotas:     make(map[string]QuotaConfig, len(snapshot.Tenants)),
		tenants:    len(snapshot.Tenants),
	}

	tenantIDs := make(map[string]struct{}, len(snapshot.Tenants))
	tokenIDs := make(map[string]struct{})
	for _, tenant := range snapshot.Tenants {
		tenantID := strings.TrimSpace(tenant.TenantID)
		if err := validateIdentifier("tenant_id", tenantID); err != nil {
			return nil, err
		}
		if _, duplicate := tenantIDs[tenantID]; duplicate {
			return nil, fmt.Errorf("auth snapshot: duplicate tenant_id %q", tenantID)
		}
		tenantIDs[tenantID] = struct{}{}

		quota, err := normalizeQuota(tenant.IngestQuota)
		if err != nil {
			return nil, fmt.Errorf("auth snapshot: tenant %q: %w", tenantID, err)
		}
		compiled.quotas[tenantID] = quota

		if len(tenant.Projects) == 0 {
			return nil, fmt.Errorf("auth snapshot: tenant %q has no projects", tenantID)
		}
		projectIDs := make(map[string]struct{}, len(tenant.Projects))
		for _, project := range tenant.Projects {
			projectID := strings.TrimSpace(project.ProjectID)
			if err := validateIdentifier("project_id", projectID); err != nil {
				return nil, err
			}
			if _, duplicate := projectIDs[projectID]; duplicate {
				return nil, fmt.Errorf("auth snapshot: tenant %q has duplicate project_id %q", tenantID, projectID)
			}
			projectIDs[projectID] = struct{}{}

			if len(project.Tokens) == 0 {
				return nil, fmt.Errorf("auth snapshot: project %q has no tokens", projectID)
			}
			for _, token := range project.Tokens {
				tokenID := strings.TrimSpace(token.TokenID)
				if err := validateIdentifier("token_id", tokenID); err != nil {
					return nil, err
				}
				if _, duplicate := tokenIDs[tokenID]; duplicate {
					return nil, fmt.Errorf("auth snapshot: duplicate token_id %q", tokenID)
				}
				tokenIDs[tokenID] = struct{}{}

				hash, err := parseTokenHash(token.SHA256)
				if err != nil {
					return nil, fmt.Errorf("auth snapshot: token %q: %w", tokenID, err)
				}
				if _, duplicate := compiled.tokens[hash]; duplicate {
					return nil, fmt.Errorf("auth snapshot: duplicate token hash for token %q", tokenID)
				}

				scopes, err := normalizeScopes(token.Scopes)
				if err != nil {
					return nil, fmt.Errorf("auth snapshot: token %q: %w", tokenID, err)
				}
				compiledToken := compiledToken{
					identity: Identity{
						TenantID:  tenantID,
						ProjectID: projectID,
						TokenID:   tokenID,
					},
					scopes: scopes,
				}
				if token.ExpiresAt != nil {
					compiledToken.expiresAt = token.ExpiresAt.UTC()
				}
				compiled.tokens[hash] = compiledToken
			}
		}
	}
	return compiled, nil
}

func normalizeQuota(quota QuotaConfig) (QuotaConfig, error) {
	if math.IsNaN(quota.EventsPerSecond) || math.IsInf(quota.EventsPerSecond, 0) {
		return QuotaConfig{}, errors.New("ingest_quota.events_per_second is not finite")
	}
	if quota.EventsPerSecond <= 0 {
		return QuotaConfig{}, errors.New("ingest_quota.events_per_second must be greater than zero")
	}
	if quota.BurstEvents < 0 {
		return QuotaConfig{}, errors.New("ingest_quota.burst_events must not be negative")
	}
	if quota.BurstEvents == 0 {
		quota.BurstEvents = int(math.Ceil(quota.EventsPerSecond))
	}
	return quota, nil
}

func parseTokenHash(raw string) ([sha256.Size]byte, error) {
	var hash [sha256.Size]byte
	decoded, err := hex.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return hash, fmt.Errorf("sha256 must be hex: %w", err)
	}
	if len(decoded) != sha256.Size {
		return hash, fmt.Errorf("sha256 is %d bytes, want %d", len(decoded), sha256.Size)
	}
	copy(hash[:], decoded)
	return hash, nil
}

func normalizeScopes(scopes []string) (map[string]struct{}, error) {
	if len(scopes) == 0 {
		return nil, errors.New("scopes must not be empty")
	}
	normalized := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == "" {
			continue
		}
		normalized[scope] = struct{}{}
	}
	if len(normalized) == 0 {
		return nil, errors.New("scopes must not be empty")
	}
	return normalized, nil
}

func validateIdentifier(field, value string) error {
	if value == "" {
		return fmt.Errorf("auth snapshot: %s is required", field)
	}
	if len(value) > maxIdentifierBytes {
		return fmt.Errorf("auth snapshot: %s is longer than %d bytes", field, maxIdentifierBytes)
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == ':', r == '@':
		default:
			return fmt.Errorf("auth snapshot: %s %q contains an unsupported character", field, value)
		}
	}
	return nil
}

// AuthState is a point-in-time view of the loaded snapshot.
type AuthState struct {
	ConfigVersion string
	Generation    int64
	Age           time.Duration
	ExpiresAt     time.Time
	Expired       bool
	Tenants       int
}

// Authenticator validates ingest tokens and enforces per-tenant event quotas.
// It keeps the last known snapshot in memory and never touches PostgreSQL on
// the request path (AD-1, AD-19).
type Authenticator struct {
	source         SnapshotSource
	audit          AuditSink
	logger         *slog.Logger
	reloadInterval time.Duration
	now            func() time.Time

	current  atomic.Pointer[compiledSnapshot]
	loadedAt atomic.Int64
	limiter  *eventLimiter

	startOnce sync.Once
	stopOnce  sync.Once
	stop      chan struct{}
	done      chan struct{}
}

// NewAuthenticator loads the first snapshot and prepares the quota limiter.
func NewAuthenticator(cfg AuthConfig) (*Authenticator, error) {
	if cfg.Source == nil {
		return nil, errors.New("ingest: auth snapshot source is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	interval := cfg.ReloadInterval
	if interval == 0 {
		interval = DefaultAuthReloadInterval
	}
	if interval < 0 {
		interval = 0
	}
	a := &Authenticator{
		source:         cfg.Source,
		audit:          cfg.Audit,
		logger:         cfg.Logger,
		reloadInterval: interval,
		now:            cfg.Now,
		limiter:        newEventLimiter(),
	}
	if err := a.Reload(); err != nil {
		return nil, err
	}
	return a, nil
}

// Start begins the snapshot reload loop. It is safe to call more than once.
func (a *Authenticator) Start() {
	a.startOnce.Do(func() {
		if a.reloadInterval <= 0 {
			return
		}
		a.stop = make(chan struct{})
		a.done = make(chan struct{})
		go a.reloadLoop()
	})
}

// Close stops the reload loop. It is safe to call more than once.
func (a *Authenticator) Close() {
	a.stopOnce.Do(func() {
		if a.stop == nil {
			return
		}
		close(a.stop)
		<-a.done
	})
}

func (a *Authenticator) reloadLoop() {
	defer close(a.done)
	ticker := time.NewTicker(a.reloadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := a.Reload(); err != nil {
				a.logger.Error("ingest auth snapshot reload failed", "error", err)
			}
		case <-a.stop:
			return
		}
	}
}

// Reload reads and compiles the snapshot, swapping it atomically. On failure
// the previous snapshot stays active; if it later expires, security and quota
// decisions fail closed (AD-19).
func (a *Authenticator) Reload() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snapshot, err := a.source.Load(ctx)
	if err != nil {
		return fmt.Errorf("load ingest auth snapshot: %w", err)
	}
	compiled, err := compileSnapshot(snapshot)
	if err != nil {
		return err
	}
	a.current.Store(compiled)
	a.loadedAt.Store(a.now().UnixNano())
	a.limiter.setQuotas(compiled.quotas)
	a.logger.Info(
		"ingest auth snapshot loaded",
		"config_version", compiled.version,
		"generation", compiled.generation,
		"tenants", compiled.tenants,
		"expires_at", compiled.expiresAt,
	)
	a.auditConfig(compiled)
	return nil
}

func (a *Authenticator) auditConfig(snapshot *compiledSnapshot) {
	event := AuditEvent{
		ActorType:     AuditActorSystem,
		ActorID:       "gateway",
		Action:        AuditActionConfigLoaded,
		Outcome:       AuditOutcomeSuccess,
		ConfigVersion: snapshot.version,
		Generation:    snapshot.generation,
	}
	if err := a.recordAudit(event); err != nil {
		a.logger.Error("ingest auth snapshot audit failed", "error", err)
	}
}

// State returns the loaded snapshot metadata for health and metrics.
func (a *Authenticator) State() AuthState {
	snapshot := a.current.Load()
	if snapshot == nil {
		return AuthState{}
	}
	state := AuthState{
		ConfigVersion: snapshot.version,
		Generation:    snapshot.generation,
		ExpiresAt:     snapshot.expiresAt,
		Expired:       snapshot.expired(a.now()),
		Tenants:       snapshot.tenants,
	}
	if loaded := a.loadedAt.Load(); loaded > 0 {
		state.Age = a.now().Sub(time.Unix(0, loaded))
	}
	return state
}

// Authenticate resolves an Authorization header to a tenant/project identity.
func (a *Authenticator) Authenticate(requestID, authorization string) (Identity, *requestError) {
	snapshot := a.current.Load()
	if snapshot == nil {
		return Identity{}, errConfigStale(requestID)
	}
	now := a.now()
	if snapshot.expired(now) {
		return Identity{}, errConfigStale(requestID)
	}

	token, ok := bearerToken(authorization)
	if !ok {
		return Identity{}, errUnauthorized(requestID, "missing_token", "an OTLP ingest token is required")
	}
	if len(token) > maxTokenBytes {
		return Identity{}, errUnauthorized(requestID, "invalid_token", "the OTLP ingest token is invalid")
	}
	compiled, ok := snapshot.tokens[sha256.Sum256([]byte(token))]
	if !ok {
		return Identity{}, errUnauthorized(requestID, "invalid_token", "the OTLP ingest token is invalid")
	}
	if !compiled.expiresAt.IsZero() && !now.Before(compiled.expiresAt) {
		return Identity{}, errUnauthorized(requestID, "expired_token", "the OTLP ingest token has expired")
	}
	identity := compiled.identity
	identity.ConfigVersion = snapshot.version
	identity.Generation = snapshot.generation
	if _, allowed := compiled.scopes[ScopeIngest]; !allowed {
		return identity, errForbidden(requestID, "insufficient_scope", "the OTLP ingest token lacks the ingest scope")
	}
	return identity, nil
}

// Admit consumes events from the tenant's quota bucket. It returns a
// Retry-After duration and a 429 error when the quota is exhausted.
func (a *Authenticator) Admit(requestID string, identity Identity, events int) (time.Duration, *requestError) {
	snapshot := a.current.Load()
	if snapshot == nil || snapshot.expired(a.now()) {
		return time.Second, errConfigStale(requestID)
	}
	retryAfter, allowed, configured := a.limiter.admit(identity.TenantID, events, a.now())
	if !configured {
		return time.Second, errConfigStale(requestID).withCause(fmt.Errorf("tenant %q has no quota in the snapshot", identity.TenantID))
	}
	if !allowed {
		return retryAfter, errQuotaExceeded(requestID, retryAfter)
	}
	return 0, nil
}

// Reserve charges an HTTP request before its body is decoded. It returns nil
// when there is no Content-Length or the tenant has no payload estimate yet;
// the caller then falls back to Admit after decoding.
func (a *Authenticator) Reserve(requestID string, identity Identity, bodyBytes int64) (*Reservation, *requestError) {
	snapshot := a.current.Load()
	if snapshot == nil || snapshot.expired(a.now()) {
		return nil, errConfigStale(requestID)
	}
	if bodyBytes <= 0 {
		return nil, nil
	}
	estimated, warm, configured := a.limiter.estimate(identity.TenantID, bodyBytes)
	if !configured {
		return nil, errConfigStale(requestID).withCause(fmt.Errorf("tenant %q has no quota in the snapshot", identity.TenantID))
	}
	if warm {
		return nil, nil
	}
	reservation, retryAfter, allowed, configured := a.limiter.reserve(identity.TenantID, estimated, bodyBytes, a.now())
	if !configured {
		return nil, errConfigStale(requestID)
	}
	if !allowed {
		return nil, errQuotaExceeded(requestID, retryAfter)
	}
	return &reservation, nil
}

// Settle reconciles a reservation with the decoded event count.
func (a *Authenticator) Settle(requestID string, reservation *Reservation, events int) *requestError {
	if reservation == nil {
		return nil
	}
	snapshot := a.current.Load()
	if snapshot == nil || snapshot.expired(a.now()) {
		a.limiter.release(*reservation)
		return errConfigStale(requestID)
	}
	retryAfter, allowed, configured := a.limiter.settle(*reservation, events, a.now())
	if !configured {
		return errConfigStale(requestID).withCause(fmt.Errorf("tenant %q has no quota in the snapshot", reservation.tenant))
	}
	if !allowed {
		return errQuotaExceeded(requestID, retryAfter)
	}
	return nil
}

// Release refunds a reservation that was not settled.
func (a *Authenticator) Release(reservation *Reservation) {
	if reservation == nil {
		return
	}
	a.limiter.release(*reservation)
}

// RecordPayload teaches the payload estimate for a request that skipped
// pre-decode admission.
func (a *Authenticator) RecordPayload(identity Identity, bodyBytes int64, events int) {
	if bodyBytes <= 0 || events <= 0 {
		return
	}
	a.limiter.recordPayload(identity.TenantID, bodyBytes, events)
}

// Audit records a security event through the configured sink, falling back to
// the structured logger.
func (a *Authenticator) Audit(event AuditEvent) {
	if err := a.recordAudit(event); err != nil {
		a.logger.Error(
			"ingest audit write failed",
			"action", event.Action,
			"request_id", event.RequestID,
			"tenant_id", event.TenantID,
			"project_id", event.ProjectID,
			"error", err,
		)
	}
}

func (a *Authenticator) recordAudit(event AuditEvent) error {
	if a.audit == nil {
		a.logger.Warn(
			"ingest audit event",
			"action", event.Action,
			"outcome", event.Outcome,
			"actor_id", event.ActorID,
			"tenant_id", event.TenantID,
			"project_id", event.ProjectID,
			"target_tenant_id", event.TargetTenantID,
			"target_project_id", event.TargetProjectID,
			"request_id", event.RequestID,
			"reason", event.Reason,
		)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return a.audit.Record(ctx, event)
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}
