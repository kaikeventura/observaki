// Package api owns the versioned HTTP contract of Observaki.
//
// The contract is generated from the same Go operation table that registers the
// routes (AD-9, AD-26): /openapi.json and the checked-in artifact under
// api/openapi/openapi.json can never drift from the handlers. Every response
// carries a request_id and every failure uses the AD-15 envelope
// {code, message, details, request_id}, never a stack trace.
package api

import (
	"encoding/json"
	"net/http"
)

// Error codes are stable contract values. Clients may switch on them; the
// message is for humans and may change without a version bump.
const (
	CodeUnauthorized        = "unauthorized"
	CodeForbidden           = "forbidden"
	CodeNotFound            = "not_found"
	CodeMethodNotAllowed    = "method_not_allowed"
	CodeInvalidRequest      = "invalid_request"
	CodeInternalError       = "internal_error"
	CodeHealthUnavailable   = "health_overview_unavailable"
	CodeMetadataUnavailable = "metadata_unavailable"
	CodeQueryNotAvailable   = "query_service_unavailable"
	CodeControlPlaneDown    = "control_plane_unavailable"
)

// Error is the AD-15 error envelope. Details carries machine-readable context
// (for example the offending field) and never includes internal errors or
// stack traces.
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	RequestID string         `json:"request_id"`
}

// NewError builds the envelope and stamps the request id from the context.
func NewError(r *http.Request, code, message string, details map[string]any) Error {
	if details == nil {
		details = map[string]any{}
	}
	return Error{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: RequestIDFromContext(r.Context()),
	}
}

// WriteJSON writes a JSON response with the shared content type.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteError writes the AD-15 envelope.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	WriteJSON(w, status, NewError(r, code, message, details))
}
