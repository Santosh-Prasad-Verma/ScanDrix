package sso

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// F-30: FetchAndCacheJWKS merged into the existing cache and was never called
// from production, so a rotated-out IdP key stayed trusted for the process
// lifetime and automated rotation never happened. These tests pin the two
// behaviours that fix it: a fresh fetch *replaces* the key set, and an unknown
// kid triggers a throttled refresh instead of a hard failure.
func newTestRSAKey(t *testing.T, kid string) (*rsa.PrivateKey, JSONWebKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	jwk := JSONWebKey{
		Kty: "RSA",
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
	}
	return priv, jwk
}

func jwksServer(t *testing.T, keys ...JSONWebKey) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JSONWebKeySet{Keys: keys})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signedWith(t *testing.T, priv *rsa.PrivateKey, kid string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "user-1", "iss": "https://idp.example.com", "aud": "client-1",
		"email": "user@scandrix.internal", "email_verified": true,
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	})
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return raw
}

// A key that disappears from the JWKS must stop verifying. Merging would have
// kept it forever.
func TestJWKSRefreshDropsRotatedOutKeys(t *testing.T) {
	oldPriv, oldJWK := newTestRSAKey(t, "old-key")
	newPriv, newJWK := newTestRSAKey(t, "new-key")

	var currentKeys []JSONWebKey
	srv := jwksServer(t)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(JSONWebKeySet{Keys: currentKeys})
	})

	h := NewOIDCHandler()
	ctx := context.Background()

	currentKeys = []JSONWebKey{oldJWK}
	if err := h.FetchAndCacheJWKS(ctx, srv.URL); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := h.ParseAndVerifyIDToken(ctx, signedWith(t, oldPriv, "old-key"),
		"https://idp.example.com", "client-1", time.Now()); err != nil {
		t.Fatalf("old key should verify before rotation: %v", err)
	}

	// The IdP rotates: old key removed, new key published.
	currentKeys = []JSONWebKey{newJWK}
	if err := h.FetchAndCacheJWKS(ctx, srv.URL); err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if _, err := h.ParseAndVerifyIDToken(ctx, signedWith(t, newPriv, "new-key"),
		"https://idp.example.com", "client-1", time.Now()); err != nil {
		t.Errorf("new key should verify after rotation: %v", err)
	}
	if _, err := h.ParseAndVerifyIDToken(ctx, signedWith(t, oldPriv, "old-key"),
		"https://idp.example.com", "client-1", time.Now()); err == nil {
		t.Error("a rotated-out key must stop verifying")
	}
}

// An unknown kid is what a rotation looks like from the verifier's side; it must
// trigger a refresh rather than fail outright.
func TestUnknownKidTriggersRefresh(t *testing.T) {
	priv, jwk := newTestRSAKey(t, "rotated-in")

	var currentKeys []JSONWebKey
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(JSONWebKeySet{Keys: currentKeys})
	}))
	t.Cleanup(srv.Close)

	h := NewOIDCHandler()
	ctx := context.Background()

	// Seed the cache with an unrelated key and point the handler at the endpoint.
	_, other := newTestRSAKey(t, "unrelated")
	currentKeys = []JSONWebKey{other}
	if err := h.FetchAndCacheJWKS(ctx, srv.URL); err != nil {
		t.Fatalf("seed fetch: %v", err)
	}

	// The IdP publishes a new key after the cache was filled.
	currentKeys = []JSONWebKey{other, jwk}

	// A token under the new kid must verify via the unknown-kid refresh.
	if _, err := h.ParseAndVerifyIDToken(ctx, signedWith(t, priv, "rotated-in"),
		"https://idp.example.com", "client-1", time.Now()); err != nil {
		t.Errorf("unknown kid should trigger a refresh and verify: %v", err)
	}
}
