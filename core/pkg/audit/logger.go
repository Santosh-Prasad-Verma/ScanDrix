package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventInput represents a single security audit event to log.
type EventInput struct {
	TenantID     uuid.UUID      `json:"tenant_id"`
	UserID       *uuid.UUID     `json:"user_id,omitempty"`
	ActorType    string         `json:"actor_type"` // 'USER', 'API_KEY', 'GITHUB_BOT', 'TEMPORAL_SYSTEM'
	Action       string         `json:"action"`     // 'SCAN_LAUNCHED', 'TARGET_VERIFIED', 'PATCH_APPLIED'
	ResourceType string         `json:"resource_type"`
	ResourceID   uuid.UUID      `json:"resource_id"`
	IPAddress    *string        `json:"ip_address,omitempty"`
	UserAgent    *string        `json:"user_agent,omitempty"`
	Payload      map[string]any `json:"payload"`
}

// Logger provides high-throughput, non-blocking partitioned audit logging.
type Logger struct {
	pool       *pgxpool.Pool
	queue      chan EventInput
	wg         sync.WaitGroup
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewLogger initializes the asynchronous audit logging pipeline.
func NewLogger(pool *pgxpool.Pool, bufferSize int) *Logger {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	ctx, cancel := context.WithCancel(context.Background())
	l := &Logger{
		pool:       pool,
		queue:      make(chan EventInput, bufferSize),
		ctx:        ctx,
		cancelFunc: cancel,
	}

	l.wg.Add(1)
	go l.workerLoop()

	return l
}

// Log queues an audit event without blocking the caller.
func (l *Logger) Log(evt EventInput) {
	select {
	case l.queue <- evt:
	default:
		log.Printf("⚠️ [AuditLogger] Queue full, dropped event: %s on %s", evt.Action, evt.ResourceID)
	}
}

// LogSync writes an audit event synchronously (useful for critical compliance barriers).
func (l *Logger) LogSync(ctx context.Context, evt EventInput) error {
	payloadJSON, err := json.Marshal(evt.Payload)
	if err != nil {
		payloadJSON = []byte("{}")
	}

	query := `
		INSERT INTO audit_events (
			id, tenant_id, user_id, actor_type, action, resource_type, resource_id, ip_address, user_agent, payload, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, clock_timestamp()
		);
	`
	_, err = l.pool.Exec(ctx, query,
		uuid.New(), evt.TenantID, evt.UserID, evt.ActorType, evt.Action,
		evt.ResourceType, evt.ResourceID, evt.IPAddress, evt.UserAgent, payloadJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to persist audit event: %w", err)
	}
	return nil
}

func (l *Logger) workerLoop() {
	defer l.wg.Done()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var batch []EventInput

	for {
		select {
		case <-l.ctx.Done():
			// Drain remaining events
			l.flushBatch(batch)
			for {
				select {
				case evt := <-l.queue:
					batch = append(batch, evt)
					if len(batch) >= 100 {
						l.flushBatch(batch)
						batch = batch[:0]
					}
				default:
					l.flushBatch(batch)
					return
				}
			}

		case evt := <-l.queue:
			batch = append(batch, evt)
			if len(batch) >= 50 {
				l.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				l.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (l *Logger) flushBatch(batch []EventInput) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, evt := range batch {
		_ = l.LogSync(ctx, evt)
	}
}

// Close gracefully flushes all queued audit logs and stops the worker.
func (l *Logger) Close() {
	l.cancelFunc()
	l.wg.Wait()
}
