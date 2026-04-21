package errors

import (
	"errors"
	"fmt"
)

// APIHTTPError is returned by HTTP clients to preserve the status code
// and Retry-After header for the CLI layer to handle. Library-level
// clients should return this instead of calling Emit/EmitRateLimited
// directly, so callers can inspect and handle the error.
type APIHTTPError struct {
	StatusCode int
	Message    string
	RetryAfter string // raw Retry-After header value
}

func (e *APIHTTPError) Error() string {
	return e.Message
}

// EmitHTTPError emits the appropriate structured error envelope for an
// APIHTTPError. This should be called at the CLI layer, not in library clients.
func EmitHTTPError(err *APIHTTPError) {
	if err.StatusCode == 429 {
		EmitRateLimited(err.Message, err.RetryAfter)
		return
	}
	code := ErrorCodeFromHTTP(err.StatusCode)
	Emit(code, err.Message, fmt.Sprintf("HTTP %d", err.StatusCode))
}

// EmitAPIError emits the right error envelope for any error. If the error
// is an APIHTTPError (from a library client), it emits the HTTP-aware
// envelope including rate-limit handling. Otherwise emits a generic API_ERROR.
// Use this at the CLI layer for all API call errors.
func EmitAPIError(err error) {
	var httpErr *APIHTTPError
	if errors.As(err, &httpErr) {
		EmitHTTPError(httpErr)
		return
	}
	Emit(APIError, err.Error(), "")
}
