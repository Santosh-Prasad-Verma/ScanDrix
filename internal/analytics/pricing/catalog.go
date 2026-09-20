package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	ModelsDevURL     = "https://models.dev/api.json"
	LiteLLMPricingURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

	DefaultCacheTTL     = 24 * time.Hour
	DefaultFetchTimeout = 15 * time.Second
	MaxCatalogBytes     = 64 * 1024 * 1024 // 64MB protection against memory exhaustion
	PerMillion          = 1_000_000.0

	TierThreshold200K = 200_000
)

var NativeProviders = map[string]bool{
	"openai":         true,
	"anthropic":      true,
	"google":         true,
	"google-vertex":  true,
	"google-ai-studio": true,
	"moonshotai":     true,
	"zhipuai":        true,
	"z-ai":           true,
	"deepseek":       true,
	"xai":            true,
	"alibaba":        true,
	"mistral":        true,
	"meta":           true,
	"meta-llama":     true,
	"cohere":         true,
	"amazon-bedrock": true,
}

// CatalogEntry represents a source-agnostic normalized catalog item with per-token rates.
type CatalogEntry struct {
	Provider   string     `json:"provider,omitempty"`
	Input      TokenRate  `json:"input"`
	Output     TokenRate  `json:"output"`
	CacheRead  TokenRate  `json:"cacheRead"`
	CacheWrite TokenRate  `json:"cacheWrite"`
	Reasoning  *float64   `json:"reasoning,omitempty"`
}

// ModelPricingInfo is normalized pricing for a single model returned by the catalog.
type ModelPricingInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider,omitempty"`
	Pricing  struct {
		Input             TokenRate `json:"input"`
		Output            TokenRate `json:"output"`
		CacheRead         TokenRate `json:"cacheRead"`
		CacheWrite        TokenRate `json:"cacheWrite"`
		Prompt            float64   `json:"prompt"`
		Completion        float64   `json:"completion"`
		InternalReasoning float64   `json:"internal_reasoning"`
	} `json:"pricing"`
}

// PricingCatalog maps model keys to catalog entries.
type PricingCatalog map[string]CatalogEntry

// TokenPricingCatalog manages embedded and upstream pricing catalogs.
type TokenPricingCatalog struct {
	mu                 sync.RWMutex
	httpClient         *http.Client
	onlineFetching     bool
	modelsDevCatalog   PricingCatalog
	modelsDevFetchedAt time.Time
	liteLLMCatalog     PricingCatalog
	liteLLMFetchedAt   time.Time
	embeddedCatalog    PricingCatalog
}

// CatalogOption configures TokenPricingCatalog.
type CatalogOption func(*TokenPricingCatalog)

