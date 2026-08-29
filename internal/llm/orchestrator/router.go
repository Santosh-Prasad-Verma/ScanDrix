package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/llm"
)

// ProviderClient defines the execution interface for an inference backend.
type ProviderClient interface {
	Complete(ctx context.Context, req InferenceRequest, model string) (*InferenceResponse, error)
}

// FallbackRouter orchestrates multi-cloud LLM routing with resilience and circuit breaking.
type FallbackRouter struct {
	mu          sync.RWMutex
	clients     map[LLMProviderType]ProviderClient
	breakers    map[LLMProviderType]*llm.CircuitBreaker
	byokManager *BYOKManager
}

// NewFallbackRouter initializes the multi-provider orchestrator.
func NewFallbackRouter(byok *BYOKManager) *FallbackRouter {
	router := &FallbackRouter{
		clients:     make(map[LLMProviderType]ProviderClient),
		breakers:    make(map[LLMProviderType]*llm.CircuitBreaker),
		byokManager: byok,
	}

	for _, p := range []LLMProviderType{ProviderAnthropic, ProviderOpenAI, ProviderGemini, ProviderNovita} {
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

// Execute orchestrates inference with automatic fallback across models.
func (r *FallbackRouter) Execute(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	startTime := time.Now()

	modelsToTry := make([]string, 0, 1+len(req.FallbackChain))
	if req.PreferredModel != "" {
		modelsToTry = append(modelsToTry, req.PreferredModel)
	} else {
		modelsToTry = append(modelsToTry, "claude-3-7-sonnet")
	}
	modelsToTry = append(modelsToTry, req.FallbackChain...)

	// Default fallback chain if none specified
	if len(modelsToTry) == 1 {
		if modelsToTry[0] == "claude-3-7-sonnet" {
			modelsToTry = append(modelsToTry, "gpt-4o", "gemini-2.5-pro")
		} else {
			modelsToTry = append(modelsToTry, "claude-3-7-sonnet", "gemini-2.5-pro")
		}
	}

	var lastErr error
	fallbackUsed := false

	for i, model := range modelsToTry {
		profile, ok := GetModelProfile(model)
		if !ok {
			profile = defaultCatalog["gpt-4o"]
		}

		r.mu.RLock()
		client, hasClient := r.clients[profile.Provider]
		breaker := r.breakers[profile.Provider]
		r.mu.RUnlock()

		if !hasClient {
			lastErr = fmt.Errorf("no provider registered for %s", profile.Provider)
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
