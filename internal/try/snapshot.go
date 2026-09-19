package try

import (
	"sync"
	"time"
)

// StorageKeySnapshotPrefix is the client storage prefix for review snapshots.
const StorageKeySnapshotPrefix = "scandrix-review:"

// SnapshotStore provides thread-safe caching of review snapshots.
type SnapshotStore struct {
	mu        sync.RWMutex
	snapshots map[string]snapshotEntry
	maxSize   int
}

type snapshotEntry struct {
	snapshot  ReviewSnapshot
	updatedAt time.Time
}

// NewSnapshotStore creates an initialized snapshot store with capacity limits.
func NewSnapshotStore(maxSize int) *SnapshotStore {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &SnapshotStore{
		snapshots: make(map[string]snapshotEntry),
		maxSize:   maxSize,
	}
}

// Save stores a review snapshot by job ID or slug.
func (s *SnapshotStore) Save(jobID string, snapshot ReviewSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Evict oldest entry if at capacity
	if len(s.snapshots) >= s.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range s.snapshots {
			if oldestKey == "" || v.updatedAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.updatedAt
			}
		}
		if oldestKey != "" {
			delete(s.snapshots, oldestKey)
		}
	}

	s.snapshots[jobID] = snapshotEntry{
		snapshot:  snapshot,
		updatedAt: time.Now().UTC(),
	}
}

// Load retrieves a cached review snapshot by job ID or slug.
func (s *SnapshotStore) Load(jobID string) (*ReviewSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.snapshots[jobID]
	if !ok {
		return nil, false
	}
	return &entry.snapshot, true
}

// Delete removes a cached snapshot.
func (s *SnapshotStore) Delete(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.snapshots, jobID)
}
