package try

import "sync"

// StorageKeyViewedPrefix is the client storage prefix for viewed files tracking.
const StorageKeyViewedPrefix = "scandrix-review-viewed:"

// ViewedStore tracks reviewed and marked files per pull request session.
type ViewedStore struct {
	mu     sync.RWMutex
	viewed map[string]map[string]bool
}

// NewViewedStore initializes an empty viewed files tracking store.
func NewViewedStore() *ViewedStore {
	return &ViewedStore{
		viewed: make(map[string]map[string]bool),
	}
}

// SetViewed marks or unmarks a file as viewed for a given job or PR slug.
func (s *ViewedStore) SetViewed(jobID, filePath string, viewed bool) map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.viewed[jobID]
	if !ok {
		m = make(map[string]bool)
		s.viewed[jobID] = m
	}
	m[filePath] = viewed

	// Return a copy
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// GetViewed retrieves the map of viewed files for a given job or PR slug.
func (s *ViewedStore) GetViewed(jobID string) map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.viewed[jobID]
	if !ok {
		return make(map[string]bool)
	}

	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Clear removes all viewed tracking for a given job.
func (s *ViewedStore) Clear(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.viewed, jobID)
}
