package utils

import (
	"context"
	"sync"
)

// ComposeContext coordinates cancellation from a parent context into a derived context.
// It invokes onAbort at most once before cancelling the derived context, and returns a detach func.
func ComposeContext(parent context.Context, onAbort func()) (context.Context, context.CancelFunc) {
	derivedCtx, cancel := context.WithCancel(context.Background())
	if parent == nil {
		return derivedCtx, cancel
	}

	var once sync.Once
	trigger := func() {
		once.Do(func() {
			if onAbort != nil {
				onAbort()
			}
			cancel()
		})
	}

	if parent.Err() != nil {
		trigger()
		return derivedCtx, cancel
	}

	stop := make(chan struct{})
	go func() {
		select {
		case <-parent.Done():
			trigger()
		case <-stop:
		}
	}()

	detach := func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
		cancel()
	}

	return derivedCtx, detach
}
