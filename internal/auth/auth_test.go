package auth_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

func TestPasswordHashingAndVerification(t *testing.T) {
	password := "CorrectHorseBatteryStaple123!"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("failed hashing password: %v", err)
	}

	if !auth.VerifyPassword(password, hash) {
		t.Fatal("expected correct password to verify successfully")
	}

	if auth.VerifyPassword("WrongPassword456!", hash) {
		t.Fatal("expected incorrect password verification to fail")
	}

	if auth.VerifyPassword("", hash) {
		t.Fatal("expected empty password to fail")
	}
}

func TestJWTSigningAndVerification(t *testing.T) {
	jwtSecret := "super-secure-test-jwt-secret-key-32b"
	authenticator := auth.NewAuthenticator(jwtSecret)

	userID := uuid.New()
	wsID := uuid.New()

	// 1. Generate valid token
	token, err := authenticator.GenerateToken(userID, wsID, models.RoleOwner)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	// 2. Verify valid token
	claims, err := authenticator.VerifyToken(token)
	if err != nil {
		t.Fatalf("expected valid token verification, got: %v", err)
	}

	if claims.UserID != userID || claims.WorkspaceID != wsID || claims.Role != models.RoleOwner {
		t.Fatalf("claims mismatch: got %+v", claims)
	}

	// 3. Reject tampered signature
	tamperedToken := token + "tamper"
	_, err = authenticator.VerifyToken(tamperedToken)
	if err == nil {
		t.Fatal("expected verification failure for tampered token")
	}

	// 4. Reject token signed with wrong secret
	otherAuth := auth.NewAuthenticator("wrong-jwt-secret-key-00000000000")
	_, err = otherAuth.VerifyToken(token)
	if err == nil {
		t.Fatal("expected verification failure for wrong secret")
	}
}

func TestGenerateTokenPair(t *testing.T) {
	authenticator := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	userID := uuid.New()
	wsID := uuid.New()

	access, refresh, err := authenticator.GenerateTokenPair(userID, wsID, models.RoleMember)
	if err != nil {
		t.Fatalf("failed generating token pair: %v", err)
	}

	if access == "" || refresh == "" {
		t.Fatal("expected non-empty access and refresh tokens")
	}

	if len(refresh) != 64 { // 32 bytes hex encoded
		t.Fatalf("expected 64-character hex refresh token, got %d chars", len(refresh))
	}

	claims, err := authenticator.VerifyToken(access)
	if err != nil {
		t.Fatalf("failed verifying access token from pair: %v", err)
	}
	if claims.Role != models.RoleMember {
		t.Fatalf("expected RoleMember, got %s", claims.Role)
	}
}

func TestWorkspaceContextAndHashing(t *testing.T) {
	wsID := uuid.New()
	ctx := auth.WithWorkspaceContext(context.Background(), wsID)

	extractedID, err := auth.WorkspaceFromContext(ctx)
	if err != nil {
		t.Fatalf("expected valid workspace context, got error: %v", err)
	}
	if extractedID != wsID {
		t.Errorf("expected %s, got %s", wsID, extractedID)
	}

	rawKey := "scandrix_live_9f823a9e"
	hashed := auth.HashAPIKey(rawKey)
	if len(hashed) != 64 {
		t.Errorf("expected 64-char sha256 hex string, got len %d", len(hashed))
	}

	if !auth.ConstantTimeCompare("secret123", "secret123") {
		t.Error("expected equal strings to return true")
	}
	if auth.ConstantTimeCompare("secret123", "wrongsecret") {
		t.Error("expected unequal strings to return false")
	}
}
