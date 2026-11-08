package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"
	"time"
)

// maxTokenBytes bounds the bearer token before hashing so a hostile request
// cannot make the API hash megabytes.
const maxTokenBytes = 4096

var (
	// ErrTokenNotFound is returned when the digest is unknown or expired. The
	// API maps it to 401 so an attacker cannot distinguish the two.
	ErrTokenNotFound = errors.New("api: token not found")
	// ErrProjectNotFound is returned when a project does not exist.
	ErrProjectNotFound = errors.New("api: project not found")
)

// Project is the project metadata the API exposes. It never includes token
// digests or any telemetry (AD-3).
type Project struct {
	ID         string    `json:"id"`
	OrgID      string    `json:"org_id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Generation int64     `json:"generation"`
	CreatedAt  time.Time `json:"created_at"`
}

// IngestToken is the metadata of an ingestion token. The digest is never
// serialized (AD-19).
type IngestToken struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Principal is the authenticated caller derived from the bearer token. Tenant
// scope comes from the token, never from a client header (AD-3/AD-20).
type Principal struct {
	TokenID   string
	OrgID     string
	ProjectID string
	Scopes    []string
}

// HasScope reports whether the token grants scope. The admin scope implies
// every other scope in the MVP.
func (p Principal) HasScope(scope string) bool {
	if scope == "" {
		return true
	}
	for _, granted := range p.Scopes {
		if granted == scope || granted == "admin" {
			return true
		}
	}
	return false
}

// ControlPlane is the read-only projection of the control plane the API needs
// (AD-7). The PostgreSQL adapter implements it; tests use an in-memory fake.
type ControlPlane interface {
	AuthenticateToken(ctx context.Context, tokenHash [sha256.Size]byte) (Principal, error)
	Project(ctx context.Context, projectID string) (Project, error)
	IngestTokens(ctx context.Context, projectID string) ([]IngestToken, error)
}

type principalKey struct{}

// WithPrincipal stores the authenticated caller in the request context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

// PrincipalFromContext returns the caller set by the auth step.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}

// authenticate enforces the bearer token and the route scope. It writes the
// response itself and reports whether the handler may run.
func (a *API) authenticate(w http.ResponseWriter, r *http.Request, scope string) (Principal, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		WriteError(w, r, http.StatusUnauthorized, CodeUnauthorized,
			"an API token is required", map[string]any{"reason": "missing_credentials"})
		return Principal{}, false
	}
	scheme, token, found := strings.Cut(header, " ")
	token = strings.TrimSpace(token)
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		WriteError(w, r, http.StatusUnauthorized, CodeUnauthorized,
			"the Authorization header must use the Bearer scheme", map[string]any{"reason": "invalid_credentials"})
		return Principal{}, false
	}
	if len(token) > maxTokenBytes {
		WriteError(w, r, http.StatusUnauthorized, CodeUnauthorized,
			"the API token is invalid", map[string]any{"reason": "invalid_token"})
		return Principal{}, false
	}
	if a.controlPlane == nil {
		WriteError(w, r, http.StatusServiceUnavailable, CodeMetadataUnavailable,
			"the control plane is not configured", map[string]any{"reason": "control_plane_unconfigured"})
		return Principal{}, false
	}

	principal, err := a.controlPlane.AuthenticateToken(r.Context(), sha256.Sum256([]byte(token)))
	if errors.Is(err, ErrTokenNotFound) {
		WriteError(w, r, http.StatusUnauthorized, CodeUnauthorized,
			"the API token is invalid", map[string]any{"reason": "invalid_token"})
		return Principal{}, false
	}
	if err != nil {
		a.logger.Error("api token authentication failed",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err,
		)
		WriteError(w, r, http.StatusServiceUnavailable, CodeControlPlaneDown,
			"the control plane is unavailable", map[string]any{"reason": "control_plane_unavailable"})
		return Principal{}, false
	}
	if !principal.HasScope(scope) {
		WriteError(w, r, http.StatusForbidden, CodeForbidden,
			"the API token does not grant the required scope",
			map[string]any{"reason": "insufficient_scope", "required_scope": scope})
		return Principal{}, false
	}
	return principal, true
}