// WithOnlineFetching toggles upstream fetching from models.dev and litellm.
func WithOnlineFetching(enable bool) CatalogOption {
	return func(c *TokenPricingCatalog) {
		c.onlineFetching = enable
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) CatalogOption {
	return func(c *TokenPricingCatalog) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// NewTokenPricingCatalog initializes a catalog with comprehensive embedded model rates.
func NewTokenPricingCatalog(opts ...CatalogOption) *TokenPricingCatalog {
	c := &TokenPricingCatalog{
		httpClient: &http.Client{
			Timeout: DefaultFetchTimeout,
		},
		onlineFetching:  true,
		embeddedCatalog: make(PricingCatalog),
	}
	c.loadEmbeddedDefaults()
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Execute retrieves normalized pricing for a model.
func (c *TokenPricingCatalog) Execute(ctx context.Context, model string, provider string) ModelPricingInfo {
	info, err := c.GetModelInfo(ctx, model, provider)
	if err != nil || info == nil {
		return c.emptyPricing(model, provider)
	}
	return *info
}

// ExecuteMany batch resolves model pricing for multiple models.
func (c *TokenPricingCatalog) ExecuteMany(ctx context.Context, models []string, provider string) map[string]ModelPricingInfo {
	res := make(map[string]ModelPricingInfo, len(models))
	seen := make(map[string]bool)

	for _, m := range models {
		clean := strings.TrimSpace(m)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		res[clean] = c.Execute(ctx, clean, provider)
	}
	return res
}

// TieredInputThresholds returns canonical model names -> sorted input tier thresholds.
func (c *TokenPricingCatalog) TieredInputThresholds(ctx context.Context) map[string][]int64 {
	out := make(map[string][]int64)

	catalogs := []PricingCatalog{c.embeddedCatalog}
	if devCat, err := c.GetModelsDevCatalog(ctx); err == nil && len(devCat) > 0 {
		catalogs = append(catalogs, devCat)
	} else if liteCat, err := c.GetLiteLLMCatalog(ctx); err == nil && len(liteCat) > 0 {
		catalogs = append(catalogs, liteCat)
	}

	for _, cat := range catalogs {
		for key, entry := range cat {
			if len(entry.Input.Tiers) == 0 {
				continue
			}
			thresholds := make([]int64, len(entry.Input.Tiers))
			for i, t := range entry.Input.Tiers {
				thresholds[i] = t.Threshold
			}
			for _, name := range c.canonicalNames(key) {
				out[name] = thresholds
			}
		}
	}
	return out
}

// CanonicalNames returns variants of a model id (colon-stripped, full, and bare last segment).
func (c *TokenPricingCatalog) canonicalNames(key string) []string {
	colonStripped := CanonicalModelID(key)
	parts := strings.Split(colonStripped, "/")
	bare := parts[len(parts)-1]
	if colonStripped == bare {
		return []string{colonStripped}
	}
	return []string{colonStripped, bare}
}

// CanonicalModelID strips provider prefix and bedrock version suffixes.
func CanonicalModelID(model string) string {
	m := strings.TrimSpace(model)
	if idx := strings.Index(m, ":"); idx != -1 && !strings.Contains(m[:idx], "/") {
		m = m[idx+1:]
	}
	// Bedrock often ends with :0 or similar version suffix
	if idx := strings.LastIndex(m, ":"); idx != -1 {
		suffix := m[idx+1:]
		if len(suffix) <= 2 && unicode.IsDigit(rune(suffix[0])) {
			m = m[:idx]
		}
	}
	return m
}

// GetModelInfo looks up pricing across models.dev, litellm, and embedded defaults.
func (c *TokenPricingCatalog) GetModelInfo(ctx context.Context, model string, provider string) (*ModelPricingInfo, error) {
	clean := strings.TrimSpace(model)
	if clean == "" {
		return nil, fmt.Errorf("empty model identifier")
	}

	if provider == "" && strings.Contains(clean, "/") {
		parts := strings.Split(clean, "/")
		if len(parts) > 1 {
			provider = parts[0]
		}
	}

	if c.onlineFetching {
		// 1. Try models.dev
		if devCat, err := c.GetModelsDevCatalog(ctx); err == nil && devCat != nil {
			if _, entry := c.lookupModel(devCat, clean, provider); entry != nil {
				info := c.toPricingInfo(clean, *entry, provider)
				return &info, nil
			}
		}

		// 2. Try LiteLLM
		if liteCat, err := c.GetLiteLLMCatalog(ctx); err == nil && liteCat != nil {
			if _, entry := c.lookupModel(liteCat, clean, provider); entry != nil {
				info := c.toPricingInfo(clean, *entry, provider)
				return &info, nil
			}
		}
	}

	// 3. Try embedded defaults
	if _, entry := c.lookupModel(c.embeddedCatalog, clean, provider); entry != nil {
		info := c.toPricingInfo(clean, *entry, provider)
		return &info, nil
	}

	return nil, fmt.Errorf("model %q not found in pricing catalogs", clean)
}

func (c *TokenPricingCatalog) lookupModel(catalog PricingCatalog, model string, provider string) (string, *CatalogEntry) {
	if len(catalog) == 0 || model == "" {
		return "", nil
	}

	normalized := strings.TrimSpace(model)
	lowered := strings.ToLower(normalized)

	// Provider separator may be ':' (Scandrix internal BYOK format) or '/'
	colonNormalized := lowered
	if idx := strings.Index(lowered, ":"); idx != -1 && !strings.Contains(lowered[:idx], "/") {
		colonNormalized = strings.Replace(lowered, ":", "/", 1)
	}

	withoutPrefix := colonNormalized
	if strings.Contains(colonNormalized, "/") {
		parts := strings.Split(colonNormalized, "/")
		withoutPrefix = strings.Join(parts[1:], "/")
	}

	parts := strings.Split(colonNormalized, "/")
	bareLastSegment := parts[len(parts)-1]

	direct := []string{
		normalized,
		lowered,
		colonNormalized,
		withoutPrefix,
		bareLastSegment,
	}

	for _, key := range direct {
		if entry, ok := catalog[key]; ok {
			return key, &entry
		}
	}

	if provider != "" {
		pLower := strings.ToLower(provider)
		pVariants := []string{
			pLower,
			strings.ReplaceAll(pLower, "google-vertex", "vertex_ai"),
			strings.ReplaceAll(pLower, "google-gemini", "gemini"),
			strings.ReplaceAll(pLower, "google-gemini", "google"),
		}
		for _, prov := range pVariants {
			for _, key := range direct {
				cand := fmt.Sprintf("%s/%s", prov, key)
				if entry, ok := catalog[cand]; ok {
					return cand, &entry
				}
			}
		}
	}

	// Prefix fallback — require at least one digit in the model id to prevent matching family stems (like 'gpt' -> 'gpt-4o')
	hasDigit := false
	for _, r := range withoutPrefix {
		if unicode.IsDigit(r) {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return "", nil
	}

	var bestKey string
	var bestSegLen int = 999999

	for key := range catalog {
		keyLower := strings.ToLower(key)
		segParts := strings.Split(keyLower, "/")
		seg := segParts[len(segParts)-1]

		if !strings.HasPrefix(keyLower, withoutPrefix) && !strings.HasPrefix(seg, withoutPrefix) {
			continue
		}

		if bestKey == "" || len(seg) < bestSegLen || (len(seg) == bestSegLen && key < bestKey) {
			bestKey = key
			bestSegLen = len(seg)
		}
	}

	if bestKey != "" {
		entry := catalog[bestKey]
		return bestKey, &entry
	}

	return "", nil
}

func (c *TokenPricingCatalog) toPricingInfo(id string, entry CatalogEntry, provider string) ModelPricingInfo {
	var info ModelPricingInfo
	info.ID = id
	if provider != "" {
		info.Provider = provider
	} else {
		info.Provider = entry.Provider
	}

	info.Pricing.Input = entry.Input
	info.Pricing.Output = entry.Output
	info.Pricing.CacheRead = entry.CacheRead
	info.Pricing.CacheWrite = entry.CacheWrite

	info.Pricing.Prompt = entry.Input.Default
	info.Pricing.Completion = entry.Output.Default
	if entry.Reasoning != nil {
		info.Pricing.InternalReasoning = *entry.Reasoning
	} else {
		info.Pricing.InternalReasoning = entry.Output.Default
	}

	return info
}

func (c *TokenPricingCatalog) emptyPricing(id string, provider string) ModelPricingInfo {
	var info ModelPricingInfo
	info.ID = id
	info.Provider = provider
	return info
}

// GetModelsDevCatalog fetches or returns cached models.dev catalog.
func (c *TokenPricingCatalog) GetModelsDevCatalog(ctx context.Context) (PricingCatalog, error) {
	if !c.onlineFetching {
		return nil, nil
	}
	c.mu.RLock()
	if c.modelsDevCatalog != nil && time.Since(c.modelsDevFetchedAt) < DefaultCacheTTL {
		cat := c.modelsDevCatalog
		c.mu.RUnlock()
		return cat, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.modelsDevCatalog != nil && time.Since(c.modelsDevFetchedAt) < DefaultCacheTTL {
		return c.modelsDevCatalog, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ModelsDevURL, nil)
	if err != nil {
		if c.modelsDevCatalog != nil {
			return c.modelsDevCatalog, nil
		}
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.modelsDevCatalog != nil {
			return c.modelsDevCatalog, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if c.modelsDevCatalog != nil {
			return c.modelsDevCatalog, nil
		}
		return nil, fmt.Errorf("models.dev returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxCatalogBytes))
	if err != nil {
		if c.modelsDevCatalog != nil {
			return c.modelsDevCatalog, nil
		}
		return nil, err
	}

	var raw map[string]struct {
		ID     string `json:"id"`
		Models map[string]struct {
			ID   string `json:"id"`
			Cost struct {
				Input           *float64 `json:"input"`
				Output          *float64 `json:"output"`
				CacheRead       *float64 `json:"cache_read"`
				CacheWrite      *float64 `json:"cache_write"`
				Reasoning       *float64 `json:"reasoning"`
				ContextOver200K *struct {
					Input      *float64 `json:"input"`
					Output     *float64 `json:"output"`
					CacheRead  *float64 `json:"cache_read"`
					CacheWrite *float64 `json:"cache_write"`
				} `json:"context_over_200k"`
				Tiers []struct {
					Input      *float64 `json:"input"`
					Output     *float64 `json:"output"`
					CacheRead  *float64 `json:"cache_read"`
					CacheWrite *float64 `json:"cache_write"`
					Tier       struct {
						Type string `json:"type"`
						Size int64  `json:"size"`
					} `json:"tier"`
				} `json:"tiers"`
			} `json:"cost"`
		} `json:"models"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		if c.modelsDevCatalog != nil {
			return c.modelsDevCatalog, nil
		}
		return nil, err
	}

	catalog := make(PricingCatalog)
	bareOwner := make(map[string]string)

	for providerKey, provider := range raw {
		pID := strings.ToLower(provider.ID)
		if pID == "" {
			pID = strings.ToLower(providerKey)
		}

		for modelKey, model := range provider.Models {
			mID := strings.ToLower(model.ID)
			if mID == "" {
				mID = strings.ToLower(modelKey)
			}

			entry := c.fromModelsDevCost(model.Cost, pID)
			catalog[fmt.Sprintf("%s/%s", pID, mID)] = entry

			if c.claimBareAlias(catalog, bareOwner, mID, entry) {
				bareOwner[mID] = pID
			}
		}
	}

	c.modelsDevCatalog = catalog
	c.modelsDevFetchedAt = time.Now()
	return catalog, nil
}

func (c *TokenPricingCatalog) fromModelsDevCost(cost any, provider string) CatalogEntry {
	// Parse cost details
	b, _ := json.Marshal(cost)
	var parsed struct {
		Input           *float64 `json:"input"`
		Output          *float64 `json:"output"`
		CacheRead       *float64 `json:"cache_read"`
		CacheWrite      *float64 `json:"cache_write"`
		Reasoning       *float64 `json:"reasoning"`
		ContextOver200K *struct {
			Input      *float64 `json:"input"`
			Output     *float64 `json:"output"`
			CacheRead  *float64 `json:"cache_read"`
			CacheWrite *float64 `json:"cache_write"`
		} `json:"context_over_200k"`
		Tiers []struct {
			Input      *float64 `json:"input"`
			Output     *float64 `json:"output"`
			CacheRead  *float64 `json:"cache_read"`
			CacheWrite *float64 `json:"cache_write"`
			Tier       struct {
				Type string `json:"type"`
				Size int64  `json:"size"`
			} `json:"tier"`
		} `json:"tiers"`
	}
	_ = json.Unmarshal(b, &parsed)

	perToken := func(pm *float64) float64 {
		if pm == nil {
			return 0
		}
		return *pm / PerMillion
	}

	entry := CatalogEntry{
		Provider: provider,
		Input: TokenRate{
			Default: perToken(parsed.Input),
		},
		Output: TokenRate{
			Default: perToken(parsed.Output),
		},
		CacheRead: TokenRate{
			Default: perToken(parsed.CacheRead),
		},
		CacheWrite: TokenRate{
			Default: perToken(parsed.CacheWrite),
		},
	}

	if parsed.Reasoning != nil {
		r := perToken(parsed.Reasoning)
		entry.Reasoning = &r
	}

	// Context tiers
	if len(parsed.Tiers) > 0 {
		for _, t := range parsed.Tiers {
			if t.Tier.Size > 0 {
				if t.Input != nil {
					entry.Input.Tiers = append(entry.Input.Tiers, TierRate{Threshold: t.Tier.Size, Rate: perToken(t.Input)})
				}
				if t.Output != nil {
					entry.Output.Tiers = append(entry.Output.Tiers, TierRate{Threshold: t.Tier.Size, Rate: perToken(t.Output)})
				}
				if t.CacheRead != nil {
					entry.CacheRead.Tiers = append(entry.CacheRead.Tiers, TierRate{Threshold: t.Tier.Size, Rate: perToken(t.CacheRead)})
				}
				if t.CacheWrite != nil {
					entry.CacheWrite.Tiers = append(entry.CacheWrite.Tiers, TierRate{Threshold: t.Tier.Size, Rate: perToken(t.CacheWrite)})
				}
			}
		}
	} else if parsed.ContextOver200K != nil {
		c2 := parsed.ContextOver200K
		if c2.Input != nil {
			entry.Input.Tiers = append(entry.Input.Tiers, TierRate{Threshold: TierThreshold200K, Rate: perToken(c2.Input)})
		}
		if c2.Output != nil {
			entry.Output.Tiers = append(entry.Output.Tiers, TierRate{Threshold: TierThreshold200K, Rate: perToken(c2.Output)})
		}
		if c2.CacheRead != nil {
			entry.CacheRead.Tiers = append(entry.CacheRead.Tiers, TierRate{Threshold: TierThreshold200K, Rate: perToken(c2.CacheRead)})
		}
		if c2.CacheWrite != nil {
			entry.CacheWrite.Tiers = append(entry.CacheWrite.Tiers, TierRate{Threshold: TierThreshold200K, Rate: perToken(c2.CacheWrite)})
		}
	}

	sortTiers := func(tiers []TierRate) []TierRate {
		sort.Slice(tiers, func(i, j int) bool {
			return tiers[i].Threshold < tiers[j].Threshold
		})
		return tiers
	}
	entry.Input.Tiers = sortTiers(entry.Input.Tiers)
	entry.Output.Tiers = sortTiers(entry.Output.Tiers)
	entry.CacheRead.Tiers = sortTiers(entry.CacheRead.Tiers)
	entry.CacheWrite.Tiers = sortTiers(entry.CacheWrite.Tiers)

	return entry
}

func (c *TokenPricingCatalog) aliasRank(entry CatalogEntry) int {
	priced := entry.Input.Default > 0 || entry.Output.Default > 0
	rich := len(entry.Input.Tiers) > 0 || len(entry.Output.Tiers) > 0 || entry.CacheRead.Default > 0 || entry.CacheWrite.Default > 0 || entry.Reasoning != nil
	native := NativeProviders[entry.Provider]

	rank := 0
	if priced {
		rank += 4
	}
	if rich {
		rank += 2
	}
	if native {
		rank += 1
	}
	return rank
}

func (c *TokenPricingCatalog) claimBareAlias(catalog PricingCatalog, bareOwner map[string]string, modelID string, entry CatalogEntry) bool {
	_, exists := bareOwner[modelID]
	if !exists {
		catalog[modelID] = entry
		bareOwner[modelID] = entry.Provider
		return true
	}
	existingEntry := catalog[modelID]
	if c.aliasRank(entry) > c.aliasRank(existingEntry) {
		catalog[modelID] = entry
		bareOwner[modelID] = entry.Provider
		return true
	}
	return false
}

// GetLiteLLMCatalog fetches or returns cached LiteLLM catalog.
func (c *TokenPricingCatalog) GetLiteLLMCatalog(ctx context.Context) (PricingCatalog, error) {
	if !c.onlineFetching {
		return nil, nil
	}
	c.mu.RLock()
	if c.liteLLMCatalog != nil && time.Since(c.liteLLMFetchedAt) < DefaultCacheTTL {
		cat := c.liteLLMCatalog
		c.mu.RUnlock()
		return cat, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.liteLLMCatalog != nil && time.Since(c.liteLLMFetchedAt) < DefaultCacheTTL {
		return c.liteLLMCatalog, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LiteLLMPricingURL, nil)
	if err != nil {
		if c.liteLLMCatalog != nil {
			return c.liteLLMCatalog, nil
		}
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.liteLLMCatalog != nil {
			return c.liteLLMCatalog, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if c.liteLLMCatalog != nil {
			return c.liteLLMCatalog, nil
		}
		return nil, fmt.Errorf("litellm pricing returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxCatalogBytes))
	if err != nil {
		if c.liteLLMCatalog != nil {
			return c.liteLLMCatalog, nil
		}
		return nil, err
	}

	var raw map[string]struct {
		InputCostPerToken                      *float64 `json:"input_cost_per_token"`
		InputCostPerTokenAbove200k             *float64 `json:"input_cost_per_token_above_200k_tokens"`
		OutputCostPerToken                     *float64 `json:"output_cost_per_token"`
		OutputCostPerTokenAbove200k            *float64 `json:"output_cost_per_token_above_200k_tokens"`
		CacheReadInputTokenCost                *float64 `json:"cache_read_input_token_cost"`
		CacheReadInputTokenCostAbove200k       *float64 `json:"cache_read_input_token_cost_above_200k_tokens"`
		CacheCreationInputTokenCost            *float64 `json:"cache_creation_input_token_cost"`
		CacheCreationInputTokenCostAbove200k   *float64 `json:"cache_creation_input_token_cost_above_200k_tokens"`
		LiteLLMProvider                        string   `json:"litellm_provider"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		if c.liteLLMCatalog != nil {
			return c.liteLLMCatalog, nil
		}
		return nil, err
	}

	catalog := make(PricingCatalog, len(raw))
	for key, item := range raw {
		entry := CatalogEntry{
			Provider: item.LiteLLMProvider,
		}
		if item.InputCostPerToken != nil {
			entry.Input.Default = *item.InputCostPerToken
		}
		if item.InputCostPerTokenAbove200k != nil {
			entry.Input.Tiers = []TierRate{{Threshold: TierThreshold200K, Rate: *item.InputCostPerTokenAbove200k}}
		}
		if item.OutputCostPerToken != nil {
			entry.Output.Default = *item.OutputCostPerToken
		}
		if item.OutputCostPerTokenAbove200k != nil {
			entry.Output.Tiers = []TierRate{{Threshold: TierThreshold200K, Rate: *item.OutputCostPerTokenAbove200k}}
		}
		if item.CacheReadInputTokenCost != nil {
			entry.CacheRead.Default = *item.CacheReadInputTokenCost
		}
		if item.CacheReadInputTokenCostAbove200k != nil {
			entry.CacheRead.Tiers = []TierRate{{Threshold: TierThreshold200K, Rate: *item.CacheReadInputTokenCostAbove200k}}
		}
		if item.CacheCreationInputTokenCost != nil {
			entry.CacheWrite.Default = *item.CacheCreationInputTokenCost
		}
		if item.CacheCreationInputTokenCostAbove200k != nil {
			entry.CacheWrite.Tiers = []TierRate{{Threshold: TierThreshold200K, Rate: *item.CacheCreationInputTokenCostAbove200k}}
		}
		catalog[key] = entry
	}

	c.liteLLMCatalog = catalog
	c.liteLLMFetchedAt = time.Now()
	return catalog, nil
}

// loadEmbeddedDefaults seeds full baseline pricing for offline reliability.
func (c *TokenPricingCatalog) loadEmbeddedDefaults() {
	pm := func(usdPer1M float64) float64 {
		return usdPer1M / PerMillion
	}

	add := func(id, provider string, in, out, cacheR, cacheW float64, in200k, out200k *float64) {
		entry := CatalogEntry{
			Provider: provider,
			Input: TokenRate{
				Default: pm(in),
			},
			Output: TokenRate{
				Default: pm(out),
			},
			CacheRead: TokenRate{
				Default: pm(cacheR),
			},
			CacheWrite: TokenRate{
				Default: pm(cacheW),
			},
		}
		if in200k != nil {
			entry.Input.Tiers = []TierRate{{Threshold: TierThreshold200K, Rate: pm(*in200k)}}
		}
		if out200k != nil {
			entry.Output.Tiers = []TierRate{{Threshold: TierThreshold200K, Rate: pm(*out200k)}}
		}

		c.embeddedCatalog[id] = entry
		c.embeddedCatalog[fmt.Sprintf("%s/%s", provider, id)] = entry
	}

	fPtr := func(v float64) *float64 { return &v }

	// Anthropic Claude
	add("claude-3-5-sonnet-20241022", "anthropic", 3.00, 15.00, 0.30, 3.75, nil, nil)
	add("claude-3-5-sonnet", "anthropic", 3.00, 15.00, 0.30, 3.75, nil, nil)
	add("claude-3-7-sonnet", "anthropic", 3.00, 15.00, 0.30, 3.75, nil, nil)
	add("claude-3-5-haiku", "anthropic", 0.80, 4.00, 0.08, 1.00, nil, nil)
	add("claude-3-opus", "anthropic", 15.00, 75.00, 1.50, 18.75, nil, nil)

	// OpenAI
	add("gpt-4o", "openai", 2.50, 10.00, 1.25, 0, nil, nil)
	add("gpt-4o-mini", "openai", 0.15, 0.60, 0.075, 0, nil, nil)
	add("gpt-4.5-preview", "openai", 75.00, 150.00, 37.50, 0, nil, nil)
	add("o1", "openai", 15.00, 60.00, 7.50, 0, nil, nil)
	add("o1-mini", "openai", 1.10, 4.40, 0.55, 0, nil, nil)
	add("o3-mini", "openai", 1.10, 4.40, 0.55, 0, nil, nil)

	// Google Gemini (with 200k context tiers)
	add("gemini-2.5-pro", "google", 1.25, 5.00, 0.3125, 0, fPtr(2.50), fPtr(10.00))
	add("gemini-2.0-flash", "google", 0.10, 0.40, 0.025, 0, nil, nil)
	add("gemini-1.5-pro", "google", 1.25, 5.00, 0.3125, 0, fPtr(2.50), fPtr(10.00))
	add("gemini-1.5-flash", "google", 0.075, 0.30, 0.01875, 0, fPtr(0.15), fPtr(0.60))

	// DeepSeek
	add("deepseek-chat", "deepseek", 0.14, 0.28, 0.014, 0, nil, nil)
	add("deepseek-coder", "deepseek", 0.14, 0.28, 0.014, 0, nil, nil)
	add("deepseek-reasoner", "deepseek", 0.55, 2.19, 0.14, 0, nil, nil)

	// Mistral
	add("mistral-large-latest", "mistral", 2.00, 6.00, 0, 0, nil, nil)
	add("mistral-small-latest", "mistral", 0.20, 0.60, 0, 0, nil, nil)
	add("codestral-latest", "mistral", 0.30, 0.90, 0, 0, nil, nil)

	// Meta Llama / Groq / Fireworks / Together
	add("llama-3.3-70b", "meta", 0.59, 0.79, 0, 0, nil, nil)
	add("llama-3.1-405b", "meta", 3.00, 3.00, 0, 0, nil, nil)

	// Moonshot Kimi
	add("kimi-k2.6", "moonshotai", 0.60, 2.40, 0.15, 0, nil, nil)
	add("kimi-k2.5", "moonshotai", 0.50, 2.00, 0.125, 0, nil, nil)
}
