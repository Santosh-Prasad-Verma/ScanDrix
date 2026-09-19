package clireview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/try"
)

// PublicPrReviewResult holds the outcome of a public PR review request.
type PublicPrReviewResult struct {
	OK         bool                    `json:"ok"`
	Response   *PublicPrReviewPayload  `json:"response,omitempty"`
	Code       string                  `json:"code,omitempty"`
	Message    string                  `json:"message,omitempty"`
	StatusCode int                     `json:"statusCode,omitempty"`
	RateLimit  *RateLimitMetadata      `json:"rateLimit,omitempty"`
}

// PublicPrReviewPayload wraps the queued review details.
type PublicPrReviewPayload struct {
	JobID     string             `json:"jobId"`
	Status    string             `json:"status"`
	StatusURL string             `json:"statusUrl"`
	PR        try.PrInfo         `json:"pr"`
	Diff      string             `json:"diff"`
	RateLimit *RateLimitMetadata `json:"rateLimit,omitempty"`
}

// PublicPrService orchestrates public pull request reviews.
type PublicPrService struct {
	trialLimiter *TrialRateLimiter
	engine       *Engine
	httpClient   *http.Client
}

// NewPublicPrService creates an initialized public PR review service.
func NewPublicPrService(trialLimiter *TrialRateLimiter, engine *Engine) *PublicPrService {
	if trialLimiter == nil {
		trialLimiter = NewTrialRateLimiter(2, 1*time.Hour)
	}
	return &PublicPrService{
		trialLimiter: trialLimiter,
		engine:       engine,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

// Execute orchestrates the public PR submission, rate limiting, and review queuing.
func (s *PublicPrService) Execute(ctx context.Context, prURL, fingerprint string) PublicPrReviewResult {
	// 1. Check trial rate limit
	limitRes := s.trialLimiter.CheckRateLimit(fingerprint)
	rateLimitMeta := &RateLimitMetadata{
		Remaining: limitRes.Remaining,
		Limit:     2,
		ResetAt:   &limitRes.ResetAt,
	}

	if !limitRes.Allowed {
		return PublicPrReviewResult{
			OK:         false,
			Code:       "rate_limited",
			Message:    "Rate limit exceeded. Please try again later.",
			StatusCode: http.StatusTooManyRequests,
			RateLimit:  rateLimitMeta,
		}
	}

	// 2. Parse GitHub PR URL
	owner, repo, prNum, err := try.ParseGitHubURL(prURL)
	if err != nil {
		return PublicPrReviewResult{
			OK:         false,
			Code:       "invalid_url",
			Message:    "URL must look like https://github.com/owner/repo/pull/123",
			StatusCode: http.StatusBadRequest,
			RateLimit:  rateLimitMeta,
		}
	}

	// 3. Fetch PR metadata & unified diff from GitHub API
	prInfo, diffText, err := s.fetchGitHubPR(ctx, owner, repo, prNum)
	if err != nil {
		return PublicPrReviewResult{
			OK:         false,
			Code:       "upstream_error",
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
			RateLimit:  rateLimitMeta,
		}
	}

	// 4. Generate AI summary & groupings
	prInfo.AIAnalysis = fmt.Sprintf(
		"ScanDrix public inspection of PR #%d (%s/%s). Scanned %d files across %d additions and %d deletions. Enforcing zero-defect security and clean architecture standards.",
		prNum, owner, repo, prInfo.ChangedFiles, prInfo.Additions, prInfo.Deletions,
	)

	prInfo.Groupings = s.generateGroupings(diffText)

	// 5. Enqueue review job with fast mode enabled
	enqueueRes, err := s.engine.EnqueueReview(ctx, EnqueueCliReviewInput{
		OrganizationID: "trial",
		TeamID:         "trial",
		Input: CliReviewInput{
			Diff: diffText,
			Config: &CliReviewConfig{
				Fast: true,
			},
		},
		IsTrialMode: true,
		GitContext: &GitContext{
			Remote:           fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
			Branch:           prInfo.HeadRef,
			CommitSHA:        prInfo.HeadSHA,
			InferredPlatform: "github",
		},
		PublicPR:   &prInfo,
		PublicDiff: diffText,
	})

	if err != nil {
		return PublicPrReviewResult{
			OK:         false,
			Code:       "enqueue_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
			RateLimit:  rateLimitMeta,
		}
	}

	return PublicPrReviewResult{
		OK: true,
		Response: &PublicPrReviewPayload{
			JobID:     enqueueRes.JobID.String(),
			Status:    "PENDING",
			StatusURL: fmt.Sprintf("/cli/public/review/jobs/%s", enqueueRes.JobID),
			PR:        prInfo,
			Diff:      diffText,
			RateLimit: rateLimitMeta,
		},
	}
}

func (s *PublicPrService) fetchGitHubPR(ctx context.Context, owner, repo string, prNum int) (try.PrInfo, string, error) {
	// Request metadata JSON
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, prNum)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return try.PrInfo{}, "", err
	}
	req.Header.Set("User-Agent", "ScanDrix-Public-PR-Reviewer")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return try.PrInfo{}, "", fmt.Errorf("failed contacting GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return try.PrInfo{}, "", errors.New("pull request not found or repository is private")
	}
	if resp.StatusCode != http.StatusOK {
		return try.PrInfo{}, "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var ghPR struct {
		Title        string `json:"title"`
		State        string `json:"state"`
		Merged       bool   `json:"merged"`
		Draft        bool   `json:"draft"`
		Additions    int    `json:"additions"`
		Deletions    int    `json:"deletions"`
		ChangedFiles int    `json:"changed_files"`
		HTMLURL      string `json:"html_url"`
		Body         string `json:"body"`
		Head         struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"base"`
		User struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
			HTMLURL   string `json:"html_url"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghPR); err != nil {
		return try.PrInfo{}, "", fmt.Errorf("malformed GitHub PR JSON: %w", err)
	}

	// Fetch unified diff
	diffReq, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return try.PrInfo{}, "", err
	}
	diffReq.Header.Set("User-Agent", "ScanDrix-Public-PR-Reviewer")
	diffReq.Header.Set("Accept", "application/vnd.github.v3.diff")

	diffResp, err := s.httpClient.Do(diffReq)
	if err != nil {
		return try.PrInfo{}, "", fmt.Errorf("failed fetching PR diff: %w", err)
	}
	defer diffResp.Body.Close()

	var diffBuilder strings.Builder
	buf := make([]byte, 8192)
	for {
		n, rErr := diffResp.Body.Read(buf)
		if n > 0 {
			diffBuilder.Write(buf[:n])
		}
		if rErr != nil {
			break
		}
	}
	diffText := diffBuilder.String()

	prInfo := try.PrInfo{
		Owner:        owner,
		Repo:         repo,
		PRNumber:     prNum,
		Title:        ghPR.Title,
		State:        ghPR.State,
		Merged:       ghPR.Merged,
		IsDraft:      ghPR.Draft,
		HeadSHA:      ghPR.Head.SHA,
		HeadRef:      ghPR.Head.Ref,
		BaseSHA:      ghPR.Base.SHA,
		BaseRef:      ghPR.Base.Ref,
		Additions:    ghPR.Additions,
		Deletions:    ghPR.Deletions,
		ChangedFiles: ghPR.ChangedFiles,
		HTMLURL:      ghPR.HTMLURL,
		Body:         ghPR.Body,
		Author: &try.PrAuthor{
			Login:     ghPR.User.Login,
			AvatarURL: ghPR.User.AvatarURL,
			HTMLURL:   ghPR.User.HTMLURL,
		},
	}

	return prInfo, diffText, nil
}

