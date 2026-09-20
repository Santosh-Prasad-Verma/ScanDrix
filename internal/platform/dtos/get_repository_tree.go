// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"errors"
	"strings"
)

// GetRepositoryTreeByDirectoryDTO represents parameters for directory-based tree browsing.
type GetRepositoryTreeByDirectoryDTO struct {
	TeamID        string `json:"teamId"`
	RepositoryID  string `json:"repositoryId"`
	DirectoryPath string `json:"directoryPath,omitempty"`
	UseCache      bool   `json:"useCache"`
}

// NewGetRepositoryTreeByDirectoryDTO constructs DTO with default cache enabled.
func NewGetRepositoryTreeByDirectoryDTO(teamID, repositoryID, directoryPath string) *GetRepositoryTreeByDirectoryDTO {
	return &GetRepositoryTreeByDirectoryDTO{
		TeamID:        teamID,
		RepositoryID:  repositoryID,
		DirectoryPath: directoryPath,
		UseCache:      true,
	}
}

// Validate ensures identifiers are present.
func (d *GetRepositoryTreeByDirectoryDTO) Validate() error {
	if strings.TrimSpace(d.TeamID) == "" {
		return errors.New("teamId is required")
	}
	if strings.TrimSpace(d.RepositoryID) == "" {
		return errors.New("repositoryId is required")
	}
	return nil
}
