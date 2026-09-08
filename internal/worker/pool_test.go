package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"load-tester/internal/stats"
)

// TestPool_BasicRequests verifies that requests actually get sent and
// recorded with the correct status code against a real (test) server.
func TestPool_BasicRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	recorder := stats.NewRecorder()
	pool := New(Config{
		URL:         srv.URL,
		Method:      http.MethodGet,
		Rate:        20,
		Concurrency: 4,
		Duration:    200 * time.Millisecond,
		Timeout:     time.Second,
	}, recorder)

	if err := pool.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	summary := recorder.Snapshot(200 * time.Millisecond)
	if summary.Total == 0 {
		t.Fatal("expected at least one recorded request, got 0")
	}
	if summary.Errors != 0 {
		t.Fatalf("expected 0 errors against a healthy server, got %d", summary.Errors)
	}
	if summary.Successes != summary.Total {
		t.Fatalf("expected all %d requests to succeed, got %d successes", summary.Total, summary.Successes)
	}
}

// TestPool_RecordsServerErrors verifies that 5xx responses are counted as
// errors in the summary, not silently treated as successes.
func TestPool_RecordsServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	recorder := stats.NewRecorder()
	pool := New(Config{
		URL:         srv.URL,
		Method:      http.MethodGet,
		Rate:        20,
		Concurrency: 4,
		Duration:    150 * time.Millisecond,
		Timeout:     time.Second,
	}, recorder)

	if err := pool.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	summary := recorder.Snapshot(150 * time.Millisecond)
	if summary.Total == 0 {
		t.Fatal("expected at least one recorded request, got 0")
	}
	if summary.Errors != summary.Total {
		t.Fatalf("expected all %d requests to be recorded as errors (500s), got %d", summary.Total, summary.Errors)
	}
}

// TestPool_RequestTimeout verifies that a server slower than the
// configured per-request timeout produces recorded errors rather than
// hanging the pool.
func TestPool_RequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	recorder := stats.NewRecorder()
	pool := New(Config{
		URL:         srv.URL,
		Method:      http.MethodGet,
		Rate:        10,
		Concurrency: 2,
		Duration:    150 * time.Millisecond,
		Timeout:     10 * time.Millisecond, // much shorter than the server's 100ms sleep
	}, recorder)

	if err := pool.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	summary := recorder.Snapshot(150 * time.Millisecond)
	if summary.Total == 0 {
		t.Fatal("expected at least one recorded request, got 0")
	}
	if summary.Errors == 0 {
		t.Fatal("expected timeout errors given a client timeout shorter than server latency, got 0 errors")
	}
}

// TestPool_StopsSchedulingOnCancellation verifies that cancelling the
// context stops the pool promptly rather than running for the full
// configured duration, and that Run still returns cleanly (no hang, no
// panic) with whatever partial results were collected.
func TestPool_StopsSchedulingOnCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	recorder := stats.NewRecorder()
	pool := New(Config{
		URL:         srv.URL,
		Method:      http.MethodGet,
		Rate:        20,
		Concurrency: 4,
		Duration:    5 * time.Second, // long duration; we expect cancellation to cut this short
		Timeout:     time.Second,
	}, recorder)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	start := time.Now()
	go func() {
		_ = pool.Run(ctx)
		close(done)
	}()

	// Let it run briefly, then cancel well before the 5s duration would
	// naturally elapse.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		elapsed := time.Since(start)
		if elapsed > 2*time.Second {
			t.Fatalf("expected Run to stop promptly after cancellation, took %s", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of context cancellation")
	}

	// We should have collected some results from before cancellation.
	summary := recorder.Snapshot(time.Since(start))
	if summary.Total == 0 {
		t.Fatal("expected some requests to have completed before cancellation, got 0")
	}
}

// TestPool_ZeroRateReturnsError verifies that a non-positive Rate is
// rejected with a clear error instead of causing a divide-by-zero panic
// in scheduleLoop's interval calculation (time.Second / Rate).
func TestPool_ZeroRateReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	recorder := stats.NewRecorder()
	pool := New(Config{
		URL:         srv.URL,
		Rate:        0,
		Concurrency: 1,
		Duration:    50 * time.Millisecond,
		Timeout:     time.Second,
	}, recorder)

	if err := pool.Run(context.Background()); err == nil {
		t.Fatal("expected an error for Rate=0, got nil")
	}
}

// TestPool_ZeroConcurrencyReturnsError verifies the same guard for
// Concurrency, which controls how many worker goroutines are spawned.
func TestPool_ZeroConcurrencyReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	recorder := stats.NewRecorder()
	pool := New(Config{
		URL:         srv.URL,
		Rate:        10,
		Concurrency: 0,
		Duration:    50 * time.Millisecond,
		Timeout:     time.Second,
	}, recorder)

	if err := pool.Run(context.Background()); err == nil {
		t.Fatal("expected an error for Concurrency=0, got nil")
	}
}

// TestPool_ZeroDurationReturnsError verifies the same guard for Duration, which controls how long the test runs for.
func TestPool_ZeroDurationReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	recorder := stats.NewRecorder()
	pool := New(Config{
		URL:         srv.URL,
		Rate:        10,
		Concurrency: 10,
		Duration:    0 * time.Millisecond,
		Timeout:     time.Second,
	}, recorder)

	if err := pool.Run(context.Background()); err == nil {
		t.Fatal("expected an error fur Duration=0, got nil")
	}
}

// TestPool_ZeroTimeoutReturnsError verifies the same guard for Duration, which controls how long the test runs for.
func TestPool_ZeroTimeoutReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	recorder := stats.NewRecorder()
	pool := New(Config{
		URL:         srv.URL,
		Rate:        10,
		Concurrency: 10,
		Duration:    10 * time.Millisecond,
		Timeout:     0 * time.Second,
	}, recorder)

	if err := pool.Run(context.Background()); err == nil {
		t.Fatal("expected an error fur Timeout=0, got nil")
	}
}
