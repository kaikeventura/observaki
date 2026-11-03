package ingest

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// requestError is the single error shape used by the gateway (AD-15). It maps
// to an HTTP status plus a google.rpc.Status body and to a gRPC status with an
// ErrorInfo detail carrying the request ID.
type requestError struct {
	httpStatus int
	code       codes.Code
	reason     string
	message    string
	requestID  string
	retryAfter time.Duration
	cause      error
}

func (e *requestError) Error() string { return e.message }

func newRequestError(httpStatus int, code codes.Code, reason, message, requestID string) *requestError {
	return &requestError{
		httpStatus: httpStatus,
		code:       code,
		reason:     reason,
		message:    message,
		requestID:  requestID,
	}
}

func (e *requestError) withCause(cause error) *requestError {
	e.cause = cause
	return e
}

func (e *requestError) withRetryAfter(retryAfter time.Duration) *requestError {
	e.retryAfter = retryAfter
	return e
}

func (e *requestError) retryAfterSeconds() int {
	if e.retryAfter <= 0 {
		return 0
	}
	seconds := int(math.Ceil(e.retryAfter.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return seconds
}

func (e *requestError) errorInfo() *errdetails.ErrorInfo {
	return &errdetails.ErrorInfo{
		Reason: e.reason,
		Domain: "observaki.io",
		Metadata: map[string]string{
			"request_id": e.requestID,
		},
	}
}

func (e *requestError) grpcStatus() error {
	st := status.New(e.code, e.message)
	var (
		detailed *status.Status
		err      error
	)
	if e.retryAfter > 0 {
		detailed, err = st.WithDetails(
			e.errorInfo(),
			&errdetails.RetryInfo{RetryDelay: durationpb.New(e.retryAfter)},
		)
	} else {
		detailed, err = st.WithDetails(e.errorInfo())
	}
	if err == nil {
		st = detailed
	}
	return st.Err()
}

func (e *requestError) proto() *statuspb.Status {
	st := status.New(e.code, e.message)
	var (
		detailed *status.Status
		err      error
	)
	if e.retryAfter > 0 {
		detailed, err = st.WithDetails(
			e.errorInfo(),
			&errdetails.RetryInfo{RetryDelay: durationpb.New(e.retryAfter)},
		)
	} else {
		detailed, err = st.WithDetails(e.errorInfo())
	}
	if err == nil {
		st = detailed
	}
	return st.Proto()
}

func errMethodNotAllowed(requestID, method string) *requestError {
	return newRequestError(
		http.StatusMethodNotAllowed,
		codes.InvalidArgument,
		"method_not_allowed",
		fmt.Sprintf("method %s is not allowed; use POST", method),
		requestID,
	)
}

func errUnsupportedMediaType(requestID, contentType string) *requestError {
	return newRequestError(
		http.StatusUnsupportedMediaType,
		codes.InvalidArgument,
		"unsupported_media_type",
		fmt.Sprintf("unsupported Content-Type %q; use application/x-protobuf or application/json", contentType),
		requestID,
	)
}

func errUnsupportedContentEncoding(requestID, contentEncoding string) *requestError {
	return newRequestError(
		http.StatusUnsupportedMediaType,
		codes.InvalidArgument,
		"unsupported_content_encoding",
		fmt.Sprintf("unsupported Content-Encoding %q; use gzip or identity", contentEncoding),
		requestID,
	)
}

func errInvalidArgument(requestID, message string) *requestError {
	return newRequestError(http.StatusBadRequest, codes.InvalidArgument, "invalid_argument", message, requestID)
}

func errTooLarge(requestID string, maxBytes int64) *requestError {
	return newRequestError(
		http.StatusRequestEntityTooLarge,
		codes.ResourceExhausted,
		"request_too_large",
		fmt.Sprintf("request body exceeds the %d-byte limit", maxBytes),
		requestID,
	)
}

func errTooManyEvents(requestID string, events, maxEvents int) *requestError {
	return newRequestError(
		http.StatusRequestEntityTooLarge,
		codes.ResourceExhausted,
		"request_too_large",
		fmt.Sprintf("request has %d events, limit is %d", events, maxEvents),
		requestID,
	)
}

func errNotFound(requestID, path string) *requestError {
	return newRequestError(
		http.StatusNotFound,
		codes.NotFound,
		"not_found",
		fmt.Sprintf("unknown endpoint %s", path),
		requestID,
	)
}

func errSinkUnavailable(requestID string, cause error) *requestError {
	return newRequestError(
		http.StatusServiceUnavailable,
		codes.Unavailable,
		"sink_unavailable",
		"ingest sink is unavailable; retry with backoff",
		requestID,
	).withRetryAfter(time.Second).withCause(cause)
}

func errInternal(requestID, message string, cause error) *requestError {
	return newRequestError(
		http.StatusInternalServerError,
		codes.Internal,
		"internal_error",
		message,
		requestID,
	).withCause(cause)
}

func errUnauthorized(requestID, reason, message string) *requestError {
	return newRequestError(
		http.StatusUnauthorized,
		codes.Unauthenticated,
		reason,
		message,
		requestID,
	)
}

func errForbidden(requestID, reason, message string) *requestError {
	return newRequestError(
		http.StatusForbidden,
		codes.PermissionDenied,
		reason,
		message,
		requestID,
	)
}

// errCrossProjectWrite rejects a request that asserts a tenant/project other
// than the one resolved from its token (Story 4.2, AC2).
func errCrossProjectWrite(requestID string) *requestError {
	return newRequestError(
		http.StatusForbidden,
		codes.PermissionDenied,
		"cross_project_write",
		"the ingest token is not authorized to write to the requested tenant/project",
		requestID,
	)
}

func errQuotaExceeded(requestID string, retryAfter time.Duration) *requestError {
	seconds := int(math.Ceil(retryAfter.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return newRequestError(
		http.StatusTooManyRequests,
		codes.ResourceExhausted,
		"quota_exceeded",
		fmt.Sprintf("ingest quota exceeded; retry after %ds", seconds),
		requestID,
	).withRetryAfter(time.Duration(seconds) * time.Second)
}

func errConfigStale(requestID string) *requestError {
	return newRequestError(
		http.StatusServiceUnavailable,
		codes.Unavailable,
		"config_stale",
		"ingest configuration snapshot is expired or unavailable; retry after the control plane republishes it",
		requestID,
	).withRetryAfter(time.Second)
}
