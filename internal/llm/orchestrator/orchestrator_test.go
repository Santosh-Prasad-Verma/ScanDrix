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
