package try

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
)

var (
	githubPRRegex = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/pull/(\d+)/?.*$`)
)

// PublicReviewService coordinates anonymous public PR reviews.
type PublicReviewService struct {
	mu         sync.RWMutex
	limiter    *RateLimiter
	featured   *FeaturedRegistry
	evaluator  *rules.Evaluator
	jobs       map[uuid.UUID]*ReviewJob
	httpClient *http.Client
	sem        chan struct{}
	stopChan   chan struct{}
	maxLines   int
	maxFiles   int
	maxJobs    int
}

// NewPublicReviewService initializes the public PR review engine.
func NewPublicReviewService(limiter *RateLimiter, featured *FeaturedRegistry, evaluator *rules.Evaluator) *PublicReviewService {
	svc := &PublicReviewService{
		limiter:    limiter,
		featured:   featured,
		evaluator:  evaluator,
		jobs:       make(map[uuid.UUID]*ReviewJob),
		httpClient: &http.Client{Timeout: 15 * time.Second},
		sem:        make(chan struct{}, 16),
		stopChan:   make(chan struct{}),
		maxLines:   10000,
		maxFiles:   80,
		maxJobs:    10000,
	}
	go svc.reapExpiredJobs()
	return svc
}

// Stop halts background maintenance routines.
func (s *PublicReviewService) Stop() {
	select {
	case <-s.stopChan:
	default:
		close(s.stopChan)
	}
}

// ParseGitHubURL extracts owner, repo, and pull number from a GitHub PR URL.
func ParseGitHubURL(rawURL string) (owner, repo string, pullNumber int, err error) {
	matches := githubPRRegex.FindStringSubmatch(strings.TrimSpace(rawURL))
	if len(matches) < 4 {
		return "", "", 0, errors.New("invalid_url: not a parseable public GitHub PR URL")
	}

	num, err := strconv.Atoi(matches[3])
	if err != nil || num <= 0 {
		return "", "", 0, errors.New("invalid_url: invalid pull request number")
	}

	return matches[1], matches[2], num, nil
}

// EnqueueReview handles validation, rate-limiting, diff scraping, and job registration.
func (s *PublicReviewService) EnqueueReview(ctx context.Context, req EnqueueRequest) (*EnqueueResponse, error) {
	if strings.TrimSpace(req.Fingerprint) == "" {
		return nil, errors.New("missing fingerprint")
	}

	// 1. Parse GitHub PR URL (validate before consuming rate limit)
	owner, repo, prNumber, err := ParseGitHubURL(req.PRURL)
	if err != nil {
		return nil, err
	}

	// 2. Rate Limit Check (2 reviews per hour per fingerprint)
	rateRes := s.limiter.CheckAndRecord(req.Fingerprint)
	if !rateRes.Allowed {
		return nil, fmt.Errorf("rate_limited: %d reviews per hour allowed, resets at %s", rateRes.Limit, rateRes.ResetAt.Format(time.RFC3339))
	}

	// 3. Enforce max concurrent jobs cap
	s.mu.RLock()
	jobCount := len(s.jobs)
	s.mu.RUnlock()
	if jobCount >= s.maxJobs {
		return nil, errors.New("service_busy: too many concurrent reviews, please try again later")
	}

	// 4. Fetch Unified Diff from GitHub
	diffURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d.diff", owner, repo, prNumber)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, diffURL, nil)
	if err != nil {
		return nil, fmt.Errorf("upstream_error: %w", err)
	}
	httpReq.Header.Set("User-Agent", "ScanDrix-Public-Review-Bot/1.0")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream_error: failed fetching github diff: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("requires_auth: PR is private or not found")
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream_error: github returned HTTP %d", resp.StatusCode)
	}

	rawDiffBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("upstream_error: error reading diff payload: %w", err)
	}
	rawDiff := string(rawDiffBytes)

	// 5. Parse diff and enforce size caps
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil {
		return nil, fmt.Errorf("invalid_diff: %w", err)
	}

	var totalAdditions, totalDeletions int
	for _, p := range patches {
		totalAdditions += p.Additions
		totalDeletions += p.Deletions
	}
	totalLines := totalAdditions + totalDeletions

	if len(patches) > s.maxFiles || totalLines > s.maxLines {
		return nil, fmt.Errorf("too_large: PR has %d files and %d lines (cap: %d files, %d lines)", len(patches), totalLines, s.maxFiles, s.maxLines)
	}

	// 6. Create Job
	jobID := uuid.New()
	prInfo := PrInfo{
		Owner:        owner,
		Repo:         repo,
		PRNumber:     prNumber,
		Title:        fmt.Sprintf("%s/%s PR #%d", owner, repo, prNumber),
		ChangedFiles: len(patches),
		Additions:    totalAdditions,
		Deletions:    totalDeletions,
		HTMLURL:      req.PRURL,
	}

	now := time.Now().UTC()
	job := &ReviewJob{
		JobID:       jobID,
		Status:      JobStatusPending,
		Fingerprint: req.Fingerprint,
		PR:          prInfo,
		Diff:        rawDiff,
		CreatedAt:   now,
	}

	s.mu.Lock()
	s.jobs[jobID] = job
	s.mu.Unlock()

	// Launch async review worker for public job with bounded context and worker slot
	jobCtx, jobCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	go s.processJob(jobCtx, jobCancel, jobID, patches, rawDiff)

	return &EnqueueResponse{
		JobID:     jobID,
		Status:    JobStatusPending,
		StatusURL: fmt.Sprintf("/cli/public/review/jobs/%s", jobID),
		PR:        prInfo,
		Diff:      rawDiff,
	}, nil
}

func (s *PublicReviewService) processJob(ctx context.Context, cancel context.CancelFunc, jobID uuid.UUID, patches []*diff.FilePatch, rawDiff string) {
	defer cancel()

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		s.mu.Lock()
		if j, ok := s.jobs[jobID]; ok {
			nowFail := time.Now().UTC()
			j.Status = JobStatusFailed
			j.CompletedAt = &nowFail
			j.Error = "timeout waiting for review worker slot"
		}
		s.mu.Unlock()
		return
	}

	// Recover from evaluator panics so the job never stays in PROCESSING forever.
	defer func() {
		if r := recover(); r != nil {
			nowFail := time.Now().UTC()
			s.mu.Lock()
			if j, ok := s.jobs[jobID]; ok {
				j.Status = JobStatusFailed
				j.CompletedAt = &nowFail
				j.Error = fmt.Sprintf("internal error: review engine panic: %v", r)
			}
			s.mu.Unlock()
		}
	}()

	s.mu.Lock()
	job, ok := s.jobs[jobID]
	if !ok {
		s.mu.Unlock()
		return
	}
	job.Status = JobStatusProcessing
	now := time.Now().UTC()
	job.StartedAt = &now
	s.mu.Unlock()

	start := time.Now()
	// Run deterministic rule evaluation
	findings := s.evaluator.EvaluatePatches(jobID, uuid.Nil, patches)

	var issues []ReviewIssue
	for _, f := range findings {
		issues = append(issues, ReviewIssue{
			File:           f.FilePath,
			Line:           f.StartLine,
			EndLine:        f.EndLine,
			Severity:       string(f.Severity),
			Category:       f.Category,
			Message:        f.Description,
			Suggestion:     f.SuggestedDiff,
			Recommendation: f.Remediation,
		})
	}

	summary := fmt.Sprintf("ScanDrix analyzed %d modified files and detected %d findings.", len(patches), len(issues))
	completedAt := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	job.Status = JobStatusCompleted
	job.CompletedAt = &completedAt
	job.Result = &ReviewResult{
		Summary:       summary,
		Issues:        issues,
		FilesAnalyzed: len(patches),
		Duration:      time.Since(start).Milliseconds(),
	}
}

// reapExpiredJobs periodically removes completed/failed jobs older than 1 hour
// to prevent unbounded memory growth on the anonymous public endpoint.
func (s *PublicReviewService) reapExpiredJobs() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			cutoff := time.Now().UTC().Add(-1 * time.Hour)
			s.mu.Lock()
			for id, job := range s.jobs {
				if (job.Status == JobStatusCompleted || job.Status == JobStatusFailed) && job.CompletedAt != nil && job.CompletedAt.Before(cutoff) {
					delete(s.jobs, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

// GetJob retrieves job status and optional payload for polling.
func (s *PublicReviewService) GetJob(jobID uuid.UUID, omitPayload bool) (*ReviewJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return nil, false
	}

	jobCopy := *job
	if omitPayload {
		jobCopy.Diff = ""
	}
	return &jobCopy, true
}
