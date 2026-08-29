package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAuthContextAndHashing(t *testing.T) {
	wsID := uuid.New()
	ctx := WithWorkspaceContext(context.Background(), wsID)

	extractedID, err := WorkspaceFromContext(ctx)
	if err != nil {
		t.Fatalf("expected valid workspace context, got error: %v", err)
	}
	if extractedID != wsID {
		t.Errorf("expected %s, got %s", wsID, extractedID)
	}

	rawKey := "scandrix_live_9f823a9e"
	hashed := HashAPIKey(rawKey)
	if len(hashed) != 64 {
		t.Errorf("expected 64-char sha256 hex string, got len %d", len(hashed))
	}

	if !ConstantTimeCompare("secret123", "secret123") {
		t.Error("expected equal strings to return true")
	}
	if ConstantTimeCompare("secret123", "wrongsecret") {
		t.Error("expected unequal strings to return false")
	}
}
