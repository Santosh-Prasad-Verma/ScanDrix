package settings

import (
	"strings"
	"sync"

	"github.com/google/uuid"
)

// ModelOverrideManager coordinates hierarchical model routing overrides.
type ModelOverrideManager struct {
	mu        sync.RWMutex
	overrides map[uuid.UUID][]ModelOverride // workspaceID -> overrides
}

func NewModelOverrideManager() *ModelOverrideManager {
	return &ModelOverrideManager{
		overrides: make(map[uuid.UUID][]ModelOverride),
	}
}

// SetOverride registers or updates a model override at global, repo, or directory scope.
func (m *ModelOverrideManager) SetOverride(override ModelOverride) {
	m.mu.Lock()
	defer m.mu.Unlock()

	list := m.overrides[override.WorkspaceID]
	var updated []ModelOverride
	replaced := false

	for _, o := range list {
		if o.Scope == override.Scope &&
			o.RepositoryPath == override.RepositoryPath &&
			o.DirectoryPath == override.DirectoryPath {
			updated = append(updated, override)
			replaced = true
		} else {
			updated = append(updated, o)
		}
	}

	if !replaced {
		updated = append(updated, override)
	}

	m.overrides[override.WorkspaceID] = updated
}

// ResolveModel determines the effective model ID based on scope precedence.
// Precedence: Directory > Repository > Global Default.
func (m *ModelOverrideManager) ResolveModel(workspaceID uuid.UUID, repoPath, dirPath, defaultModel string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list, exists := m.overrides[workspaceID]
	if !exists || len(list) == 0 {
		return defaultModel
	}

	var repoModel string
	var globalModel string

	for _, o := range list {
		// 1. Directory Scope (Highest Precedence)
		if o.Scope == ScopeDirectory && o.RepositoryPath == repoPath && strings.HasPrefix(dirPath, o.DirectoryPath) {
			return o.ModelID
		}
		// 2. Repository Scope
		if o.Scope == ScopeRepository && o.RepositoryPath == repoPath {
			repoModel = o.ModelID
		}
		// 3. Global Scope
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

// ListOverrides returns all active model overrides for a workspace.
func (m *ModelOverrideManager) ListOverrides(workspaceID uuid.UUID) []ModelOverride {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := m.overrides[workspaceID]
	out := make([]ModelOverride, len(list))
	copy(out, list)
	return out
}

// ClearOverrides removes targeted overrides by repository or scope.
func (m *ModelOverrideManager) ClearOverrides(workspaceID uuid.UUID, repoPath string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	list := m.overrides[workspaceID]
	var remaining []ModelOverride
	removed := 0

	for _, o := range list {
		if repoPath != "" && o.RepositoryPath == repoPath {
			removed++
			continue
		}
		if repoPath == "" {
			removed++
			continue
		}
		remaining = append(remaining, o)
	}

	m.overrides[workspaceID] = remaining
	return removed
}
