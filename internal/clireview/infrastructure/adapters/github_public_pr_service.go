package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// ParseURL on GitHubPublicPrService parses a PR URL.
func (s *GitHubPublicPrService) ParseURL(rawURL string) (*domain.ParsedPrURL, error) {
	return ParsePrURL(rawURL)
}

func detectOtherProvider(host, pathname string) string {
	path := strings.ToLower(pathname)

	if host == "gitlab.com" || strings.HasSuffix(host, ".gitlab.com") || strings.Contains(path, "/-/merge_requests/") {
		return "GitLab"
	}

	if host == "bitbucket.org" || strings.HasSuffix(host, ".bitbucket.org") || strings.Contains(path, "/pull-requests/") {
		return "Bitbucket"
	}

	if host == "dev.azure.com" || strings.HasSuffix(host, ".visualstudio.com") || strings.Contains(path, "/_git/") || strings.Contains(path, "/pullrequest/") {
		return "Azure DevOps"
	}

	if host != "github.com" && host != "www.github.com" {
		pullRe := regexp.MustCompile(`/pull/\d+`)
		if pullRe.MatchString(path) {
			return "GitHub Enterprise"
		}
	}

	return ""
}

// ParsePrURL extracts owner, repo, and PR number from a GitHub PR URL.
func ParsePrURL(rawURL string) (*domain.ParsedPrURL, error) {
	clean := strings.TrimSpace(rawURL)
	u, err := url.Parse(clean)
	if err != nil || u.Host == "" {
		return nil, &domain.PublicPrFetchError{
			Message:    "Invalid URL — paste a full https://github.com/... URL",
			Code:       "invalid_url",
			StatusCode: 400,
		}
	}

	host := strings.ToLower(u.Host)
	path := u.Path

	other := detectOtherProvider(host, path)
	if other != "" {
		return nil, &domain.PublicPrFetchError{
			Message:    fmt.Sprintf("ScanDrix's public demo only supports GitHub today. Sign up and connect %s to review %s PRs.", other, other),
			Code:       "requires_auth",
			StatusCode: 403,
		}
	}

	if host != "github.com" && host != "www.github.com" {
		return nil, &domain.PublicPrFetchError{
			Message:    "Only github.com URLs are supported in the public demo. Sign up to review PRs from self-hosted GitHub / GitLab / Bitbucket / Azure DevOps.",
			Code:       "requires_auth",
			StatusCode: 403,
		}
	}

	segments := strings.Split(strings.Trim(path, "/"), "/")
	pullIdx := -1
	for i, seg := range segments {
		if seg == "pull" {
			pullIdx = i
			break
		}
	}

	if pullIdx < 2 || pullIdx+1 >= len(segments) {
		return nil, &domain.PublicPrFetchError{
			Message:    "URL must be in the form github.com/owner/repo/pull/123",
			Code:       "invalid_url",
			StatusCode: 400,
		}
	}

	owner := segments[pullIdx-2]
	repo := strings.TrimSuffix(segments[pullIdx-1], ".git")
	prNumber, err := strconv.Atoi(segments[pullIdx+1])
	if err != nil || prNumber < 1 || owner == "" || repo == "" {
		return nil, &domain.PublicPrFetchError{
			Message:    "URL must be in the form github.com/owner/repo/pull/123",
			Code:       "invalid_url",
			StatusCode: 400,
		}
	}

	return &domain.ParsedPrURL{
		Owner:    owner,
		Repo:     repo,
		PRNumber: prNumber,
	}, nil
}

// GitHubPublicPrService fetches pull request snapshots from the GitHub public API.
type GitHubPublicPrService struct {
	httpClient *http.Client
	token      string
	cache      map[string]*cachedPrEntry
	mu         sync.RWMutex
}

type cachedPrEntry struct {
	data      *domain.PublicPrMetadata
	etag      string
	fetchedAt time.Time
}

// NewGitHubPublicPrService creates a new GitHub public PR client.
func NewGitHubPublicPrService(client *http.Client) *GitHubPublicPrService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_PAT")
	}

	return &GitHubPublicPrService{
		httpClient: client,
		token:      token,
		cache:      make(map[string]*cachedPrEntry),
	}
}

