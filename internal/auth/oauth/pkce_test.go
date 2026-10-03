package oauth

import (
	"net/url"
	"strings"
	"testing"
)

// The user OAuth flow previously had no PKCE at all, so an intercepted
// authorization code could be replayed. RFC 9700 (OAuth 2.0 Security BCP)
// requires it even for a confidential client.
//
// AUDIT_REMEDIATION.md.
func TestAuthorizationURLCarriesS256ChallengeAndNonce(t *testing.T) {
	svc := NewOAuthService(
		ProviderConfig{ClientID: "gh-client", AuthURL: "https://idp.example/authorize", Scope: "read:user"},
		ProviderConfig{},
	)

	state, verifier, nonce, err := NewStateStore(0).GeneratePKCE(ProviderGitHub)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	raw, err := svc.GetAuthorizationURLWithPKCE(ProviderGitHub, state, S256Challenge(verifier), nonce)
	if err != nil {
		t.Fatalf("build authorize URL: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()

	if got := q.Get("code_challenge"); got == "" {
		t.Fatal("authorize URL must carry a code_challenge")
	}
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method must be S256, got %q", got)
	}
	if got := q.Get("nonce"); got != nonce {
		t.Fatalf("nonce mismatch: %q", got)
	}
	if got := q.Get("state"); got != state {
		t.Fatalf("state mismatch: %q", got)
	}

	// The challenge must be the S256 of the verifier, and the verifier itself
	// must not appear anywhere in the URL the browser sees.
	if want := S256Challenge(verifier); q.Get("code_challenge") != want {
		t.Fatal("challenge is not S256(verifier)")
	}
	if strings.Contains(raw, verifier) {
		t.Fatal("the code verifier must never be sent to the browser")
	}
}

// The known-answer vector from RFC 7636 appendix B.
func TestS256ChallengeMatchesRFC7636Vector(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := S256Challenge(verifier); got != want {
		t.Fatalf("S256 challenge = %q, want %q", got, want)
	}
}

func TestConsumeReturnsVerifierAndNonceAndIsSingleUse(t *testing.T) {
	store := NewStateStore(0)
	state, verifier, nonce, err := store.GeneratePKCE(ProviderGitHub)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	gotVerifier, gotNonce, ok := store.Consume(state, ProviderGitHub)
	if !ok {
		t.Fatal("first consume should succeed")
	}
	if gotVerifier != verifier || gotNonce != nonce {
		t.Fatal("consume must return the verifier and nonce bound at generation")
	}

	if _, _, ok := store.Consume(state, ProviderGitHub); ok {
		t.Fatal("a state must be single-use")
	}
}

func TestConsumeRejectsWrongProvider(t *testing.T) {
	store := NewStateStore(0)
	state, _, _, err := store.GeneratePKCE(ProviderGitHub)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, _, ok := store.Consume(state, ProviderGitLab); ok {
		t.Fatal("a GitHub state must not be redeemable as GitLab")
	}
}
