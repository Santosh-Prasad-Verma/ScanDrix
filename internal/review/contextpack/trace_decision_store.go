// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package contextpack

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/review/pipeline"
)

// TraceDecisionStore provides in-memory and persistent storage of ScanDrix Trace decisions.
type TraceDecisionStore struct {
	decisionsByRepo map[string]map[string]TraceDecision // key: orgID:repoID -> decisionKey -> Decision
	filesIndex      map[string]map[string][]string      // key: orgID:repoID -> cleanFilePath -> []decisionKey
	enabledRepos    map[string]bool                     // key: orgID:repoID -> bool
	mu              sync.RWMutex
}

// NewTraceDecisionStore constructs a new decision store.
func NewTraceDecisionStore() *TraceDecisionStore {
	return &TraceDecisionStore{
		decisionsByRepo: make(map[string]map[string]TraceDecision),
		filesIndex:      make(map[string]map[string][]string),
		enabledRepos:    make(map[string]bool),
	}
}

func repoKey(orgID, repoID string) string {
	return orgID + ":" + repoID
}

// SetTraceEnabled configures whether Trace context loading is active for a repository.
func (s *TraceDecisionStore) SetTraceEnabled(orgID, repoID string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabledRepos[repoKey(orgID, repoID)] = enabled
}

// IsTraceContextEnabled checks if Trace context loading is active for a repository.
func (s *TraceDecisionStore) IsTraceContextEnabled(ctx context.Context, orgID, repoID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	enabled, ok := s.enabledRepos[repoKey(orgID, repoID)]
	if !ok {
		// Enabled by default if decisions are registered
		return len(s.decisionsByRepo[repoKey(orgID, repoID)]) > 0, nil
	}
	return enabled, nil
}

// AddDecision registers an architectural decision record.
func (s *TraceDecisionStore) AddDecision(orgID, repoID string, d TraceDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := repoKey(orgID, repoID)
	if s.decisionsByRepo[k] == nil {
		s.decisionsByRepo[k] = make(map[string]TraceDecision)
		s.filesIndex[k] = make(map[string][]string)
	}

	s.decisionsByRepo[k][d.DecisionKey] = d

	for _, f := range d.Files {
		clean := filepath.ToSlash(filepath.Clean(f))
		clean = strings.TrimPrefix(strings.TrimPrefix(clean, "./"), "/")
		s.filesIndex[k][clean] = append(s.filesIndex[k][clean], d.DecisionKey)
	}
}

// GetDecisionsForFiles retrieves all decisions referencing the given changed files.
func (s *TraceDecisionStore) GetDecisionsForFiles(orgID, repoID string, filePaths []string) []TraceDecision {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := repoKey(orgID, repoID)
	index, ok := s.filesIndex[k]
	if !ok {
		return nil
	}

	seenKeys := make(map[string]struct{})
	for _, f := range filePaths {
		clean := filepath.ToSlash(filepath.Clean(f))
		clean = strings.TrimPrefix(strings.TrimPrefix(clean, "./"), "/")

		if keys, found := index[clean]; found {
			for _, key := range keys {
				seenKeys[key] = struct{}{}
			}
		}
	}

	results := make([]TraceDecision, 0, len(seenKeys))
	for key := range seenKeys {
		if d, found := s.decisionsByRepo[k][key]; found {
			results = append(results, d)
		}
	}
	return results
}

// LoadDecisionsForFiles implements pipeline/stages.ITraceDecisionLoader.
func (s *TraceDecisionStore) LoadDecisionsForFiles(ctx context.Context, orgID, repoID string, filePaths []string) ([]pipeline.TraceDecisionInfo, error) {
	decisions := s.GetDecisionsForFiles(orgID, repoID, filePaths)
	out := make([]pipeline.TraceDecisionInfo, len(decisions))
	for i, d := range decisions {
		out[i] = pipeline.TraceDecisionInfo{
			ID:          d.ID.String(),
			DecisionKey: d.DecisionKey,
			Title:       d.Title,
			Summary:     d.Summary,
			Rationale:   d.Rationale,
			Files:       d.Files,
		}
	}
	return out, nil
}
