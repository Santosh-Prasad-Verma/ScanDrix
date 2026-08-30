package orchestrator_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm/orchestrator"
)

func TestModelPricingAndCapabilityCatalog(t *testing.T) {
	profile, ok := orchestrator.GetModelProfile("claude-3-7-sonnet")
	if !ok || !profile.SupportsThinking {
		t.Fatalf("expected claude-3-7-sonnet profile with thinking, got %+v", profile)
	}

	// Test exact pricing calculation
	// 1,000,000 prompt tokens @ $3.00 + 1,000,000 completion tokens @ $15.00 = $18.00
	cost := orchestrator.CalculateCost("claude-3-7-sonnet", 1_000_000, 1_000_000)
	if cost != 18.00 {
		t.Fatalf("expected cost 18.00, got %f", cost)
	}

	// 500 prompt tokens + 200 completion tokens on gpt-4o ($2.50 / $10.00)
	// (500/1M)*2.5 + (200/1M)*10 = 0.00125 + 0.002 = 0.00325
	costGpt := orchestrator.CalculateCost("gpt-4o", 500, 200)
	if costGpt != 0.00325 {
		t.Fatalf("expected cost 0.00325, got %f", costGpt)
	}
}

func TestBYOKEncryptionAndDecryption(t *testing.T) {
	wsID := uuid.New()
	byok, err := orchestrator.NewBYOKManager("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("failed initializing BYOK manager: %v", err)
	}

	rawKey := "sk-ant-api03-abcdef1234567890abcdef1234567890"

	cred, err := byok.EncryptKey(wsID, orchestrator.ProviderAnthropic, rawKey)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	// Verify masking
	if !strings.HasPrefix(cred.MaskedKey, "sk-a...") || !strings.HasSuffix(cred.MaskedKey, "7890") {
		t.Fatalf("unexpected masked key format: %s", cred.MaskedKey)
	}

	// Decrypt key
	decrypted, err := byok.DecryptKey(cred)
	if err != nil || decrypted != rawKey {
		t.Fatalf("decrypted key '%s' does not match original '%s', err: %v", decrypted, rawKey, err)
	}

	// Verify fingerprint matching
	if !byok.ValidateFingerprint(cred, rawKey) {
		t.Fatalf("expected fingerprint match for raw key")
	}
	if byok.ValidateFingerprint(cred, "invalid-key") {
		t.Fatalf("expected fingerprint mismatch for wrong key")
	}

	// Verify key rotation
	newKey := "sk-ant-api03-0987654321fedcba0987654321fedcba"
	rotated, err := byok.RotateKey(wsID, orchestrator.ProviderAnthropic, cred, newKey)
	if err != nil {
		t.Fatalf("key rotation failed: %v", err)
	}
	if rotated.CreatedAt != cred.CreatedAt {
		t.Fatalf("expected original CreatedAt timestamp preserved across rotation")
	}
	decryptedNew, err := byok.DecryptKey(rotated)
	if err != nil || decryptedNew != newKey {
		t.Fatalf("expected decrypted rotated key to match newKey: %v", err)
	}
}

func TestFallbackRouterExecutionAndResilience(t *testing.T) {
	ctx := context.Background()
	byok, _ := orchestrator.NewBYOKManager("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	router := orchestrator.NewFallbackRouter(byok)

	anthropicClient := &orchestrator.MockProviderClient{
		ShouldFail: true, // Primary fails!
	}
	openaiClient := &orchestrator.MockProviderClient{
		ShouldFail: false, // Fallback succeeds!
		Content:    "OpenAI fallback: Patch reviewed successfully.",
		PromptTok:  400,
		CompTok:    150,
	}

	router.RegisterClient(orchestrator.ProviderAnthropic, anthropicClient)
	router.RegisterClient(orchestrator.ProviderOpenAI, openaiClient)

	req := orchestrator.InferenceRequest{
		WorkspaceID:    uuid.New(),
		PreferredModel: "claude-3-7-sonnet",
		PlanTier:       "TEAM",
		FallbackChain:  []string{"gpt-4o"},
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleUser, Content: "Review diff"},
		},
	}

	resp, err := router.Execute(ctx, req)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}

	// Verify fallback occurred
	if !resp.FallbackOccurred {
		t.Fatalf("expected FallbackOccurred=true")
	}
	if resp.ModelUsed != "gpt-4o" || resp.ProviderUsed != orchestrator.ProviderOpenAI {
		t.Fatalf("expected gpt-4o on openai, got model=%s, provider=%s", resp.ModelUsed, resp.ProviderUsed)
	}
	if resp.TotalTokens != 550 {
		t.Fatalf("expected total tokens 550, got %d", resp.TotalTokens)
	}
	if resp.CostUSD <= 0 {
		t.Fatalf("expected positive cost USD, got %f", resp.CostUSD)
	}
}

