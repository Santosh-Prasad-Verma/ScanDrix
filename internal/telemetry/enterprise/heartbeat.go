package enterprise

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// HeartbeatRunner sends periodic health signals to BetterStack / external uptime monitors.
type HeartbeatRunner struct {
	mu           sync.RWMutex
	cfg          HeartbeatConfig
	httpClient   *http.Client
	stopChan     chan struct{}
	running      bool
	lastPing     *time.Time
	successCount int64
	failCount    int64
}

// NewHeartbeatRunner initializes the heartbeat daemon.
func NewHeartbeatRunner(cfg HeartbeatConfig) *HeartbeatRunner {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &HeartbeatRunner{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		stopChan: make(chan struct{}),
	}
}

// Ping sends an immediate heartbeat check.
func (h *HeartbeatRunner) Ping(ctx context.Context) error {
	if h.cfg.URL == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, h.cfg.URL, nil)
	if err != nil {
		h.recordFailure()
		return err
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.recordFailure()
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.recordFailure()
		return err
	}

	h.recordSuccess()
	return nil
}

// Start launches the background heartbeat scheduler.
func (h *HeartbeatRunner) Start() {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	go func() {
		ticker := time.NewTicker(h.cfg.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-h.stopChan:
				return
			case <-ticker.C:
				_ = h.Ping(context.Background())
			}
		}
	}()
}

// Stop halts the heartbeat scheduler.
func (h *HeartbeatRunner) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	close(h.stopChan)
	h.running = false
}

// Stats returns the total pings and last timestamp.
func (h *HeartbeatRunner) Stats() (int64, int64, *time.Time) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.successCount, h.failCount, h.lastPing
}

func (h *HeartbeatRunner) recordSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now().UTC()
	h.lastPing = &now
	h.successCount++
}

func (h *HeartbeatRunner) recordFailure() {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now().UTC()
	h.lastPing = &now
	h.failCount++
}
