package auth_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
)

func TestPasswordResetTokenLifecycle(t *testing.T) {
	userID := uuid.New()
	email := "operator@scandrix.dev"
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

	// 4. Reject expired token
	expiredToken, err := auth.CreatePasswordResetToken(userID, email, oldHash, jwtSecret, -1*time.Minute)
	if err != nil {
		t.Fatalf("failed creating expired token: %v", err)
	}
	_, err = auth.VerifyPasswordResetToken(expiredToken, oldHash, jwtSecret)
	if err != auth.ErrResetTokenExpired {
		t.Fatalf("expected ErrResetTokenExpired, got: %v", err)
	}

	// 5. Reject tampered token
	tamperedToken := token + "evil"
	_, err = auth.VerifyPasswordResetToken(tamperedToken, oldHash, jwtSecret)
	if err == nil {
		t.Fatal("expected tampered token to fail verification")
	}
}