func TestDynamicHeuristicModelResolution(t *testing.T) {
	testCases := []struct {
		model            string
		expectedProvider orchestrator.LLMProviderType
		minContext       int
	}{
		{"claude opus 4.8", orchestrator.ProviderAnthropic, 1_000_000},
		{"opus-4.8", orchestrator.ProviderAnthropic, 1_000_000},
		{"gemini 3.5", orchestrator.ProviderGemini, 1_000_000},
		{"gemini-3.6-flash", orchestrator.ProviderGemini, 1_000_000},
		{"gpt 5.6 sol", orchestrator.ProviderOpenAI, 1_000_000},
		{"gpt-5.5-tera", orchestrator.ProviderOpenAI, 1_000_000},
		{"gpt-5.4-luna", orchestrator.ProviderOpenAI, 1_000_000},
		{"deepseek v4 pro", orchestrator.ProviderDeepSeek, 128_000},
		{"qwen 3.8 max", orchestrator.ProviderAlibaba, 1_000_000},
		{"kimi k3", orchestrator.ProviderMoonshot, 262_144},
		{"moonshot/kimi-k3", orchestrator.ProviderOpenRouter, 1_000_000},
	}

	for _, tc := range testCases {
		profile, ok := orchestrator.GetModelProfile(tc.model)
		if !ok {
			t.Errorf("model %s: expected ok=true", tc.model)
		}
		if profile.Provider != tc.expectedProvider {
			t.Errorf("model %s: expected provider %s, got %s", tc.model, tc.expectedProvider, profile.Provider)
		}
		if profile.ContextWindow < tc.minContext {
			t.Errorf("model %s: expected context window >= %d, got %d", tc.model, tc.minContext, profile.ContextWindow)
		}
		if !profile.SupportsThinking {
			t.Errorf("model %s: expected reasoning/thinking support", tc.model)
		}
	}
}

func TestFallbackRouterFrontierProviders(t *testing.T) {
	ctx := context.Background()
	byok, _ := orchestrator.NewBYOKManager("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	router := orchestrator.NewFallbackRouter(byok)

	kimiClient := &orchestrator.MockProviderClient{
		Content:   "Kimi K3 reviewed patch successfully.",
		PromptTok: 500,
		CompTok:   100,
	}
	router.RegisterClient(orchestrator.ProviderMoonshot, kimiClient)

	req := orchestrator.InferenceRequest{
		WorkspaceID:    uuid.New(),
		PreferredModel: "kimi-k3",
		PlanTier:       "TEAM",
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleUser, Content: "Review diff"},
		},
	}

	resp, err := router.Execute(ctx, req)
	if err != nil {
		t.Fatalf("expected kimi execution to succeed: %v", err)
	}
	if resp.ProviderUsed != orchestrator.ProviderMoonshot {
		t.Fatalf("expected ProviderMoonshot, got %s", resp.ProviderUsed)
	}
	if resp.ModelUsed != "kimi-k3" {
		t.Fatalf("expected kimi-k3, got %s", resp.ModelUsed)
	}
	if resp.TotalTokens != 600 {
		t.Fatalf("expected total tokens 600, got %d", resp.TotalTokens)
	}
}

func TestFallbackRouterModelAuthorizationGating(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()
	byok, _ := orchestrator.NewBYOKManager("")
	router := orchestrator.NewFallbackRouter(byok)

	anthropicClient := &orchestrator.MockProviderClient{
		Content: "Claude Sonnet reviewed successfully.",
	}
	router.RegisterClient(orchestrator.ProviderAnthropic, anthropicClient)

	// 1. Free plan request for Claude Sonnet 5 WITHOUT BYOK should be rejected
	reqFreeNoBYOK := orchestrator.InferenceRequest{
		WorkspaceID:    wsID,
		PreferredModel: "claude-sonnet-5",
		PlanTier:       "COMMUNITY",
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleUser, Content: "Review code"},
		},
	}
	_, err := router.Execute(ctx, reqFreeNoBYOK)
	if err == nil {
		t.Fatalf("expected free plan without BYOK to be blocked for claude-sonnet-5")
	}

	// 2. Free plan request WITH BYOK key should be allowed
	reqFreeWithBYOK := orchestrator.InferenceRequest{
		WorkspaceID:    wsID,
		PreferredModel: "claude-sonnet-5",
		PlanTier:       "COMMUNITY",
		TenantAPIKey:   "sk-ant-test-custom-key-12345",
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleUser, Content: "Review code"},
		},
	}
	resp, err := router.Execute(ctx, reqFreeWithBYOK)
	if err != nil {
		t.Fatalf("expected BYOK request to succeed on free tier: %v", err)
	}
	if resp.ModelUsed != "claude-sonnet-5" {
		t.Fatalf("expected claude-sonnet-5, got %s", resp.ModelUsed)
	}

	// 3. Pro / Team plan request without BYOK should succeed for Claude Sonnet 5
	reqPro := orchestrator.InferenceRequest{
		WorkspaceID:    wsID,
		PreferredModel: "claude-sonnet-5",
		PlanTier:       "TEAM",
		Messages: []orchestrator.ChatMessage{
			{Role: orchestrator.RoleUser, Content: "Review code"},
		},
	}
	respPro, err := router.Execute(ctx, reqPro)
	if err != nil {
		t.Fatalf("expected pro plan to succeed for claude-sonnet-5: %v", err)
	}
	if respPro.ModelUsed != "claude-sonnet-5" {
		t.Fatalf("expected claude-sonnet-5, got %s", respPro.ModelUsed)
	}
}