// Fetch retrieves full PR metadata, diff, commits, discussion, and checks.
func (s *GitHubPublicPrService) Fetch(ctx context.Context, prURL string) (*domain.PublicPrMetadata, error) {
	parsed, err := ParsePrURL(prURL)
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("%s/%s#%d", parsed.Owner, parsed.Repo, parsed.PRNumber)
	s.mu.RLock()
	cached := s.cache[cacheKey]
	s.mu.RUnlock()

	if cached != nil && time.Since(cached.fetchedAt) < 2*time.Minute {
		return cached.data, nil
	}

	apiBase := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", parsed.Owner, parsed.Repo, parsed.PRNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	if cached != nil && cached.etag != "" {
		req.Header.Set("If-None-Match", cached.etag)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && cached != nil {
		cached.fetchedAt = time.Now()
		return cached.data, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, &domain.PublicPrFetchError{
			Message:    "pull request not found or repository is private",
			Code:       "requires_auth",
			StatusCode: 403,
		}
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, &domain.PublicPrFetchError{
			Message:    "github rate limit exceeded for public PRs",
			Code:       "requires_auth",
			StatusCode: 403,
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &domain.PublicPrFetchError{
			Message:    fmt.Sprintf("github api error: status %d", resp.StatusCode),
			Code:       "upstream_error",
			StatusCode: resp.StatusCode,
		}
	}

	var ghPR struct {
		Title          string `json:"title"`
		State          string `json:"state"`
		Merged         bool   `json:"merged"`
		Draft          bool   `json:"draft"`
		Additions      int    `json:"additions"`
		Deletions      int    `json:"deletions"`
		ChangedFiles   int    `json:"changed_files"`
		Commits        int    `json:"commits"`
		Comments       int    `json:"comments"`
		ReviewComments int    `json:"review_comments"`
		HTMLURL        string `json:"html_url"`
		Body           string `json:"body"`
		Head           struct {
			Sha string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Sha  string `json:"sha"`
			Ref  string `json:"ref"`
			Repo struct {
				CloneURL string `json:"clone_url"`
			} `json:"repo"`
		} `json:"base"`
		User struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
			HTMLURL   string `json:"html_url"`
		} `json:"user"`
		Labels []struct {
			Name        string `json:"name"`
			Color       string `json:"color"`
			Description string `json:"description"`
		} `json:"labels"`
		Assignees []struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
			HTMLURL   string `json:"html_url"`
		} `json:"assignees"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(bodyBytes, &ghPR); err != nil {
		return nil, err
	}

	totalLines := ghPR.Additions + ghPR.Deletions
	if totalLines > 10000 {
		return nil, &domain.PublicPrFetchError{
			Message:    fmt.Sprintf("This PR has %d lines changed — a bit much for the free demo. Sign up (free) and Drixy reviews PRs of any size on your own repos.", totalLines),
			Code:       "too_large",
			StatusCode: 413,
		}
	}
	if ghPR.ChangedFiles > 80 {
		return nil, &domain.PublicPrFetchError{
			Message:    fmt.Sprintf("This PR touches %d files — past the free demo cap. Sign up (free) and Drixy reviews PRs of any size on your own repos.", ghPR.ChangedFiles),
			Code:       "too_large",
			StatusCode: 413,
		}
	}

	diff, _ := s.FetchDiff(ctx, parsed.Owner, parsed.Repo, parsed.PRNumber)

	var labels []domain.PublicPrLabel
	for _, l := range ghPR.Labels {
		labels = append(labels, domain.PublicPrLabel{
			Name:        l.Name,
			Color:       l.Color,
			Description: l.Description,
		})
	}

	var assignees []domain.PublicPrAssignee
	for _, a := range ghPR.Assignees {
		assignees = append(assignees, domain.PublicPrAssignee{
			Login:     a.Login,
			AvatarURL: a.AvatarURL,
			HTMLURL:   a.HTMLURL,
		})
	}

	commits, _ := s.FetchCommits(ctx, parsed.Owner, parsed.Repo, parsed.PRNumber)
	comments, _ := s.FetchComments(ctx, parsed.Owner, parsed.Repo, parsed.PRNumber)
	checks, _ := s.FetchChecks(ctx, parsed.Owner, parsed.Repo, ghPR.Head.Sha)
	reviewers, _ := s.FetchReviewers(ctx, parsed.Owner, parsed.Repo, parsed.PRNumber)

	meta := &domain.PublicPrMetadata{
		Owner:           parsed.Owner,
		Repo:            parsed.Repo,
		PRNumber:        parsed.PRNumber,
		Title:           ghPR.Title,
		State:           ghPR.State,
		Merged:          ghPR.Merged,
		IsDraft:         ghPR.Draft,
		HeadSha:         ghPR.Head.Sha,
		HeadRef:         ghPR.Head.Ref,
		BaseSha:         ghPR.Base.Sha,
		BaseRef:         ghPR.Base.Ref,
		Additions:       ghPR.Additions,
		Deletions:       ghPR.Deletions,
		ChangedFiles:    ghPR.ChangedFiles,
		CommitsCount:    ghPR.Commits,
		DiscussionCount: ghPR.Comments + ghPR.ReviewComments,
		HTMLURL:         ghPR.HTMLURL,
		CloneURL: func() string {
			if ghPR.Base.Repo.CloneURL != "" {
				return ghPR.Base.Repo.CloneURL
			}
			return fmt.Sprintf("https://github.com/%s/%s.git", parsed.Owner, parsed.Repo)
		}(),
		Diff: diff,
		Author: &domain.PublicPrAuthor{
			Login:     ghPR.User.Login,
			AvatarURL: ghPR.User.AvatarURL,
			HTMLURL:   ghPR.User.HTMLURL,
		},
		Reviewers: reviewers,
		Checks:    checks,
		Commits:   commits,
		Comments:  comments,
		Labels:    labels,
		Assignees: assignees,
		Body:      ghPR.Body,
	}

	etag := resp.Header.Get("ETag")
	s.mu.Lock()
	s.cache[cacheKey] = &cachedPrEntry{
		data:      meta,
		etag:      etag,
		fetchedAt: time.Now(),
	}
	s.mu.Unlock()

	return meta, nil
}

// FetchDiff fetches the raw unified diff for the PR.
func (s *GitHubPublicPrService) FetchDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	diffURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", diffURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3.diff")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch diff: status %d", resp.StatusCode)
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// FetchCommits fetches commit list for the PR.
func (s *GitHubPublicPrService) FetchCommits(ctx context.Context, owner, repo string, prNumber int) ([]domain.PublicPrCommit, error) {
	commitsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/commits?per_page=50", owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", commitsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	}
	defer resp.Body.Close()

	var ghCommits []struct {
		Sha    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
		Author struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"author"`
		HTMLURL string `json:"html_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghCommits); err != nil {
		return nil, err
	}

	var commits []domain.PublicPrCommit
	for _, c := range ghCommits {
		commits = append(commits, domain.PublicPrCommit{
			Sha:             c.Sha,
			Message:         c.Commit.Message,
			AuthorLogin:     c.Author.Login,
			AuthorAvatarURL: c.Author.AvatarURL,
			AuthoredAt:      c.Commit.Author.Date,
			HTMLURL:         c.HTMLURL,
		})
	}
	return commits, nil
}

// FetchComments fetches issue comments and review comments for the PR.
func (s *GitHubPublicPrService) FetchComments(ctx context.Context, owner, repo string, prNumber int) ([]domain.PublicPrComment, error) {
	issueCommentsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments?per_page=50", owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", issueCommentsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	}
	defer resp.Body.Close()

	var ghComments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
		User struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
		CreatedAt string `json:"created_at"`
		HTMLURL   string `json:"html_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghComments); err != nil {
		return nil, err
	}

	var comments []domain.PublicPrComment
	for _, c := range ghComments {
		comments = append(comments, domain.PublicPrComment{
			ID:              c.ID,
			AuthorLogin:     c.User.Login,
			AuthorAvatarURL: c.User.AvatarURL,
			Body:            c.Body,
			CreatedAt:       c.CreatedAt,
			HTMLURL:         c.HTMLURL,
			Kind:            "issue",
		})
	}
	return comments, nil
}

