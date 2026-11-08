package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/observaki/observaki/internal/selfobs"
)

// OverviewFunc builds the health snapshot. The second result is false when
// self-observability sources are not configured.
type OverviewFunc func(context.Context) (selfobs.Overview, bool)

// Config wires the API dependencies.
type Config struct {
	Version      string
	Logger       *slog.Logger
	Overview     OverviewFunc
	ControlPlane ControlPlane
}

// API serves the versioned HTTP contract.
type API struct {
	version      string
	logger       *slog.Logger
	overview     OverviewFunc
	controlPlane ControlPlane
	openAPI      []byte
}

// New builds the API and generates the OpenAPI document from the operation
// table, so the served contract always matches the registered routes.
func New(cfg Config) *API {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &API{
		version:      cfg.Version,
		logger:       logger,
		overview:     cfg.Overview,
		controlPlane: cfg.ControlPlane,
		openAPI:      OpenAPIJSON(),
	}
}

// Health is the liveness payload.
type Health struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

// ProjectList is the projects endpoint payload.
type ProjectList struct {
	Projects []Project `json:"projects"`
}

// IngestTokenList is the ingestion-token endpoint payload.
type IngestTokenList struct {
	ProjectID string        `json:"project_id"`
	Tokens    []IngestToken `json:"tokens"`
}

// SampleSeries is one deterministic series of the sample query.
type SampleSeries struct {
	Labels map[string]string `json:"labels"`
	Points [][]float64       `json:"points"`
}

// SampleQuery is the contract sample returned before the Query Service lands.
// It is explicitly marked sample=true so no client can mistake it for tenant
// telemetry (AD-3/AD-6).
type SampleQuery struct {
	Sample       bool           `json:"sample"`
	QueryService string         `json:"query_service"`
	GeneratedAt  string         `json:"generated_at"`
	RequestID    string         `json:"request_id"`
	Signal       string         `json:"signal"`
	Query        string         `json:"query"`
	OrgID        string         `json:"org_id"`
	ProjectID    string         `json:"project_id"`
	Series       []SampleSeries `json:"series"`
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (a *API) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, Health{Status: "ok"})
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, Health{Status: "ok", Version: a.version})
}

func (a *API) handleOverview(w http.ResponseWriter, r *http.Request) {
	if a.overview == nil {
		WriteError(w, r, http.StatusServiceUnavailable, CodeHealthUnavailable,
			"self-observability sources are not configured", nil)
		return
	}
	overview, ok := a.overview(r.Context())
	if !ok {
		WriteError(w, r, http.StatusServiceUnavailable, CodeHealthUnavailable,
			"self-observability sources are not configured", nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, overview)
}

func (a *API) handleProjects(w http.ResponseWriter, r *http.Request) {
	principal, _ := PrincipalFromContext(r.Context())
	if a.controlPlane == nil {
		WriteError(w, r, http.StatusServiceUnavailable, CodeMetadataUnavailable,
			"the control plane is not configured", map[string]any{"reason": "control_plane_unconfigured"})
		return
	}
	project, err := a.controlPlane.Project(r.Context(), principal.ProjectID)
	if err != nil {
		a.writeControlPlaneError(w, r, err, "project")
		return
	}
	WriteJSON(w, http.StatusOK, ProjectList{Projects: []Project{project}})
}

func (a *API) handleIngestTokens(w http.ResponseWriter, r *http.Request) {
	principal, _ := PrincipalFromContext(r.Context())
	projectID := r.PathValue("projectID")
	if !uuidPattern.MatchString(projectID) {
		WriteError(w, r, http.StatusBadRequest, CodeInvalidRequest,
			"projectID must be a UUID", map[string]any{"field": "projectID", "reason": "invalid_uuid"})
		return
	}
	if projectID != principal.ProjectID {
		// A project-scoped token can never read another project (AD-3). The
		// message deliberately does not confirm that the project exists.
		WriteError(w, r, http.StatusForbidden, CodeForbidden,
			"the API token is scoped to another project", map[string]any{"reason": "project_scope"})
		return
	}
	if a.controlPlane == nil {
		WriteError(w, r, http.StatusServiceUnavailable, CodeMetadataUnavailable,
			"the control plane is not configured", map[string]any{"reason": "control_plane_unconfigured"})
		return
	}
	tokens, err := a.controlPlane.IngestTokens(r.Context(), projectID)
	if err != nil {
		a.writeControlPlaneError(w, r, err, "ingestion tokens")
		return
	}
	if tokens == nil {
		tokens = []IngestToken{}
	}
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, IngestTokenList{ProjectID: projectID, Tokens: tokens})
}

var sampleQueries = map[string]string{
	"metrics": "avg(observaki_ingest_lag_p95_seconds)",
	"logs":    `{service="gateway"} |= "error"`,
	"traces":  `{service="gateway"} && duration > 500ms`,
}

func (a *API) handleSampleQuery(w http.ResponseWriter, r *http.Request) {
	principal, _ := PrincipalFromContext(r.Context())
	signal := strings.TrimSpace(r.URL.Query().Get("signal"))
	if signal == "" {
		signal = "metrics"
	}
	query, known := sampleQueries[signal]
	if !known {
		WriteError(w, r, http.StatusBadRequest, CodeInvalidRequest,
			"signal must be one of metrics, logs or traces",
			map[string]any{"field": "signal", "allowed": []string{"logs", "metrics", "traces"}})
		return
	}

	now := time.Now().UTC().Truncate(time.Minute)
	points := make([][]float64, 0, 5)
	for offset := 4; offset >= 0; offset-- {
		timestamp := float64(now.Add(-time.Duration(offset) * time.Minute).Unix())
		points = append(points, []float64{timestamp, 0.42 + 0.05*float64(4-offset)})
	}

	WriteJSON(w, http.StatusOK, SampleQuery{
		Sample:       true,
		QueryService: "phase-1",
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		RequestID:    RequestIDFromContext(r.Context()),
		Signal:       signal,
		Query:        query,
		OrgID:        principal.OrgID,
		ProjectID:    principal.ProjectID,
		Series: []SampleSeries{{
			Labels: map[string]string{"service": "gateway", "status": "ok"},
			Points: points,
		}},
	})
}

func (a *API) handleQueryPlaceholder(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, http.StatusNotImplemented, CodeQueryNotAvailable,
		"the Query Service lands in Phase 1; use /api/v1/query/sample for the contract",
		map[string]any{"sample_endpoint": "/api/v1/query/sample"})
}

func (a *API) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(a.openAPI)
}

// handleAPINotFound keeps unknown API routes JSON so a client never receives
// the SPA shell instead of an error.
func (a *API) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, http.StatusNotFound, CodeNotFound,
		"no route matches "+r.URL.Path, map[string]any{"path": r.URL.Path})
}

func (a *API) writeControlPlaneError(w http.ResponseWriter, r *http.Request, err error, resource string) {
	switch {
	case errors.Is(err, ErrProjectNotFound):
		WriteError(w, r, http.StatusNotFound, CodeNotFound,
			"the "+resource+" does not exist", map[string]any{"resource": resource})
	default:
		a.logger.Error("api control-plane read failed",
			"request_id", RequestIDFromContext(r.Context()),
			"resource", resource,
			"error", err,
		)
		WriteError(w, r, http.StatusServiceUnavailable, CodeControlPlaneDown,
			"the control plane is unavailable", map[string]any{"reason": "control_plane_unavailable"})
	}
}
