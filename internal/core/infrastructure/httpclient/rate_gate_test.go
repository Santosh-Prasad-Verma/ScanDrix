package httpclient

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateGateKey(t *testing.T) {
	k1 := RateGateKey("gh", "token-secret-abc")
	k2 := RateGateKey("gh", "token-secret-abc")
	k3 := RateGateKey("gh", "token-secret-xyz")

	assert.Equal(t, k1, k2, "identical tokens must yield identical keys")
	assert.NotEqual(t, k1, k3, "different tokens must yield distinct keys")
	assert.Contains(t, k1, "gh:", "prefix must be preserved")
}

func TestRunWithRateGate_ConcurrencyLimit(t *testing.T) {
	ResetRateGatesForTest()
	key := "test:concurrency"
	concurrency := 2
	opts := RateGateOptions{
		Concurrency: concurrency,
	}

	var active int32
	var maxActive int32
	var wg sync.WaitGroup

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = RunWithRateGate(context.Background(), key, opts, func(ctx context.Context) (struct{}, error) {
				curr := atomic.AddInt32(&active, 1)
				for {
					max := atomic.LoadInt32(&maxActive)
					if curr <= max || atomic.CompareAndSwapInt32(&maxActive, max, curr) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&active, -1)
				return struct{}{}, nil
			})
		}()
	}

	wg.Wait()
	assert.LessOrEqual(t, int(maxActive), concurrency, "active concurrent routines must not exceed concurrency option")
}

func TestParkRateGate(t *testing.T) {
	ResetRateGatesForTest()
	key := "test:park"
	opts := RateGateOptions{
		Concurrency: 1,
	}

	// First acquire and park for 50ms
	_, err := RunWithRateGate(context.Background(), key, opts, func(ctx context.Context) (int, error) {
		ParkRateGate(key, time.Now().Add(50*time.Millisecond))
		return 1, nil
	})
	assert.NoError(t, err)

	start := time.Now()
	_, err = RunWithRateGate(context.Background(), key, opts, func(ctx context.Context) (int, error) {
		return 2, nil
	})
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond, "second call should have waited for park window")
}
