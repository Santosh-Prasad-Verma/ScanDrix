package enterprise

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

// Profiler gathers and submits continuous runtime profiling data to Pyroscope / Grafana Phlare.
type Profiler struct {
	mu         sync.Mutex
	cfg        ProfilerConfig
	httpClient *http.Client
	stopChan   chan struct{}
	running    bool
}

// NewProfiler initializes the continuous profiler.
func NewProfiler(cfg ProfilerConfig) *Profiler {
	if cfg.UploadPeriod <= 0 {
		cfg.UploadPeriod = 15 * time.Second
	}
	return &Profiler{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		stopChan: make(chan struct{}),
	}
}

// Start begins the background profiling loop.
func (p *Profiler) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.mu.Unlock()

	go p.loop()
}

// Stop halts background profiling.
func (p *Profiler) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	close(p.stopChan)
	p.running = false
}

func (p *Profiler) loop() {
	ticker := time.NewTicker(p.cfg.UploadPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			_ = p.captureAndUpload(context.Background())
		}
	}
}

// CaptureSnapshot generates runtime memory and goroutine profiles for inspection.
func (p *Profiler) CaptureSnapshot() (map[string]any, []byte, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := map[string]any{
		"goroutines":    runtime.NumGoroutine(),
		"heap_alloc":    m.HeapAlloc,
		"heap_inuse":    m.HeapInuse,
		"total_alloc":   m.TotalAlloc,
		"num_gc":        m.NumGC,
		"service":       p.cfg.ServiceName,
		"environment":   p.cfg.Environment,
		"captured_at":   time.Now().UTC(),
	}

	var buf bytes.Buffer
	if err := pprof.WriteHeapProfile(&buf); err != nil {
		return nil, nil, err
	}

	return stats, buf.Bytes(), nil
}

func (p *Profiler) captureAndUpload(ctx context.Context) error {
	if p.cfg.ServerAddress == "" {
		return nil
	}

	_, heapProfile, err := p.CaptureSnapshot()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/ingest?name=%s.heap&from=%d&until=%d",
		p.cfg.ServerAddress,
		p.cfg.ServiceName,
		time.Now().Add(-p.cfg.UploadPeriod).Unix(),
		time.Now().Unix(),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(heapProfile))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	if p.cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.AuthToken)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
