package usecases

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/clireview/domain"
	"github.com/scandrix/backend/internal/try"
)

// PublicPrReviewInput parameters to trigger a public PR demo review.
type PublicPrReviewInput struct {
	PRURL       string `json:"prUrl"`
	Fingerprint string `json:"fingerprint"`
}

// IEnqueueCliReviewUseCase defines the enqueue interface.
type IEnqueueCliReviewUseCase interface {
	Execute(ctx context.Context, input domain.EnqueueCliReviewInput) (*domain.EnqueueCliReviewResult, error)
}

// PublicPrReviewUseCase orchestrates the public demonstration review workflow.
type PublicPrReviewUseCase struct {
	trialRateLimiter         domain.ITrialRateLimiterService
	githubPublicPrService    domain.IGitHubPublicPrService
	enqueueCliReviewUseCase  IEnqueueCliReviewUseCase
	publicPrAiSummaryService domain.IPublicPrAiSummaryService
	publicPrGroupingService  domain.IPublicPrGroupingService
}

// NewPublicPrReviewUseCase creates an initialized PublicPrReviewUseCase.
func NewPublicPrReviewUseCase(
	trialRateLimiter domain.ITrialRateLimiterService,
	githubPublicPrService domain.IGitHubPublicPrService,
	enqueueCliReviewUseCase IEnqueueCliReviewUseCase,
	publicPrAiSummaryService domain.IPublicPrAiSummaryService,
	publicPrGroupingService domain.IPublicPrGroupingService,
) *PublicPrReviewUseCase {
	return &PublicPrReviewUseCase{
		trialRateLimiter:         trialRateLimiter,
		githubPublicPrService:    githubPublicPrService,
		enqueueCliReviewUseCase:  enqueueCliReviewUseCase,
		publicPrAiSummaryService: publicPrAiSummaryService,
		publicPrGroupingService:  publicPrGroupingService,
	}
}

// Execute checks quota, queries GitHub, generates AI summaries and groups, and queues job.
func (uc *PublicPrReviewUseCase) Execute(ctx context.Context, input PublicPrReviewInput) (*domain.PublicPrReviewResult, error) {
	rateLimitResult, err := uc.trialRateLimiter.CheckRateLimit(ctx, input.Fingerprint)
	if err != nil || (rateLimitResult != nil && !rateLimitResult.Allowed) {
		rem := 0
		var resetAt *time.Time
		if rateLimitResult != nil {
			rem = rateLimitResult.Remaining
			resetAt = rateLimitResult.ResetAt
		}
		return &domain.PublicPrReviewResult{
			OK:         false,
			Code:       "rate_limited",
			Message:    "Rate limit exceeded. Please try again later.",
			StatusCode: 429,
			RateLimit: &domain.RateLimitMetadata{
				Remaining: rem,
				Limit:     2,
				ResetAt:   resetAt,
			},
		}, nil
	}

	pr, err := uc.githubPublicPrService.Fetch(ctx, input.PRURL)
	if err != nil {
		if fetchErr, ok := err.(*domain.PublicPrFetchError); ok {
			return &domain.PublicPrReviewResult{
				OK:         false,
				Code:       fetchErr.Code,
				Message:    fetchErr.Message,
				StatusCode: fetchErr.StatusCode,
			}, nil
		}
		return &domain.PublicPrReviewResult{
			OK:         false,
			Code:       "upstream_error",
			Message:    err.Error(),
			StatusCode: 500,
		}, nil
	}

	changedFiles := ExtractChangedFiles(pr.Diff)

	var aiAnalysis string
	var groupings []domain.PublicPrGrouping
	var wg sync.WaitGroup

	if uc.publicPrAiSummaryService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, genErr := uc.publicPrAiSummaryService.Generate(ctx, pr, pr.Diff)
			if genErr == nil {
				aiAnalysis = res
			}
		}()
	}

	if uc.publicPrGroupingService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, grpErr := uc.publicPrGroupingService.Generate(ctx, pr, pr.Diff, changedFiles)
			if grpErr == nil {
				groupings = res
			}
		}()
	}

	wg.Wait()
	_ = aiAnalysis
	_ = groupings

	jobID := uuid.New()
	if uc.enqueueCliReviewUseCase != nil {
		authorName := pr.Owner
		if pr.Author != nil && pr.Author.Login != "" {
			authorName = pr.Author.Login
		}

		enqueueRes, qErr := uc.enqueueCliReviewUseCase.Execute(ctx, domain.EnqueueCliReviewInput{
			OrganizationID: "trial",
			TeamID:         "trial",
			Input: domain.CliReviewInput{
				Diff: pr.Diff,
				Config: &domain.CliReviewConfig{
					Fast: true,
				},
			},
			IsTrialMode: true,
			GitContext: &domain.GitContext{
				Remote:           pr.CloneURL,
				Branch:           pr.HeadRef,
				CommitSHA:        pr.HeadSha,
				MergeBaseSHA:     pr.BaseSha,
				InferredPlatform: "github",
			},
			PublicPR: &try.PrInfo{
				Owner:          pr.Owner,
				Repo:           pr.Repo,
				PRNumber:       pr.PRNumber,
				Title:          pr.Title,
				AuthorUsername: authorName,
				HeadSHA:        pr.HeadSha,
				BaseSHA:        pr.BaseSha,
				Additions:      pr.Additions,
				Deletions:      pr.Deletions,
				ChangedFiles:   pr.ChangedFiles,
				Body:           pr.Body,
				AIAnalysis:     aiAnalysis,
			},
			PublicDiff: pr.Diff,
		})
		if qErr == nil && enqueueRes != nil {
			jobID = enqueueRes.JobID
		}
	}

	return &domain.PublicPrReviewResult{
		OK: true,
		Response: &domain.PublicPrReviewPayload{
			JobID:     jobID.String(),
			Status:    "PENDING",
			StatusURL: fmt.Sprintf("/cli/public/review/jobs/%s", jobID.String()),
			PR:        pr,
			Diff:      pr.Diff,
			RateLimit: &domain.RateLimitMetadata{
				Remaining: rateLimitResult.Remaining,
				Limit:     2,
				ResetAt:   rateLimitResult.ResetAt,
			},
		},
	}, nil
}

// ExtractChangedFiles extracts destination file paths from unified diff headers.
func ExtractChangedFiles(diff string) []string {
	var paths []string
	re := regexp.MustCompile(`(?m)^diff --git (?:"a/|a/).+?"? (?:"b/|b/)(.+?)"?$`)
	matches := re.FindAllStringSubmatch(diff, -1)
	for _, m := range matches {
		if len(m) >= 2 {
			paths = append(paths, m[1])
		}
	}
	return paths
}
