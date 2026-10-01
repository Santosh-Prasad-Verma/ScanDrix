package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AuditLogFilter allows multi-tenant, categorized querying of audit trails.
type AuditLogFilter struct {
	OrganizationID *uuid.UUID          `json:"organization_id,omitempty"`
	WorkspaceID    *uuid.UUID          `json:"workspace_id,omitempty"`
	Category       *AuditEventCategory `json:"category,omitempty"`
	Action         string              `json:"action,omitempty"`
	ActorID        string              `json:"actor_id,omitempty"`
	ActorEmail     string              `json:"actor_email,omitempty"`
	StartTime      *time.Time          `json:"start_time,omitempty"`
	EndTime        *time.Time          `json:"end_time,omitempty"`
	SearchQuery    string              `json:"search_query,omitempty"`
	Limit          int                 `json:"limit,omitempty"`
	Offset         int                 `json:"offset,omitempty"`
}

// IAuditLogRepository defines persistence and tamper-verification contracts.
type IAuditLogRepository interface {
	AppendLog(ctx context.Context, event EnterpriseLogEvent) (*EnterpriseLogEvent, error)
	BatchAppend(ctx context.Context, events []EnterpriseLogEvent) ([]EnterpriseLogEvent, error)
	QueryLogs(ctx context.Context, filter AuditLogFilter) ([]EnterpriseLogEvent, int64, error)
	GetLastLog(ctx context.Context, orgID uuid.UUID) (*EnterpriseLogEvent, error)
	VerifyChainIntegrity(ctx context.Context, orgID uuid.UUID) (bool, *uuid.UUID, error)
}

// MemoryAuditRepository provides an in-memory, thread-safe implementation with hash chaining.
type MemoryAuditRepository struct {
	mu         sync.RWMutex
	logs       []EnterpriseLogEvent
	lastHashes map[uuid.UUID]string
	hmacSecret []byte
}

// NewMemoryAuditRepository initializes a memory audit repository.
//
// SECURITY: the HMAC secret is required. It previously defaulted to a
// hardcoded string committed to the repository, which meant the tamper-evident
// hash chain was verifiable by anyone and forgeable by anyone
// (AUDIT_REMEDIATION.md F-14). A nil return means the caller misconfigured the
// audit store, which must be treated as a hard failure rather than a weaker
// chain.
func NewMemoryAuditRepository(hmacSecret string) *MemoryAuditRepository {
	if strings.TrimSpace(hmacSecret) == "" {
		return nil
	}
	return &MemoryAuditRepository{
		logs:       make([]EnterpriseLogEvent, 0),
		lastHashes: make(map[uuid.UUID]string),
		hmacSecret: []byte(hmacSecret),
	}
}

// AppendLog appends a new event and computes its cryptographic hash in the org's chain.
func (r *MemoryAuditRepository) AppendLog(ctx context.Context, event EnterpriseLogEvent) (*EnterpriseLogEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	orgID := event.Target.OrganizationID
	prevHash, exists := r.lastHashes[orgID]
	if !exists {
		prevHash = strings.Repeat("0", 64)
	}

	event.PrevHash = prevHash
	event.Hash = r.computeEventHash(event)

	r.logs = append(r.logs, event)
	r.lastHashes[orgID] = event.Hash

	return &event, nil
}

// BatchAppend commits multiple audit records in transaction order.
func (r *MemoryAuditRepository) BatchAppend(ctx context.Context, events []EnterpriseLogEvent) ([]EnterpriseLogEvent, error) {
	var results []EnterpriseLogEvent
	for _, ev := range events {
		res, err := r.AppendLog(ctx, ev)
		if err != nil {
			return results, err
		}
		results = append(results, *res)
	}
	return results, nil
}

