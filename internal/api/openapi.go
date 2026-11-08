package api

import (
	"encoding/json"
	"strings"
)

// OpenAPIVersion is the semantic version of the contract. It matches the
// /api/v1 surface; a breaking change bumps the major and adds /api/v2 (AD-9).
const OpenAPIVersion = "1.0.0"

// OpenAPIGoldenPath is the checked-in artifact compared by `make api-check`.
const OpenAPIGoldenPath = "api/openapi/openapi.json"

// OpenAPIJSON renders the generated contract. encoding/json sorts map keys, so
// the bytes are stable across runs and machines.
func OpenAPIJSON() []byte {
	document := OpenAPIDocument()
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		// The document only contains strings, numbers, booleans, slices and
		// maps, so a marshal error is a programming mistake, not input.
		panic("api: marshal OpenAPI document: " + err.Error())
	}
	return append(data, '\n')
}

// OpenAPIDocument builds the OpenAPI 3.1 document from the operation table.
func OpenAPIDocument() map[string]any {
	paths := make(map[string]any, len(Operations()))
	for _, operation := range Operations() {
		item, ok := paths[operation.Path].(map[string]any)
		if !ok {
			item = make(map[string]any, 2)
			paths[operation.Path] = item
		}
		documented := map[string]any{
			"operationId": operation.OperationID,
			"summary":     operation.Summary,
			"tags":        operation.Tags,
			"responses":   operation.Responses,
		}
		if operation.Description != "" {
			documented["description"] = operation.Description
		}
		if len(operation.Parameters) > 0 {
			documented["parameters"] = operation.Parameters
		}
		if operation.Scope != "" {
			documented["security"] = []any{map[string]any{"bearerAuth": []any{}}}
			documented["x-required-scope"] = operation.Scope
		}
		item[strings.ToLower(operation.Method)] = documented
	}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "Observaki API",
			"version": OpenAPIVersion,
			"summary": "Versioned HTTP API for Observaki.",
			"description": "The /api/v1 contract for health, project metadata, ingestion tokens and the " +
				"sample query. Telemetry reads go through the Query Service; the API never connects to " +
				"ClickHouse or VictoriaMetrics (AD-3, AD-6). Errors use the AD-15 envelope " +
				"{code, message, details, request_id} and never leak a stack trace.",
			"license":             map[string]any{"name": "Apache-2.0", "identifier": "Apache-2.0"},
			"x-changelog":         "api/openapi/CHANGELOG.md",
			"x-versioning-policy": "docs/api/versioning.md",
		},
		"servers": []any{map[string]any{"url": "/", "description": "Same origin as the API"}},
		"tags": []any{
			map[string]any{"name": "system", "description": "Health and contract discovery."},
			map[string]any{"name": "metadata", "description": "Control-plane metadata scoped to the token."},
			map[string]any{"name": "query", "description": "Telemetry queries through the Query Service."},
		},
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "API token",
					"description": "Project-scoped API token. Only its SHA-256 digest is stored; " +
						"the token resolves tenant/project and scopes server-side (AD-19, AD-20).",
				},
			},
			"responses": map[string]any{
				"Unauthorized":       response("The token is missing, malformed, expired or unknown (401).", "Error"),
				"Forbidden":          response("The token lacks the required scope or targets another project (403).", "Error"),
				"InvalidRequest":     response("The input is invalid (400).", "Error"),
				"NotFound":           response("The resource does not exist (404).", "Error"),
				"ServiceUnavailable": response("A required control-plane dependency is unavailable (503).", "Error"),
				"InternalError":      response("An unexpected error occurred (500); no internal detail is leaked.", "Error"),
				"MethodNotAllowed":   response("The method is not supported for this route (405).", "Error"),
			},
			"schemas": map[string]any{
				"Error": object(map[string]any{
					"code":       map[string]any{"type": "string", "description": "Stable error code."},
					"message":    map[string]any{"type": "string", "description": "Human-readable message."},
					"details":    map[string]any{"type": "object", "additionalProperties": true},
					"request_id": map[string]any{"type": "string", "format": "uuid", "description": "UUIDv7 correlation id."},
				}, "code", "message", "details", "request_id"),
				"Health": object(map[string]any{
					"status":  map[string]any{"type": "string", "enum": []string{"ok"}},
					"version": map[string]any{"type": "string"},
				}, "status"),
				"HealthOverview": object(map[string]any{
					"generated_at": map[string]any{"type": "string", "format": "date-time"},
					"status":       statusEnum(),
					"summary":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"ingestion":    schemaRef("IngestionHealth"),
					"query":        schemaRef("QueryHealth"),
					"storage":      schemaRef("StorageHealth"),
					"components":   map[string]any{"type": "array", "items": schemaRef("ComponentHealth")},
					"alerting":     schemaRef("AlertingHealth"),
				}, "generated_at", "status", "summary", "ingestion", "query", "storage", "components", "alerting"),
				"IngestionHealth": object(map[string]any{
					"status":                   statusEnum(),
					"lag_p95_seconds":          map[string]any{"type": "number"},
					"pending_messages":         map[string]any{"type": "integer", "format": "int64"},
					"dlq_messages":             map[string]any{"type": "integer", "format": "int64"},
					"written_events":           map[string]any{"type": "integer", "format": "int64", "minimum": 0},
					"writer_connected":         map[string]any{"type": "boolean"},
					"gateway_connected":        map[string]any{"type": "boolean"},
					"gateway_published_events": map[string]any{"type": "integer", "format": "int64", "minimum": 0},
					"observed_at":              map[string]any{"type": "string", "format": "date-time"},
					"source":                   map[string]any{"type": "string"},
					"detail":                   map[string]any{"type": "string"},
				}, "status"),
				"QueryHealth": object(map[string]any{
					"status":        statusEnum(),
					"requests":      map[string]any{"type": "integer", "format": "int64", "minimum": 0},
					"errors":        map[string]any{"type": "integer", "format": "int64", "minimum": 0},
					"client_errors": map[string]any{"type": "integer", "format": "int64", "minimum": 0},
					"error_rate":    map[string]any{"type": "number", "minimum": 0, "maximum": 1},
					"since":         map[string]any{"type": "string", "format": "date-time"},
					"detail":        map[string]any{"type": "string"},
				}, "status", "requests", "errors", "client_errors", "error_rate", "since"),
				"StorageHealth": object(map[string]any{
					"status":                 statusEnum(),
					"clickhouse_bytes":       map[string]any{"type": "integer", "format": "int64", "minimum": 0},
					"clickhouse_parts":       map[string]any{"type": "integer", "format": "int64", "minimum": 0},
					"victoriametrics_bytes":  map[string]any{"type": "integer", "format": "int64", "minimum": 0},
					"victoriametrics_series": map[string]any{"type": "integer", "format": "int64", "minimum": 0},
					"observed_at":            map[string]any{"type": "string", "format": "date-time"},
					"detail":                 map[string]any{"type": "string"},
				}, "status"),
				"ComponentHealth": object(map[string]any{
					"name":        map[string]any{"type": "string"},
					"status":      statusEnum(),
					"latency_ms":  map[string]any{"type": "number", "minimum": 0},
					"detail":      map[string]any{"type": "string"},
					"observed_at": map[string]any{"type": "string", "format": "date-time"},
				}, "name", "status", "latency_ms", "observed_at"),
				"AlertingHealth": object(map[string]any{
					"status":                 statusEnum(),
					"enabled":                map[string]any{"type": "boolean"},
					"last_evaluation_at":     map[string]any{"type": "string", "format": "date-time"},
					"evaluation_age_seconds": map[string]any{"type": "number", "minimum": 0},
					"stale_after_seconds":    map[string]any{"type": "number", "minimum": 0},
					"firing":                 map[string]any{"type": "integer", "minimum": 0},
					"pending":                map[string]any{"type": "integer", "minimum": 0},
					"notifications_pending":  map[string]any{"type": "integer", "minimum": 0},
					"last_error":             map[string]any{"type": "string"},
					"detail":                 map[string]any{"type": "string"},
				}, "status", "enabled", "firing", "pending", "notifications_pending"),
				"Project": object(map[string]any{
					"id":         map[string]any{"type": "string", "format": "uuid"},
					"org_id":     map[string]any{"type": "string", "format": "uuid"},
					"name":       map[string]any{"type": "string"},
					"slug":       map[string]any{"type": "string"},
					"generation": map[string]any{"type": "integer", "format": "int64", "minimum": 1},
					"created_at": map[string]any{"type": "string", "format": "date-time"},
				}, "id", "org_id", "name", "slug", "generation", "created_at"),
				"ProjectList": object(map[string]any{
					"projects": map[string]any{"type": "array", "items": schemaRef("Project")},
				}, "projects"),
				"IngestToken": object(map[string]any{
					"id":         map[string]any{"type": "string", "format": "uuid"},
					"name":       map[string]any{"type": "string"},
					"scopes":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"expires_at": map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
					"created_at": map[string]any{"type": "string", "format": "date-time"},
				}, "id", "name", "scopes", "created_at"),
				"IngestTokenList": object(map[string]any{
					"project_id": map[string]any{"type": "string", "format": "uuid"},
					"tokens":     map[string]any{"type": "array", "items": schemaRef("IngestToken")},
				}, "project_id", "tokens"),
				"SampleSeries": object(map[string]any{
					"labels": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
					"points": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "array", "items": map[string]any{"type": "number"}, "minItems": 2, "maxItems": 2},
					},
				}, "labels", "points"),
				"SampleQuery": object(map[string]any{
					"sample":        map[string]any{"type": "boolean", "description": "Always true: the payload is not tenant telemetry."},
					"query_service": map[string]any{"type": "string", "description": "Where real queries will be served."},
					"generated_at":  map[string]any{"type": "string", "format": "date-time"},
					"request_id":    map[string]any{"type": "string", "format": "uuid"},
					"signal":        map[string]any{"type": "string", "enum": []string{"logs", "metrics", "traces"}},
					"query":         map[string]any{"type": "string"},
					"org_id":        map[string]any{"type": "string", "format": "uuid"},
					"project_id":    map[string]any{"type": "string", "format": "uuid"},
					"series":        map[string]any{"type": "array", "items": schemaRef("SampleSeries")},
				}, "sample", "query_service", "generated_at", "request_id", "signal", "query", "org_id", "project_id", "series"),
			},
		},
	}
}

func object(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func statusEnum() map[string]any {
	return map[string]any{"type": "string", "enum": []string{
		"healthy", "degraded", "critical", "no_data", "idle", "not_configured", "disabled", "unknown",
	}}
}

func jsonResponse(description string, schema map[string]any) map[string]any {
	return map[string]any{
		"description": description,
		"content":     map[string]any{"application/json": map[string]any{"schema": schema}},
	}
}

func refResponse(name string) map[string]any {
	return map[string]any{"$ref": "#/components/responses/" + name}
}

func response(description, schemaName string) map[string]any {
	return jsonResponse(description, schemaRef(schemaName))
}