func TestExtendedProviderCatalogAndHeuristics(t *testing.T) {
	// 1. Check Z.AI GLM models
	pGLM53, ok := orchestrator.GetModelProfile("glm-5.3")
	if !ok || pGLM53.Provider != orchestrator.ProviderZAI || pGLM53.ContextWindow != 1_000_000 {
		t.Fatalf("unexpected profile for glm-5.3: %+v", pGLM53)
	}
	pGLMFlash, ok := orchestrator.GetModelProfile("glm-5.3-flash")
	if !ok || pGLMFlash.Provider != orchestrator.ProviderZAI || pGLMFlash.ContextWindow != 1_310_000 {
		t.Fatalf("unexpected profile for glm-5.3-flash: %+v", pGLMFlash)
	}
	pGLMFree, ok := orchestrator.GetModelProfile("glm-4.7-flash")
	if !ok || pGLMFree.InputPerMillion != 0.0 || pGLMFree.OutputPerMillion != 0.0 {
		t.Fatalf("expected 0 cost for glm-4.7-flash: %+v", pGLMFree)
	}

	// 2. Check Meta Llama models (10M context on Scout)
	pScout, ok := orchestrator.GetModelProfile("llama-4-scout")
	if !ok || pScout.Provider != orchestrator.ProviderMeta || pScout.ContextWindow != 10_000_000 {
		t.Fatalf("unexpected profile for llama-4-scout: %+v", pScout)
	}

	// 3. Check NVIDIA NIM
	pNemotron, ok := orchestrator.GetModelProfile("nvidia-llama-3.1-nemotron-ultra-253b")
	if !ok || pNemotron.Provider != orchestrator.ProviderNvidia || !pNemotron.SupportsThinking {
		t.Fatalf("unexpected profile for nvidia nemotron: %+v", pNemotron)
	}

	// 4. Check Tencent Hunyuan
	pHunyuan, ok := orchestrator.GetModelProfile("hunyuan-hy3-preview")
	if !ok || pHunyuan.Provider != orchestrator.ProviderTencent {
		t.Fatalf("unexpected profile for hunyuan-hy3-preview: %+v", pHunyuan)
	}

	// 5. Check Cohere and Perplexity
	pCohere, ok := orchestrator.GetModelProfile("command-r-plus")
	if !ok || pCohere.Provider != orchestrator.ProviderCohere {
		t.Fatalf("unexpected profile for command-r-plus: %+v", pCohere)
	}
	pSonar, ok := orchestrator.GetModelProfile("sonar-pro")
	if !ok || pSonar.Provider != orchestrator.ProviderPerplexity {
		t.Fatalf("unexpected profile for sonar-pro: %+v", pSonar)
	}

	// 6. Check heuristic matching for unseen future models
	pHeuristicGLM, ok := orchestrator.GetModelProfile("glm-6-quantum")
	if !ok || pHeuristicGLM.Provider != orchestrator.ProviderZAI {
		t.Fatalf("expected glm-6-quantum to resolve to ProviderZAI, got %s", pHeuristicGLM.Provider)
	}
	pHeuristicLlama, ok := orchestrator.GetModelProfile("meta-llama-5-70b")
	if !ok || pHeuristicLlama.Provider != orchestrator.ProviderMeta {
		t.Fatalf("expected meta-llama-5-70b to resolve to ProviderMeta, got %s", pHeuristicLlama.Provider)
	}
	pHeuristicGroq, ok := orchestrator.GetModelProfile("groq-custom-accelerated-v1")
	if !ok || pHeuristicGroq.Provider != orchestrator.ProviderGroq {
		t.Fatalf("expected groq to resolve to ProviderGroq, got %s", pHeuristicGroq.Provider)
	}
}


