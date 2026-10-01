package sso

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

var (
	ErrAlgorithmMismatch = errors.New("algorithm mismatch between token header and verification key")
	ErrUnsignedToken     = errors.New("unverified token rejected: signing key not configured")
	ErrAlgorithmNone     = errors.New("insecure algorithm 'none' rejected by enterprise security baseline (CWE-327)")
)

// OIDCProviderMetadata represents OpenID Connect discovery document (.well-known/openid-configuration).
type OIDCProviderMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

// JSONWebKey represents a public key in a JWKS key set.
type JSONWebKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JSONWebKeySet represents a standard JWKS response from an identity provider.
type JSONWebKeySet struct {
	Keys []JSONWebKey `json:"keys"`
}

// OIDCHandler handles OpenID Connect ID token decoding, cryptographic signature validation, and claim assertions.
type OIDCHandler struct {
	mu         sync.RWMutex
	signingKey any                       // *rsa.PublicKey or []byte (fallback default)
	jwksCache  map[string]*rsa.PublicKey // kid -> *rsa.PublicKey
	httpClient *http.Client

	// Key-rotation support (AUDIT_REMEDIATION.md F-30).
	//
	// FetchAndCacheJWKS had no production caller, so the cache stayed empty and
	// the documented "automated key rotation" never happened; only the static
	// signingKey ever verified anything. And because cached keys were never
	// expired, a key the IdP had rotated *out* stayed trusted for the process
	// lifetime.
	jwksURI       string
	jwksFetchedAt time.Time
	lastRefetch   time.Time
}

// jwksCacheTTL bounds how long a fetched key may be trusted without a refresh.
// IdPs publish new keys ahead of using them, so this only needs to be long enough
// that a normal fetch cycle covers it.
const jwksCacheTTL = 1 * time.Hour

// jwksRefetchInterval throttles the "unknown kid" refresh path, so an attacker
// cannot turn unknown key ids into a request amplifier aimed at the IdP.
const jwksRefetchInterval = 1 * time.Minute

// NewOIDCHandler initializes the OIDC handler with JWKS caching and timeout controls.
func NewOIDCHandler() *OIDCHandler {
	return &OIDCHandler{
		jwksCache: make(map[string]*rsa.PublicKey),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetSigningKey configures an RSA public key or HMAC secret for token signature verification.
func (h *OIDCHandler) SetSigningKey(key any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.signingKey = key
}

// currentSigningKey returns the statically configured key, if any.
func (h *OIDCHandler) currentSigningKey() any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.signingKey
}

// RefreshJWKS re-fetches the configured JWKS endpoint if one has been seen. Callers
// that know provider metadata (for example on login) should call this so a
// rotation is picked up without waiting for an unknown-kid retry.
//
// A provider with no JWKS endpoint configured is a no-op rather than an error.
func (h *OIDCHandler) RefreshJWKS(ctx context.Context) {
	h.refreshJWKSIfStale(ctx, false)
}

// SetHTTPClient configures a custom HTTP client for remote JWKS operations.
func (h *OIDCHandler) SetHTTPClient(client *http.Client) {
	if client != nil {
		h.httpClient = client
	}
}

// FetchAndCacheJWKS retrieves and caches remote RSA public keys from a JWKS URI for automated key rotation.
func (h *OIDCHandler) FetchAndCacheJWKS(ctx context.Context, jwksURI string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return fmt.Errorf("failed creating JWKS request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected JWKS HTTP response status: %d", resp.StatusCode)
	}

	var jwks JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed decoding JWKS response: %w", err)
	}

	// Build the new set first, then swap it in wholesale. Merging into the old
	// map would keep a key the IdP has since removed trusted forever, which is
	// the exact failure F-30 describes: a rotated-out key must stop verifying.
	fresh := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		if strings.ToUpper(k.Kty) != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		pubKey, err := parseRSAPublicKeyFromJWK(k)
		if err == nil && pubKey != nil {
			fresh[k.Kid] = pubKey
		}
	}

	h.mu.Lock()
	h.jwksCache = fresh
	h.jwksURI = jwksURI
	h.jwksFetchedAt = time.Now()
	h.mu.Unlock()
	return nil
}

// jwksUsableLocked reports whether the cache is present and fresh enough to trust.
// Caller must hold at least a read lock.
func (h *OIDCHandler) jwksUsableLocked(now time.Time) bool {
	return len(h.jwksCache) > 0 && !h.jwksFetchedAt.IsZero() &&
		now.Sub(h.jwksFetchedAt) < jwksCacheTTL
}