// QueryLogs filters audit records according to enterprise criteria with pagination.
func (r *MemoryAuditRepository) QueryLogs(ctx context.Context, filter AuditLogFilter) ([]EnterpriseLogEvent, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []EnterpriseLogEvent

	for _, ev := range r.logs {
		// Org filter
		if filter.OrganizationID != nil && ev.Target.OrganizationID != *filter.OrganizationID {
			continue
		}
		// Workspace filter
		if filter.WorkspaceID != nil && ev.Target.WorkspaceID != *filter.WorkspaceID {
			continue
		}
		// Category filter
		if filter.Category != nil && ev.Category != *filter.Category {
			continue
		}
		// Action filter
		if filter.Action != "" && !strings.EqualFold(ev.Action, filter.Action) {
			continue
		}
		// Actor ID filter
		if filter.ActorID != "" && ev.Actor.UserID != filter.ActorID {
			continue
		}
		// Actor Email filter
		if filter.ActorEmail != "" && !strings.EqualFold(ev.Actor.Email, filter.ActorEmail) {
			continue
		}
		// Start time
		if filter.StartTime != nil && ev.Timestamp.Before(*filter.StartTime) {
			continue
		}
		// End time
		if filter.EndTime != nil && ev.Timestamp.After(*filter.EndTime) {
			continue
		}
		// Search query in metadata or changes
		if filter.SearchQuery != "" {
			q := strings.ToLower(filter.SearchQuery)
			found := strings.Contains(strings.ToLower(ev.Action), q) ||
				strings.Contains(strings.ToLower(ev.Actor.Email), q) ||
				strings.Contains(strings.ToLower(ev.Target.TargetEntityID), q)
			if !found {
				continue
			}
		}

		matched = append(matched, ev)
	}

	total := int64(len(matched))

	// Sort descending by timestamp (newest first)
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].Timestamp.After(matched[j].Timestamp)
	})

	// Pagination
	offset := filter.Offset
	if offset > len(matched) {
		return []EnterpriseLogEvent{}, total, nil
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}

	return matched[offset:end], total, nil
}

// GetLastLog returns the most recent audit record for an organization.
func (r *MemoryAuditRepository) GetLastLog(ctx context.Context, orgID uuid.UUID) (*EnterpriseLogEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := len(r.logs) - 1; i >= 0; i-- {
		if r.logs[i].Target.OrganizationID == orgID {
			return &r.logs[i], nil
		}
	}
	return nil, nil
}

// VerifyChainIntegrity verifies that all events in an organization's hash chain are untampered.
// Returns (true, nil, nil) if pristine, or (false, &corruptedEventID, nil) if modified.
func (r *MemoryAuditRepository) VerifyChainIntegrity(ctx context.Context, orgID uuid.UUID) (bool, *uuid.UUID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	expectedPrev := strings.Repeat("0", 64)

	for _, ev := range r.logs {
		if ev.Target.OrganizationID != orgID {
			continue
		}

		// Check link to previous hash
		if ev.PrevHash != expectedPrev {
			corruptID := ev.ID
			return false, &corruptID, fmt.Errorf("hash chain broken at event %s: prev_hash mismatch", ev.ID)
		}

		// Recompute hash
		computed := r.computeEventHash(ev)
		if computed != ev.Hash {
			corruptID := ev.ID
			return false, &corruptID, fmt.Errorf("tamper detected at event %s: hash verification failed", ev.ID)
		}

		expectedPrev = ev.Hash
	}

	return true, nil, nil
}

func (r *MemoryAuditRepository) computeEventHash(ev EnterpriseLogEvent) string {
	changesJSON, _ := json.Marshal(ev.Changes)
	metaJSON, _ := json.Marshal(ev.Metadata)

	canonical := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		ev.ID.String(),
		ev.Target.OrganizationID.String(),
		ev.Category,
		ev.Action,
		ev.Actor.UserID,
		ev.Actor.Email,
		ev.Target.TargetEntityID,
		string(changesJSON),
		string(metaJSON),
		ev.Timestamp.Format(time.RFC3339Nano),
		ev.PrevHash,
	)

	mac := hmac.New(sha256.New, r.hmacSecret)
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}
