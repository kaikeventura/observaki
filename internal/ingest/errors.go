package ingest

import (
	"fmt"
	"net/http"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	if detailed, err := st.WithDetails(e.errorInfo()); err == nil {
		st = detailed
	}
	return st.Err()
}

func (e *requestError) proto() *statuspb.Status {
	st := status.New(e.code, e.message)
	if detailed, err := st.WithDetails(e.errorInfo()); err == nil {
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
	).withCause(cause)
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
