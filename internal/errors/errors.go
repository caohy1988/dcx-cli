// Package errors provides structured error handling for dcx.
//
// All errors are emitted as JSON envelopes on stderr with typed error codes
// and semantic exit codes. This ensures machine-safe error handling for
// agents, scripts, and CI pipelines.
package errors

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ErrorCode identifies the category of error.
type ErrorCode string

const (
	MissingArgument   ErrorCode = "MISSING_ARGUMENT"
	InvalidIdentifier ErrorCode = "INVALID_IDENTIFIER"
	InvalidConfig     ErrorCode = "INVALID_CONFIG"
	UnknownCommand    ErrorCode = "UNKNOWN_COMMAND"
	AuthError         ErrorCode = "AUTH_ERROR"
	APIError          ErrorCode = "API_ERROR"
	NotFound          ErrorCode = "NOT_FOUND"
	Conflict          ErrorCode = "CONFLICT"
	RateLimited       ErrorCode = "RATE_LIMITED"
	EvalFailed        ErrorCode = "EVAL_FAILED"
	InfraError        ErrorCode = "INFRA_ERROR"
	Internal          ErrorCode = "INTERNAL"
)

// ExitCode maps error categories to process exit codes.
// These must match the Rust implementation exactly.
const (
	ExitSuccess    = 0 // success
	ExitValidation = 1 // validation / eval failure
	ExitInfra      = 2 // infrastructure / API error (500, 502, 503, 504)
	ExitAuth       = 3 // auth error (401, 403)
	ExitNotFound   = 4 // not found (404)
	ExitConflict   = 5 // conflict (409)
)

// ExitCodeFor returns the semantic exit code for an error code.
func ExitCodeFor(code ErrorCode) int {
	switch code {
	case MissingArgument, InvalidIdentifier, InvalidConfig, UnknownCommand, EvalFailed:
		return ExitValidation
	case APIError, InfraError, RateLimited, Internal:
		return ExitInfra
	case AuthError:
		return ExitAuth
	case NotFound:
		return ExitNotFound
	case Conflict:
		return ExitConflict
	default:
		return ExitInfra
	}
}

// RetryableFor returns whether an error code's underlying condition is
// typically retryable.
func RetryableFor(code ErrorCode) bool {
	switch code {
	case APIError, InfraError, RateLimited:
		return true
	default:
		return false
	}
}

// ErrorDetail is the inner payload of an error envelope.
type ErrorDetail struct {
	Code              ErrorCode `json:"code"`
	Message           string    `json:"message"`
	Hint              string    `json:"hint,omitempty"`
	ExitCode          int       `json:"exit_code"`
	Retryable         bool      `json:"retryable"`
	RetryAfterSeconds *int      `json:"retry_after_seconds,omitempty"`
	Status            string    `json:"status"`
}

// ErrorEnvelope is the top-level JSON written to stderr.
type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

// Emit writes a structured error envelope to stderr and exits.
func Emit(code ErrorCode, message, hint string) {
	exitCode := ExitCodeFor(code)
	envelope := ErrorEnvelope{
		Error: ErrorDetail{
			Code:      code,
			Message:   sanitizeErrorText(message),
			Hint:      sanitizeErrorText(hint),
			ExitCode:  exitCode,
			Retryable: RetryableFor(code),
			Status:    "error",
		},
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		// Last-resort fallback if JSON marshal fails.
		fmt.Fprintf(os.Stderr, `{"error":{"code":"INTERNAL","message":"failed to marshal error","exit_code":2,"retryable":false,"status":"error"}}`)
		os.Exit(ExitInfra)
	}
	fmt.Fprintln(os.Stderr, string(data))
	os.Exit(exitCode)
}

