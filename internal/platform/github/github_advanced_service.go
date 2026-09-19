package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ReviewThread represents a discussion thread on a pull request.
type ReviewThread struct {
	ID         string           `json:"id"`
	IsResolved bool             `json:"isResolved"`
	Path       string           `json:"path"`
	Line       int              `json:"line"`
	StartLine  int              `json:"startLine,omitempty"`
	Comments   []ThreadComment  `json:"comments"`
}

// ThreadComment represents an individual comment within a review thread.
type ThreadComment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// BatchReviewParams contains parameters for submitting a bulk code review.
type BatchReviewParams struct {
	Owner       string               `json:"owner"`
	Repo        string               `json:"repo"`
	PullNumber  int                  `json:"pullNumber"`
	CommitSHA   string               `json:"commitSha"`
	Body        string               `json:"body"`
	Event       string               `json:"event"` // "APPROVE", "REQUEST_CHANGES", "COMMENT"
	Comments    []BatchReviewComment `json:"comments"`
}

// BatchReviewComment represents an inline comment submitted as part of a batch review.
type BatchReviewComment struct {
	Path      string `json:"path"`
	Position  int    `json:"position,omitempty"`
	Line      int    `json:"line,omitempty"`
	Side      string `json:"side,omitempty"` // "LEFT", "RIGHT"
	StartLine int    `json:"startLine,omitempty"`
	StartSide string `json:"startSide,omitempty"`
	Body      string `json:"body"`
}

// BatchReviewResult models the result of submitting a batch review.
type BatchReviewResult struct {
	ID          int64     `json:"id"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submittedAt"`
	HTMLURL     string    `json:"htmlUrl"`
}

// ReactionCounts tallies emoji reactions on a comment.
type ReactionCounts struct {
	TotalCount int            `json:"totalCount"`
	PlusOne    int            `json:"plusOne"`
	MinusOne   int            `json:"minusOne"`
	Laugh      int            `json:"laugh"`
	Hooray     int            `json:"hooray"`
	Confused   int            `json:"confused"`
	Heart      int            `json:"heart"`
	Rocket     int            `json:"rocket"`
	Eyes       int            `json:"eyes"`
	Breakdown  map[string]int `json:"breakdown"`
}

