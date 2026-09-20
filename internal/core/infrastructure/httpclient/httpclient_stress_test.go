package httpclient_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/infrastructure/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRateGateStress_ConcurrencyBoundaries verifies that RunWithRateGate strictly enforces
// the configured concurrency limit across 30 concurrent competing goroutines.
func TestRateGateStress_ConcurrencyBoundaries(t *testing.T) {
	const maxConcurrency = 3
	const numGoroutines = 30

	key := httpclient.RateGateKey("github_pat", "ghp_secret_token_1234567890abcdef")
	opts := httpclient.RateGateOptions{
		Concurrency: maxConcurrency,
		MinInterval: 2 * time.Millisecond,
	}

	var activeCalls atomic.Int32
	var maxObservedActive atomic.Int32
	var completedCalls atomic.Int32

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			res, err := httpclient.RunWithRateGate(ctx, key, opts, func(innerCtx context.Context) (string, error) {
				current := activeCalls.Add(1)
				defer activeCalls.Add(-1)

				// Record peak concurrency
				for {
					peak := maxObservedActive.Load()
					if current <= peak || maxObservedActive.CompareAndSwap(peak, current) {
						break
					}
				}

				time.Sleep(10 * time.Millisecond)
				return "ok", nil
			})

			assert.NoError(t, err)
			assert.Equal(t, "ok", res)
			completedCalls.Add(1)
		}()
	}

	wg.Wait()

	assert.Equal(t, int32(numGoroutines), completedCalls.Load())
	assert.True(t, maxObservedActive.Load() <= int32(maxConcurrency),
		"Concurrency exceeded: observed %d, max allowed %d", maxObservedActive.Load(), maxConcurrency)
}

// TestRateGateStress_ContextCancellationFreesSlot confirms that when a caller context
// cancels while queued for a rate gate slot, the slot is immediately released for others.
func TestRateGateStress_ContextCancellationFreesSlot(t *testing.T) {
	key := httpclient.RateGateKey("gemini_key", "AIzaSy_secret_key_123")
	opts := httpclient.RateGateOptions{
		Concurrency: 1, // Single slot
		MinInterval: 0,
	}

	startedFirst := make(chan struct{})
	releaseFirst := make(chan struct{})

	// 1. First caller acquires the only slot and holds it
	go func() {
		_, _ = httpclient.RunWithRateGate(context.Background(), key, opts, func(ctx context.Context) (string, error) {
			close(startedFirst)
			<-releaseFirst
			return "done", nil
		})
	}()

	<-startedFirst

	// 2. Second caller attempts with a fast-cancelling context
	ctxCancel, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := httpclient.RunWithRateGate(ctxCancel, key, opts, func(ctx context.Context) (string, error) {
		return "should_not_run", nil
	})
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// 3. Release first caller
	close(releaseFirst)

	// 4. Third caller should now succeed immediately
	ctxThird, cancelThird := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelThird()

	res, err := httpclient.RunWithRateGate(ctxThird, key, opts, func(ctx context.Context) (string, error) {
		return "third_ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "third_ok", res)
}

// TestRetryStress_429WithRetryAfterHeader simulates an HTTP 429 response with Retry-After
// and tests automatic backoff and eventual success.
func TestRetryStress_429WithRetryAfterHeader(t *testing.T) {
	var attempts atomic.Int32

	opts := httpclient.WithRetryOptions{
		MaxAttempts: 4,
		BaseDelay:   5 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		Label:       "test-retry-429",
	}

	res, err := httpclient.With429Retry(context.Background(), opts, func(ctx context.Context, attempt int) (string, error) {
		curr := attempts.Add(1)
		if curr < 3 {
			headers := make(http.Header)
			headers.Set("Retry-After", "0.01") // 10ms
			return "", &httpclient.HTTPStatusError{
				Code:    429,
				Headers: headers,
				Message: "Too Many Requests",
			}
		}
		return "success", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "success", res)
	assert.Equal(t, int32(3), attempts.Load())
}

// TestRetryStress_PermanentErrorStopsRetrying verifies that non-transient, non-429 errors
// (e.g. 401 Unauthorized, 403 Forbidden, 404 Not Found) fail fast without looping.
func TestRetryStress_PermanentErrorStopsRetrying(t *testing.T) {
	var attempts atomic.Int32

	opts := httpclient.WithRetryOptions{
		MaxAttempts: 5,
		BaseDelay:   10 * time.Millisecond,
	}

	permanentErrors := []error{
		&httpclient.HTTPStatusError{Code: 401, Message: "Unauthorized"},
		&httpclient.HTTPStatusError{Code: 403, Message: "Forbidden"},
		&httpclient.HTTPStatusError{Code: 404, Message: "Not Found"},
		errors.New("generic fatal non-network error"),
	}

	for _, permErr := range permanentErrors {
		attempts.Store(0)
		_, err := httpclient.With429Retry(context.Background(), opts, func(ctx context.Context, attempt int) (string, error) {
			attempts.Add(1)
			return "", permErr
		})

		assert.Error(t, err)
		assert.Equal(t, int32(1), attempts.Load(), "Expected exactly 1 attempt for permanent error %v", permErr)
	}
}

// TestRetryStress_ContextCancellationDuringRetryLoop ensures that cancelling the root context
// immediately aborts the retry loop.
func TestRetryStress_ContextCancellationDuringRetryLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	opts := httpclient.WithRetryOptions{
		MaxAttempts: 10,
		BaseDelay:   100 * time.Millisecond,
	}

	var attempts atomic.Int32
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	_, err := httpclient.With429Retry(ctx, opts, func(ctx context.Context, attempt int) (string, error) {
		attempts.Add(1)
		return "", &httpclient.HTTPStatusError{Code: 429, Message: "Rate limited"}
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.True(t, attempts.Load() < 5)
}

// TestPooledClientStress_RoundtripAndConcurrency tests NewPooledClient against
// an httptest.Server under concurrent requests.
func TestPooledClientStress_RoundtripAndConcurrency(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := httpclient.NewPooledClient(5 * time.Second)
	const numRequests = 40

	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			resp, err := client.Get(server.URL)
			assert.NoError(t, err)
			if resp != nil {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				_ = resp.Body.Close()
			}
		}()
	}

	wg.Wait()
}

// TestRateGateKey_DeterminismAndCollisions verifies key derivation hashing properties.
func TestRateGateKey_DeterminismAndCollisions(t *testing.T) {
	k1 := httpclient.RateGateKey("token", "secret-token-1")
	k2 := httpclient.RateGateKey("token", "secret-token-1")
	assert.Equal(t, k1, k2)

	k3 := httpclient.RateGateKey("token", "secret-token-2")
	assert.NotEqual(t, k1, k3)

	k4 := httpclient.RateGateKey("other_prefix", "secret-token-1")
	assert.NotEqual(t, k1, k4)
}
