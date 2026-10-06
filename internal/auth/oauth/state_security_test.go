package oauth

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPKCEStateRejectsExpiry(t *testing.T) {
	store := NewStateStore(time.Nanosecond)
	state, _, _, err := store.GeneratePKCE(ProviderGitHub)
	if err != nil {
		t.Fatal("could not create test state")
	}
	if _, _, accepted := store.Consume(state, ProviderGitHub); accepted {
		t.Fatal("expired PKCE state accepted")
	}
}

func TestOAuthStateTransportRetainsBindingMetadata(t *testing.T) {
	entry := stateEntry{Provider: ProviderGitLab, CreatedAt: time.Now().UTC(), CodeVerifier: "test-verifier", Nonce: "test-nonce"}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal("could not encode state")
	}
	var restored stateEntry
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal("could not decode state")
	}
	if restored.Provider != entry.Provider || !restored.CreatedAt.Equal(entry.CreatedAt) {
		t.Fatal("distributed state lost provider or expiry metadata")
	}
}
