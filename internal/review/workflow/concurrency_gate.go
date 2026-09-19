package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/review/domain"
)

// ByokConcurrencyGate regulates concurrent LLM API requests per organization and model.
type ByokConcurrencyGate struct {
	mu           sync.Mutex
	activeSlots  map[string]int
	defaultSlots int
	limits       map[string]int // hash(org:provider:model) -> maxSlots
}

// NewByokConcurrencyGate constructs a concurrency limiter.
func NewByokConcurrencyGate(defaultConcurrent int) *ByokConcurrencyGate {
	if defaultConcurrent <= 0 {
		defaultConcurrent = 5
	}
	return &ByokConcurrencyGate{
		activeSlots:  make(map[string]int),
		defaultSlots: defaultConcurrent,
		limits:       make(map[string]int),
	}
}

// SetLimit configures specific maximum concurrent slots for an org and provider.
func (g *ByokConcurrencyGate) SetLimit(orgID, provider, model string, maxConcurrent int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := g.buildKey(orgID, provider, model)
	g.limits[key] = maxConcurrent
}

func (g *ByokConcurrencyGate) buildKey(orgID, provider, model string) string {
	raw := fmt.Sprintf("%s:%s:%s", orgID, provider, model)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:8])
}

// AcquireSlot blocks or attempts to obtain a concurrency slot. Returns release function.
func (g *ByokConcurrencyGate) AcquireSlot(ctx context.Context, orgID, provider, model string) (func(), error) {
	key := g.buildKey(orgID, provider, model)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			g.mu.Lock()
			limit := g.defaultSlots
			if custom, ok := g.limits[key]; ok && custom > 0 {
				limit = custom
			}

			current := g.activeSlots[key]
			if current < limit {
				g.activeSlots[key] = current + 1
				g.mu.Unlock()

				var once sync.Once
				release := func() {
					once.Do(func() {
						g.mu.Lock()
						if g.activeSlots[key] > 0 {
							g.activeSlots[key]--
						}
						g.mu.Unlock()
					})
				}
				return release, nil
			}
			g.mu.Unlock()
		}
	}
}

// Verify ByokConcurrencyGate satisfies domain.IByokConcurrencyGate contract.
var _ domain.IByokConcurrencyGate = (*ByokConcurrencyGate)(nil)
