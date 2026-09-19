package contracts

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// BuildAuthHeader constructs the platform-specific HTTP Authorization header for git over HTTPS.
func BuildAuthHeader(platform models.SCMProvider, token, username string) (string, error) {
	if token == "" {
		return "", nil
	}

	switch platform {
	case models.ProviderGitHub:
		creds := "x-access-token:" + token
		return "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(creds)), nil

	case models.ProviderGitLab:
		creds := "oauth2:" + token
		return "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(creds)), nil

	case models.ProviderAzure:
		creds := "oauth2:" + token
		return "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(creds)), nil

	case models.ProviderBitbucket:
		gitUsername := username
		if strings.HasPrefix(token, "ATATT") {
			gitUsername = "x-bitbucket-api-token-auth"
		}
		if gitUsername == "" {
			return "", fmt.Errorf("bitbucket authentication requires a username or an Atlassian API token")
		}
		creds := gitUsername + ":" + token
		return "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(creds)), nil

	case models.ProviderForgejo:
		return "Authorization: token " + token, nil

	default:
		creds := "x-access-token:" + token
		return "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(creds)), nil
	}
}

// GetPRRefspec determines the remote refspec for fetching the PR/MR head.
func GetPRRefspec(platform models.SCMProvider, prNumber int, cloneURL, branch string) string {
	switch platform {
	case models.ProviderGitHub:
		return fmt.Sprintf("refs/pull/%d/head", prNumber)
	case models.ProviderGitLab:
		return fmt.Sprintf("refs/merge-requests/%d/head", prNumber)
	case models.ProviderBitbucket:
		if strings.Contains(strings.ToLower(cloneURL), "bitbucket.org") {
			return fmt.Sprintf("refs/heads/%s", branch)
		}
		return fmt.Sprintf("refs/pull-requests/%d/from", prNumber)
	case models.ProviderAzure:
		return fmt.Sprintf("refs/pull/%d/merge", prNumber)
	case models.ProviderForgejo:
		return fmt.Sprintf("refs/pull/%d/head", prNumber)
	default:
		return fmt.Sprintf("refs/pull/%d/head", prNumber)
	}
}
