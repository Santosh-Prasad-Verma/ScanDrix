package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/llm"
)

// ProviderClient defines the execution interface for an inference backend.
type ProviderClient interface {
	Complete(ctx context.Context, req InferenceRequest, model string) (*InferenceResponse, error)
}

// FallbackRouter orchestrates multi-cloud LLM routing with resilience and circuit breaking.
type FallbackRouter struct {
	mu            sync.RWMutex
	clients       map[LLMProviderType]ProviderClient
	breakers      map[LLMProviderType]*llm.CircuitBreaker
	byokManager   *BYOKManager
	budgetLimiter *llm.TokenBudgetLimiter
}

// NewFallbackRouter initializes the multi-provider orchestrator.
func NewFallbackRouter(byok *BYOKManager) *FallbackRouter {
	router := &FallbackRouter{
		clients:     make(map[LLMProviderType]ProviderClient),
		breakers:    make(map[LLMProviderType]*llm.CircuitBreaker),
		byokManager: byok,
	}

	// NOTE: keep this list in sync with every Provider* constant used in
	// defaultCatalog (orchestrator/catalog.go). Missing an entry here just
	// means that provider never gets circuit-breaker protection — it won't
	// crash, but it defeats the purpose of the breaker.
	allProviders := []LLMProviderType{
		ProviderAnthropic, ProviderOpenAI, ProviderGemini, ProviderNovita,
		ProviderDeepSeek, ProviderBedrock, ProviderVertex, ProviderOpenRouter,
		ProviderOllama, ProviderVLLM,
		ProviderMoonshot, ProviderAlibaba, ProviderMiniMax, ProviderTencent,
		ProviderXAI, ProviderMistral,
		ProviderZAI, // Z.ai — GLM model family (Zhipu AI)
		ProviderMeta, ProviderNvidia, ProviderGroq, ProviderTogether,
		ProviderFireworks, ProviderDeepInfra, ProviderCerebras,
		ProviderSambaNova, ProviderCohere, ProviderPerplexity,
	}
	for _, p := range allProviders {
		router.breakers[p] = llm.NewCircuitBreaker(3, 5*time.Second)
	}

	return router
}

// RegisterClient associates a provider backend with the router.
func (r *FallbackRouter) RegisterClient(provider LLMProviderType, client ProviderClient) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[provider] = client
}

// SetBudgetLimiter assigns the token and burst rate limiter.
func (r *FallbackRouter) SetBudgetLimiter(limiter *llm.TokenBudgetLimiter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.budgetLimiter = limiter
}

// defaultPrimaryModel is the router's baseline model when no preference is
// given by the caller. Update this centrally when the "best default" shifts.
const defaultPrimaryModel = "claude-sonnet-5"

// defaultFallbackChain is used only when the caller supplies neither a
// PreferredModel nor a FallbackChain of their own.
var defaultFallbackChain = []string{"gpt-5.6-terra", "gemini-3.1-pro"}

// Execute orchestrates inference with automatic fallback across models.
func (r *FallbackRouter) Execute(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	startTime := time.Now()

	modelsToTry := make([]string, 0, 1+len(req.FallbackChain))
	if req.PreferredModel != "" {
		modelsToTry = append(modelsToTry, req.PreferredModel)
	} else {
		modelsToTry = append(modelsToTry, defaultPrimaryModel)
	}
	modelsToTry = append(modelsToTry, req.FallbackChain...)

	// Default fallback chain if none specified
	if len(modelsToTry) == 1 {
		if modelsToTry[0] == defaultPrimaryModel {
			modelsToTry = append(modelsToTry, defaultFallbackChain...)
		} else {
			// Caller picked a custom PreferredModel but gave no chain —
			// still fall back through our known-good baseline models.
			modelsToTry = append(modelsToTry, defaultPrimaryModel, defaultFallbackChain[len(defaultFallbackChain)-1])
		}
	}

	var lastErr error
	fallbackUsed := false

	for i, model := range modelsToTry {
		// 1. Model Entitlement Verification (Kodus parity)
		// Community/Free tier is restricted to low-cost/trial models unless the customer
		// brings their own BYOK API key. Pro/Team tier has access to standard frontier models.
		// Enterprise tier has access to all models including ultra-flagships.
		hasBYOK := req.TenantAPIKey != ""
		planTier := license.LicenseTier(req.PlanTier)
		if allowed, reason := license.CanAccessModel(planTier, model, hasBYOK); !allowed {
			lastErr = errors.New(reason)
			continue
		}

		// 2. Token Budget & Rate Limiting Enforcement
		r.mu.RLock()
		limiter := r.budgetLimiter
		r.mu.RUnlock()
		if limiter != nil && req.WorkspaceID != uuid.Nil {
			estimatedTokens := int64(req.MaxTokens)
			if estimatedTokens <= 0 {
				estimatedTokens = 1000
			}
			if err := limiter.ConsumeTokens(req.WorkspaceID, estimatedTokens); err != nil {
				return nil, fmt.Errorf("token rate limit exceeded: %w", err)
			}
		}

		profile, ok := GetModelProfile(model)
		if !ok {
			profile = defaultCatalog[defaultPrimaryModel]
		}

		r.mu.RLock()
		client, hasClient := r.clients[profile.Provider]
		breaker := r.breakers[profile.Provider]
		r.mu.RUnlock()

		if !hasClient {
			lastErr = fmt.Errorf("no provider registered for %s", profile.Provider)
			continue
		}

		if breaker == nil {
			// Provider wasn't wired into allProviders at startup — fail
			// loudly instead of nil-pointer panicking on breaker.Allow().
			lastErr = fmt.Errorf("no circuit breaker configured for provider %s", profile.Provider)
			continue
		}

		if !breaker.Allow() {
			lastErr = fmt.Errorf("circuit breaker open for provider %s", profile.Provider)
			fallbackUsed = true
			continue
		}

		resp, err := client.Complete(ctx, req, model)
		if err != nil {
			breaker.RecordFailure()
			lastErr = err
			fallbackUsed = true
			continue
		}

		breaker.RecordSuccess()
		resp.ModelUsed = model
		resp.ProviderUsed = profile.Provider
		resp.Latency = time.Since(startTime)
		resp.FallbackOccurred = (i > 0) || fallbackUsed

		// Compute exact token dollar cost
		if resp.CostUSD == 0 {
			resp.CostUSD = CalculateCost(model, resp.PromptTokens, resp.CompletionTokens)
		}
		resp.TotalTokens = resp.PromptTokens + resp.CompletionTokens

		return resp, nil
	}

	return nil, fmt.Errorf("all inference providers failed in fallback chain: %w", lastErr)
}

// GetBYOKManager returns the cryptographic credential manager.
func (r *FallbackRouter) GetBYOKManager() *BYOKManager {
	return r.byokManager
}

// MockProviderClient provides testing stubs for unit and integration verification.
type MockProviderClient struct {
	ShouldFail bool
	Content    string
	PromptTok  int
	CompTok    int
}

func (m *MockProviderClient) Complete(ctx context.Context, req InferenceRequest, model string) (*InferenceResponse, error) {
	if m.ShouldFail {
		return nil, errors.New("simulated upstream 503 service unavailable")
	}

	promptTok := m.PromptTok
	if promptTok == 0 {
		promptTok = 250
	}
	compTok := m.CompTok
	if compTok == 0 {
		compTok = 100
	}

	content := m.Content
	if content == "" {
		content = "Code analysis complete: No critical security regressions found."
	}

	return &InferenceResponse{
		Content:          content,
		PromptTokens:     promptTok,
		CompletionTokens: compTok,
	}, nil
}
