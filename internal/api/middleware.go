package api

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/observaki/observaki/internal/id"
)

// RequestIDHeader is the correlation header echoed on every response.
const RequestIDHeader = "X-Request-Id"

type requestIDKey struct{}

// RequestID stamps a UUIDv7 on each request, exposes it through the context and
// the X-Request-Id header, and keeps it on the response even when a handler
// panics.
//
// The client cannot supply the id: a caller-controlled value would let a
// request forge another request's correlation. Gateway and writer requests use
// the same header so a trace can span the pipeline.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := id.NewUUIDv7()
		w.Header().Set(RequestIDHeader, requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the request id, or an empty string outside the
// middleware chain.
func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

// Recover converts a panic into the AD-15 internal_error envelope. The panic
// and stack are logged server-side only (AD-15: no stack trace in a response).
func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			logger.Error("api panic recovered",
				"request_id", RequestIDFromContext(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
			WriteError(w, r, http.StatusInternalServerError, CodeInternalError,
				"internal server error", nil)
		}()
		next.ServeHTTP(w, r)
	})
}

// methodRestriction enforces one method per operation and answers with a
// structured 405 instead of the plain-text body http.ServeMux would emit.
func methodRestriction(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == method || (method == http.MethodGet && r.Method == http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Allow", method)
		WriteError(w, r, http.StatusMethodNotAllowed, CodeMethodNotAllowed,
			r.Method+" is not supported for this route", map[string]any{"allow": []string{method}})
	})
}
