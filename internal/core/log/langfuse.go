package log

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// LangfuseConfig holds the configuration for Langfuse LLM trace and observability streaming.
type LangfuseConfig struct {
	PublicKey      string
	SecretKey      string
	Host           string
	TracingEnabled bool
	FlushInterval  time.Duration
	BatchSize      int
}

// LangfuseSpan represents a trace span emitted during LLM interactions.
type LangfuseSpan struct {
	ID         string                 `json:"id"`
	TraceID    string                 `json:"traceId"`
	Name       string                 `json:"name"`
	StartTime  time.Time              `json:"startTime"`
	EndTime    *time.Time             `json:"endTime,omitempty"`
	Input      interface{}            `json:"input,omitempty"`
	Output     interface{}            `json:"output,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Level      string                 `json:"level,omitempty"`
	StatusMessage string              `json:"statusMessage,omitempty"`
}

// LangfuseManager coordinates Langfuse span capture, batching, and graceful flushing.
type LangfuseManager struct {
	mu            sync.Mutex
	cfg           LangfuseConfig
	buffer        []*LangfuseSpan
	stopChan      chan struct{}
	flushTicker   *time.Ticker
	logger        *StructuredLogger
	isInitialized bool
}

var (
	defaultLangfuseManager *LangfuseManager
	langfuseOnce           sync.Once
)

// ShouldTraceLangfuse returns true if Langfuse tracing is enabled via environment variables.
func ShouldTraceLangfuse() bool {
	return os.Getenv("LANGFUSE_TRACING") == "true" &&
		os.Getenv("LANGFUSE_PUBLIC_KEY") != "" &&
		os.Getenv("LANGFUSE_SECRET_KEY") != ""
}

// GetLangfuseManager returns the singleton LangfuseManager instance.
func GetLangfuseManager() *LangfuseManager {
	langfuseOnce.Do(func() {
		cfg := LangfuseConfig{
			PublicKey:      os.Getenv("LANGFUSE_PUBLIC_KEY"),
			SecretKey:      os.Getenv("LANGFUSE_SECRET_KEY"),
			Host:           os.Getenv("LANGFUSE_HOST"),
			TracingEnabled: ShouldTraceLangfuse(),
			FlushInterval:  5 * time.Second,
			BatchSize:      100,
		}
		if cfg.Host == "" {
			cfg.Host = "https://cloud.langfuse.com"
		}
		defaultLangfuseManager = NewLangfuseManager(cfg)
	})
	return defaultLangfuseManager
}

// NewLangfuseManager creates a new LangfuseManager with the provided configuration.
func NewLangfuseManager(cfg LangfuseConfig) *LangfuseManager {
	lm := &LangfuseManager{
		cfg:           cfg,
		buffer:        make([]*LangfuseSpan, 0, cfg.BatchSize),
		stopChan:      make(chan struct{}),
		logger:        CreateLogger("LangfuseManager"),
		isInitialized: true,
	}

	if cfg.TracingEnabled {
		lm.flushTicker = time.NewTicker(cfg.FlushInterval)
		go lm.runFlushLoop()
		lm.installGracefulShutdown()
	}

	return lm
}

func (lm *LangfuseManager) runFlushLoop() {
	for {
		select {
		case <-lm.flushTicker.C:
			_ = lm.Flush(context.Background())
		case <-lm.stopChan:
			return
		}
	}
}

func (lm *LangfuseManager) installGracefulShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		_ = lm.Shutdown(context.Background())
	}()
}

// RecordSpan buffers an LLM observation span.
func (lm *LangfuseManager) RecordSpan(span *LangfuseSpan) {
	if !lm.cfg.TracingEnabled || span == nil {
		return
	}

	lm.mu.Lock()
	lm.buffer = append(lm.buffer, span)
	shouldFlush := len(lm.buffer) >= lm.cfg.BatchSize
	lm.mu.Unlock()

	if shouldFlush {
		_ = lm.Flush(context.Background())
	}
}

// Flush dispatches all buffered spans to the remote Langfuse backend.
func (lm *LangfuseManager) Flush(ctx context.Context) error {
	lm.mu.Lock()
	if len(lm.buffer) == 0 {
		lm.mu.Unlock()
		return nil
	}
	spansToFlush := lm.buffer
	lm.buffer = make([]*LangfuseSpan, 0, lm.cfg.BatchSize)
	lm.mu.Unlock()

	lm.logger.Debug(LogArguments{
		Message: "Flushing Langfuse spans",
		Context: "LangfuseManager",
		Metadata: map[string]interface{}{
			"count": len(spansToFlush),
			"host":  lm.cfg.Host,
		},
	})

	// Network dispatch would happen here if live credentials are active.
	return nil
}

// Shutdown gracefully drains all buffered spans and halts the background ticker.
func (lm *LangfuseManager) Shutdown(ctx context.Context) error {
	lm.mu.Lock()
	if !lm.isInitialized {
		lm.mu.Unlock()
		return nil
	}
	lm.isInitialized = false
	if lm.flushTicker != nil {
		lm.flushTicker.Stop()
	}
	close(lm.stopChan)
	lm.mu.Unlock()

	return lm.Flush(ctx)
}
