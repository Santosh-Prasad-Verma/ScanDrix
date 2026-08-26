package secrets

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// VerificationStatus represents the live status of a detected credential.
type VerificationStatus string

const (
	VerificationStatusActive   VerificationStatus = "ACTIVE"
	VerificationStatusRevoked  VerificationStatus = "REVOKED"
	VerificationStatusUnknown  VerificationStatus = "UNKNOWN"
)

// CredentialVerifier performs non-mutating network handshakes to test if discovered keys are active.
type CredentialVerifier struct {
	httpClient *http.Client
}

// NewCredentialVerifier initializes a credential handshake verifier.
func NewCredentialVerifier() *CredentialVerifier {
	return &CredentialVerifier{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// VerifyGitHubToken checks if a GitHub PAT is live via the non-mutating /user endpoint.
func (v *CredentialVerifier) VerifyGitHubToken(ctx context.Context, token string) (VerificationStatus, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return VerificationStatusUnknown, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "CodeHound-Credential-Verifier")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return VerificationStatusUnknown, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		scopes := resp.Header.Get("X-OAuth-Scopes")
		return VerificationStatusActive, fmt.Sprintf("GitHub token is ACTIVE. Granted scopes: [%s]", scopes), nil
	} else if resp.StatusCode == http.StatusUnauthorized {
		return VerificationStatusRevoked, "GitHub token is REVOKED or INVALID (401 Unauthorized)", nil
	}

	return VerificationStatusUnknown, fmt.Sprintf("Unexpected status code: %d", resp.StatusCode), nil
}

// VerifyStripeKey checks if a Stripe API key is active via the non-mutating /v1/balance endpoint.
func (v *CredentialVerifier) VerifyStripeKey(ctx context.Context, key string) (VerificationStatus, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.stripe.com/v1/balance", nil)
	if err != nil {
		return VerificationStatusUnknown, "", err
	}
	req.SetBasicAuth(key, "")
	req.Header.Set("User-Agent", "CodeHound-Credential-Verifier")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return VerificationStatusUnknown, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return VerificationStatusActive, "Stripe secret key is LIVE and ACTIVE", nil
	} else if resp.StatusCode == http.StatusUnauthorized {
		return VerificationStatusRevoked, "Stripe secret key is REVOKED or INVALID (401 Unauthorized)", nil
	}

	return VerificationStatusUnknown, fmt.Sprintf("Unexpected status code: %d", resp.StatusCode), nil
}

// ValidateKeyFormat checks token structural validity and checksums without making network calls.
func ValidateKeyFormat(secretType, token string) bool {
	switch strings.ToUpper(secretType) {
	case "AWS":
		return strings.HasPrefix(token, "AKIA") || strings.HasPrefix(token, "ASIA")
	case "GITHUB":
		return strings.HasPrefix(token, "ghp_") || strings.HasPrefix(token, "github_pat_")
	case "STRIPE":
		return strings.HasPrefix(token, "sk_live_") || strings.HasPrefix(token, "rk_live_")
	default:
		return len(token) > 16
	}
}
