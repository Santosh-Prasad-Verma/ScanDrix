// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dtos

import "testing"

func TestDTOValidation(t *testing.T) {
	// FinishOnboardingDTO
	dto := &FinishOnboardingDTO{
		TeamID:   "",
		ReviewPR: false,
	}
	if err := dto.Validate(); err == nil {
		t.Errorf("expected error when teamId is empty")
	}

	dto.TeamID = "team-123"
	if err := dto.Validate(); err != nil {
		t.Errorf("unexpected error for valid dto: %v", err)
	}

	dto.ReviewPR = true
	if err := dto.Validate(); err == nil {
		t.Errorf("expected error when reviewPR is true without repo and pull number")
	}

	repoName := "repo-xyz"
	pullNum := 10
	dto.RepositoryName = &repoName
	dto.PullNumber = &pullNum
	if err := dto.Validate(); err != nil {
		t.Errorf("unexpected error for valid reviewPR dto: %v", err)
	}

	// GetRepositoryTreeByDirectoryDTO
	treeDTO := NewGetRepositoryTreeByDirectoryDTO("", "", "")
	if err := treeDTO.Validate(); err == nil {
		t.Errorf("expected error when teamId is empty")
	}

	treeDTO.TeamID = "team-123"
	if err := treeDTO.Validate(); err == nil {
		t.Errorf("expected error when repositoryId is empty")
	}

	treeDTO.RepositoryID = "repo-456"
	if err := treeDTO.Validate(); err != nil {
		t.Errorf("unexpected error for valid treeDTO: %v", err)
	}
	if !treeDTO.UseCache {
		t.Errorf("expected default UseCache to be true")
	}
}
