package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/core/cache"
)

// CapabilityResourcePlan defines resource tool mappings for a provider capability.
type CapabilityResourcePlan struct {
	ProviderType string   `json:"providerType"`
	Capability   string   `json:"capability"`
	Tools        []string `json:"tools"`
}

const (
	// ResourcePlanCacheTTLMs is the cache TTL for resource plans (24h).
	ResourcePlanCacheTTLMs = int64(24 * 60 * 60 * 1000)
	primarySeedDir         = "capability-seeds"
	legacySeedDir          = "resources"
)

var (
	safeSegmentRegex    = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	safeCapabilityRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
)

// CapabilityResourcePlanService manages caching and seed resolution for capability tools.
type CapabilityResourcePlanService struct {
	memoryCache  *BoundedMap[string, []string]
	seedCache    *BoundedMap[string, []string]
	cacheService *cache.CacheService
	seedDirs     []string
	customSeeds  map[string][]string
	mu           sync.RWMutex
}

// NewCapabilityResourcePlanService creates a new CapabilityResourcePlanService.
func NewCapabilityResourcePlanService(
	cacheService *cache.CacheService,
	customSeedDirs ...string,
) *CapabilityResourcePlanService {
	return &CapabilityResourcePlanService{
		memoryCache:  NewBoundedMap[string, []string](256),
		seedCache:    NewBoundedMap[string, []string](64),
		cacheService: cacheService,
		seedDirs:     customSeedDirs,
		customSeeds:  make(map[string][]string),
	}
}

// GetCachedTools retrieves cached tool names for a capability scope.
func (s *CapabilityResourcePlanService) GetCachedTools(
	ctx context.Context,
	scope CapabilityStrategyScope,
) []string {
	key := s.buildKey(scope)
	if tools, found := s.memoryCache.Get(key); found {
		return tools
	}

	if s.cacheService == nil {
		return nil
	}

	var cached []string
	found, err := s.cacheService.GetFromCache(ctx, key, &cached)
	if err == nil && found && len(cached) > 0 {
		s.memoryCache.Set(key, cached)
		return cached
	}

	return nil
}

// SaveCachedTools persists tools for a capability scope in memory and external cache.
func (s *CapabilityResourcePlanService) SaveCachedTools(
	ctx context.Context,
	scope CapabilityStrategyScope,
	tools []string,
) {
	key := s.buildKey(scope)
	normalized := normalizeToolSlice(tools)
	s.memoryCache.Set(key, normalized)

	if s.cacheService != nil {
		_ = s.cacheService.AddToCache(ctx, key, normalized, ResourcePlanCacheTTLMs)
	}
}

// RegisterSeedTools allows programmatic registration of seed tools for a provider and capability.
func (s *CapabilityResourcePlanService) RegisterSeedTools(
	providerType, capability string,
	tools []string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s:%s", s.normalizeProviderToken(providerType), capability)
	s.customSeeds[key] = normalizeToolSlice(tools)
	s.seedCache.Set(key, normalizeToolSlice(tools))
}

