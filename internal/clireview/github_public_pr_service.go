package clireview

import (
	"net/http"

	"github.com/scandrix/backend/internal/clireview/infrastructure/adapters"
)

// GitHubPublicPrService fetches pull request snapshots from the GitHub public API.
type GitHubPublicPrService = adapters.GitHubPublicPrService

// ParsePrURL extracts owner, repo, and PR number from a GitHub PR URL.
var ParsePrURL = adapters.ParsePrURL

// NewGitHubPublicPrService creates a new GitHub public PR client.
func NewGitHubPublicPrService(client *http.Client) *GitHubPublicPrService {
	return adapters.NewGitHubPublicPrService(client)
}
