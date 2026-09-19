// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: add_library_drixy_rules_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// DirectoryInfo targets specific directory locations for rule attachment.
type DirectoryInfo struct {
	RepositoryID string `json:"repositoryId"`
	DirectoryID  string `json:"directoryId"`
}

// AddLibraryDrixyRulesDto defines payload for importing catalog rules into repos.
type AddLibraryDrixyRulesDto struct {
	Title           string                         `json:"title"`
	Rule            string                         `json:"rule"`
	Path            string                         `json:"path,omitempty"`
	Severity        string                         `json:"severity"`
	RepositoriesIDs []string                       `json:"repositoriesIds"`
	DirectoriesInfo []DirectoryInfo                `json:"directoriesInfo,omitempty"`
	Examples        []interfaces.DrixyRulesExample `json:"examples,omitempty"`
	TeamID          string                         `json:"teamId,omitempty"`
}
