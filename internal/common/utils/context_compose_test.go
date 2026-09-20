package utils_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/common/utils"
)

func TestComposeContext(t *testing.T) {
	t.Run("is a no-op when no parent context is provided", func(t *testing.T) {
		ctx, detach := utils.ComposeContext(nil, nil)
		if ctx.Err() != nil {
			t.Fatal("context should not be cancelled")
		}
		detach()
	})

	t.Run("cancels the derived context synchronously when parent is already cancelled", func(t *testing.T) {
		parent, cancelParent := context.WithCancel(context.Background())
		cancelParent()

		ctx, detach := utils.ComposeContext(parent, nil)
		defer detach()

		if ctx.Err() == nil {
			t.Fatal("derived context should be cancelled immediately")
		}
	})

	t.Run("cancels the derived context when parent cancels later", func(t *testing.T) {
		parent, cancelParent := context.WithCancel(context.Background())
		defer cancelParent()

		ctx, detach := utils.ComposeContext(parent, nil)
		defer detach()

		if ctx.Err() != nil {
			t.Fatal("derived context should not be cancelled yet")
		}

		cancelParent()
		select {
		case <-ctx.Done():
			// OK
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for derived context cancellation")
		}
	})

	t.Run("invokes onAbort callback exactly once before cancelling", func(t *testing.T) {
		parent, cancelParent := context.WithCancel(context.Background())
		defer cancelParent()

		var callCount int32
		onAbort := func() {
			atomic.AddInt32(&callCount, 1)
		}

		ctx, detach := utils.ComposeContext(parent, onAbort)
		defer detach()

		cancelParent()
		select {
		case <-ctx.Done():
			// OK
		case <-time.After(1 * time.Second):
			t.Fatal("timeout")
		}

		if atomic.LoadInt32(&callCount) != 1 {
			t.Fatalf("expected callback to be called once, got %d", callCount)
		}
	})
}
