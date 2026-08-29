package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AuditAction defines the state mutation executed on enterprise configs.
type AuditAction string

const (
	ActionCreate AuditAction = "CREATE"
	ActionUpdate AuditAction = "UPDATE"
	ActionDelete AuditAction = "DELETE"
)

// EnterpriseAuditEvent details an immutable compliance event for SIEM streaming.
type EnterpriseAuditEvent struct {
	EventID     uuid.UUID   `json:"event_id"`
	WorkspaceID uuid.UUID   `json:"workspace_id"`
	ActorID     string      `json:"actor_id"`
	ActorEmail  string      `json:"actor_email"`
	ClientIP    string      `json:"client_ip"`
	Action      AuditAction `json:"action"`
	ResourceType string     `json:"resource_type"` // CodeReviewSettings, BYOK, Rules
	ResourceID  string      `json:"resource_id"`
	OldValue    string      `json:"old_value,omitempty"`
	NewValue    string      `json:"new_value,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
	PrevHash    string      `json:"prev_hash"`
	Hash        string      `json:"hash"`
}

// SIEMAuditStreamer generates compliant CEF and RFC-5424 audit logs with tamper-evident chaining.
type SIEMAuditStreamer struct {
	mu       sync.Mutex
	lastHash string
}

func NewSIEMAuditStreamer() *SIEMAuditStreamer {
	return &SIEMAuditStreamer{
		lastHash: "0000000000000000000000000000000000000000000000000000000000000000",
	}
}

// RecordEvent appends a tamper-evident audit record into the hash chain.
func (s *SIEMAuditStreamer) RecordEvent(workspaceID uuid.UUID, actorID, actorEmail, clientIP string, action AuditAction, resourceType, resourceID, oldVal, newVal string) EnterpriseAuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	eventID := uuid.New()

	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		eventID, workspaceID, actorID, actorEmail, clientIP, action, resourceType, resourceID, oldVal, newVal, now.Format(time.RFC3339Nano))

	hasher := sha256.New()
	hasher.Write([]byte(s.lastHash + "|" + payload))
	currentHash := hex.EncodeToString(hasher.Sum(nil))

	event := EnterpriseAuditEvent{
		EventID:      eventID,
		WorkspaceID:  workspaceID,
		ActorID:      actorID,
		ActorEmail:   actorEmail,
		ClientIP:     clientIP,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		OldValue:     oldVal,
		NewValue:     newVal,
		Timestamp:    now,
		PrevHash:     s.lastHash,
		Hash:         currentHash,
	}

	s.lastHash = currentHash
	return event
}

// FormatCEF formats an audit event to HP/ArcSight Common Event Format (CEF:0).
func FormatCEF(event EnterpriseAuditEvent) string {
	// CEF:Version|Device Vendor|Device Product|Device Version|Device Event Class ID|Name|Severity|[Extension]
	name := fmt.Sprintf("%s %s", event.Action, event.ResourceType)
	return fmt.Sprintf("CEF:0|Scandrix|EnterprisePlatform|2.0|%s|%s|5|src=%s suser=%s cs1Label=WorkspaceID cs1=%s cs2Label=ResourceID cs2=%s msg=%s",
		event.Action, name, event.ClientIP, event.ActorEmail, event.WorkspaceID, event.ResourceID, sanitizeCEF(fmt.Sprintf("From %s to %s", event.OldValue, event.NewValue)))
}

// FormatRFC5424 formats an audit event to standard IETF Syslog RFC-5424.
func FormatRFC5424(event EnterpriseAuditEvent) string {
	// <PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID [STRUCTURED-DATA] MSG
	pri := 134 // Facility 16 (local0) + Severity 6 (info) -> 16*8 + 6 = 134
	return fmt.Sprintf("<%d>1 %s scandrix-api scandrix - %s [audit@52467 eventId=\"%s\" actor=\"%s\" clientIP=\"%s\" hash=\"%s\"] %s %s on %s",
		pri, event.Timestamp.Format(time.RFC3339Nano), event.Action, event.EventID, event.ActorEmail, event.ClientIP, event.Hash, event.Action, event.ResourceType, event.ResourceID)
}

func sanitizeCEF(input string) string {
	r := strings.ReplaceAll(input, "\\", "\\\\")
	return strings.ReplaceAll(r, "=", "\\=")
}
