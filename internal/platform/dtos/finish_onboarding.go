// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"errors"
	"strings"
)

// FinishOnboardingDTO carries parameters needed to conclude team onboarding.
type FinishOnboardingDTO struct {
	TeamID         string  `json:"teamId"`
	ReviewPR       bool    `json:"reviewPR"`
	RepositoryID   *string `json:"repositoryId,omitempty"`
	RepositoryName *string `json:"repositoryName,omitempty"`
	PullNumber     *int    `json:"pullNumber,omitempty"`
}

// Validate ensures required fields are set correctly according to business rules.
func (d *FinishOnboardingDTO) Validate() error {
	if strings.TrimSpace(d.TeamID) == "" {
		return errors.New("teamId is required")
	}
	if d.ReviewPR {
		if d.RepositoryID == nil && d.RepositoryName == nil {
			return errors.New("repositoryId or repositoryName is required when reviewPR is true")
		}
		if d.PullNumber == nil || *d.PullNumber <= 0 {
			return errors.New("valid pullNumber is required when reviewPR is true")
		}
	}
	return nil
}
