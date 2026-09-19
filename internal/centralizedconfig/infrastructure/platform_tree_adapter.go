package infrastructure

import (
	"context"
	"fmt"
	"sync"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
)

// MemoryTreeProvider implements RepositoryTreeProvider for testing and in-process tree management.
type MemoryTreeProvider struct {
	mu       sync.RWMutex
	trees    map[string][]domain.TreeItem
	contents map[string][]byte
}

// NewMemoryTreeProvider creates an initialized tree provider.
func NewMemoryTreeProvider() *MemoryTreeProvider {
	return &MemoryTreeProvider{
		trees:    make(map[string][]domain.TreeItem),
		contents: make(map[string][]byte),
	}
}

// SetTree registers a file tree for an org/team/repo tuple.
func (m *MemoryTreeProvider) SetTree(orgID, teamID, repoID string, items []domain.TreeItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", orgID, teamID, repoID)
	m.trees[key] = items
}

// SetFileContent registers file content for path.
func (m *MemoryTreeProvider) SetFileContent(orgID, teamID, repoID, path string, content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s:%s", orgID, teamID, repoID, path)
	m.contents[key] = content
}

// GetRepositoryTree retrieves git tree items for the repository.
func (m *MemoryTreeProvider) GetRepositoryTree(ctx context.Context, orgID, teamID, repoID string) ([]domain.TreeItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s", orgID, teamID, repoID)
	return m.trees[key], nil
}

// GetFileContent retrieves file content for path.
func (m *MemoryTreeProvider) GetFileContent(ctx context.Context, orgID, teamID, repoID, path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s:%s", orgID, teamID, repoID, path)
	content, ok := m.contents[key]
	if !ok {
		return nil, fmt.Errorf("file %s not found in central repo %s", path, repoID)
	}
	return content, nil
}
