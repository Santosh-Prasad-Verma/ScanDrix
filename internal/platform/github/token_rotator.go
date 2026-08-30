package github

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AppInstallationToken represents a scoped short-lived GitHub App installation token.
type AppInstallationToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AppTokenRotator manages GitHub App private keys, generating JWTs and auto-refreshing installation tokens.
type AppTokenRotator struct {
	appID          string
	privateKey     *rsa.PrivateKey
	httpClient     *http.Client
	mu             sync.RWMutex
	cachedTokens   map[int64]*AppInstallationToken
}

// NewAppTokenRotator initializes a token rotator from PEM-encoded RSA private key bytes.
func NewAppTokenRotator(appID string, privateKeyPEM []byte) (*AppTokenRotator, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, errors.New("failed decoding PEM block from private key")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Attempt PKCS8 fallback
		pkcs8Key, errPKCS8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if errPKCS8 != nil {
			return nil, fmt.Errorf("failed parsing RSA private key: %w (pkcs8: %v)", err, errPKCS8)
		}
		var ok bool
		key, ok = pkcs8Key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("parsed PKCS8 key is not an RSA private key")
		}
	}

	return &AppTokenRotator{
		appID:        appID,
		privateKey:   key,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		cachedTokens: make(map[int64]*AppInstallationToken),
	}, nil
}

// GenerateAppJWT creates a signed RS256 JWT valid for 10 minutes to authenticate as the GitHub App.
func (r *AppTokenRotator) GenerateAppJWT() (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(), // 60s clock drift margin
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": r.appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(r.privateKey)
}

// GetInstallationToken returns an unexpired token for the specified installation ID, auto-refreshing if needed.
func (r *AppTokenRotator) GetInstallationToken(ctx context.Context, installationID int64) (string, error) {
	r.mu.RLock()
	cached, exists := r.cachedTokens[installationID]
	r.mu.RUnlock()

	// If token exists and is valid for at least 5 more minutes, return cached
	if exists && time.Now().UTC().Add(5*time.Minute).Before(cached.ExpiresAt) {
		return cached.Token, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after lock
	if cached, exists = r.cachedTokens[installationID]; exists && time.Now().UTC().Add(5*time.Minute).Before(cached.ExpiresAt) {
		return cached.Token, nil
	}

	jwtStr, err := r.GenerateAppJWT()
	if err != nil {
		return "", fmt.Errorf("failed generating GitHub App JWT: %w", err)
	}

	reqURL := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Authorization", "Bearer "+jwtStr)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("User-Agent", "ScanDrix-Engine")

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed requesting installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github api error (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp AppInstallationToken
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed parsing installation token response: %w", err)
	}

	r.cachedTokens[installationID] = &tokenResp
	return tokenResp.Token, nil
}