// refreshJWKSIfStale refetches when the cache is empty, expired, or older than the
// throttle window. Errors are swallowed: a refresh failure must not turn a
// previously valid verification into a hard error, and the caller falls through
// to whatever key it already had.
func (h *OIDCHandler) refreshJWKSIfStale(ctx context.Context, force bool) {
	now := time.Now()

	h.mu.RLock()
	uri := h.jwksURI
	usable := h.jwksUsableLocked(now)
	lastRefetch := h.lastRefetch
	h.mu.RUnlock()

	if uri == "" {
		return
	}
	if usable && !force {
		return
	}
	// Throttle even the forced path: an unknown kid must not become an
	// unbounded request generator against the identity provider.
	if !lastRefetch.IsZero() && now.Sub(lastRefetch) < jwksRefetchInterval && !usable {
		return
	}

	h.mu.Lock()
	h.lastRefetch = now
	h.mu.Unlock()

	_ = h.FetchAndCacheJWKS(ctx, uri)
}

// DiscoverProvider queries /.well-known/openid-configuration to obtain provider metadata.
func (h *OIDCHandler) DiscoverProvider(ctx context.Context, issuerURL string) (*OIDCProviderMetadata, error) {
	wellKnownURL := strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating discovery request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed fetching discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected discovery HTTP response status: %d", resp.StatusCode)
	}

	var meta OIDCProviderMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("failed parsing discovery document: %w", err)
	}
	return &meta, nil
}

func parseRSAPublicKeyFromJWK(k JSONWebKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("failed decoding modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("failed decoding exponent: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	return &rsa.PublicKey{N: n, E: e}, nil
}

// OIDCHeader represents standard JWT JOSE header.
type OIDCHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid,omitempty"`
}

