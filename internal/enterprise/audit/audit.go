package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ActionType represents an auditable security event in Scandrix.
type ActionType string

const (
	ActionUserLogin           ActionType = "user.login"
	ActionUserLogout          ActionType = "user.logout"
	ActionReviewTriggered     ActionType = "review.triggered"
	ActionReviewDismissed     ActionType = "review.dismissed"
	ActionRuleCreated         ActionType = "rule.created"
	ActionRuleUpdated         ActionType = "rule.updated"
	ActionRuleDeleted         ActionType = "rule.deleted"
	ActionAPIKeyCreated       ActionType = "api_key.created"
	ActionAPIKeyRevoked       ActionType = "api_key.revoked"
	ActionSCIMUserProvisioned ActionType = "scim.user.provisioned"
)

// AuditEvent models a single immutable audit log record.
type AuditEvent struct {
	ID          uuid.UUID      `json:"id"`
	WorkspaceID uuid.UUID      `json:"workspace_id"`
	ActorID     string         `json:"actor_id"`
	ActorEmail  string         `json:"actor_email"`
	IPAddress   string         `json:"ip_address"`
	Action      ActionType     `json:"action"`
	TargetType  string         `json:"target_type"`
	TargetID    string         `json:"target_id"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
}

// AuditLogger manages buffered, asynchronous compliance audit events.
type AuditLogger struct {
	mu     sync.Mutex
	buffer []AuditEvent
	sink   io.Writer
}

// NewAuditLogger initializes the compliance audit logger.
func NewAuditLogger(sink io.Writer) *AuditLogger {
	return &AuditLogger{
		buffer: make([]AuditEvent, 0, 100),
		sink:   sink,
	}
}

// Record emits an audit event into the buffered sink.
func (l *AuditLogger) Record(ctx context.Context, event AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	l.buffer = append(l.buffer, event)

	if l.sink != nil {
		line, err := json.Marshal(event)
		if err == nil {
			_, _ = fmt.Fprintln(l.sink, string(line))
		}
	}

	return nil
}

// ToCEF formats the audit event into standard Common Event Format (CEF) for SIEM ingest.
func (e *AuditEvent) ToCEF() string {
	return fmt.Sprintf("CEF:0|Scandrix|CoreEngine|1.0|%s|%s|5|src=%s suser=%s cs1Label=WorkspaceID cs1=%s cs2Label=TargetID cs2=%s",
		e.Action, e.Action, e.IPAddress, e.ActorEmail, e.WorkspaceID, e.TargetID,
	)
}

// Flush returns the currently buffered events and clears internal queue.
func (l *AuditLogger) Flush() []AuditEvent {
	l.mu.Lock()
	defer l.mu.Unlock()

	flushed := l.buffer
	l.buffer = make([]AuditEvent, 0, 100)
	return flushed
}
