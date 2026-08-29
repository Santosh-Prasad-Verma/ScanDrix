package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerTrippingAndRecovery(t *testing.T) {
	cb := NewCircuitBreaker(3, 50*time.Millisecond)

	failErr := errors.New("upstream service unavailable")

	// Trigger 3 failures
	for i := 0; i < 3; i++ {
		_ = cb.Execute(context.Background(), 0, func(ctx context.Context) error {
			return failErr
		})
	}

	// 4th call should immediately fail fast with ErrCircuitOpen without executing function
	executed := false
	err := cb.Execute(context.Background(), 0, func(ctx context.Context) error {
		executed = true
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got: %v", err)
	}
	if executed {
		t.Error("expected function not to execute while circuit is open")
	}

	// Wait for cooldown to transition to Half-Open
	time.Sleep(60 * time.Millisecond)

	// In Half-Open, 3 successful calls should close the circuit
	for i := 0; i < 3; i++ {
		err := cb.Execute(context.Background(), 0, func(ctx context.Context) error {
			return nil
		})
		if err != nil {
			t.Fatalf("call %d failed unexpectedly: %v", i, err)
		}
	}

	// Verify circuit is closed
	if cb.state != StateClosed {
		t.Errorf("expected circuit to be CLOSED, got %d", cb.state)
	}
}
