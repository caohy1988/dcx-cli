package errors

import (
	"encoding/json"
	"testing"
	"time"
)

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want int
	}{
		{MissingArgument, ExitValidation},
		{InvalidIdentifier, ExitValidation},
		{InvalidConfig, ExitValidation},
		{UnknownCommand, ExitValidation},
		{EvalFailed, ExitValidation},
		{APIError, ExitInfra},
		{InfraError, ExitInfra},
		{RateLimited, ExitInfra},
		{Internal, ExitInfra},
		{AuthError, ExitAuth},
		{NotFound, ExitNotFound},
		{Conflict, ExitConflict},
	}
	for _, tt := range tests {
		if got := ExitCodeFor(tt.code); got != tt.want {
			t.Errorf("ExitCodeFor(%s) = %d, want %d", tt.code, got, tt.want)
		}
	}
}

func TestRetryableFor(t *testing.T) {
	retryable := []ErrorCode{APIError, InfraError, RateLimited}
	notRetryable := []ErrorCode{MissingArgument, AuthError, NotFound, Conflict, Internal}

	for _, code := range retryable {
		if !RetryableFor(code) {
			t.Errorf("RetryableFor(%s) = false, want true", code)
		}
	}
	for _, code := range notRetryable {
		if RetryableFor(code) {
			t.Errorf("RetryableFor(%s) = true, want false", code)
		}
	}
}

func TestNew(t *testing.T) {
	env := New(NotFound, "Dataset not found: x", "Check dataset ID")

	if env.Error.Code != NotFound {
		t.Errorf("Code = %s, want %s", env.Error.Code, NotFound)
	}
	if env.Error.Message != "Dataset not found: x" {
		t.Errorf("Message = %s, want 'Dataset not found: x'", env.Error.Message)
	}
	if env.Error.Hint != "Check dataset ID" {
		t.Errorf("Hint = %s, want 'Check dataset ID'", env.Error.Hint)
	}
	if env.Error.ExitCode != ExitNotFound {
		t.Errorf("ExitCode = %d, want %d", env.Error.ExitCode, ExitNotFound)
	}
	if env.Error.Retryable {
		t.Error("Retryable = true, want false for NOT_FOUND")
	}
	if env.Error.Status != "error" {
		t.Errorf("Status = %s, want 'error'", env.Error.Status)
	}
}

func TestEnvelopeJSONShape(t *testing.T) {
	env := New(APIError, "Bad gateway", "Retry in 30s")
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Unmarshal into a generic map to verify JSON keys.
	var raw map[string]map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	errObj, ok := raw["error"]
	if !ok {
		t.Fatal("missing top-level 'error' key")
	}

	requiredKeys := []string{"code", "message", "hint", "exit_code", "retryable", "status"}
	for _, key := range requiredKeys {
		if _, ok := errObj[key]; !ok {
			t.Errorf("missing key 'error.%s'", key)
		}
	}
}

func TestEnvelopeJSONOmitsEmptyHint(t *testing.T) {
	env := New(AuthError, "Unauthorized", "")
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var raw map[string]map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if _, ok := raw["error"]["hint"]; ok {
		t.Error("expected 'hint' to be omitted when empty")
	}
}

func TestExitCodeFromHTTP(t *testing.T) {
	tests := []struct {
		status int
		want   int
	}{
		{401, ExitAuth},
		{403, ExitAuth},
		{404, ExitNotFound},
		{409, ExitConflict},
		{500, ExitInfra},
		{502, ExitInfra},
		{503, ExitInfra},
		{504, ExitInfra},
		{400, ExitInfra}, // default for unmapped client errors
	}
	for _, tt := range tests {
		if got := ExitCodeFromHTTP(tt.status); got != tt.want {
			t.Errorf("ExitCodeFromHTTP(%d) = %d, want %d", tt.status, got, tt.want)
		}
	}
}

func TestErrorCodeFromHTTP(t *testing.T) {
	tests := []struct {
		status int
		want   ErrorCode
	}{
		{401, AuthError},
		{403, AuthError},
		{404, NotFound},
		{409, Conflict},
		{429, RateLimited},
		{500, InfraError},
		{502, InfraError},
		{400, APIError},
	}
	for _, tt := range tests {
		if got := ErrorCodeFromHTTP(tt.status); got != tt.want {
			t.Errorf("ErrorCodeFromHTTP(%d) = %s, want %s", tt.status, got, tt.want)
		}
	}
}

func TestParseRetryAfter_IntegerSeconds(t *testing.T) {
	result := ParseRetryAfter("12")
	if result == nil || *result != 12 {
		t.Errorf("ParseRetryAfter(\"12\") = %v, want 12", result)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().UTC().Add(30 * time.Second).Format("Mon, 02 Jan 2006 15:04:05 GMT")
	result := ParseRetryAfter(future)
	if result == nil {
		t.Fatal("ParseRetryAfter(future date) = nil, want > 0")
	}
	if *result < 25 || *result > 35 {
		t.Errorf("ParseRetryAfter(future date) = %d, want ~30", *result)
	}
}

func TestParseRetryAfter_PastDate(t *testing.T) {
	past := time.Now().UTC().Add(-60 * time.Second).Format("Mon, 02 Jan 2006 15:04:05 GMT")
	result := ParseRetryAfter(past)
	if result != nil {
		t.Errorf("ParseRetryAfter(past date) = %v, want nil", *result)
	}
}

func TestParseRetryAfter_Empty(t *testing.T) {
	result := ParseRetryAfter("")
	if result != nil {
		t.Error("ParseRetryAfter(\"\") should return nil")
	}
}

func TestParseRetryAfter_Malformed(t *testing.T) {
	result := ParseRetryAfter("not-a-number-or-date")
	if result != nil {
		t.Error("ParseRetryAfter(malformed) should return nil")
	}
}

func TestParseRetryAfter_Zero(t *testing.T) {
	result := ParseRetryAfter("0")
	if result != nil {
		t.Error("ParseRetryAfter(\"0\") should return nil (non-positive)")
	}
}

func TestRateLimitedEnvelopeShape(t *testing.T) {
	secs := 12
	envelope := ErrorEnvelope{
		Error: ErrorDetail{
			Code:              RateLimited,
			Message:           "API rate limit exceeded",
			Hint:              "Retry after 12 seconds",
			ExitCode:          ExitCodeFor(RateLimited),
			Retryable:         true,
			RetryAfterSeconds: &secs,
			Status:            "error",
		},
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	errObj := raw["error"]
	if errObj["code"] != "RATE_LIMITED" {
		t.Errorf("code = %v, want RATE_LIMITED", errObj["code"])
	}
	if errObj["retryable"] != true {
		t.Error("retryable should be true")
	}
	if errObj["retry_after_seconds"] != float64(12) {
		t.Errorf("retry_after_seconds = %v, want 12", errObj["retry_after_seconds"])
	}
	if errObj["exit_code"] != float64(ExitInfra) {
		t.Errorf("exit_code = %v, want %d", errObj["exit_code"], ExitInfra)
	}
}

func TestRateLimitedEnvelopeOmitsRetryAfterWhenNil(t *testing.T) {
	envelope := ErrorEnvelope{
		Error: ErrorDetail{
			Code:      RateLimited,
			Message:   "rate limited",
			Hint:      "Retry later",
			ExitCode:  ExitInfra,
			Retryable: true,
			Status:    "error",
		},
	}

	data, _ := json.Marshal(envelope)
	var raw map[string]map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, ok := raw["error"]["retry_after_seconds"]; ok {
		t.Error("retry_after_seconds should be omitted when nil")
	}
}
