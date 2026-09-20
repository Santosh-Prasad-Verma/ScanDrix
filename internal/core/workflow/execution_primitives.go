package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrJobAborted indicates execution was canceled by an external signal or context.
var ErrJobAborted = errors.New("job aborted by parent signal")

// RunWithTimeout executes a work function with a hard deadline.
// If the deadline expires before work finishes, context cancellation is propagated
// and an error with the specified timeoutMessage is returned.
func RunWithTimeout[T any](
	ctx context.Context,
	timeout time.Duration,
	timeoutMessage string,
	work func(ctx context.Context) (T, error),
) (T, error) {
	childCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		val T
		err error
	}
	resChan := make(chan result, 1)

	go func() {
		val, err := work(childCtx)
		resChan <- result{val: val, err: err}
	}()

	select {
	case res := <-resChan:
		return res.val, res.err
	case <-childCtx.Done():
		var zero T
		if errors.Is(childCtx.Err(), context.DeadlineExceeded) {
			return zero, fmt.Errorf("%s (timeout: %v)", timeoutMessage, timeout)
		}
		return zero, childCtx.Err()
	}
}

// RaceWithAbortSignal races a channel/work against an external abort signal.
// If cancelChan fires before work finishes, ErrJobAborted is returned immediately,
// allowing the worker slot to be released without blocking.
func RaceWithAbortSignal[T any](
	cancelChan <-chan struct{},
	work func() (T, error),
) (T, error) {
	type result struct {
		val T
		err error
	}
	resChan := make(chan result, 1)

	go func() {
		val, err := work()
		resChan <- result{val: val, err: err}
	}()

	select {
	case res := <-resChan:
		return res.val, res.err
	case <-cancelChan:
		var zero T
		return zero, ErrJobAborted
	}
}
