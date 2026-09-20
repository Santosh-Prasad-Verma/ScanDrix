// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"strings"

	"github.com/google/uuid"
)

// ScopeType represents the override granularity.
type ScopeType string

const (
	ScopeGlobal     ScopeType = "global"
	ScopeRepository ScopeType = "repository"
	ScopeDirectory  ScopeType = "directory"
)

// ModelOverride represents an explicit LLM assignment rule.
type ModelOverride struct {
	ID             string    `json:"id"`
	WorkspaceID    uuid.UUID `json:"workspaceId"`
	Scope          ScopeType `json:"scope"`
	RepositoryPath string    `json:"repositoryPath,omitempty"`
	DirectoryPath  string    `json:"directoryPath,omitempty"`
	ModelID        string    `json:"modelId"`
	Reason         string    `json:"reason,omitempty"`
}

// ResolveModelWithPrecedence determines the active model based on Directory > Repo > Global precedence.
func ResolveModelWithPrecedence(overrides []ModelOverride, repoPath, dirPath, defaultModel string) string {
	var repoModel string
	var globalModel string

	for _, o := range overrides {
		if o.Scope == ScopeDirectory && o.RepositoryPath == repoPath && strings.HasPrefix(dirPath, o.DirectoryPath) {
			return o.ModelID
		}
		if o.Scope == ScopeRepository && o.RepositoryPath == repoPath {
			repoModel = o.ModelID
		}
		if o.Scope == ScopeGlobal {
			globalModel = o.ModelID
		}
	}

	if repoModel != "" {
		return repoModel
	}
	if globalModel != "" {
		return globalModel
	}
	return defaultModel
}
