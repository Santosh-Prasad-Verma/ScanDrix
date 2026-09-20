// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

const (
	ThresholdRemaining  = 200
	TTLHealthy          = 30 * time.Second
	TTLNearEdge         = 5 * time.Second
	EdgeProximityFactor = 3 // near edge = remaining < threshold * 3
)

// RateLimitError is raised when GitHub quota is near exhaustion.
type RateLimitError struct {
	ResetAt        time.Time
	Remaining      int
	Platform       models.SCMProvider
	OrganizationID string
	TeamID         string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("GitHub rate limit near exhaustion: %d requests remaining, resets at %s", e.Remaining, e.ResetAt.Format(time.RFC3339))
}

// CachedRateLimit holds rate limit state with TTL.
type CachedRateLimit struct {
	Remaining int       `json:"remaining"`
	Limit     int       `json:"limit"`
	ResetAt   time.Time `json:"reset_at"`
	CachedAt  time.Time `json:"cached_at"`
	TTL       time.Duration
}

func (c *CachedRateLimit) IsStale() bool {
	return time.Since(c.CachedAt) > c.TTL
}

// IRateLimitGateService verifies GitHub rate limits before queueing or executing jobs.
type IRateLimitGateService interface {
	Check(ctx context.Context, data types.OrganizationAndTeamData, platform models.SCMProvider) error
	GetSnapshot(ctx context.Context, data types.OrganizationAndTeamData) (*CachedRateLimit, error)
}

// GitHubRateLimitGateService prevents hitting GitHub rate limit walls.
type GitHubRateLimitGateService struct {
	client     *Client
	mu         sync.RWMutex
	cache      map[string]*CachedRateLimit
	logger     *slog.Logger
	httpClient *http.Client
}

// NewGitHubRateLimitGateService creates a new rate limit gate.
func NewGitHubRateLimitGateService(client *Client, logger *slog.Logger) *GitHubRateLimitGateService {
	if logger == nil {
		logger = slog.Default()
	}
	return &GitHubRateLimitGateService{
		client:     client,
		cache:      make(map[string]*CachedRateLimit),
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *GitHubRateLimitGateService) makeCacheKey(data types.OrganizationAndTeamData) string {
	if data.OrganizationID != "" {
		return "ratelimit:" + data.OrganizationID + ":" + data.TeamID
	}
	return "ratelimit:" + data.WorkspaceID.String()
}

// Check inspects remaining GitHub API quota.
func (g *GitHubRateLimitGateService) Check(ctx context.Context, data types.OrganizationAndTeamData, platform models.SCMProvider) error {
	if platform != models.SCMProviderGitHub && platform != "" {
		return nil // Other platforms handled separately
	}

	key := g.makeCacheKey(data)

	g.mu.RLock()
	cached, exists := g.cache[key]
	g.mu.RUnlock()

	var snapshot *CachedRateLimit
	if exists && !cached.IsStale() {
		snapshot = cached
	} else {
		var err error
		snapshot, err = g.refreshSnapshot(ctx, data, key)
		if err != nil {
			g.logger.WarnContext(ctx, "Failed to inspect GitHub /rate_limit endpoint, proceeding gracefully",
				slog.String("orgId", data.OrganizationID),
				slog.Any("error", err),
			)
			return nil // Graceful fail
		}
	}

	if snapshot != nil && snapshot.Remaining < ThresholdRemaining {
		g.logger.WarnContext(ctx, "Proactively halting job execution: GitHub rate limit near exhaustion",
			slog.Int("remaining", snapshot.Remaining),
			slog.Time("resetAt", snapshot.ResetAt),
			slog.String("orgId", data.OrganizationID),
		)
		return &RateLimitError{
			ResetAt:        snapshot.ResetAt,
			Remaining:      snapshot.Remaining,
			Platform:       models.SCMProviderGitHub,
			OrganizationID: data.OrganizationID,
			TeamID:         data.TeamID,
		}
	}

	return nil
}

func (g *GitHubRateLimitGateService) GetSnapshot(ctx context.Context, data types.OrganizationAndTeamData) (*CachedRateLimit, error) {
	key := g.makeCacheKey(data)
	g.mu.RLock()
	cached, ok := g.cache[key]
	g.mu.RUnlock()

	if ok && !cached.IsStale() {
		return cached, nil
	}
	return g.refreshSnapshot(ctx, data, key)
}

func (g *GitHubRateLimitGateService) refreshSnapshot(ctx context.Context, data types.OrganizationAndTeamData, key string) (*CachedRateLimit, error) {
	token := data.AuthToken
	if token == "" && g.client != nil {
		token = g.client.token
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/rate_limit", nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub /rate_limit returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Resources struct {
			Core struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"core"`
		} `json:"resources"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	core := payload.Resources.Core
	ttl := TTLHealthy
	if core.Remaining < ThresholdRemaining*EdgeProximityFactor {
		ttl = TTLNearEdge
	}

	snapshot := &CachedRateLimit{
		Remaining: core.Remaining,
		Limit:     core.Limit,
		ResetAt:   time.Unix(core.Reset, 0),
		CachedAt:  time.Now(),
		TTL:       ttl,
	}

	g.mu.Lock()
	g.cache[key] = snapshot
	g.mu.Unlock()

	return snapshot, nil
}
