package feedback

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// PRMessageManager evaluates path pattern rules to inject custom compliance and review notices.
type PRMessageManager struct {
	mu    sync.RWMutex
	rules map[uuid.UUID][]ScopedMessageRule // workspaceID -> rules
}

func NewPRMessageManager() *PRMessageManager {
	return &PRMessageManager{
		rules: make(map[uuid.UUID][]ScopedMessageRule),
	}
}

// AddRule registers a new path-based notice rule.
func (m *PRMessageManager) AddRule(rule ScopedMessageRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[rule.WorkspaceID] = append(m.rules[rule.WorkspaceID], rule)
}

// EvaluateNotices checks modified files and returns applicable headers, footers, and whether merge is blocked.
func (m *PRMessageManager) EvaluateNotices(workspaceID uuid.UUID, repoPath string, changedFiles []string) (headers []string, footers []string, blockMerge bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rules := m.rules[workspaceID]
	for _, rule := range rules {
		// Repo filter check (if rule is scoped to specific repo)
		if rule.RepositoryPath != "" && rule.RepositoryPath != repoPath {
			continue
		}

		// Check if any changed file matches the path pattern
		matched := false
		for _, file := range changedFiles {
			if matchPathPattern(rule.PathPattern, file) {
				matched = true
				break
			}
		}

		if matched {
			if rule.HeaderNotice != "" {
				headers = append(headers, rule.HeaderNotice)
			}
			if rule.FooterNotice != "" {
				footers = append(footers, rule.FooterNotice)
			}
			if rule.BlockMerge {
				blockMerge = true
			}
		}
	}

	return headers, footers, blockMerge
}

func matchPathPattern(pattern, targetPath string) bool {
	// Simple wildcard globbing
	pattern = filepath.Clean(pattern)
	targetPath = filepath.Clean(targetPath)

	if pattern == "**" || pattern == "*" {
		return true
	}

	// Suffix wildcard: e.g. "migrations/**" or "migrations/*"
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(targetPath, prefix)
	}

	matched, err := filepath.Match(pattern, targetPath)
	if err == nil && matched {
		return true
	}

	return strings.Contains(targetPath, strings.Trim(pattern, "*"))
}
