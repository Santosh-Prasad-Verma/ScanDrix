package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GitHubGraphQLService provides access to GitHub's GraphQL v4 API for
// complex relational queries such as review threads, timeline items, and check suites.
type GitHubGraphQLService struct {
	graphqlURL string
	httpClient *http.Client
}

// NewGitHubGraphQLService creates a new instance of GitHubGraphQLService.
func NewGitHubGraphQLService(httpClient *http.Client, endpoint ...string) *GitHubGraphQLService {
	url := "https://api.github.com/graphql"
	if len(endpoint) > 0 && endpoint[0] != "" {
		url = endpoint[0]
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GitHubGraphQLService{
		graphqlURL: url,
		httpClient: httpClient,
	}
}

// GraphQLReviewThread represents a review discussion thread fetched via GraphQL.
type GraphQLReviewThread struct {
	ID         string                 `json:"id"`
	IsResolved bool                   `json:"isResolved"`
	Path       string                 `json:"path"`
	Line       int                    `json:"line"`
	StartLine  int                    `json:"startLine,omitempty"`
	Comments   []GraphQLThreadComment `json:"comments"`
}

// GraphQLThreadComment represents an individual comment within a GraphQL thread.
type GraphQLThreadComment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

// CheckSuiteStatus represents CI/CD check suite status from GitHub GraphQL.
type CheckSuiteStatus struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`     // "QUEUED", "IN_PROGRESS", "COMPLETED"
	Conclusion string    `json:"conclusion"` // "SUCCESS", "FAILURE", "NEUTRAL", "CANCELLED", "SKIPPED", "TIMED_OUT", "ACTION_REQUIRED"
	AppName    string    `json:"appName"`
	CreatedAt  time.Time `json:"createdAt"`
	CheckRuns  int       `json:"checkRuns"`
}

// SecurityVulnerabilityAlert represents Dependabot / code scanning alert from GitHub.
type SecurityVulnerabilityAlert struct {
	ID           string    `json:"id"`
	Severity     string    `json:"severity"` // "LOW", "MODERATE", "HIGH", "CRITICAL"
	PackageName  string    `json:"packageName"`
	AdvisoryGHSA string    `json:"advisoryGhsa"`
	Summary      string    `json:"summary"`
	CreatedAt    time.Time `json:"createdAt"`
	State        string    `json:"state"` // "OPEN", "FIXED", "DISMISSED"
}

// ExecuteQuery runs an arbitrary GraphQL query against the GitHub GraphQL v4 API.
func (s *GitHubGraphQLService) ExecuteQuery(
	ctx context.Context,
	token string,
	query string,
	variables map[string]any,
	result any,
) error {
	payload := map[string]any{
		"query": query,
	}
	if variables != nil {
		payload["variables"] = variables
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal graphql payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.graphqlURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create graphql request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(token, "Bearer "))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute graphql request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github graphql api error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Data   json.RawMessage  `json:"data"`
		Errors []map[string]any `json:"errors,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return fmt.Errorf("decode graphql response: %w", err)
	}

	if len(wrapper.Errors) > 0 {
		errBytes, _ := json.Marshal(wrapper.Errors)
		return fmt.Errorf("graphql execution errors: %s", string(errBytes))
	}

	if result != nil && len(wrapper.Data) > 0 {
		if err := json.Unmarshal(wrapper.Data, result); err != nil {
			return fmt.Errorf("unmarshal data into target: %w", err)
		}
	}
	return nil
}

// GetPullRequestReviewThreads queries all review threads for a pull request using GraphQL.
func (s *GitHubGraphQLService) GetPullRequestReviewThreads(
	ctx context.Context,
	token, owner, repo string,
	prNumber int,
) ([]GraphQLReviewThread, error) {
	query := `query($owner: String!, $repo: String!, $prNumber: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $prNumber) {
				reviewThreads(first: 100) {
					nodes {
						id
						isResolved
						path
						line
						startLine
						comments(first: 50) {
							nodes {
								id
								body
								createdAt
								author {
									login
								}
							}
						}
					}
				}
			}
		}
	}`

	variables := map[string]any{
		"owner":    owner,
		"repo":     repo,
		"prNumber": prNumber,
	}

	var data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []struct {
						ID         string `json:"id"`
						IsResolved bool   `json:"isResolved"`
						Path       string `json:"path"`
						Line       int    `json:"line"`
						StartLine  int    `json:"startLine"`
						Comments   struct {
							Nodes []struct {
								ID        string    `json:"id"`
								Body      string    `json:"body"`
								CreatedAt time.Time `json:"createdAt"`
								Author    struct {
									Login string `json:"login"`
								} `json:"author"`
							} `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	if err := s.ExecuteQuery(ctx, token, query, variables, &data); err != nil {
		return nil, err
	}

	threads := make([]GraphQLReviewThread, len(data.Repository.PullRequest.ReviewThreads.Nodes))
	for i, node := range data.Repository.PullRequest.ReviewThreads.Nodes {
		th := GraphQLReviewThread{
			ID:         node.ID,
			IsResolved: node.IsResolved,
			Path:       node.Path,
			Line:       node.Line,
			StartLine:  node.StartLine,
			Comments:   make([]GraphQLThreadComment, len(node.Comments.Nodes)),
		}
		for j, c := range node.Comments.Nodes {
			th.Comments[j] = GraphQLThreadComment{
				ID:        c.ID,
				Body:      c.Body,
				Author:    c.Author.Login,
				CreatedAt: c.CreatedAt,
			}
		}
		threads[i] = th
	}
	return threads, nil
}

// ResolveReviewThread marks a review thread as resolved via GraphQL mutation.
func (s *GitHubGraphQLService) ResolveReviewThread(
	ctx context.Context,
	token string,
	threadID string,
) error {
	mutation := `mutation($threadId: ID!) {
		resolveReviewThread(input: { threadId: $threadId }) {
			thread {
				id
				isResolved
			}
		}
	}`

	variables := map[string]any{
		"threadId": threadID,
	}

	return s.ExecuteQuery(ctx, token, mutation, variables, nil)
}

// UnresolveReviewThread marks a review thread as unresolved via GraphQL mutation.
func (s *GitHubGraphQLService) UnresolveReviewThread(
	ctx context.Context,
	token string,
	threadID string,
) error {
	mutation := `mutation($threadId: ID!) {
		unresolveReviewThread(input: { threadId: $threadId }) {
			thread {
				id
				isResolved
			}
		}
	}`

	variables := map[string]any{
		"threadId": threadID,
	}

	return s.ExecuteQuery(ctx, token, mutation, variables, nil)
}
