package api

import "net/http"

// Operation is one versioned API route and the OpenAPI operation that
// documents it. Operations is the single source of truth for both the router
// and /openapi.json (AD-26): adding a route without documenting it, or
// documenting a route that is not registered, is not possible.
type Operation struct {
	Method      string
	Path        string
	OperationID string
	Summary     string
	Description string
	Tags        []string
	// Scope is the token scope the route requires; an empty scope is public.
	Scope      string
	Parameters []map[string]any
	Responses  map[string]any
	Handle     func(*API, http.ResponseWriter, *http.Request)
}

// Operations returns the MVP contract. The order is not significant; the
// OpenAPI document sorts by path when it serializes.
func Operations() []Operation {
	projectIDParam := map[string]any{
		"name": "projectID", "in": "path", "required": true,
		"description": "Project UUID bound to the presented token.",
		"schema":      map[string]any{"type": "string", "format": "uuid"},
	}

	return []Operation{
		{
			Method: http.MethodGet, Path: "/healthz",
			OperationID: "getLiveness", Summary: "Liveness probe",
			Description: "Reports process liveness. It never depends on the data plane.",
			Tags:        []string{"system"},
			Responses: map[string]any{
				"200": jsonResponse("The process is alive.", schemaRef("Health")),
			},
			Handle: (*API).handleHealthz,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/health",
			OperationID: "getHealth", Summary: "API health",
			Description: "Reports liveness plus the running build version.",
			Tags:        []string{"system"},
			Responses: map[string]any{
				"200": jsonResponse("The API is healthy.", schemaRef("Health")),
			},
			Handle: (*API).handleHealth,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/health/overview",
			OperationID: "getHealthOverview", Summary: "Self-observability overview",
			Description: "Ingestion lag, query error rate, storage usage and component health. " +
				"Read-only, no tenant telemetry, no direct ClickHouse/VictoriaMetrics access (AD-3, AD-24).",
			Tags: []string{"system"},
			Responses: map[string]any{
				"200": jsonResponse("The current health snapshot.", schemaRef("HealthOverview")),
				"503": refResponse("ServiceUnavailable"),
			},
			Handle: (*API).handleOverview,
		},
		{
			Method: http.MethodGet, Path: "/openapi.json",
			OperationID: "getOpenAPI", Summary: "OpenAPI 3.1 contract",
			Description: "The generated contract for this API version.",
			Tags:        []string{"system"},
			Responses: map[string]any{
				"200": map[string]any{
					"description": "The OpenAPI document.",
					"content": map[string]any{
						"application/json": map[string]any{"schema": map[string]any{"type": "object"}},
					},
				},
			},
			Handle: (*API).handleOpenAPI,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects",
			OperationID: "listProjects", Summary: "List accessible projects",
			Description: "Returns the project bound to the presented token. The MVP is project-scoped: " +
				"tenancy comes from the token, never from a header (AD-3).",
			Tags:  []string{"metadata"},
			Scope: "query",
			Responses: map[string]any{
				"200": jsonResponse("Projects visible to the token.", schemaRef("ProjectList")),
				"401": refResponse("Unauthorized"),
				"403": refResponse("Forbidden"),
				"503": refResponse("ServiceUnavailable"),
			},
			Handle: (*API).handleProjects,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projectID}/ingest-tokens",
			OperationID: "listIngestTokens", Summary: "List ingestion token metadata",
			Description: "Lists name, scopes and expiry of the project's tokens. Token digests are never returned (AD-19).",
			Tags:        []string{"metadata"},
			Scope:       "admin",
			Parameters:  []map[string]any{projectIDParam},
			Responses: map[string]any{
				"200": jsonResponse("Token metadata for the project.", schemaRef("IngestTokenList")),
				"400": refResponse("InvalidRequest"),
				"401": refResponse("Unauthorized"),
				"403": refResponse("Forbidden"),
				"503": refResponse("ServiceUnavailable"),
			},
			Handle: (*API).handleIngestTokens,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/query/sample",
			OperationID: "getSampleQuery", Summary: "Sample query contract",
			Description: "Returns a deterministic sample marked sample=true. The Query Service that " +
				"serves real telemetry arrives in Phase 1; the API never queries ClickHouse or VictoriaMetrics (AD-6).",
			Tags:  []string{"query"},
			Scope: "query",
			Parameters: []map[string]any{{
				"name": "signal", "in": "query", "required": false,
				"description": "Signal whose sample shape is returned.",
				"schema": map[string]any{
					"type": "string", "enum": []string{"metrics", "logs", "traces"}, "default": "metrics",
				},
			}},
			Responses: map[string]any{
				"200": jsonResponse("A deterministic sample result.", schemaRef("SampleQuery")),
				"400": refResponse("InvalidRequest"),
				"401": refResponse("Unauthorized"),
				"403": refResponse("Forbidden"),
				"503": refResponse("ServiceUnavailable"),
			},
			Handle: (*API).handleSampleQuery,
		},
		{
			Method: http.MethodPost, Path: "/api/v1/query",
			OperationID: "query", Summary: "Telemetry query (reserved)",
			Description: "Reserved for the Query Service (Phase 1). Until then it returns a structured 501 " +
				"and points at /api/v1/query/sample.",
			Tags:  []string{"query"},
			Scope: "query",
			Responses: map[string]any{
				"501": jsonResponse("The Query Service is not available in this build.", schemaRef("Error")),
				"401": refResponse("Unauthorized"),
				"403": refResponse("Forbidden"),
			},
			Handle: (*API).handleQueryPlaceholder,
		},
	}
}

// Register installs every operation and the structured API catch-all on mux.
func (a *API) Register(mux *http.ServeMux) {
	for _, operation := range Operations() {
		mux.Handle(operation.Path, a.wrap(operation))
	}
	mux.HandleFunc("/api/", a.handleAPINotFound)
}

func (a *API) wrap(operation Operation) http.Handler {
	return methodRestriction(operation.Method, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if operation.Scope != "" {
			principal, ok := a.authenticate(w, r, operation.Scope)
			if !ok {
				return
			}
			r = r.WithContext(WithPrincipal(r.Context(), principal))
		}
		operation.Handle(a, w, r)
	}))
}
