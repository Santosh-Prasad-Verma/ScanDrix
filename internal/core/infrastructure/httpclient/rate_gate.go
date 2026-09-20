// Package httpclient implements per-key rate gating for external SCM endpoints.
package httpclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type gateState struct {
	mu            sync.Mutex
	sem           chan struct{}
	nextAllowedAt time.Time
	pausedUntil   time.Time
}

var (
	gatesMu sync.Mutex
	gates   = make(map[string]*gateState)
)

// RateGateOptions defines configuration for gating calls sharing a single credential.
type RateGateOptions struct {
	// MinInterval is the minimum spacing between the start of two consecutive calls.
	MinInterval time.Duration
	// Concurrency is the number of concurrent call slots. Defaults to 1 (full serialization).
	Concurrency int
}

// RateGateKey derives a non-reversible, hashed key from a raw credential or token.
func RateGateKey(prefix, rawSecret string) string {
	h := sha256.Sum256([]byte(rawSecret))
	digest := hex.EncodeToString(h[:8]) // 16 hex chars
	return fmt.Sprintf("%s:%s", prefix, digest)
}

func getOrCreateGate(key string, concurrency int) *gateState {
	if concurrency <= 0 {
		concurrency = 1
	}

	gatesMu.Lock()
	defer gatesMu.Unlock()

	g, exists := gates[key]
	if !exists {
		g = &gateState{
			sem: make(chan struct{}, concurrency),
		}
		gates[key] = g
	}
	return g
}

// RunWithRateGate executes fn under the rate gate for the specified key.
func RunWithRateGate[T any](ctx context.Context, key string, opts RateGateOptions, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	gate := getOrCreateGate(key, concurrency)

	// Acquire concurrency slot
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case gate.sem <- struct{}{}:
		defer func() { <-gate.sem }()
	}

	// Spacing & park loop
	for {
		gate.mu.Lock()
		now := time.Now()
		waitUntil := gate.pausedUntil
		if gate.nextAllowedAt.After(waitUntil) {
			waitUntil = gate.nextAllowedAt
		}

		if !waitUntil.After(now) {
			// Reserve next slot before initiating call
			if opts.MinInterval > 0 {
				gate.nextAllowedAt = now.Add(opts.MinInterval)
			}
			gate.mu.Unlock()
			break
		}

		sleepDur := waitUntil.Sub(now)
		gate.mu.Unlock()

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(sleepDur):
			// Check again in case pausedUntil was extended by another call
		}
	}

	return fn(ctx)
}

// ParkRateGate parks key until at least untilTime. Idempotent and monotonic (never shortens).
func ParkRateGate(key string, untilTime time.Time) {
	gatesMu.Lock()
	g, exists := gates[key]
	gatesMu.Unlock()

	if exists {
		g.mu.Lock()
		if untilTime.After(g.pausedUntil) {
			g.pausedUntil = untilTime
		}
		g.mu.Unlock()
	}
}

// ResetRateGatesForTest clears the gate registry for isolated unit tests.
func ResetRateGatesForTest() {
	gatesMu.Lock()
	defer gatesMu.Unlock()
	gates = make(map[string]*gateState)
}
