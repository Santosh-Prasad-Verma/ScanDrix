// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package operations

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PRLifecycleState represents the evaluation state of a pull request.
type PRLifecycleState string

const (
	PRStatePending          PRLifecycleState = "PENDING"
	PRStateScanning         PRLifecycleState = "SCANNING"
	PRStatePassed           PRLifecycleState = "PASSED"
	PRStateChangesRequested PRLifecycleState = "CHANGES_REQUESTED"
	PRStateSecurityAlert    PRLifecycleState = "SECURITY_ALERT"
	PRStateBlocked          PRLifecycleState = "BLOCKED"
)

// PRLifecycleRecord tracks status, findings, and metadata for a pull request.
type PRLifecycleRecord struct {
	ID             uuid.UUID        `json:"id"`
	OrganizationID string           `json:"organizationId"`
	Repository     string           `json:"repository"`
	PRNumber       int              `json:"prNumber"`
	HeadSHA        string           `json:"headSha"`
	BaseBranch     string           `json:"baseBranch"`
	HeadBranch     string           `json:"headBranch"`
	IsDraft        bool             `json:"isDraft"`
	State          PRLifecycleState `json:"state"`
	CriticalCount  int              `json:"criticalCount"`
	HighCount      int              `json:"highCount"`
	MediumCount    int              `json:"mediumCount"`
	LowCount       int              `json:"lowCount"`
	TotalFiles     int              `json:"totalFiles"`
	LabelsApplied  []string         `json:"labelsApplied"`
	LastEvaluated  time.Time        `json:"lastEvaluated"`
	SummaryComment string           `json:"summaryComment,omitempty"`
}

// ReviewCriteria specifies quality gates for pull request approval.
type ReviewCriteria struct {
	MaxCriticalAllowed int  `json:"maxCriticalAllowed"`
	MaxHighAllowed     int  `json:"maxHighAllowed"`
	BlockOnSecrets     bool `json:"blockOnSecrets"`
	RequireNoConflicts bool `json:"requireNoConflicts"`
	IgnoreDraftPRs     bool `json:"ignoreDraftPRs"`
}

// DefaultReviewCriteria provides default strict production criteria.
func DefaultReviewCriteria() ReviewCriteria {
	return ReviewCriteria{
		MaxCriticalAllowed: 0,
		MaxHighAllowed:     0,
		BlockOnSecrets:     true,
		RequireNoConflicts: true,
		IgnoreDraftPRs:     true,
	}
}

// PRLifecycleManager manages pull request states, labels, and quality gates across providers.
type PRLifecycleManager struct {
	mu       sync.RWMutex
	records  map[string]*PRLifecycleRecord // key: org:repo:prNumber
	criteria ReviewCriteria
}

// NewPRLifecycleManager creates a new PRLifecycleManager instance.
func NewPRLifecycleManager(criteria ReviewCriteria) *PRLifecycleManager {
	return &PRLifecycleManager{
		records:  make(map[string]*PRLifecycleRecord),
		criteria: criteria,
	}
}

func (m *PRLifecycleManager) recordKey(org, repo string, prNumber int) string {
	return fmt.Sprintf("%s:%s:%d", strings.ToLower(org), strings.ToLower(repo), prNumber)
}

// RegisterOrUpdatePR records or updates a pull request status.
func (m *PRLifecycleManager) RegisterOrUpdatePR(record PRLifecycleRecord) *PRLifecycleRecord {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.recordKey(record.OrganizationID, record.Repository, record.PRNumber)
	existing, found := m.records[key]
	if !found {
		if record.ID == uuid.Nil {
			record.ID = uuid.New()
		}
		record.LastEvaluated = time.Now().UTC()
		m.records[key] = &record
		return &record
	}

	existing.HeadSHA = record.HeadSHA
	existing.BaseBranch = record.BaseBranch
	existing.HeadBranch = record.HeadBranch
	existing.IsDraft = record.IsDraft
	existing.State = record.State
	existing.CriticalCount = record.CriticalCount
	existing.HighCount = record.HighCount
	existing.MediumCount = record.MediumCount
	existing.LowCount = record.LowCount
	existing.TotalFiles = record.TotalFiles
	existing.LabelsApplied = record.LabelsApplied
	existing.LastEvaluated = time.Now().UTC()
	if record.SummaryComment != "" {
		existing.SummaryComment = record.SummaryComment
	}
	return existing
}

// GetPRRecord retrieves a pull request record.
func (m *PRLifecycleManager) GetPRRecord(org, repo string, prNumber int) (*PRLifecycleRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.recordKey(org, repo, prNumber)
	record, found := m.records[key]
	if !found {
		return nil, false
	}
	copyRecord := *record
	return &copyRecord, true
}

// EvaluateQualityGate evaluates a PR against configured review criteria.
func (m *PRLifecycleManager) EvaluateQualityGate(ctx context.Context, record *PRLifecycleRecord, hasConflicts bool, hasSecrets bool) (PRLifecycleState, []string) {
	if record.IsDraft && m.criteria.IgnoreDraftPRs {
		return PRStatePending, []string{"scandrix:draft"}
	}

	var labels []string
	if hasConflicts && m.criteria.RequireNoConflicts {
		labels = append(labels, "scandrix:conflicts-detected")
		return PRStateBlocked, labels
	}

	if hasSecrets && m.criteria.BlockOnSecrets {
		labels = append(labels, "scandrix:secret-leak-alert", "scandrix:security-alert")
		return PRStateSecurityAlert, labels
	}

	if record.CriticalCount > m.criteria.MaxCriticalAllowed {
		labels = append(labels, "scandrix:security-alert", "scandrix:changes-requested")
		return PRStateSecurityAlert, labels
	}

	if record.HighCount > m.criteria.MaxHighAllowed {
		labels = append(labels, "scandrix:changes-requested")
		return PRStateChangesRequested, labels
	}

	labels = append(labels, "scandrix:approved")
	return PRStatePassed, labels
}

// FormatNotificationPayload formats a structured notification payload for Slack/Teams/Webhooks.
func (m *PRLifecycleManager) FormatNotificationPayload(record *PRLifecycleRecord) map[string]any {
	title := fmt.Sprintf("ScanDrix Code Review: %s #%d", record.Repository, record.PRNumber)
	statusText := string(record.State)

	severityColor := "#22c55e" // green
	if record.State == PRStateSecurityAlert || record.State == PRStateBlocked {
		severityColor = "#ef4444" // red
	} else if record.State == PRStateChangesRequested {
		severityColor = "#f59e0b" // yellow
	}

	return map[string]any{
		"title":       title,
		"state":       statusText,
		"color":       severityColor,
		"repository":  record.Repository,
		"pr_number":   record.PRNumber,
		"head_sha":    record.HeadSHA,
		"files_count": record.TotalFiles,
		"metrics": map[string]int{
			"critical": record.CriticalCount,
			"high":     record.HighCount,
			"medium":   record.MediumCount,
			"low":      record.LowCount,
		},
		"labels":    record.LabelsApplied,
		"timestamp": record.LastEvaluated.Format(time.RFC3339),
	}
}