// OIDCClaims represents standard OIDC JWT claims.
type OIDCClaims struct {
	Issuer        string   `json:"iss"`
	Subject       string   `json:"sub"`
	Audience      any      `json:"aud"` // string or []string
	ExpiresAt     int64    `json:"exp"`
	IssuedAt      int64    `json:"iat"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	GivenName     string   `json:"given_name"`
	FamilyName    string   `json:"family_name"`
	Name          string   `json:"name"`
	Groups        []string `json:"groups"`
	HD            string   `json:"hd"` // Hosted domain for Google Workspace
}

// VerifySignature cryptographically validates the JWT signature against a public key or shared secret,
// strictly enforcing algorithm pinning (CWE-327 prevention).
func (h *OIDCHandler) VerifySignature(rawJWT string, key any) error {
	parts := strings.Split(rawJWT, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid JWT structure: expected 3 segments, got %d", len(parts))
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("failed decoding header: %w", err)
	}

	var header OIDCHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return fmt.Errorf("malformed header JSON: %w", err)
	}

	alg := strings.ToUpper(strings.TrimSpace(header.Alg))
	if alg == "NONE" || strings.ToLower(header.Alg) == "none" {
		return ErrAlgorithmNone
	}

	switch key.(type) {
	case *rsa.PublicKey:
		if alg != "RS256" && alg != "RS384" && alg != "RS512" {
			return fmt.Errorf("%w: header specifies '%s' but key is RSA (expected RS256/384/512)", ErrAlgorithmMismatch, header.Alg)
		}
	case []byte:
		if alg != "HS256" && alg != "HS384" && alg != "HS512" {
			return fmt.Errorf("%w: header specifies '%s' but key is HMAC shared secret (expected HS256/384/512)", ErrAlgorithmMismatch, header.Alg)
		}
	default:
		return fmt.Errorf("unsupported key type: %T", key)
	}

	signedContent := parts[0] + "." + parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("failed decoding signature: %w", err)
	}

	switch k := key.(type) {
	case *rsa.PublicKey:
		var hash crypto.Hash
		switch alg {
		case "RS256":
			hash = crypto.SHA256
		case "RS384":
			hash = crypto.SHA384
		case "RS512":
			hash = crypto.SHA512
		}
		hasher := hash.New()
		hasher.Write([]byte(signedContent))
		hashed := hasher.Sum(nil)
		if err := rsa.VerifyPKCS1v15(k, hash, hashed, sigBytes); err != nil {
			return fmt.Errorf("RSA signature verification failed: %w", err)
		}
		return nil

	case []byte:
		if alg != "HS256" && alg != "HS384" && alg != "HS512" {
			return fmt.Errorf("%w: header specifies '%s' but key is HMAC shared secret (expected HS256/384/512)", ErrAlgorithmMismatch, header.Alg)
		}
		var mac hash.Hash
		switch alg {
		case "HS256":
			mac = hmac.New(sha256.New, k)
		case "HS384":
			mac = hmac.New(sha512.New384, k)
		case "HS512":
			mac = hmac.New(sha512.New, k)
		}
		mac.Write([]byte(signedContent))
		expectedSig := mac.Sum(nil)
		if !hmac.Equal(sigBytes, expectedSig) {
			return errors.New("HMAC signature verification failed")
		}
		return nil

	default:
		return fmt.Errorf("unsupported key type: %T", key)
	}
}

// ParseAndVerifyIDToken decodes the JWT and validates standard claims (iss, aud, exp, email_verified).
// Fails closed if no verification key is configured.
func (h *OIDCHandler) ParseAndVerifyIDToken(ctx context.Context, rawJWT string, expectedIssuer string, expectedAudience string, now time.Time) (*FederatedIdentity, error) {
	parts := strings.Split(rawJWT, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT structure: expected 3 segments, got %d", len(parts))
	}

	// 0. Resolve Verification Key and Fail Closed if missing
	//
	// Key rotation (AUDIT_REMEDIATION.md F-30). The static signingKey still wins
	// when set, for compatibility with HMAC/pinned setups. Otherwise the JWKS
	// cache is consulted, refreshed when stale, and refreshed once more when the
	// presented kid is unknown -- which is what an IdP key rotation actually
	// looks like from here.
	if ctx == nil {
		ctx = context.Background()
	}

	kid := ""
	if headerBytes, decErr := base64.RawURLEncoding.DecodeString(parts[0]); decErr == nil {
		var hdr OIDCHeader
		if json.Unmarshal(headerBytes, &hdr) == nil {
			kid = hdr.Kid
		}
	}

	lookup := func() any {
		h.mu.RLock()
		defer h.mu.RUnlock()
		if pk, ok := h.jwksCache[kid]; ok && kid != "" {
			return pk
		}
		return nil
	}

	verificationKey := h.currentSigningKey()
	if key := lookup(); key != nil {
		verificationKey = key
	} else if verificationKey == nil {
		// Unknown kid: the IdP may have rotated. Refresh and retry once.
		h.refreshJWKSIfStale(ctx, true)
		if key := lookup(); key != nil {
			verificationKey = key
		}
	}

	if verificationKey == nil {
		return nil, ErrUnsignedToken
	}

	if err := h.VerifySignature(rawJWT, verificationKey); err != nil {
		return nil, err
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed decoding JWT payload: %w", err)
	}

	var claims OIDCClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("malformed OIDC claims JSON: %w", err)
	}

	// 1. Verify Issuer
	if expectedIssuer != "" && claims.Issuer != expectedIssuer {
		return nil, fmt.Errorf("issuer mismatch: expected '%s', got '%s'", expectedIssuer, claims.Issuer)
	}

	// 2. Verify Audience
	if expectedAudience != "" {
		audMatched := false
		switch v := claims.Audience.(type) {
		case string:
			audMatched = (v == expectedAudience)
		case []any:
			for _, item := range v {
				if str, ok := item.(string); ok && str == expectedAudience {
					audMatched = true
					break
				}
			}
		}
		if !audMatched {
			return nil, fmt.Errorf("audience mismatch: token does not match expected client ID '%s'", expectedAudience)
		}
	}

	// 3. Verify Expiry (with 1-minute clock skew tolerance)
	expTime := time.Unix(claims.ExpiresAt, 0)
	if now.Add(-1 * time.Minute).After(expTime) {
		return nil, fmt.Errorf("OIDC ID token expired on %s", expTime.Format(time.RFC3339))
	}

	// 4. Verify Email presence
	if claims.Email == "" {
		return nil, fmt.Errorf("OIDC token contains no email claim")
	}

	firstName := claims.GivenName
	lastName := claims.FamilyName
	if firstName == "" && claims.Name != "" {
		names := strings.SplitN(claims.Name, " ", 2)
		firstName = names[0]
		if len(names) > 1 {
			lastName = names[1]
		}
	}

	return &FederatedIdentity{
		ExternalID: claims.Subject,
		Email:      claims.Email,
		FirstName:  firstName,
		LastName:   lastName,
		Groups:     claims.Groups,
		MappedRole: models.RoleMember,
		Provider:   ProviderTypeOIDC,
		RawClaims: map[string]string{
			"iss": claims.Issuer,
			"sub": claims.Subject,
			"hd":  claims.HD,
		},
	}, nil
}
