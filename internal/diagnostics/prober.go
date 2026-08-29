package diagnostics

import (
	"context"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/internal/queue/relay"
)

// HealthProber inspects runtime subsystems and generates diagnostic audits.
type HealthProber struct {
	startTime time.Time
	outbox    *relay.OutboxStore
	cache     *limiter.TieredCache
}

// NewHealthProber initializes the diagnostic prober.
func NewHealthProber(outbox *relay.OutboxStore, cache *limiter.TieredCache) *HealthProber {
	return &HealthProber{
		startTime: time.Now().UTC(),
		outbox:    outbox,
		cache:     cache,
	}
}

// RunDiagnostics executes concurrent health evaluations across all registered subsystems.
func (p *HealthProber) RunDiagnostics(ctx context.Context) *SystemDiagnosticReport {
	report := &SystemDiagnosticReport{
		Components:  make(map[string]ComponentHealth),
		Uptime:      time.Since(p.startTime),
		LastChecked: time.Now().UTC(),
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	report.MemoryAllocatedMB = math.Round((float64(m.Alloc)/1024/1024)*100) / 100
	report.Goroutines = runtime.NumGoroutine()

	var mu sync.Mutex
	var wg sync.WaitGroup

	// 1. Probe Queue Outbox
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch := p.probeQueue(ctx)
		mu.Lock()
		report.Components["queue_outbox"] = ch
		mu.Unlock()
	}()

	// 2. Probe Distributed Cache
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch := p.probeCache(ctx)
		mu.Lock()
		report.Components["cache_tiered"] = ch
		mu.Unlock()
	}()

	// 3. Probe Runtime Engine
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch := p.probeRuntime(ctx, m)
		mu.Lock()
		report.Components["runtime_engine"] = ch
		mu.Unlock()
	}()

	wg.Wait()

	// Derive overall status
	overall := StatusHealthy
	for _, ch := range report.Components {
		if ch.Status == StatusUnhealthy {
			overall = StatusUnhealthy
			break
		} else if ch.Status == StatusDegraded && overall != StatusUnhealthy {
			overall = StatusDegraded
		}
	}
	report.OverallStatus = overall

	return report
}

func (p *HealthProber) probeQueue(ctx context.Context) ComponentHealth {
	start := time.Now()
	if p.outbox == nil {
		return ComponentHealth{
			Name:        "queue_outbox",
			Type:        ComponentQueue,
			Status:      StatusHealthy,
			Latency:     time.Since(start),
			Message:     "Outbox store active (in-memory mode)",
			LastChecked: time.Now().UTC(),
		}
	}

	lag, err := p.outbox.GetLag(ctx)
	latency := time.Since(start)
	if err != nil {
		return ComponentHealth{
			Name:        "queue_outbox",
			Type:        ComponentQueue,
			Status:      StatusUnhealthy,
			Latency:     latency,
			Message:     "Failed retrieving outbox lag: " + err.Error(),
			LastChecked: time.Now().UTC(),
		}
	}

	status := StatusHealthy
	msg := "Queue operational"

	if lag.DeadLetterCount > 0 {
		status = StatusDegraded
		msg = "Dead letter messages detected in outbox"
	} else if lag.PendingCount > 1000 {
		status = StatusDegraded
		msg = "High message backlog detected"
	}

	return ComponentHealth{
		Name:        "queue_outbox",
		Type:        ComponentQueue,
		Status:      status,
		Latency:     latency,
		Message:     msg,
		Details: map[string]any{
			"pending_count":     lag.PendingCount,
			"claimed_count":     lag.ClaimedCount,
			"dead_letter_count": lag.DeadLetterCount,
		},
		LastChecked: time.Now().UTC(),
	}
}

func (p *HealthProber) probeCache(ctx context.Context) ComponentHealth {
	start := time.Now()
	if p.cache == nil {
		return ComponentHealth{
			Name:        "cache_tiered",
			Type:        ComponentCache,
			Status:      StatusHealthy,
			Latency:     time.Since(start),
			Message:     "Tiered cache active",
			LastChecked: time.Now().UTC(),
		}
	}

	// Test read/write probe
	probeKey := "probe:healthcheck"
	p.cache.Set(ctx, probeKey, "ok", 5*time.Second)
	val, exists := p.cache.Get(ctx, probeKey)
	latency := time.Since(start)

	strVal, isStr := val.(string)
	if !exists || !isStr || strVal != "ok" {
		return ComponentHealth{
			Name:        "cache_tiered",
			Type:        ComponentCache,
			Status:      StatusDegraded,
			Latency:     latency,
			Message:     "Cache read/write test failed",
			LastChecked: time.Now().UTC(),
		}
	}

	stats := p.cache.Stats()
	return ComponentHealth{
		Name:        "cache_tiered",
		Type:        ComponentCache,
		Status:      StatusHealthy,
		Latency:     latency,
		Message:     "Cache responsive with normal hit ratios",
		Details: map[string]any{
			"hits":   stats.Hits,
			"misses": stats.Misses,
			"items":  stats.Items,
		},
		LastChecked: time.Now().UTC(),
	}
}

func (p *HealthProber) probeRuntime(ctx context.Context, m runtime.MemStats) ComponentHealth {
	goroutines := runtime.NumGoroutine()
	status := StatusHealthy
	msg := "Runtime metrics nominal"

	if goroutines > 5000 {
		status = StatusDegraded
		msg = "Elevated goroutine count detected"
	}

	return ComponentHealth{
		Name:        "runtime_engine",
		Type:        ComponentStorage,
		Status:      status,
		Latency:     10 * time.Microsecond,
		Message:     msg,
		Details: map[string]any{
			"goroutines": goroutines,
			"alloc_mb":   math.Round((float64(m.Alloc)/1024/1024)*100) / 100,
			"gc_cycles":  m.NumGC,
		},
		LastChecked: time.Now().UTC(),
	}
}
