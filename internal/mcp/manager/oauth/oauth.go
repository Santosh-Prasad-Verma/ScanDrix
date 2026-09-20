// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/mcp/manager/models"
)

// GeneratePKCE creates a cryptographic verifier and S256 code challenge (RFC 7636).
func GeneratePKCE() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", "", fmt.Errorf("failed generating pkce verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// GenerateState generates a 16-byte hex-encoded random state parameter for CSRF defense.
func GenerateState() string {
	b := make([]byte, 16)
	_, _ = io.ReadFull(rand.Reader, b)
	return hex.EncodeToString(b)
}

// GetCanonicalResourceURI formats a URL to canonical RFC 8707 resource URI.
func GetCanonicalResourceURI(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	path := u.Path
	if path == "/" {
		path = ""
	}
	canonical := fmt.Sprintf("%s://%s%s", scheme, host, path)
	if strings.HasSuffix(canonical, "/") && path != "" {
		canonical = strings.TrimSuffix(canonical, "/")
	}
	return canonical, nil
}

// BuildWellKnownURL constructs RFC 8414 /.well-known endpoint URLs.
func BuildWellKnownURL(baseURL, wellKnownName string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	basePath := u.Path
	if basePath == "/" {
		basePath = ""
	}
	wellKnownPath := fmt.Sprintf("/.well-known/%s%s", wellKnownName, basePath)
	dest := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   wellKnownPath,
	}
	return dest.String(), nil
}

// DiscoverOAuth queries authorization server metadata via RFC 8414 and RFC 8707.
func DiscoverOAuth(ctx context.Context, client *http.Client, baseURL string) (*models.OAuthProtectedResourceMetadata, *models.OAuthAuthorizationServerMetadata, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" {
		return nil, nil, errors.New("only HTTPS is allowed for OAuth discovery")
	}

	// 1. Discover Protected Resource Metadata
	rsURL, _ := BuildWellKnownURL(baseURL, "oauth-protected-resource")
	var rs models.OAuthProtectedResourceMetadata

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rsURL, nil)
	if err == nil {
		resp, rErr := client.Do(req)
		if rErr == nil {
			defer resp.Body.Close()
			if resp.StatusCode < 400 {
				_ = json.NewDecoder(resp.Body).Decode(&rs)
			}
		}
	}

	asIssuer := baseURL
	if len(rs.AuthorizationServers) > 0 && rs.AuthorizationServers[0] != "" {
		asIssuer = rs.AuthorizationServers[0]
	}

	// 2. Discover Authorization Server Metadata
	asURL, _ := BuildWellKnownURL(asIssuer, "oauth-authorization-server")
	reqAS, err := http.NewRequestWithContext(ctx, http.MethodGet, asURL, nil)
	if err != nil {
		return nil, nil, err
	}

	respAS, err := client.Do(reqAS)
	if err != nil || respAS.StatusCode >= 400 {
		// Fallback: try baseURL root origin
		origin := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		fallbackURL, _ := BuildWellKnownURL(origin, "oauth-authorization-server")
		reqFallback, errFB := http.NewRequestWithContext(ctx, http.MethodGet, fallbackURL, nil)
		if errFB != nil {
			return nil, nil, fmt.Errorf("failed fetching authorization server metadata: %w", err)
		}
		respFallback, errFBDo := client.Do(reqFallback)
		if errFBDo != nil || respFallback.StatusCode >= 400 {
			return nil, nil, errors.New("failed to fetch authorization server metadata from all well-known paths")
		}
		defer respFallback.Body.Close()
		var asFallback models.OAuthAuthorizationServerMetadata
		if err := json.NewDecoder(respFallback.Body).Decode(&asFallback); err != nil {
			return nil, nil, err
		}
		return &rs, &asFallback, nil
	}
	defer respAS.Body.Close()

	var as models.OAuthAuthorizationServerMetadata
	if err := json.NewDecoder(respAS.Body).Decode(&as); err != nil {
		return nil, nil, err
	}

	return &rs, &as, nil
}

// RegisterOAuthClient executes Dynamic Client Registration (RFC 7591).
func RegisterOAuthClient(ctx context.Context, client *http.Client, registrationEndpoint, redirectURI string, scopes []string) (string, string, error) {
	body := map[string]any{
		"client_name":                "ScanDrix MCP Manager",
		"client_uri":                 "https://scandrix.io",
		"logo_uri":                   "https://scandrix.io/assets/logo.png",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	if len(scopes) > 0 {
		body["scope"] = strings.Join(scopes, " ")
	}

	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("dynamic client registration request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("dynamic client registration failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var res struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", "", err
	}

	return res.ClientID, res.ClientSecret, nil
}

// BuildAuthorizationURL creates the full browser redirect URL for authorization.
func BuildAuthorizationURL(authEndpoint, clientID, redirectURI, challenge, state, resource string, scopes []string) (string, error) {
	u, err := url.Parse(authEndpoint)
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}
	if resource != "" {
		canonical, err := GetCanonicalResourceURI(resource)
		if err == nil && canonical != "" {
			q.Set("resource", canonical)
		}
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// ExchangeCodeForTokens exchanges an authorization code for access and refresh tokens.
func ExchangeCodeForTokens(ctx context.Context, client *http.Client, tokenEndpoint, clientID, clientSecret, code, codeVerifier, redirectURI, resource string) (*models.OAuthTokenData, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", clientID)
	data.Set("code", code)
	data.Set("code_verifier", codeVerifier)
	data.Set("redirect_uri", redirectURI)
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}
	if resource != "" {
		data.Set("resource", resource)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed decoding token response: %w", err)
	}

	var expiresAt int64
	if raw.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second).Unix()
	}

	return &models.OAuthTokenData{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    raw.TokenType,
		Scope:        raw.Scope,
	}, nil
}

// RefreshToken exchanges an expired token for fresh access credentials.
func RefreshToken(ctx context.Context, client *http.Client, tokenEndpoint, clientID, clientSecret, refreshToken string) (*models.OAuthTokenData, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", clientID)
	data.Set("refresh_token", refreshToken)
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	var expiresAt int64
	if raw.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second).Unix()
	}

	newRefreshToken := raw.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = refreshToken
	}

	return &models.OAuthTokenData{
		AccessToken:  raw.AccessToken,
		RefreshToken: newRefreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    raw.TokenType,
		Scope:        raw.Scope,
	}, nil
}
