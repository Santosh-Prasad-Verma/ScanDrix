package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OAuthProvider string

const (
	ProviderGitHub    OAuthProvider = "github"
	ProviderGitLab    OAuthProvider = "gitlab"
	ProviderBitbucket OAuthProvider = "bitbucket"
)

var (
	ErrUnsupportedProvider = errors.New("unsupported oauth provider")
	ErrInvalidOAuthCode    = errors.New("invalid or expired oauth code")
	ErrEmailNotFound       = errors.New("verified primary email not found from oauth provider")
)

// OAuthUserProfile contains normalized user information from the SCM provider.
type OAuthUserProfile struct {
	Provider    OAuthProvider `json:"provider"`
	ProviderID  string        `json:"provider_id"`
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name"`
	Username    string        `json:"username"`
	AvatarURL   string        `json:"avatar_url"`
}

// ProviderConfig specifies OAuth 2.0 credentials and endpoints.
type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AuthURL      string
	TokenURL     string
	UserURL      string
	EmailURL     string
	Scope        string
}

// OAuthService coordinates social login with GitHub, GitLab, and Bitbucket.
type OAuthService struct {
	providers  map[OAuthProvider]ProviderConfig
	httpClient *http.Client
}

// NewOAuthService initializes the OAuth provider registry.
func NewOAuthService(githubCfg, gitlabCfg ProviderConfig, bitbucketCfgs ...ProviderConfig) *OAuthService {
	providers := make(map[OAuthProvider]ProviderConfig)

	// Defaults for GitHub
	if githubCfg.AuthURL == "" {
		githubCfg.AuthURL = "https://github.com/login/oauth/authorize"
	}
	if githubCfg.TokenURL == "" {
		githubCfg.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if githubCfg.UserURL == "" {
		githubCfg.UserURL = "https://api.github.com/user"
	}
	if githubCfg.EmailURL == "" {
		githubCfg.EmailURL = "https://api.github.com/user/emails"
	}
	if githubCfg.Scope == "" {
		githubCfg.Scope = "read:user,user:email"
	}
	providers[ProviderGitHub] = githubCfg

	// Defaults for GitLab
	if gitlabCfg.AuthURL == "" {
		gitlabCfg.AuthURL = "https://gitlab.com/oauth/authorize"
	}
	if gitlabCfg.TokenURL == "" {
		gitlabCfg.TokenURL = "https://gitlab.com/oauth/token"
	}
	if gitlabCfg.UserURL == "" {
		gitlabCfg.UserURL = "https://gitlab.com/api/v4/user"
	}
	if gitlabCfg.Scope == "" {
		gitlabCfg.Scope = "read_user"
	}
	providers[ProviderGitLab] = gitlabCfg

	// Defaults for Bitbucket
	var bitbucketCfg ProviderConfig
	if len(bitbucketCfgs) > 0 {
		bitbucketCfg = bitbucketCfgs[0]
	}
	if bitbucketCfg.AuthURL == "" {
		bitbucketCfg.AuthURL = "https://bitbucket.org/site/oauth2/authorize"
	}
	if bitbucketCfg.TokenURL == "" {
		bitbucketCfg.TokenURL = "https://bitbucket.org/site/oauth2/access_token"
	}
	if bitbucketCfg.UserURL == "" {
		bitbucketCfg.UserURL = "https://api.bitbucket.org/2.0/user"
	}
	if bitbucketCfg.EmailURL == "" {
		bitbucketCfg.EmailURL = "https://api.bitbucket.org/2.0/user/emails"
	}
	if bitbucketCfg.Scope == "" {
		bitbucketCfg.Scope = "account email"
	}
	providers[ProviderBitbucket] = bitbucketCfg

	return &OAuthService{
		providers: providers,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetHTTPClient enables injecting mock HTTP client for unit tests.
func (s *OAuthService) SetHTTPClient(client *http.Client) {
	s.httpClient = client
}

// GetAuthorizationURL returns the redirect URL to send the user's browser to.
func (s *OAuthService) GetAuthorizationURL(provider OAuthProvider, state string) (string, error) {
	cfg, ok := s.providers[provider]
	if !ok || cfg.ClientID == "" {
		return "", ErrUnsupportedProvider
	}

	u, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", cfg.RedirectURI)
	q.Set("scope", cfg.Scope)
	q.Set("state", state)
	q.Set("response_type", "code")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// ExchangeCode exchanges an authorization code for user profile details.
func (s *OAuthService) ExchangeCode(ctx context.Context, provider OAuthProvider, code string) (*OAuthUserProfile, error) {
	cfg, ok := s.providers[provider]
	if !ok {
		return nil, ErrUnsupportedProvider
	}

	// 1. Request access token
	tokenValues := url.Values{
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURI},
	}
	if provider == ProviderGitLab {
		tokenValues.Set("grant_type", "authorization_code")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.TokenURL, strings.NewReader(tokenValues.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrInvalidOAuthCode
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil || tokenResp.AccessToken == "" {
		return nil, ErrInvalidOAuthCode
	}

	// 2. Fetch user profile
	userReq, err := http.NewRequestWithContext(ctx, "GET", cfg.UserURL, nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	userResp, err := s.httpClient.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("fetching user info failed: %w", err)
	}
	defer userResp.Body.Close()

	if userResp.StatusCode != http.StatusOK {
		return nil, errors.New("failed fetching user info from provider")
	}

	var rawUser map[string]any
	if err := json.NewDecoder(userResp.Body).Decode(&rawUser); err != nil {
		return nil, err
	}

	profile := &OAuthUserProfile{
		Provider: provider,
	}

	if provider == ProviderGitHub {
		if id, ok := rawUser["id"].(float64); ok {
			profile.ProviderID = fmt.Sprintf("%.0f", id)
		}
		if login, ok := rawUser["login"].(string); ok {
			profile.Username = login
		}
		if name, ok := rawUser["name"].(string); ok {
			profile.DisplayName = name
		}
		if avatar, ok := rawUser["avatar_url"].(string); ok {
			profile.AvatarURL = avatar
		}
		if cfg.EmailURL != "" {
			// Always fetch verified primary email from GitHub /user/emails
			profile.Email = s.fetchGitHubPrimaryEmail(ctx, tokenResp.AccessToken, cfg.EmailURL)
		}
	} else if provider == ProviderGitLab {
		if id, ok := rawUser["id"].(float64); ok {
			profile.ProviderID = fmt.Sprintf("%.0f", id)
		}
		if username, ok := rawUser["username"].(string); ok {
			profile.Username = username
		}
		if name, ok := rawUser["name"].(string); ok {
			profile.DisplayName = name
		}
		if avatar, ok := rawUser["avatar_url"].(string); ok {
			profile.AvatarURL = avatar
		}
		// Require confirmed email on GitLab (state alone is not sufficient)
		confirmedAt, _ := rawUser["confirmed_at"].(string)
		if email, ok := rawUser["email"].(string); ok && email != "" {
			if confirmedAt != "" {
				profile.Email = email
			}
		}
	} else if provider == ProviderBitbucket {
		if uuidVal, ok := rawUser["uuid"].(string); ok {
			profile.ProviderID = uuidVal
		}
		if username, ok := rawUser["username"].(string); ok {
			profile.Username = username
		}
		if name, ok := rawUser["display_name"].(string); ok {
			profile.DisplayName = name
		}
		if links, ok := rawUser["links"].(map[string]any); ok {
			if avatar, ok := links["avatar"].(map[string]any); ok {
				if href, ok := avatar["href"].(string); ok {
					profile.AvatarURL = href
				}
			}
		}
		if cfg.EmailURL != "" {
			profile.Email = s.fetchBitbucketPrimaryEmail(ctx, tokenResp.AccessToken, cfg.EmailURL)
		}
	}

	if profile.Email == "" {
		return nil, ErrEmailNotFound
	}
	if profile.DisplayName == "" {
		profile.DisplayName = profile.Username
	}

	return profile, nil
}

func (s *OAuthService) fetchBitbucketPrimaryEmail(ctx context.Context, token, emailURL string) string {
	req, err := http.NewRequestWithContext(ctx, "GET", emailURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	var res struct {
		Values []struct {
			Email       string `json:"email"`
			IsPrimary   bool   `json:"is_primary"`
			IsConfirmed bool   `json:"is_confirmed"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return ""
	}

	for _, e := range res.Values {
		if e.IsPrimary && e.IsConfirmed {
			return e.Email
		}
	}
	if len(res.Values) > 0 {
		return res.Values[0].Email
	}
	return ""
}

func (s *OAuthService) fetchGitHubPrimaryEmail(ctx context.Context, token, emailURL string) string {
	req, err := http.NewRequestWithContext(ctx, "GET", emailURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return ""
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	return ""
}
