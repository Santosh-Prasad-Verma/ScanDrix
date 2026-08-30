package oauth_test

import (
	"testing"
	"time"

	"github.com/scandrix/backend/internal/auth/oauth"
)

func TestStateStoreGenerateAndValidate(t *testing.T) {
	store := oauth.NewStateStore(5 * time.Minute)

	state, err := store.Generate(oauth.ProviderGitHub)
	if err != nil {
		t.Fatalf("failed to generate state: %v", err)
	}

	if state == "" {
		t.Fatal("expected non-empty state token")
	}

	// Valid: correct provider, not expired
	if !store.Validate(state, oauth.ProviderGitHub) {
		t.Error("expected state to be valid")
	}

	// Replay attack: same token should not validate again (one-time use)
	if store.Validate(state, oauth.ProviderGitHub) {
		t.Error("expected state to be consumed and invalid on second use")
	}
}

func TestStateStoreWrongProvider(t *testing.T) {
	store := oauth.NewStateStore(5 * time.Minute)

	state, _ := store.Generate(oauth.ProviderGitHub)

	// Wrong provider should fail
	if store.Validate(state, oauth.ProviderGitLab) {
		t.Error("expected validation to fail for wrong provider")
	}
}

func TestStateStoreExpiration(t *testing.T) {
	// Use 1ms TTL to force expiration
	store := oauth.NewStateStore(1 * time.Millisecond)

	state, _ := store.Generate(oauth.ProviderGitHub)

	time.Sleep(5 * time.Millisecond) // Wait for expiration

	if store.Validate(state, oauth.ProviderGitHub) {
		t.Error("expected expired state to fail validation")
	}
}

func TestStateStoreUnknownToken(t *testing.T) {
	store := oauth.NewStateStore(5 * time.Minute)

	if store.Validate("nonexistent-token", oauth.ProviderGitHub) {
		t.Error("expected unknown token to fail validation")
	}
}

func TestStateStoreSize(t *testing.T) {
	store := oauth.NewStateStore(5 * time.Minute)

	if store.Size() != 0 {
		t.Errorf("expected 0 pending tokens, got %d", store.Size())
	}

	_, _ = store.Generate(oauth.ProviderGitHub)
	_, _ = store.Generate(oauth.ProviderGitLab)

	if store.Size() != 2 {
		t.Errorf("expected 2 pending tokens, got %d", store.Size())
	}
}
