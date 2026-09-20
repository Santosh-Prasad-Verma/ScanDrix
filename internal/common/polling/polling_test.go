package polling

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCalculateBackoff(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     2 * time.Second,
		Multiplier:      2.0,
		JitterPercent:   0.10,
	}

	delay0 := CalculateBackoff(cfg, 0)
	// Base is 100ms, with ±10% jitter it should be between 90ms and 110ms
	if delay0 < 80*time.Millisecond || delay0 > 130*time.Millisecond {
		t.Fatalf("unexpected delay0: %v", delay0)
	}

	delay2 := CalculateBackoff(cfg, 2)
	// Base is 400ms, with ±10% jitter should be between 350ms and 450ms
	if delay2 < 300*time.Millisecond || delay2 > 500*time.Millisecond {
		t.Fatalf("unexpected delay2: %v", delay2)
	}
}

func TestRetryWithBackoffSuccess(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 5 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      1.5,
		MaxAttempts:     4,
	}

	calls := 0
	err := RetryWithBackoff(context.Background(), cfg, func(attempt int) error {
		calls++
		if calls < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoffExhausted(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 2 * time.Millisecond,
		MaxInterval:     10 * time.Millisecond,
		Multiplier:      1.5,
		MaxAttempts:     3,
	}

	testErr := errors.New("permanent failure")
	calls := 0
	err := RetryWithBackoff(context.Background(), cfg, func(attempt int) error {
		calls++
		return testErr
	})

	if !errors.Is(err, testErr) {
		t.Fatalf("expected permanent failure error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}
