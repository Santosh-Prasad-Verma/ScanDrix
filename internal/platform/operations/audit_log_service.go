// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package operations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// AuditAction represents an audited VCS or security operation.
type AuditAction string

const (
	ActionReviewDispatched    AuditAction = "REVIEW_DISPATCHED"
	ActionReviewCompleted     AuditAction = "REVIEW_COMPLETED"
	ActionCommentCreated      AuditAction = "COMMENT_CREATED"
	ActionReactionAdded       AuditAction = "REACTION_ADDED"
	ActionQualityGatePassed   AuditAction = "QUALITY_GATE_PASSED"
	ActionQualityGateFailed   AuditAction = "QUALITY_GATE_FAILED"
	ActionSecretDetected      AuditAction = "SECRET_DETECTED"
	ActionBranchPolicyChecked AuditAction = "BRANCH_POLICY_CHECKED"
	ActionWebhookReceived     AuditAction = "WEBHOOK_RECEIVED"
	ActionTokenRotated        AuditAction = "TOKEN_ROTATED"
	ActionBypassAttempted     AuditAction = "BYPASS_ATTEMPTED"
)

// AuditSeverity represents the risk level of an audit record.
type AuditSeverity string

const (
	SeverityInfo     AuditSeverity = "INFO"
	SeverityWarning  AuditSeverity = "WARNING"
	SeverityCritical AuditSeverity = "CRITICAL"
)

// AuditRecord models an immutable, cryptographically chained audit log entry.
type AuditRecord struct {
	ID             uuid.UUID         `json:"id"`
	OrganizationID string            `json:"organizationId"`
	Repository     string            `json:"repository"`
	Provider       models.SCMProvider `json:"provider"`
	Actor          string            `json:"actor"`
	Action         AuditAction       `json:"action"`
	Severity       AuditSeverity     `json:"severity"`
	PRNumber       int               `json:"prNumber,omitempty"`
	CommitSHA      string            `json:"commitSha,omitempty"`
	Details        map[string]any    `json:"details,omitempty"`
	Timestamp      time.Time         `json:"timestamp"`
	PrevHash       string            `json:"prevHash"`
	CurrentHash    string            `json:"currentHash"`
}

// AuditLogFilter specifies query criteria for searching audit records.
type AuditLogFilter struct {
	OrganizationID string             `json:"organizationId,omitempty"`
	Repository     string             `json:"repository,omitempty"`
	Provider       models.SCMProvider `json:"provider,omitempty"`
	Action         AuditAction        `json:"action,omitempty"`
	Severity       AuditSeverity      `json:"severity,omitempty"`
	Actor          string             `json:"actor,omitempty"`
	FromTime       *time.Time         `json:"fromTime,omitempty"`
	ToTime         *time.Time         `json:"toTime,omitempty"`
	Limit          int                `json:"limit,omitempty"`
	Offset         int                `json:"offset,omitempty"`
}

// AuditLogService coordinates tamper-evident audit logging and verification.
type AuditLogService struct {
	mu           sync.RWMutex
	records      []*AuditRecord
	lastHash     string
	genesisHash  string
}

// NewAuditLogService initializes a new tamper-evident audit log service.
func NewAuditLogService() *AuditLogService {
	genesis := sha256.Sum256([]byte("scandrix-audit-genesis-2026"))
	genesisHex := hex.EncodeToString(genesis[:])
	return &AuditLogService{
		records:     make([]*AuditRecord, 0),
		lastHash:    genesisHex,
		genesisHash: genesisHex,
	}
}

func computeRecordHash(r *AuditRecord) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%d|%s|%s|%s",
		r.ID.String(),
		r.OrganizationID,
		r.Repository,
		string(r.Provider),
		r.Actor,
		string(r.Action),
		string(r.Severity),
		r.PRNumber,
		r.CommitSHA,
		r.Timestamp.UTC().Format(time.RFC3339Nano),
		r.PrevHash,
	)
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

// Record appends a new verified audit entry to the cryptographic chain.
func (s *AuditLogService) Record(
	ctx context.Context,
	orgID, repo string,
	provider models.SCMProvider,
	actor string,
	action AuditAction,
	severity AuditSeverity,
	prNumber int,
	commitSHA string,
	details map[string]any,
) (*AuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec := &AuditRecord{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Repository:     repo,
		Provider:       provider,
		Actor:          actor,
		Action:         action,
		Severity:       severity,
		PRNumber:       prNumber,
		CommitSHA:      commitSHA,
		Details:        details,
		Timestamp:      time.Now().UTC(),
		PrevHash:       s.lastHash,
	}

	rec.CurrentHash = computeRecordHash(rec)
	s.lastHash = rec.CurrentHash
	s.records = append(s.records, rec)

	return rec, nil
}

// Query searches audit records matching the specified filter criteria.
func (s *AuditLogService) Query(ctx context.Context, filter AuditLogFilter) ([]*AuditRecord, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []*AuditRecord
	for _, r := range s.records {
		if filter.OrganizationID != "" && r.OrganizationID != filter.OrganizationID {
			continue
		}
		if filter.Repository != "" && !strings.EqualFold(r.Repository, filter.Repository) {
			continue
		}
		if filter.Provider != "" && r.Provider != filter.Provider {
			continue
		}
		if filter.Action != "" && r.Action != filter.Action {
			continue
		}
		if filter.Severity != "" && r.Severity != filter.Severity {
			continue
		}
		if filter.Actor != "" && !strings.EqualFold(r.Actor, filter.Actor) {
			continue
		}
		if filter.FromTime != nil && r.Timestamp.Before(*filter.FromTime) {
			continue
		}
		if filter.ToTime != nil && r.Timestamp.After(*filter.ToTime) {
			continue
		}
		matched = append(matched, r)
	}

	total := len(matched)
	// Sort newest first
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Timestamp.After(matched[j].Timestamp)
	})

	start := filter.Offset
	if start > total {
		return []*AuditRecord{}, total
	}
	end := total
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}

	return matched[start:end], total
}

// VerifyIntegrity checks the complete cryptographic chain of all audit entries.
func (s *AuditLogService) VerifyIntegrity() (bool, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	expectedPrev := s.genesisHash
	for idx, r := range s.records {
		if r.PrevHash != expectedPrev {
			return false, idx, fmt.Errorf("integrity violation at index %d: prevHash mismatch", idx)
		}
		computed := computeRecordHash(r)
		if r.CurrentHash != computed {
			return false, idx, fmt.Errorf("integrity violation at index %d: currentHash corrupted", idx)
		}
		expectedPrev = r.CurrentHash
	}

	return true, len(s.records), nil
}

// ExportJSON exports all matching records as formatted JSON.
func (s *AuditLogService) ExportJSON(filter AuditLogFilter) ([]byte, error) {
	records, _ := s.Query(context.Background(), filter)
	return json.MarshalIndent(records, "", "  ")
}

// ExportCSV exports all matching records as CSV rows.
func (s *AuditLogService) ExportCSV(filter AuditLogFilter) string {
	records, _ := s.Query(context.Background(), filter)
	var sb strings.Builder
	sb.WriteString("ID,Timestamp,Organization,Repository,Provider,Actor,Action,Severity,PRNumber,CommitSHA,CurrentHash\n")
	for _, r := range records {
		sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%d,%s,%s\n",
			r.ID.String(),
			r.Timestamp.Format(time.RFC3339),
			r.OrganizationID,
			r.Repository,
			string(r.Provider),
			r.Actor,
			string(r.Action),
			string(r.Severity),
			r.PRNumber,
			r.CommitSHA,
			r.CurrentHash,
		))
	}
	return sb.String()
}