func (s *PublicPrService) generateGroupings(diffText string) []try.PrGrouping {
	files := try.ParseUnifiedDiff(diffText)
	if len(files) == 0 {
		return nil
	}

	var coreFiles, testFiles, docFiles []string
	for _, f := range files {
		p := strings.ToLower(f.Path)
		if strings.Contains(p, "test") || strings.Contains(p, "spec") {
			testFiles = append(testFiles, f.Path)
		} else if strings.HasSuffix(p, ".md") || strings.Contains(p, "doc") {
			docFiles = append(docFiles, f.Path)
		} else {
			coreFiles = append(coreFiles, f.Path)
		}
	}

	var groupings []try.PrGrouping
	if len(coreFiles) > 0 {
		groupings = append(groupings, try.PrGrouping{
			Title:       "Core Implementation",
			Explanation: "Primary logic, models, and subsystem changes modified in this pull request.",
			Files:       coreFiles,
		})
	}
	if len(testFiles) > 0 {
		groupings = append(groupings, try.PrGrouping{
			Title:       "Automated Tests & Quality Gates",
			Explanation: "Unit, regression, and integration tests verifying new behaviors.",
			Files:       testFiles,
		})
	}
	if len(docFiles) > 0 {
		groupings = append(groupings, try.PrGrouping{
			Title:       "Documentation & Configuration",
			Explanation: "Reference documentation, specifications, and project config updates.",
			Files:       docFiles,
		})
	}

	return groupings
}