// EmitWithExit writes a structured error envelope to stderr and exits
// with the given exit code, overriding the default for the error code.
func EmitWithExit(code ErrorCode, message, hint string, exitCode int) {
	envelope := ErrorEnvelope{
		Error: ErrorDetail{
			Code:      code,
			Message:   message,
			Hint:      hint,
			ExitCode:  exitCode,
			Retryable: RetryableFor(code),
			Status:    "error",
		},
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"error":{"code":"INTERNAL","message":"failed to marshal error","exit_code":2,"retryable":false,"status":"error"}}`)
		os.Exit(ExitInfra)
	}
	fmt.Fprintln(os.Stderr, string(data))
	os.Exit(exitCode)
}

// New creates an ErrorEnvelope without emitting it. Useful for building
// errors that will be rendered by the caller (e.g., in tests or when
// the caller controls exit behavior).
func New(code ErrorCode, message, hint string) ErrorEnvelope {
	return ErrorEnvelope{
		Error: ErrorDetail{
			Code:      code,
			Message:   message,
			Hint:      hint,
			ExitCode:  ExitCodeFor(code),
			Retryable: RetryableFor(code),
			Status:    "error",
		},
	}
}

// Write writes the envelope as JSON to stderr without exiting.
func (e ErrorEnvelope) Write() {
	data, err := json.Marshal(e)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"error":{"code":"INTERNAL","message":"failed to marshal error","exit_code":2,"retryable":false,"status":"error"}}`)
		return
	}
	fmt.Fprintln(os.Stderr, string(data))
}

// EmitRateLimited writes a RATE_LIMITED error envelope with retry_after_seconds
// parsed from the Retry-After header. Handles both integer-seconds and HTTP-date
// forms. Malformed or past values omit retry_after_seconds with a generic hint.
func EmitRateLimited(message string, retryAfterHeader string) {
	seconds := ParseRetryAfter(retryAfterHeader)
	hint := "Retry later"
	if seconds != nil {
		hint = fmt.Sprintf("Retry after %d seconds", *seconds)
	}

	exitCode := ExitCodeFor(RateLimited)
	envelope := ErrorEnvelope{
		Error: ErrorDetail{
			Code:              RateLimited,
			Message:           sanitizeErrorText(message),
			Hint:              sanitizeErrorText(hint),
			ExitCode:          exitCode,
			Retryable:         true,
			RetryAfterSeconds: seconds,
			Status:            "error",
		},
	}
	data, _ := json.Marshal(envelope)
	fmt.Fprintln(os.Stderr, string(data))
	os.Exit(exitCode)
}

// ParseRetryAfter parses a Retry-After header value into seconds.
// Accepts integer-seconds ("12") or HTTP-date ("Fri, 18 Apr 2026 12:30:00 GMT").
// Returns nil for malformed, empty, or past values.
func ParseRetryAfter(header string) *int {
	if header == "" {
		return nil
	}

	// Try integer-seconds first.
	if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
		return &secs
	}

	// Try HTTP-date (RFC 1123).
	if t, err := http.ParseTime(header); err == nil {
		delta := int(math.Ceil(time.Until(t).Seconds()))
		if delta > 0 {
			return &delta
		}
		return nil // past date
	}

	return nil // malformed
}

// sanitizeErrorText strips control characters and ANSI escapes from error
// messages before they are written to stderr. API responses may contain
// untrusted content that could inject terminal escape sequences.
func sanitizeErrorText(s string) string {
	result := make([]byte, 0, len(s))
	inEscape := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			inEscape = true
			i++
			continue
		}
		if inEscape {
			if b >= 0x40 && b <= 0x7E {
				inEscape = false
			}
			continue
		}
		if b == '\n' || b == '\t' {
			result = append(result, b)
			continue
		}
		if b < 0x20 || b == 0x7F {
			continue
		}
		result = append(result, b)
	}
	return string(result)
}

// ExitCodeFromHTTP maps an HTTP status code to a semantic exit code.
func ExitCodeFromHTTP(status int) int {
	switch {
	case status == 401 || status == 403:
		return ExitAuth
	case status == 404:
		return ExitNotFound
	case status == 409:
		return ExitConflict
	case status >= 500:
		return ExitInfra
	default:
		return ExitInfra
	}
}

// ErrorCodeFromHTTP maps an HTTP status code to an error code.
func ErrorCodeFromHTTP(status int) ErrorCode {
	switch {
	case status == 401 || status == 403:
		return AuthError
	case status == 404:
		return NotFound
	case status == 409:
		return Conflict
	case status == 429:
		return RateLimited
	case status >= 500:
		return InfraError
	default:
		return APIError
	}
}
