package github_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/scandrix/backend/internal/platform/github"
)

func TestAppTokenRotatorJWT(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed generating test key: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	rotator, err := github.NewAppTokenRotator("123456", pemBytes)
	if err != nil {
		t.Fatalf("failed creating rotator: %v", err)
	}

	jwtStr, err := rotator.GenerateAppJWT()
	if err != nil {
		t.Fatalf("failed generating App JWT: %v", err)
	}

	if jwtStr == "" {
		t.Error("expected non-empty JWT string")
	}
}

func TestAppTokenRotatorRateLimiting(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed generating test key: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	rotator, err := github.NewAppTokenRotator("123456", pemBytes)
	if err != nil {
		t.Fatalf("failed creating rotator: %v", err)
	}

	ctx := t.Context()
	instID := int64(99999)

	// Consuming within initial capacity (100) should be allowed
	for i := 0; i < 50; i++ {
		if !rotator.AllowInstallationRequest(ctx, instID) {
			t.Fatalf("request %d should have been allowed within burst budget", i)
		}
	}
}
