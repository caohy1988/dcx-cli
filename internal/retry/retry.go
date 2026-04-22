// Package retry provides a shared HTTP retry helper with exponential backoff.
//
// Retries on:
//   - 429 (rate limit): always, respects Retry-After header
//   - 5xx: only for transport-level failures (connection reset, timeout)
//     where the server may not have received the request
//
// Does NOT retry when the server returns a 5xx response body (request was
// processed, outcome uncertain). This is an intentional divergence from
// googleworkspace/cli which only retries 429 + connect/timeout.
package retry

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"time"

	dcxerrors "github.com/haiyuan-eng-google/dcx-cli/internal/errors"
)

const (
	// MaxRetryDelay caps the sleep duration per attempt.
	MaxRetryDelay = 60 * time.Second
)

// Do executes an HTTP request with retry logic. The buildRequest function
// is called for each attempt to produce a fresh request (body readers may
// be consumed on prior attempts).
//
// maxRetries=0 means no retries (single attempt). maxRetries=3 means up
// to 4 total attempts.
//
// Returns the successful response, or the last error/response on exhaustion.
func Do(client *http.Client, buildRequest func() (*http.Request, error), maxRetries int) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := buildRequest()
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)

		// Transport error (connection reset, timeout, DNS failure).
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				delay := backoffDelay(attempt, "")
				logRetry(attempt+1, maxRetries, delay, fmt.Sprintf("transport error: %v", err))
				time.Sleep(delay)
				continue
			}
			return nil, err
		}

		// 429: always retry, respect Retry-After.
		if resp.StatusCode == http.StatusTooManyRequests {
			lastResp = resp
			if attempt < maxRetries {
				retryAfter := resp.Header.Get("Retry-After")
				delay := backoffDelay(attempt, retryAfter)
				logRetry(attempt+1, maxRetries, delay, "rate limited (429)")
				resp.Body.Close()
				time.Sleep(delay)
				continue
			}
			return resp, nil
		}

		// Success or non-retryable error.
		return resp, nil
	}

	// Shouldn't reach here, but return last state.
	if lastResp != nil {
		return lastResp, nil
	}
	return nil, lastErr
}

// backoffDelay computes the retry delay. Uses Retry-After header if present,
// otherwise exponential backoff (1s, 2s, 4s, ...). Capped at MaxRetryDelay.
func backoffDelay(attempt int, retryAfterHeader string) time.Duration {
	// Try Retry-After header first.
	if seconds := dcxerrors.ParseRetryAfter(retryAfterHeader); seconds != nil {
		delay := time.Duration(*seconds) * time.Second
		if delay > MaxRetryDelay {
			return MaxRetryDelay
		}
		return delay
	}

	// Exponential backoff: 1s, 2s, 4s, 8s, ...
	secs := math.Pow(2, float64(attempt))
	delay := time.Duration(secs) * time.Second
	if delay > MaxRetryDelay {
		return MaxRetryDelay
	}
	return delay
}

// logRetry prints a retry message to stderr (dim styling for TTY).
func logRetry(attempt, maxRetries int, delay time.Duration, reason string) {
	fmt.Fprintf(os.Stderr, "\033[2mretry %d/%d in %s: %s\033[0m\n",
		attempt, maxRetries, delay.Round(time.Millisecond), reason)
}
