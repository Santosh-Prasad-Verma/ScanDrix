package infrastructure

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/scandrix/backend/internal/identity/domain"
)

// F-22: email-verification and password-reset tokens used to be byte-identical
// and VerifyForgotPassToken delegated to VerifyEmailToken, so a reset link
// verified an address and a verification link could be redeemed at the reset
// endpoint. Each purpose must be bound into the token and required on verify.
func TestTokenPurposesAreNotInterchangeable(t *testing.T) {
	svc := &JwtTokenService{config: jwtConfigForTest()}

	userID := uuid.New()
	email := "user@scandrix.internal"

	verifyToken, err := svc.CreateEmailToken(userID, email)
	if err != nil {
		t.Fatalf("CreateEmailToken: %v", err)
	}
	resetToken, err := svc.CreateForgotPassToken(userID, email)
	if err != nil {
		t.Fatalf("CreateForgotPassToken: %v", err)
	}

	// They must not even be the same string.
	if verifyToken == resetToken {
		t.Fatal("email-verification and password-reset tokens are byte-identical")
	}

	// Each verifies against its own purpose.
	if _, _, err := svc.VerifyEmailToken(verifyToken); err != nil {
		t.Errorf("email token must verify as an email token: %v", err)
	}
	if _, _, err := svc.VerifyForgotPassToken(resetToken); err != nil {
		t.Errorf("reset token must verify as a reset token: %v", err)
	}

	// The replay paths, which both used to succeed.
	if _, _, err := svc.VerifyEmailToken(resetToken); err == nil {
		t.Error("a password-reset token must NOT verify an email address")
	}
	if _, _, err := svc.VerifyForgotPassToken(verifyToken); err == nil {
		t.Error("an email-verification token must NOT be redeemable as a password reset")
	}
}

// The reset window is deliberately much shorter than the 24h verification
// window: a reset link is a more powerful capability.
func TestPasswordResetTokenExpiresSoonerThanVerification(t *testing.T) {
	userID := uuid.New()

	verifyClaims := decodeForTest(t, func(s *JwtTokenService) (string, error) {
		return s.CreateEmailToken(userID, "u@scandrix.internal")
	})
	resetClaims := decodeForTest(t, func(s *JwtTokenService) (string, error) {
		return s.CreateForgotPassToken(userID, "u@scandrix.internal")
	})

	verifyTTL := verifyClaims.ExpiresAt.Sub(verifyClaims.IssuedAt.Time)
	resetTTL := resetClaims.ExpiresAt.Sub(resetClaims.IssuedAt.Time)

	if resetTTL >= verifyTTL {
		t.Errorf("reset TTL %v must be shorter than verification TTL %v", resetTTL, verifyTTL)
	}
}

func jwtConfigForTest() domain.JWTConfig {
	return domain.JWTConfig{
		Secret:        "purpose-test-secret-value-32-bytes!!",
		RefreshSecret: "purpose-test-refresh-secret-32-bytes",
	}
}

// decodeForTest re-parses a generated token so the test can assert on claims.
func decodeForTest(t *testing.T, gen func(*JwtTokenService) (string, error)) *purposeClaims {
	t.Helper()
	svc := &JwtTokenService{config: jwtConfigForTest()}
	raw, err := gen(svc)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	parsed, err := jwt.ParseWithClaims(raw, &purposeClaims{}, func(*jwt.Token) (any, error) {
		return []byte(svc.config.Secret), nil
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	claims, ok := parsed.Claims.(*purposeClaims)
	if !ok {
		t.Fatalf("unexpected claims type %T", parsed.Claims)
	}
	return claims
}
