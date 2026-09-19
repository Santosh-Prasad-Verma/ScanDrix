// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/google/uuid"
)

// PreviewPRSummaryRepositoryDTO defines repository descriptor in preview requests.
type PreviewPRSummaryRepositoryDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PreviewPRSummaryDTO represents review summary preview request payload.
// Matches libs/organization/dtos/preview-pr-summary.dto.ts
type PreviewPRSummaryDTO struct {
	PRNumber                        string                        `json:"prNumber"`
	Repository                      PreviewPRSummaryRepositoryDTO `json:"repository"`
	TeamID                          uuid.UUID                     `json:"teamId"`
	BehaviourForExistingDescription string                        `json:"behaviourForExistingDescription"`
	CustomInstructions              *string                       `json:"customInstructions,omitempty"`
}
