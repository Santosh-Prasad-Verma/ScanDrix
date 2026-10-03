package audit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// IAuditEventHandler processes domain-specific audit events.
type IAuditEventHandler interface {
	Category() AuditEventCategory
	HandleEvent(ctx context.Context, event EnterpriseLogEvent) error
}

// AuditListenerConfig configures asynchronous event queue buffering and worker pool sizing.
type AuditListenerConfig struct {
	QueueCapacity int           `json:"queue_capacity"`
	WorkerCount   int           `json:"worker_count"`
	MaxRetries    int           `json:"max_retries"`
	RetryDelay    time.Duration `json:"retry_delay"`
	FlushTimeout  time.Duration `json:"flush_timeout"`
}

// DefaultAuditListenerConfig returns production standard event listener settings.
func DefaultAuditListenerConfig() AuditListenerConfig {
	return AuditListenerConfig{
		QueueCapacity: 2000,
		WorkerCount:   4,
		MaxRetries:    3,
		RetryDelay:    50 * time.Millisecond,
		FlushTimeout:  5 * time.Second,
	}
}

// AuditLogListener receives, routes, persists, and exports enterprise audit events.
type AuditLogListener struct {
	config    AuditListenerConfig
	repo      IAuditLogRepository
	siem      *SIEMAuditStreamer
	handlers  map[AuditEventCategory]IAuditEventHandler
	eventChan chan EnterpriseLogEvent
	dlq       []EnterpriseLogEvent
	dlqMu     sync.Mutex
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	closed    bool
	closeMu   sync.Mutex
}

// NewAuditLogListener constructs and starts an asynchronous audit event listener.
func NewAuditLogListener(
	repo IAuditLogRepository,
	siem *SIEMAuditStreamer,
	cfg ...AuditListenerConfig,
) *AuditLogListener {
	c := DefaultAuditListenerConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}

	ctx, cancel := context.WithCancel(context.Background())

	l := &AuditLogListener{
		config:    c,
		repo:      repo,
		siem:      siem,
		handlers:  make(map[AuditEventCategory]IAuditEventHandler),
		eventChan: make(chan EnterpriseLogEvent, c.QueueCapacity),
		dlq:       make([]EnterpriseLogEvent, 0),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start worker pool
	for i := 0; i < c.WorkerCount; i++ {
		l.wg.Add(1)
		go l.workerLoop(i)
	}

	return l
}

// RegisterHandler registers a dedicated domain handler for a given audit category.
func (l *AuditLogListener) RegisterHandler(handler IAuditEventHandler) {
	l.handlers[handler.Category()] = handler
}

// Emit enqueues an audit event for asynchronous processing.
// Returns an error if the queue is full or the listener is shut down.
func (l *AuditLogListener) Emit(event EnterpriseLogEvent) error {
	l.closeMu.Lock()
	if l.closed {
		l.closeMu.Unlock()
		return fmt.Errorf("audit log listener is closed")
	}
	l.closeMu.Unlock()

	select {
	case l.eventChan <- event:
		return nil
	default:
		// Queue full: log error and save directly to DLQ
		l.recordDLQ(event, fmt.Errorf("audit queue capacity (%d) reached", l.config.QueueCapacity))
		return fmt.Errorf("audit queue full: event dropped to DLQ")
	}
}

func (l *AuditLogListener) workerLoop(workerID int) {
	defer l.wg.Done()

	for {
		select {
		case <-l.ctx.Done():
			// Drain remaining events in channel before exiting
			for {
				select {
				case ev := <-l.eventChan:
					l.processEvent(ev)
				default:
					return
				}
			}
		case ev := <-l.eventChan:
			l.processEvent(ev)
		}
	}
}

func (l *AuditLogListener) processEvent(ev EnterpriseLogEvent) {
	// 1. Invoke category-specific domain handler if registered
	if handler, exists := l.handlers[ev.Category]; exists {
		if err := handler.HandleEvent(l.ctx, ev); err != nil {
			slog.Error("Audit handler failed", "category", ev.Category, "error", err)
		}
	}

	// 2. Persist to repository with retries
	var lastErr error
	for attempt := 0; attempt <= l.config.MaxRetries; attempt++ {
		if l.repo != nil {
			saved, err := l.repo.AppendLog(l.ctx, ev)
			if err == nil {
				ev = *saved
				lastErr = nil
				break
			}
			lastErr = err
			time.Sleep(l.config.RetryDelay * time.Duration(1<<attempt))
		} else {
			break
		}
	}

	if lastErr != nil {
		l.recordDLQ(ev, lastErr)
		return
	}

	// 3. Forward to SIEM streamer if configured
	if l.siem != nil {
		action := ActionUpdate
		if strings.EqualFold(ev.Action, "CREATE") {
			action = ActionCreate
		} else if strings.EqualFold(ev.Action, "DELETE") || strings.EqualFold(ev.Action, "REVOKE") {
			action = ActionDelete
		}

		l.siem.RecordEvent(
			ev.Target.WorkspaceID,
			ev.Actor.UserID,
			ev.Actor.Email,
			ev.Actor.ClientIP,
			action,
			string(ev.Category),
			ev.Target.TargetEntityID,
			fmt.Sprintf("%v", ev.Changes),
			fmt.Sprintf("%v", ev.Metadata),
		)
	}
}

func (l *AuditLogListener) recordDLQ(ev EnterpriseLogEvent, err error) {
	l.dlqMu.Lock()
	defer l.dlqMu.Unlock()
	slog.Error("Audit event moved to DLQ", "event_id", ev.ID, "error", err)
	l.dlq = append(l.dlq, ev)
}

// GetDLQ returns events that failed to process.
func (l *AuditLogListener) GetDLQ() []EnterpriseLogEvent {
	l.dlqMu.Lock()
	defer l.dlqMu.Unlock()
	res := make([]EnterpriseLogEvent, len(l.dlq))
	copy(res, l.dlq)
	return res
}

// Close drains the queue and waits for active workers to complete within the flush timeout.
func (l *AuditLogListener) Close() error {
	l.closeMu.Lock()
	if l.closed {
		l.closeMu.Unlock()
		return nil
	}
	l.closed = true
	l.closeMu.Unlock()

	l.cancel()

	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(l.config.FlushTimeout):
		return fmt.Errorf("timed out waiting for audit workers to flush")
	}
}