// GetSeedTools retrieves seed tools for a provider and capability from memory, disk, or built-in defaults.
func (s *CapabilityResourcePlanService) GetSeedTools(
	providerType string,
	capability string,
) []string {
	if !s.isSafeSegment(providerType) || !s.isSafeCapability(capability) {
		return nil
	}

	cacheKey := fmt.Sprintf("%s:%s", s.normalizeProviderToken(providerType), capability)
	if tools, found := s.seedCache.Get(cacheKey); found {
		return tools
	}

	s.mu.RLock()
	if tools, exists := s.customSeeds[cacheKey]; exists {
		s.mu.RUnlock()
		s.seedCache.Set(cacheKey, tools)
		return tools
	}
	s.mu.RUnlock()

	// Resolve provider candidates
	candidates := s.resolveProviderSeedCandidates(providerType)
	for _, providerCandidate := range candidates {
		// Check custom registered seeds with candidate
		candKey := fmt.Sprintf("%s:%s", providerCandidate, capability)
		s.mu.RLock()
		if tools, exists := s.customSeeds[candKey]; exists {
			s.mu.RUnlock()
			s.seedCache.Set(cacheKey, tools)
			return tools
		}
		s.mu.RUnlock()

		// Try seed file candidates from disk
		fileCandidates := s.getSeedFileCandidates(providerCandidate, capability)
		for _, filePath := range fileCandidates {
			if _, err := os.Stat(filePath); err == nil {
				data, err := os.ReadFile(filePath)
				if err == nil {
					var plan CapabilityResourcePlan
					if json.Unmarshal(data, &plan) == nil && len(plan.Tools) > 0 {
						tools := normalizeToolSlice(plan.Tools)
						s.seedCache.Set(cacheKey, tools)
						return tools
					}
				}
			}
		}

		// Built-in standard seed mappings
		if capability == "task.context.read" {
			switch providerCandidate {
			case "jira":
				tools := []string{"getJiraIssue", "searchJiraIssuesUsingJql", "search", "fetch"}
				s.seedCache.Set(cacheKey, tools)
				return tools
			case "linear":
				tools := []string{"getLinearIssue", "searchIssues", "issue", "issues"}
				s.seedCache.Set(cacheKey, tools)
				return tools
			case "notion":
				tools := []string{"getNotionPage", "searchNotion", "getPage", "search"}
				s.seedCache.Set(cacheKey, tools)
				return tools
			case "clickup":
				tools := []string{"getClickUpTask", "getTask", "searchTasks"}
				s.seedCache.Set(cacheKey, tools)
				return tools
			case "scandrix-github-issues":
				tools := []string{"SCANDRIX_LIST_ISSUES", "SCANDRIX_GET_ISSUE"}
				s.seedCache.Set(cacheKey, tools)
				return tools
			}
		}
	}

	s.seedCache.Set(cacheKey, []string{})
	return nil
}

func (s *CapabilityResourcePlanService) buildKey(scope CapabilityStrategyScope) string {
	return fmt.Sprintf("skill-capability-resource-plan:%s:%s:%s:%s:%s",
		scope.OrganizationID,
		scope.TeamID,
		scope.SkillName,
		scope.Capability,
		scope.Provider,
	)
}

func (s *CapabilityResourcePlanService) isSafeSegment(value string) bool {
	return safeSegmentRegex.MatchString(value)
}

func (s *CapabilityResourcePlanService) isSafeCapability(value string) bool {
	return safeCapabilityRegex.MatchString(value)
}

func (s *CapabilityResourcePlanService) resolveProviderSeedCandidates(providerType string) []string {
	normalized := s.normalizeProviderToken(providerType)
	compact := strings.ReplaceAll(strings.ReplaceAll(normalized, "-", ""), "_", "")
	candidates := []string{normalized}

	if strings.Contains(normalized, "jira") || strings.Contains(normalized, "atlassian") {
		candidates = append(candidates, "jira")
	}
	if strings.Contains(normalized, "linear") {
		candidates = append(candidates, "linear")
	}
	if strings.Contains(normalized, "notion") {
		candidates = append(candidates, "notion")
	}
	if strings.Contains(normalized, "clickup") {
		candidates = append(candidates, "clickup")
	}
	if strings.Contains(normalized, "scandrix-github-issues") ||
		strings.Contains(compact, "scandrixgithubissues") ||
		(strings.Contains(compact, "github") && strings.Contains(compact, "issues")) {
		candidates = append(candidates, "scandrix-github-issues")
	}

	return uniqueStrings(candidates)
}

func (s *CapabilityResourcePlanService) normalizeProviderToken(value string) string {
	val := strings.TrimSpace(strings.ToLower(value))
	reg := regexp.MustCompile(`[^a-z0-9_-]+`)
	return reg.ReplaceAllString(val, "")
}

func (s *CapabilityResourcePlanService) getSeedFileCandidates(providerType, capability string) []string {
	fileName := fmt.Sprintf("%s.json", capability)
	var candidates []string

	for _, dir := range s.seedDirs {
		candidates = append(candidates,
			filepath.Join(dir, providerType, fileName),
			filepath.Join(dir, primarySeedDir, providerType, fileName),
			filepath.Join(dir, legacySeedDir, providerType, fileName),
		)
	}

	cwd, err := os.Getwd()
	if err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "internal", "agents", "skills", "runtime", primarySeedDir, providerType, fileName),
			filepath.Join(cwd, "internal", "agents", "skills", "runtime", legacySeedDir, providerType, fileName),
		)
	}

	return candidates
}

func normalizeToolSlice(tools []string) []string {
	seen := make(map[string]struct{}, len(tools))
	var result []string
	for _, tool := range tools {
		trimmed := strings.TrimSpace(tool)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; !exists {
			seen[trimmed] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	var result []string
	for _, item := range items {
		if _, exists := seen[item]; !exists {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}
