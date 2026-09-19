package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
)

func TestPasswordResetTokenLifecycle(t *testing.T) {
	userID := uuid.New()
	email := "operator:tag@scandrix.dev" // Test email containing colon
	oldHash := "$2a$10$oldpasswordhash12345678901234567890"
	jwtSecret := "super-secure-jwt-secret-for-testing"

	// 1. Generate valid token with 15-minute TTL
	token, err := auth.CreatePasswordResetToken(userID, email, oldHash, jwtSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed creating reset token: %v", err)
	}

	// 2. Successfully verify token
	claims, err := auth.VerifyPasswordResetToken(token, oldHash, jwtSecret)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	if claims.UserID != userID || claims.Email != email {
		t.Fatalf("claims mismatch: %+v", claims)
	}

	// 3. Immediately invalid once password hash changes (old hash no longer matches)
	newHash := "$2a$10$newpasswordhash99999999999999999999"
	_, err = auth.VerifyPasswordResetToken(token, newHash, jwtSecret)
	if err == nil {
		t.Fatal("expected token to be rejected after password hash change")
	}

	// 4. Reject non-positive TTL at creation
	_, err = auth.CreatePasswordResetToken(userID, email, oldHash, jwtSecret, -1*time.Minute)
	if err == nil {
		t.Fatal("expected error for non-positive TTL at creation, got nil")
	}

	// 5. Expired token verification failure
	shortToken, err := auth.CreatePasswordResetToken(userID, email, oldHash, jwtSecret, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("failed creating short-lived token: %v", err)
	}
	time.Sleep(15 * time.Millisecond)
	_, err = auth.VerifyPasswordResetToken(shortToken, oldHash, jwtSecret)
	if err != auth.ErrResetTokenExpired {
		t.Fatalf("expected ErrResetTokenExpired, got: %v", err)
	}

	// 6. Reject tampered token
	tamperedToken := token + "evil"
	_, err = auth.VerifyPasswordResetToken(tamperedToken, oldHash, jwtSecret)
	if err == nil {
		t.Fatal("expected tampered token to fail verification")
	}

	// 7. Reject oversized token (> 1024 bytes)
	hugeToken := strings.Repeat("x", 2048)
	_, err = auth.VerifyPasswordResetToken(hugeToken, oldHash, jwtSecret)
	if err != auth.ErrResetTokenInvalid {
		t.Fatalf("expected ErrResetTokenInvalid for oversized token, got: %v", err)
	}
}
