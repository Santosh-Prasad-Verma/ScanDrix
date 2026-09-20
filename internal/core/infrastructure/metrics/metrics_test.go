package metrics

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMetricsCollector(t *testing.T) {
	mc := NewMetricsCollector(5, 100*time.Millisecond)
	defer func() { _ = mc.Close() }()

	mc.RecordCounter("test_counter", 1, map[string]string{"env": "test"})
	mc.RecordHistogram("test_duration_ms", 42.5, map[string]string{"env": "test"})
	mc.RecordGauge("test_active_workers", 3, map[string]string{"env": "test"})

	err := mc.Flush(context.Background())
	if err != nil {
		t.Fatalf("expected nil error on flush, got %v", err)
	}
}

func TestMetricsCollectorConcurrency(t *testing.T) {
	mc := NewMetricsCollector(10, 50*time.Millisecond)
	defer func() { _ = mc.Close() }()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			mc.RecordCounter("concurrent_reqs", 1, map[string]string{"worker": "worker-1"})
			mc.RecordHistogram("concurrent_lat", float64(id*10), map[string]string{"worker": "worker-1"})
		}(i)
	}
	wg.Wait()
}

func TestErrorRateMonitor(t *testing.T) {
	erm := NewErrorRateMonitor(10.0) // 10% threshold

	for i := 0; i < 18; i++ {
		erm.RecordRequest()
	}
	// Add 2 errors (total 20 reqs, 2 errors = 10%)
	erm.RecordError()
	erm.RecordError()

	rate := erm.CurrentErrorRate()
	if rate < 9.0 || rate > 11.0 {
		t.Fatalf("expected ~10%% error rate, got %f", rate)
	}
}

func TestReviewResponseMonitor(t *testing.T) {
	rrm := NewReviewResponseMonitor(100 * time.Millisecond)
	rrm.RecordReviewLatency(50 * time.Millisecond)
	rrm.RecordReviewLatency(150 * time.Millisecond) // breaches threshold
}

func TestWebhookFailureMonitor(t *testing.T) {
	wfm := NewWebhookFailureMonitor(3)
	wfm.RecordFailure("github", "invalid HMAC signature")
	wfm.RecordFailure("github", "invalid HMAC signature")
	wfm.RecordFailure("github", "invalid HMAC signature") // breaches threshold
	wfm.Reset()
}
