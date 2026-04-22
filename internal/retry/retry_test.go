package retry

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo_NoRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	resp, err := Do(nil, func() (*http.Request, error) {
		return http.NewRequest("GET", server.URL, nil)
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDo_Retries429(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	start := time.Now()
	resp, err := Do(nil, func() (*http.Request, error) {
		return http.NewRequest("GET", server.URL, nil)
	}, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("retry took too long — Retry-After:0 should be near-instant")
	}
}

func TestDo_ExhaustsRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
	}))
	defer server.Close()

	resp, err := Do(nil, func() (*http.Request, error) {
		return http.NewRequest("GET", server.URL, nil)
	}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return the last 429 response when retries exhausted.
	if resp.StatusCode != 429 {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDo_NoRetryOn400(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(400)
	}))
	defer server.Close()

	resp, err := Do(nil, func() (*http.Request, error) {
		return http.NewRequest("GET", server.URL, nil)
	}, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 400)", attempts)
	}
}

func TestDo_NoRetryOn500Response(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":"internal"}`)
	}))
	defer server.Close()

	resp, err := Do(nil, func() (*http.Request, error) {
		return http.NewRequest("GET", server.URL, nil)
	}, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 500 with a response body = server processed the request. No retry.
	if resp.StatusCode != 500 {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	resp.Body.Close()

	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 500 with body)", attempts)
	}
}

func TestDo_ZeroRetries(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(429)
	}))
	defer server.Close()

	resp, err := Do(nil, func() (*http.Request, error) {
		return http.NewRequest("GET", server.URL, nil)
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 429 {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}
	resp.Body.Close()

	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("attempts = %d, want 1 (no retries with maxRetries=0)", attempts)
	}
}

func TestDo_RespectsRetryAfterHeader(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	start := time.Now()
	resp, err := Do(nil, func() (*http.Request, error) {
		return http.NewRequest("GET", server.URL, nil)
	}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	elapsed := time.Since(start)
	if elapsed < 900*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 1s (Retry-After:1)", elapsed)
	}
}

func TestBackoffDelay(t *testing.T) {
	// No header: exponential.
	if d := backoffDelay(0, ""); d != 1*time.Second {
		t.Errorf("attempt 0 = %v, want 1s", d)
	}
	if d := backoffDelay(1, ""); d != 2*time.Second {
		t.Errorf("attempt 1 = %v, want 2s", d)
	}
	if d := backoffDelay(2, ""); d != 4*time.Second {
		t.Errorf("attempt 2 = %v, want 4s", d)
	}

	// With header: uses header value.
	if d := backoffDelay(0, "5"); d != 5*time.Second {
		t.Errorf("with Retry-After:5 = %v, want 5s", d)
	}

	// Header capped at MaxRetryDelay.
	if d := backoffDelay(0, "120"); d != MaxRetryDelay {
		t.Errorf("with Retry-After:120 = %v, want %v", d, MaxRetryDelay)
	}

	// Malformed header: falls back to exponential.
	if d := backoffDelay(1, "invalid"); d != 2*time.Second {
		t.Errorf("with invalid header = %v, want 2s", d)
	}
}