// FetchReviewers fetches requested reviewers and review states.
func (s *GitHubPublicPrService) FetchReviewers(ctx context.Context, owner, repo string, prNumber int) ([]domain.PublicPrReviewer, error) {
	reviewsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/reviews?per_page=50", owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", reviewsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	}
	defer resp.Body.Close()

	var ghReviews []struct {
		State string `json:"state"` // 'APPROVED' | 'CHANGES_REQUESTED' | 'COMMENTED' | 'PENDING'
		User  struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghReviews); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var reviewers []domain.PublicPrReviewer
	for _, r := range ghReviews {
		if r.User.Login == "" || seen[r.User.Login] {
			continue
		}
		seen[r.User.Login] = true

		state := strings.ToLower(r.State)
		reviewers = append(reviewers, domain.PublicPrReviewer{
			Login:     r.User.Login,
			AvatarURL: r.User.AvatarURL,
			State:     state,
		})
	}
	return reviewers, nil
}

// FetchChecks fetches check runs for the head commit.
func (s *GitHubPublicPrService) FetchChecks(ctx context.Context, owner, repo, headSha string) (*domain.PublicPrCheckSummary, error) {
	if headSha == "" {
		return nil, nil
	}

	checksURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s/check-runs?per_page=50", owner, repo, url.PathEscape(headSha))
	req, err := http.NewRequestWithContext(ctx, "GET", checksURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	}
	defer resp.Body.Close()

	var ghChecks struct {
		TotalCount int `json:"total_count"`
		CheckRuns  []struct {
			Status     string  `json:"status"` // 'completed' | 'in_progress' | 'queued'
			Conclusion *string `json:"conclusion"` // 'success' | 'failure' | 'neutral' | 'cancelled'
		} `json:"check_runs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghChecks); err != nil {
		return nil, err
	}

	passed := 0
	failed := 0
	pending := 0

	for _, cr := range ghChecks.CheckRuns {
		if cr.Status != "completed" {
			pending++
		} else if cr.Conclusion != nil && *cr.Conclusion == "success" {
			passed++
		} else {
			failed++
		}
	}

	conclusion := "unknown"
	if failed > 0 {
		conclusion = "failure"
	} else if pending > 0 {
		conclusion = "pending"
	} else if passed > 0 {
		conclusion = "success"
	}

	return &domain.PublicPrCheckSummary{
		Total:      ghChecks.TotalCount,
		Passed:     passed,
		Failed:     failed,
		Pending:    pending,
		Conclusion: conclusion,
	}, nil
}
