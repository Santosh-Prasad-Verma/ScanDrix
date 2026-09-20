// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
)

// OrganizationMemberSummary represents a normalized member of an SCM organization or workspace.
type OrganizationMemberSummary struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Type string `json:"type"` // "user" or "bot"
}

// OrganizationMemberListResult holds the outcome of a member list retrieval.
// An "unavailable" status means we could not confirm who belongs to the git organization.
// Callers must never read it as "empty" — seat revocation or pruning must be skipped.
type OrganizationMemberListResult struct {
	Status  string                      `json:"status"` // "ok" or "unavailable"
	Members []OrganizationMemberSummary `json:"members"`
}

// MemoryCache provides in-memory TTL caching for member summaries.
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
}

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// NewMemoryCache creates a thread-safe in-memory cache with TTL support.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		items: make(map[string]cacheEntry),
	}
}

// Get retrieves a cached value if not expired.
func (c *MemoryCache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.items[key]
	if !found {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.value, true
}

// Set stores a value with a specific TTL.
func (c *MemoryCache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
}

// Delete removes an entry from the cache.
func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}

// OrganizationMemberListService translates organization-member-list.service.ts.
// Aggregates members from organization enumeration and recent PR authors so that
// every user with repository access is accounted for across GitHub, GitLab, Bitbucket, Azure, and Forgejo.
type OrganizationMemberListService struct {
	codeManagementService contracts.ICodeManagementService
	cache                 *MemoryCache
}

// NewOrganizationMemberListService constructs a new service instance.
func NewOrganizationMemberListService(
	codeManagementService contracts.ICodeManagementService,
	cache *MemoryCache,
) *OrganizationMemberListService {
	if cache == nil {
		cache = NewMemoryCache()
	}
	return &OrganizationMemberListService{
		codeManagementService: codeManagementService,
		cache:                 cache,
	}
}

const (
	authorsCacheTTL = 10 * time.Minute
	membersCacheTTL = 30 * time.Minute
)

// Fetch retrieves and normalizes organization members and pull request authors.
func (s *OrganizationMemberListService) Fetch(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	skipCache bool,
) (*OrganizationMemberListResult, error) {
	cacheKey := fmt.Sprintf("org_members_%s_%s", orgData.OrganizationID, orgData.TeamID)

	if !skipCache {
		if cachedVal, ok := s.cache.Get(cacheKey); ok {
			if members, ok := cachedVal.([]OrganizationMemberSummary); ok && len(members) > 0 {
				return &OrganizationMemberListResult{
					Status:  "ok",
					Members: members,
				}, nil
			}
		}
	}

	var wg sync.WaitGroup
	var members []types.PullRequestAuthor
	var authors []types.PullRequestAuthor
	var memberErr, authorErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		members, memberErr = s.codeManagementService.GetListMembers(ctx, orgData)
		if memberErr != nil {
			slog.WarnContext(ctx, "Unable to fetch members from code integration",
				"organizationId", orgData.OrganizationID,
				"teamId", orgData.TeamID,
				"error", memberErr,
			)
		}
	}()

	go func() {
		defer wg.Done()
		authors, authorErr = s.fetchPullRequestAuthors(ctx, orgData, skipCache)
		if authorErr != nil {
			slog.WarnContext(ctx, "Unable to fetch pull request authors from code integration",
				"organizationId", orgData.OrganizationID,
				"teamId", orgData.TeamID,
				"error", authorErr,
			)
		}
	}()

	wg.Wait()

	// Only total failure where both returned errors and 0 items is unavailable
	if memberErr != nil && authorErr != nil && len(members) == 0 && len(authors) == 0 {
		return &OrganizationMemberListResult{
			Status:  "unavailable",
			Members: []OrganizationMemberSummary{},
		}, nil
	}

	combined := append([]types.PullRequestAuthor{}, members...)
	combined = append(combined, authors...)

	normalized := s.Normalize(combined)

	if len(normalized) == 0 {
		slog.WarnContext(ctx, "Code integration returned no usable members; treating the list as unavailable",
			"organizationId", orgData.OrganizationID,
			"teamId", orgData.TeamID,
			"rawMemberCount", len(members),
			"rawAuthorCount", len(authors),
		)
		return &OrganizationMemberListResult{
			Status:  "unavailable",
			Members: []OrganizationMemberSummary{},
		}, nil
	}

	s.cache.Set(cacheKey, normalized, membersCacheTTL)

	return &OrganizationMemberListResult{
		Status:  "ok",
		Members: normalized,
	}, nil
}

// RefreshMembers clears the cache and refetches fresh member lists.
func (s *OrganizationMemberListService) RefreshMembers(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
) (*OrganizationMemberListResult, error) {
	cacheKey := fmt.Sprintf("org_members_%s_%s", orgData.OrganizationID, orgData.TeamID)
	s.cache.Delete(cacheKey)
	return s.Fetch(ctx, orgData, true)
}

func (s *OrganizationMemberListService) fetchPullRequestAuthors(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	skipCache bool,
) ([]types.PullRequestAuthor, error) {
	cacheKey := fmt.Sprintf("org_member_pr_authors_%s_%s", orgData.OrganizationID, orgData.TeamID)

	if !skipCache {
		if cachedVal, ok := s.cache.Get(cacheKey); ok {
			if authors, ok := cachedVal.([]types.PullRequestAuthor); ok && len(authors) > 0 {
				return authors, nil
			}
		}
	}

	authors, err := s.codeManagementService.GetPullRequestAuthors(ctx, orgData, true)
	if err != nil {
		return nil, err
	}

	if len(authors) > 0 {
		s.cache.Set(cacheKey, authors, authorsCacheTTL)
	}

	return authors, nil
}

// Normalize deduplicates members by ID, categorizes user vs bot, and sorts alphabetically by name.
func (s *OrganizationMemberListService) Normalize(rawMembers []types.PullRequestAuthor) []OrganizationMemberSummary {
	if len(rawMembers) == 0 {
		return []OrganizationMemberSummary{}
	}

	unique := make(map[string]OrganizationMemberSummary)

	for _, m := range rawMembers {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			id = strings.TrimSpace(m.Username)
		}
		if id == "" {
			continue
		}

		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = strings.TrimSpace(m.Username)
		}
		if name == "" {
			continue
		}

		memberType := "user"
		if m.IsBot || strings.Contains(strings.ToLower(name), "[bot]") || strings.Contains(strings.ToLower(id), "bot") {
			memberType = "bot"
		}

		if _, exists := unique[id]; !exists {
			unique[id] = OrganizationMemberSummary{
				ID:   id,
				Name: name,
				Type: memberType,
			}
		}
	}

	result := make([]OrganizationMemberSummary, 0, len(unique))
	for _, summary := range unique {
		result = append(result, summary)
	}

	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result
}
