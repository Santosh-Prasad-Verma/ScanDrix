package zoho

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultAccountsURL = "https://accounts.zoho.in"
	defaultAPIDomain   = "https://www.zohoapis.in"
	defaultTimeout     = 10 * time.Second
)

// Lead represents a prospect or customer in Zoho CRM.
type Lead struct {
	FirstName   string         `json:"First_Name,omitempty"`
	LastName    string         `json:"Last_Name"`
	Email       string         `json:"Email"`
	Company     string         `json:"Company"`
	Phone       string         `json:"Phone,omitempty"`
	LeadSource  string         `json:"Lead_Source,omitempty"`
	LeadStatus  string         `json:"Lead_Status,omitempty"`
	Description string         `json:"Description,omitempty"`
	CustomFields map[string]any `json:"-"`
}

// Config encapsulates Zoho CRM connection settings.
type Config struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	AccountsURL  string // e.g. https://accounts.zoho.in or https://accounts.zoho.com
	APIDomain    string // e.g. https://www.zohoapis.in or https://www.zohoapis.com
	LeadSource   string
}

// Client manages Zoho CRM API interactions with automatic OAuth 2.0 token refreshing.
type Client struct {
	cfg         Config
	httpClient  *http.Client
	accessToken string
	tokenExpiry time.Time
	mu          sync.RWMutex
}

// NewClient initializes a Zoho CRM client from configuration.
func NewClient(cfg Config) *Client {
	if cfg.AccountsURL == "" {
		cfg.AccountsURL = defaultAccountsURL
	}
	if cfg.APIDomain == "" {
		cfg.APIDomain = defaultAPIDomain
	}
	if cfg.LeadSource == "" {
		cfg.LeadSource = "ScanDrix Cloud"
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// NewClientFromEnv initializes a Zoho CRM client from environment variables.
func NewClientFromEnv() *Client {
	accountsURL := os.Getenv("ZOHO_ACCOUNTS_URL")
	if accountsURL == "" {
		accountsURL = defaultAccountsURL
	}
	apiDomain := os.Getenv("ZOHO_CRM_API_DOMAIN")
	if apiDomain == "" {
		apiDomain = defaultAPIDomain
	}
	leadSource := os.Getenv("ZOHO_CRM_LEAD_SOURCE")
	if leadSource == "" {
		leadSource = "scandrix.dev"
	}

	return NewClient(Config{
		ClientID:     os.Getenv("ZOHO_CRM_CLIENT_ID"),
		ClientSecret: os.Getenv("ZOHO_CRM_CLIENT_SECRET"),
		RefreshToken: os.Getenv("ZOHO_CRM_REFRESH_TOKEN"),
		AccountsURL:  strings.TrimRight(accountsURL, "/"),
		APIDomain:    strings.TrimRight(apiDomain, "/"),
		LeadSource:   leadSource,
	})
}

// IsEnabled returns true if the required credentials are configured.
func (c *Client) IsEnabled() bool {
	return c.cfg.ClientID != "" && c.cfg.ClientSecret != "" && c.cfg.RefreshToken != ""
}

// UpsertLead creates or updates a Lead in Zoho CRM by Email.
func (c *Client) UpsertLead(ctx context.Context, lead Lead) (string, error) {
	if !c.IsEnabled() {
		return "", nil // graceful no-op if credentials are not set
	}

	if lead.LastName == "" {
		if lead.FirstName != "" {
			lead.LastName = lead.FirstName
		} else {
			lead.LastName = "ScanDrix User"
		}
	}
	if lead.Company == "" {
		lead.Company = "Individual Developer"
	}
	if lead.LeadSource == "" {
		lead.LeadSource = c.cfg.LeadSource
	}

	token, err := c.getValidAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("zoho crm: failed to obtain access token: %w", err)
	}

	payloadMap := map[string]any{
		"Last_Name":   lead.LastName,
		"Email":       lead.Email,
		"Company":     lead.Company,
		"Lead_Source": lead.LeadSource,
	}
	if lead.FirstName != "" {
		payloadMap["First_Name"] = lead.FirstName
	}
	if lead.Phone != "" {
		payloadMap["Phone"] = lead.Phone
	}
	if lead.LeadStatus != "" {
		payloadMap["Lead_Status"] = lead.LeadStatus
	}
	if lead.Description != "" {
		payloadMap["Description"] = lead.Description
	}
	for k, v := range lead.CustomFields {
		payloadMap[k] = v
	}

	reqBody := map[string]any{
		"data":           []map[string]any{payloadMap},
		"duplicate_check_fields": []string{"Email"},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("zoho crm: failed to marshal lead data: %w", err)
	}

	endpoint := fmt.Sprintf("%s/crm/v6/Leads/upsert", c.cfg.APIDomain)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("zoho crm: failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("zoho crm: request failed: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("zoho crm: failed reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("zoho crm: api returned status %d: %s", resp.StatusCode, string(respData))
	}

	var zohoResp struct {
		Data []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details struct {
				ID string `json:"id"`
			} `json:"details"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respData, &zohoResp); err != nil {
		return "", fmt.Errorf("zoho crm: failed to parse response: %w", err)
	}

	if len(zohoResp.Data) > 0 {
		return zohoResp.Data[0].Details.ID, nil
	}

	return "", nil
}

// getValidAccessToken retrieves a cached token or exchanges the refresh token for a new access token.
func (c *Client) getValidAccessToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.accessToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double check after acquiring write lock
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	tokenURL := fmt.Sprintf("%s/oauth/v2/token", c.cfg.AccountsURL)
	data := url.Values{}
	data.Set("refresh_token", c.cfg.RefreshToken)
	data.Set("client_id", c.cfg.ClientID)
	data.Set("client_secret", c.cfg.ClientSecret)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, string(respBytes))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}

	if err := json.Unmarshal(respBytes, &tokenResp); err != nil {
		return "", err
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("zoho oauth error: %s", tokenResp.Error)
	}

	c.accessToken = tokenResp.AccessToken
	// Buffer expiry by 60 seconds
	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	c.tokenExpiry = time.Now().Add(time.Duration(expiresIn-60) * time.Second)

	return c.accessToken, nil
}
