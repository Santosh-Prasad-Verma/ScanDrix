package auth_test

import (
	"strings"
	"testing"

	"github.com/codehound/codehound/shared/pkg/auth"
)

func TestGenerateAndVerifyAPIKey(t *testing.T) {
	keyObj, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("failed to generate API key: %v", err)
	}

	if !strings.HasPrefix(keyObj.RawKey, auth.KeyPrefixLive) {
		t.Errorf("expected raw key to start with '%s', got %s", auth.KeyPrefixLive, keyObj.RawKey)
	}

	if len(keyObj.KeyHash) != 64 {
		t.Errorf("expected SHA-256 hex hash of length 64, got %d", len(keyObj.KeyHash))
	}

	// Verify valid key matches its hash
	if !auth.VerifyKey(keyObj.RawKey, keyObj.KeyHash) {
		t.Errorf("expected key to verify against its hash")
	}

	// Verify invalid key does not match
	tamperedKey := keyObj.RawKey + "tampered"
	if auth.VerifyKey(tamperedKey, keyObj.KeyHash) {
		t.Errorf("expected tampered key to fail verification")
	}
}
