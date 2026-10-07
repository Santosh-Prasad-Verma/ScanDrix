package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

func TestHelpdeskTokenService(t *testing.T) {
	// 1. Generate test RSA private key
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed generating RSA key: %v", err)
	}

	keyDER := x509.MarshalPKCS1PrivateKey(rsaKey)
	keyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyDER,
	}))

	svc, err := auth.NewHelpdeskTokenService(keyPEM)
	if err != nil {
		t.Fatalf("failed initializing HelpdeskTokenService: %v", err)
	}

	user := &models.AccountProfile{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
		Email:       "support-requester@example.com",
		DisplayName: "Support Requester",
		Role:        models.RoleMember,
	}

	// 2. Mint 5-minute RS256 SSO token
	token, err := svc.GenerateHelpdeskToken(user)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3-part JWT, got %d", len(parts))
	}

	// 3. Verify token and claims
	claims, err := svc.VerifyHelpdeskToken(token)
	if err != nil {
		t.Fatalf("failed verifying valid token: %v", err)
	}

	if claims.Sub != user.ID.String() {
		t.Fatalf("mismatched subject claim: got %s, want %s", claims.Sub, user.ID.String())
	}
	if claims.Email != user.Email || claims.Issuer != "scandrix" || claims.Audience != "scandrix-helpdesk" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.ExpiresAt <= claims.IssuedAt {
		t.Fatalf("invalid expiration timestamp: exp=%d, iat=%d", claims.ExpiresAt, claims.IssuedAt)
	}

	// 4. Tampered token fails verification
	tamperedToken := token[:len(token)-5] + "XXXXX"
	_, errTampered := svc.VerifyHelpdeskToken(tamperedToken)
	if errTampered == nil {
		t.Fatal("expected error when verifying tampered token, but succeeded")
	}

	// 5. Unconfigured service returns ErrHelpdeskKeyMissing
	emptySvc, _ := auth.NewHelpdeskTokenService("")
	_, errMissing := emptySvc.GenerateHelpdeskToken(user)
	if !errors.Is(errMissing, auth.ErrHelpdeskKeyMissing) {
		t.Fatalf("expected ErrHelpdeskKeyMissing, got: %v", errMissing)
	}
}

func TestHelpdeskTokenServiceWithEscapedNewlines(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed generating RSA key: %v", err)
	}

	keyDER := x509.MarshalPKCS1PrivateKey(rsaKey)
	keyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyDER,
	}))

	// Simulate single-line escaped \n from .env or Docker env_file
	escapedPEM := `"` + strings.ReplaceAll(keyPEM, "\n", `\n`) + `"`

	svc, err := auth.NewHelpdeskTokenService(escapedPEM)
	if err != nil {
		t.Fatalf("failed initializing HelpdeskTokenService with escaped newlines: %v", err)
	}

	user := &models.AccountProfile{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
		Email:       "support-escaped@example.com",
		DisplayName: "Support Escaped",
		Role:        models.RoleMember,
	}

	token, err := svc.GenerateHelpdeskToken(user)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	claims, err := svc.VerifyHelpdeskToken(token)
	if err != nil {
		t.Fatalf("failed verifying token: %v", err)
	}

	if claims.Sub != user.ID.String() {
		t.Fatalf("mismatched subject claim: got %s, want %s", claims.Sub, user.ID.String())
	}
}
