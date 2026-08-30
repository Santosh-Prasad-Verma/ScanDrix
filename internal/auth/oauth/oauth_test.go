package oauth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/auth/oauth"
)

func TestOAuthGetAuthorizationURL(t *testing.T) {
	svc := oauth.NewOAuthService(
		oauth.ProviderConfig{
			ClientID:    "github-client-id",
			RedirectURI: "https://app.scandrix.dev/auth/callback/github",
		},
		oauth.ProviderConfig{
			ClientID:    "gitlab-client-id",
			RedirectURI: "https://app.scandrix.dev/auth/callback/gitlab",
		},
	)

	// GitHub URL
	ghURL, err := svc.GetAuthorizationURL(oauth.ProviderGitHub, "state-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ghURL, "client_id=github-client-id") || !strings.Contains(ghURL, "state=state-123") {
		t.Fatalf("malformed github url: %s", ghURL)
	}

	// GitLab URL
	glURL, err := svc.GetAuthorizationURL(oauth.ProviderGitLab, "state-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(glURL, "client_id=gitlab-client-id") || !strings.Contains(glURL, "state=state-456") {
		t.Fatalf("malformed gitlab url: %s", glURL)
	}

	// Unsupported provider
	_, err = svc.GetAuthorizationURL("bitbucket", "state")
	if err != oauth.ErrUnsupportedProvider {
		t.Fatalf("expected ErrUnsupportedProvider, got: %v", err)
	}
}

func TestGitHubExchangeCodeSuccess(t *testing.T) {
	// Mock GitHub API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "gho_mocktoken123",
				"token_type":   "bearer",
			})
		case "/user":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         123456,
				"login":      "octocat",
				"name":       "The Octocat",
				"avatar_url": "https://avatars.githubusercontent.com/u/123456",
				"email":      "", // Private email, requires /user/emails
			})
		case "/user/emails":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"email": "secondary@example.com", "primary": false, "verified": true},
				{"email": "octocat@github.com", "primary": true, "verified": true},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	svc := oauth.NewOAuthService(
		oauth.ProviderConfig{
			ClientID:     "gh_id",
			ClientSecret: "gh_secret",
			TokenURL:     mockServer.URL + "/login/oauth/access_token",
			UserURL:      mockServer.URL + "/user",
			EmailURL:     mockServer.URL + "/user/emails",
		},
		oauth.ProviderConfig{},
	)
	svc.SetHTTPClient(mockServer.Client())

	profile, err := svc.ExchangeCode(context.Background(), oauth.ProviderGitHub, "valid_code")
	if err != nil {
		t.Fatalf("code exchange failed: %v", err)
	}

	if profile.Username != "octocat" || profile.Email != "octocat@github.com" {
		t.Fatalf("profile mismatch: %+v", profile)
	}
	if profile.DisplayName != "The Octocat" {
		t.Fatalf("display name mismatch: %s", profile.DisplayName)
	}
}

func TestGitLabExchangeCodeSuccess(t *testing.T) {
	// Mock GitLab API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "glpat_mocktoken456",
				"token_type":   "bearer",
			})
		case "/api/v4/user":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           789,
				"username":     "tanuki",
				"name":         "GitLab Tanuki",
				"email":        "tanuki@gitlab.com",
				"state":        "active",
				"confirmed_at": "2023-01-01T00:00:00.000Z",
				"avatar_url":   "https://gitlab.com/uploads/-/system/user/avatar/789/avatar.png",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	svc := oauth.NewOAuthService(
		oauth.ProviderConfig{},
		oauth.ProviderConfig{
			ClientID:     "gl_id",
			ClientSecret: "gl_secret",
			TokenURL:     mockServer.URL + "/oauth/token",
			UserURL:      mockServer.URL + "/api/v4/user",
		},
	)
	svc.SetHTTPClient(mockServer.Client())

	profile, err := svc.ExchangeCode(context.Background(), oauth.ProviderGitLab, "valid_code")
	if err != nil {
		t.Fatalf("gitlab exchange failed: %v", err)
	}

	if profile.Username != "tanuki" || profile.Email != "tanuki@gitlab.com" {
		t.Fatalf("gitlab profile mismatch: %+v", profile)
	}
}