// ReactionItem models an individual reaction entry.
type ReactionItem struct {
	ID        int64     `json:"id"`
	User      string    `json:"user"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// BranchProtectionRules models required reviews and status checks.
type BranchProtectionRules struct {
	Enabled               bool     `json:"enabled"`
	RequiredReviews       int      `json:"requiredReviews"`
	DismissStaleReviews   bool     `json:"dismissStaleReviews"`
	RequireCodeOwnerReview bool    `json:"requireCodeOwnerReview"`
	RequiredStatusChecks  []string `json:"requiredStatusChecks"`
	EnforceAdmins         bool     `json:"enforceAdmins"`
}

// DiffRange represents line boundaries within a unified diff hunk.
type DiffRange struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
	Position  int `json:"position"`
}

// GitHubAdvancedService provides extended GitHub REST and GraphQL capabilities.
type GitHubAdvancedService struct {
	baseURL    string
	httpClient *http.Client
	tokenCache sync.Map // map[string]*cachedToken
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// NewGitHubAdvancedService creates a new GitHubAdvancedService.
func NewGitHubAdvancedService(httpClient *http.Client, baseURL ...string) *GitHubAdvancedService {
	url := "https://api.github.com"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = strings.TrimRight(baseURL[0], "/")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GitHubAdvancedService{
		baseURL:    url,
		httpClient: httpClient,
	}
}

// CalculateDiffPosition maps a file path and target line number to a GitHub diff hunk position.
func (s *GitHubAdvancedService) CalculateDiffPosition(diffText, targetPath string, targetLine int) (int, error) {
	lines := strings.Split(diffText, "\n")
	inTargetFile := false
	inHunk := false
	currentLine := 0
	hunkPosition := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			inTargetFile = strings.Contains(line, "b/"+targetPath) || strings.Contains(line, targetPath)
			inHunk = false
			hunkPosition = 0
			currentLine = 0
			continue
		}

		if !inTargetFile {
			continue
		}

		if strings.HasPrefix(line, "@@") {
			inHunk = true
			// Parse hunk header: @@ -oldStart,oldLen +newStart,newLen @@
			parts := strings.Split(line, " ")
			if len(parts) >= 3 && strings.HasPrefix(parts[2], "+") {
				newInfo := strings.TrimPrefix(parts[2], "+")
				newParts := strings.Split(newInfo, ",")
				if parsed, err := strconv.Atoi(newParts[0]); err == nil {
					currentLine = parsed - 1
				}
			}
			continue
		}

		if !inHunk {
			continue
		}

		hunkPosition++

		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			currentLine++
			if currentLine == targetLine {
				return hunkPosition, nil
			}
		} else if !strings.HasPrefix(line, "-") {
			currentLine++
			if currentLine == targetLine {
				return hunkPosition, nil
			}
		}
	}

	return 0, fmt.Errorf("line %d not found in diff for %s", targetLine, targetPath)
}

// SubmitBatchReview posts a batch review containing multiple inline comments and verdict in one API call.
func (s *GitHubAdvancedService) SubmitBatchReview(
	ctx context.Context,
	token string,
	params BatchReviewParams,
) (*BatchReviewResult, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews",
		s.baseURL, params.Owner, params.Repo, params.PullNumber)

	payload := map[string]any{
		"body":  params.Body,
		"event": params.Event,
	}
	if params.CommitSHA != "" {
		payload["commit_id"] = params.CommitSHA
	}

	if len(params.Comments) > 0 {
		var commentsPayload []map[string]any
		for _, c := range params.Comments {
			item := map[string]any{
				"path": c.Path,
				"body": c.Body,
			}
			if c.Line > 0 {
				item["line"] = c.Line
				if c.Side != "" {
					item["side"] = c.Side
				}
			}
			if c.StartLine > 0 {
				item["start_line"] = c.StartLine
				if c.StartSide != "" {
					item["start_side"] = c.StartSide
				}
			}
			if c.Position > 0 && c.Line == 0 {
				item["position"] = c.Position
			}
			commentsPayload = append(commentsPayload, item)
		}
		payload["comments"] = commentsPayload
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var raw struct {
		ID          int64  `json:"id"`
		State       string `json:"state"`
		SubmittedAt string `json:"submitted_at"`
		HTMLURL     string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	submittedTime, _ := time.Parse(time.RFC3339, raw.SubmittedAt)
	return &BatchReviewResult{
		ID:          raw.ID,
		State:       raw.State,
		SubmittedAt: submittedTime,
		HTMLURL:     raw.HTMLURL,
	}, nil
}

// CountReactions retrieves and categorizes all emoji reactions on a comment.
func (s *GitHubAdvancedService) CountReactions(
	ctx context.Context,
	token, owner, repo string,
	commentID int64,
) (*ReactionCounts, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/comments/%d/reactions?per_page=100",
		s.baseURL, owner, repo, commentID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch reactions: status %d", resp.StatusCode)
	}

	var items []struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	counts := &ReactionCounts{
		Breakdown: make(map[string]int),
	}

	for _, item := range items {
		counts.TotalCount++
		counts.Breakdown[item.Content]++
		switch item.Content {
		case "+1":
			counts.PlusOne++
		case "-1":
			counts.MinusOne++
		case "laugh":
			counts.Laugh++
		case "hooray":
			counts.Hooray++
		case "confused":
			counts.Confused++
		case "heart":
			counts.Heart++
		case "rocket":
			counts.Rocket++
		case "eyes":
			counts.Eyes++
		}
	}

	return counts, nil
}

// GetBranchProtection inspects required status checks and review approvals for a branch.
func (s *GitHubAdvancedService) GetBranchProtection(
	ctx context.Context,
	token, owner, repo, branch string,
) (*BranchProtectionRules, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/branches/%s/protection",
		s.baseURL, owner, repo, branch)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &BranchProtectionRules{Enabled: false}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch branch protection: status %d", resp.StatusCode)
	}

	var raw struct {
		RequiredPullRequestReviews struct {
			RequiredApprovingReviewCount int  `json:"required_approving_review_count"`
			DismissStaleReviews          bool `json:"dismiss_stale_reviews"`
			RequireCodeOwnerReviews      bool `json:"require_code_owner_reviews"`
		} `json:"required_pull_request_reviews"`
		RequiredStatusChecks struct {
			Contexts []string `json:"contexts"`
		} `json:"required_status_checks"`
		EnforceAdmins struct {
			Enabled bool `json:"enabled"`
		} `json:"enforce_admins"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	return &BranchProtectionRules{
		Enabled:               true,
		RequiredReviews:       raw.RequiredPullRequestReviews.RequiredApprovingReviewCount,
		DismissStaleReviews:   raw.RequiredPullRequestReviews.DismissStaleReviews,
		RequireCodeOwnerReview: raw.RequiredPullRequestReviews.RequireCodeOwnerReviews,
		RequiredStatusChecks:  raw.RequiredStatusChecks.Contexts,
		EnforceAdmins:         raw.EnforceAdmins.Enabled,
	}, nil
}

// GetRepositoryLanguagesBreakdown returns the language byte counts for a repository.
func (s *GitHubAdvancedService) GetRepositoryLanguagesBreakdown(
	ctx context.Context,
	token, owner, repo string,
) (map[string]int64, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/languages", s.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch languages: status %d", resp.StatusCode)
	}

	var langs map[string]int64
	if err := json.NewDecoder(resp.Body).Decode(&langs); err != nil {
		return nil, err
	}

	return langs, nil
}

// CheckCollaboratorPermission checks the access level of a collaborator on a repository.
func (s *GitHubAdvancedService) CheckCollaboratorPermission(
	ctx context.Context,
	token, owner, repo, username string,
) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/collaborators/%s/permission",
		s.baseURL, owner, repo, username)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch collaborator permission: status %d", resp.StatusCode)
	}

	var raw struct {
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}

	return raw.Permission, nil
}

func (s *GitHubAdvancedService) setHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ScanDrix-Platform-Service")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
