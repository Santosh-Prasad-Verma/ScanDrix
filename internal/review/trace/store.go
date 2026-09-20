// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package trace

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/pipeline"
)

const (
	// DefaultTraceTokenBudget sets the default maximum token budget for decisions injected into review prompts.
	DefaultTraceTokenBudget = 2000
)

// ITraceStore defines the persistence and querying interface for Architectural Decision Records.
type ITraceStore interface {
	SaveDecision(ctx context.Context, d *TraceDecision) error
	GetDecision(ctx context.Context, orgID, repoID, decisionKey string) (*TraceDecision, error)
	ListDecisionsForRepo(ctx context.Context, orgID, repoID string) ([]*TraceDecision, error)
	QueryDecisionsForFiles(ctx context.Context, orgID, repoID string, changedFiles []string, tokenBudget int) (*TracePackResult, error)
	DeleteDecision(ctx context.Context, orgID, repoID, decisionKey string) error
	SetTraceEnabled(ctx context.Context, orgID, repoID string, enabled bool) error
	IsTraceEnabled(ctx context.Context, orgID, repoID string) (bool, error)
}

// TraceStore is an in-memory, thread-safe, production-ready store for Architectural Decisions.
type TraceStore struct {
	decisionsByRepo map[string]map[string]*TraceDecision // key: orgID:repoID -> decisionKey -> Decision
	enabledByRepo   map[string]bool                      // key: orgID:repoID -> bool
	mu              sync.RWMutex
}

// NewTraceStore constructs a new TraceStore instance.
func NewTraceStore() *TraceStore {
	return &TraceStore{
		decisionsByRepo: make(map[string]map[string]*TraceDecision),
		enabledByRepo:   make(map[string]bool),
	}
}

func repoKey(orgID, repoID string) string {
	return strings.TrimSpace(orgID) + ":" + strings.TrimSpace(repoID)
}

// SetTraceEnabled configures whether Trace context loading is active for a repository.
func (s *TraceStore) SetTraceEnabled(ctx context.Context, orgID, repoID string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabledByRepo[repoKey(orgID, repoID)] = enabled
	return nil
}

// IsTraceEnabled checks if Trace context loading is active for a repository.
func (s *TraceStore) IsTraceEnabled(ctx context.Context, orgID, repoID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := repoKey(orgID, repoID)
	enabled, exists := s.enabledByRepo[k]
	if !exists {
		// Default to enabled if decisions exist for this repository
		return len(s.decisionsByRepo[k]) > 0, nil
	}
	return enabled, nil
}

// SaveDecision inserts or updates an architectural decision record.
func (s *TraceStore) SaveDecision(ctx context.Context, d *TraceDecision) error {
	if d == nil {
		return fmt.Errorf("decision cannot be nil")
	}
	if d.OrgID == "" || d.RepoID == "" || d.DecisionKey == "" {
		return fmt.Errorf("missing required decision identifiers (orgID, repoID, decisionKey)")
	}
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	d.UpdatedAt = time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	k := repoKey(d.OrgID, d.RepoID)
	if s.decisionsByRepo[k] == nil {
		s.decisionsByRepo[k] = make(map[string]*TraceDecision)
	}

	// Deep copy to prevent external mutation
	copied := *d
	s.decisionsByRepo[k][d.DecisionKey] = &copied
	return nil
}

// GetDecision retrieves a decision by its key.
func (s *TraceStore) GetDecision(ctx context.Context, orgID, repoID, decisionKey string) (*TraceDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := repoKey(orgID, repoID)
	repoMap, exists := s.decisionsByRepo[k]
	if !exists {
		return nil, fmt.Errorf("no decisions found for repository: %s", k)
	}

	d, found := repoMap[decisionKey]
	if !found {
		return nil, fmt.Errorf("decision not found: %s", decisionKey)
	}

	copied := *d
	return &copied, nil
}

// ListDecisionsForRepo returns all decisions stored for a repository.
func (s *TraceStore) ListDecisionsForRepo(ctx context.Context, orgID, repoID string) ([]*TraceDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := repoKey(orgID, repoID)
	repoMap := s.decisionsByRepo[k]
	res := make([]*TraceDecision, 0, len(repoMap))

	for _, d := range repoMap {
		copied := *d
		res = append(res, &copied)
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].DecisionKey < res[j].DecisionKey
	})

	return res, nil
}

// QueryDecisionsForFiles retrieves all active decisions matching the given files, bounded by token budget.
func (s *TraceStore) QueryDecisionsForFiles(
	ctx context.Context,
	orgID, repoID string,
	changedFiles []string,
	tokenBudget int,
) (*TracePackResult, error) {
	if tokenBudget <= 0 {
		tokenBudget = DefaultTraceTokenBudget
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	k := repoKey(orgID, repoID)
	repoMap := s.decisionsByRepo[k]
	if len(repoMap) == 0 {
		return &TracePackResult{}, nil
	}

	var matched []*TraceDecision
	for _, d := range repoMap {
		if !d.IsActive() {
			continue
		}

		if len(changedFiles) == 0 {
			copied := *d
			matched = append(matched, &copied)
			continue
		}

		// Check if decision scope matches any of the changed files
		hasMatch := false
		for _, file := range changedFiles {
			if d.MatchesPath(file) {
				hasMatch = true
				break
			}
		}

		if hasMatch {
			copied := *d
			matched = append(matched, &copied)
		}
	}

	if len(matched) == 0 {
		return &TracePackResult{}, nil
	}

	// Priority sorting:
	// 1. Pinned decisions first
	// 2. Highest confidence descending
	// 3. Most recently updated descending
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Pinned != matched[j].Pinned {
			return matched[i].Pinned
		}
		if matched[i].Confidence != matched[j].Confidence {
			return matched[i].Confidence > matched[j].Confidence
		}
		return matched[i].UpdatedAt.After(matched[j].UpdatedAt)
	})

	// Budget allocation
	var selected []*TraceDecision
	totalTokens := 0
	dropped := 0

	for _, d := range matched {
		cost := d.EstimateTokens()
		if !d.Pinned && totalTokens+cost > tokenBudget {
			dropped++
			continue
		}
		selected = append(selected, d)
		totalTokens += cost
	}

	return &TracePackResult{
		Decisions:        selected,
		TotalTokens:      totalTokens,
		DroppedForBudget: dropped,
	}, nil
}

// DeleteDecision removes a decision from the store.
func (s *TraceStore) DeleteDecision(ctx context.Context, orgID, repoID, decisionKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := repoKey(orgID, repoID)
	if repoMap, exists := s.decisionsByRepo[k]; exists {
		delete(repoMap, decisionKey)
	}
	return nil
}

// LoadDecisionsForFiles satisfies pipeline/stages.ITraceDecisionLoader.
func (s *TraceStore) LoadDecisionsForFiles(ctx context.Context, orgID, repoID string, filePaths []string) ([]pipeline.TraceDecisionInfo, error) {
	pack, err := s.QueryDecisionsForFiles(ctx, orgID, repoID, filePaths, DefaultTraceTokenBudget)
	if err != nil {
		return nil, err
	}
	out := make([]pipeline.TraceDecisionInfo, len(pack.Decisions))
	for i, d := range pack.Decisions {
		out[i] = pipeline.TraceDecisionInfo{
			ID:          d.ID.String(),
			DecisionKey: d.DecisionKey,
			Title:       d.Title,
			Summary:     d.Decision,
			Rationale:   d.Rationale,
			Files:       d.Scope,
		}
	}
	return out, nil
}

