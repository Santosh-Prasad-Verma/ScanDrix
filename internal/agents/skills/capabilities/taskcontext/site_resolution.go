package taskcontext

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

var (
	siteResolverTools = []string{"getAccessibleAtlassianResources"}
	siteHintsMu       sync.RWMutex
	siteHintsCache    = make(map[string]cachedSiteEntry)
	siteInFlightMu    sync.Mutex
	siteInFlight      = make(map[string]chan struct{})
)

type cachedSiteEntry struct {
	hints     TaskContextSiteHints
	expiresAt time.Time
}

// TaskContextSiteHints holds resolved tenant IDs and URLs.
type TaskContextSiteHints struct {
	SiteIDs  []string `json:"siteIds"`
	SiteURLs []string `json:"siteUrls"`
}

// ResetTaskContextSiteHintsCache clears cached tenant site hints.
func ResetTaskContextSiteHintsCache() {
	siteHintsMu.Lock()
	defer siteHintsMu.Unlock()
	siteHintsCache = make(map[string]cachedSiteEntry)
}

// IToolCallerSeam provides minimal tool calling for site resolution.
type IToolCallerSeam interface {
	CallTool(ctx context.Context, toolName string, args map[string]any) (any, error)
}

// ResolveTaskContextSiteHints resolves provider tenant/site metadata out-of-band.
func ResolveTaskContextSiteHints(
	ctx context.Context,
	caller IToolCallerSeam,
	registeredTools []string,
	organizationID string,
	providerType string,
	logger *slog.Logger,
) TaskContextSiteHints {
	var resolverTool string
	for _, t := range siteResolverTools {
		for _, reg := range registeredTools {
			if reg == t {
				resolverTool = t
				break
			}
		}
		if resolverTool != "" {
			break
		}
	}

	if resolverTool == "" {
		return TaskContextSiteHints{}
	}

	cacheKey := fmt.Sprintf("%s:%s:%s", organizationID, providerType, resolverTool)

	siteHintsMu.RLock()
	if entry, found := siteHintsCache[cacheKey]; found && time.Now().Before(entry.expiresAt) {
		siteHintsMu.RUnlock()
		return entry.hints
	}
	siteHintsMu.RUnlock()

	if caller == nil {
		return TaskContextSiteHints{}
	}

	rawResult, err := caller.CallTool(ctx, resolverTool, map[string]any{})
	if err != nil {
		if logger != nil {
			logger.Warn("Task context site resolution failed", "tool", resolverTool, "error", err)
		}
		return TaskContextSiteHints{}
	}

	hints := ExtractSiteHints(rawResult)
	if len(hints.SiteIDs) > 0 || len(hints.SiteURLs) > 0 {
		siteHintsMu.Lock()
		siteHintsCache[cacheKey] = cachedSiteEntry{
			hints:     hints,
			expiresAt: time.Now().Add(30 * time.Minute),
		}
		siteHintsMu.Unlock()
	}

	return hints
}

// ExtractSiteHints parses tenant lists from tool payloads.
func ExtractSiteHints(payload any) TaskContextSiteHints {
	var hints TaskContextSiteHints
	if payload == nil {
		return hints
	}

	var items []any
	switch v := payload.(type) {
	case []any:
		items = v
	case map[string]any:
		if res, ok := v["result"].([]any); ok {
			items = res
		} else if data, ok := v["data"].([]any); ok {
			items = data
		} else {
			items = []any{v}
		}
	}

	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			if id, ok := m["id"].(string); ok && id != "" {
				hints.SiteIDs = append(hints.SiteIDs, id)
			}
			if urlStr, ok := m["url"].(string); ok && urlStr != "" {
				hints.SiteURLs = append(hints.SiteURLs, urlStr)
			}
		}
	}

	hints.SiteIDs = UniqueNonEmpty(hints.SiteIDs)
	hints.SiteURLs = UniqueNonEmpty(hints.SiteURLs)
	return hints
}
