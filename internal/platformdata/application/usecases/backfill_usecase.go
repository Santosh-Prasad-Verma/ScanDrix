// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: usecases
// File: backfill_usecase.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/platformdata/domain/contracts"
	"github.com/scandrix/backend/internal/platformdata/domain/models"
)

const (
	DefaultPerPRDelayMS        = 2000 * time.Millisecond
	DefaultMaxBackfillPerRepo  = 10
)

// RepositoryTarget identifies a repository to be backfilled.
type RepositoryTarget struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"fullName,omitempty"`
	URL      string `json:"url,omitempty"`
}

// BackfillHistoricalPRsInput specifies the backfill parameters.
type BackfillHistoricalPRsInput struct {
	OrganizationID string
	Repositories   []RepositoryTarget
	StartDate      string
	EndDate        string
}

// SCMClientFetcher abstracts fetching historical pull requests from SCM providers.
type SCMClientFetcher interface {
	FetchRecentPullRequests(ctx context.Context, repoFullName string, limit int) ([]*models.PullRequest, error)
}

// BackfillHistoricalPRsUseCase runs background synchronization of historical pull requests.
type BackfillHistoricalPRsUseCase struct {
	repo       contracts.IPullRequestsRepository
	fetcher    SCMClientFetcher
	logger     *slog.Logger
	perPRDelay time.Duration
	maxPerRepo int
}

// NewBackfillHistoricalPRsUseCase constructs a rate-limited historical PR backfill usecase.
func NewBackfillHistoricalPRsUseCase(
	repo contracts.IPullRequestsRepository,
	fetcher SCMClientFetcher,
	logger *slog.Logger,
) *BackfillHistoricalPRsUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &BackfillHistoricalPRsUseCase{
		repo:       repo,
		fetcher:    fetcher,
		logger:     logger.With("component", "backfill_historical_prs_usecase"),
		perPRDelay: DefaultPerPRDelayMS,
		maxPerRepo: DefaultMaxBackfillPerRepo,
	}
}

// Execute triggers the background backfill workflow across provided repositories.
func (uc *BackfillHistoricalPRsUseCase) Execute(ctx context.Context, input BackfillHistoricalPRsInput) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				uc.logger.Error("Historical PR backfill recovered from panic", "panic", fmt.Sprintf("%v", r))
			}
		}()

		bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		uc.logger.Info("Starting historical PR backfill",
			"orgId", input.OrganizationID,
			"repoCount", len(input.Repositories),
			"maxPerRepo", uc.maxPerRepo,
		)

		for _, repoTarget := range input.Repositories {
			select {
			case <-bgCtx.Done():
				uc.logger.Warn("Historical PR backfill timed out or was cancelled")
				return
			default:
				uc.backfillRepository(bgCtx, input.OrganizationID, repoTarget)
			}
		}

		uc.logger.Info("Historical PR backfill completed for all repositories",
			"orgId", input.OrganizationID,
		)
	}()
}

func (uc *BackfillHistoricalPRsUseCase) backfillRepository(
	ctx context.Context,
	orgID string,
	target RepositoryTarget,
) {
	if uc.fetcher == nil {
		uc.logger.Debug("No SCM fetcher registered; skipping remote PR sync", "repo", target.Name)
		return
	}

	repoName := target.FullName
	if repoName == "" {
		repoName = target.Name
	}

	prs, err := uc.fetcher.FetchRecentPullRequests(ctx, repoName, uc.maxPerRepo)
	if err != nil {
		uc.logger.Warn("Failed fetching historical PRs from remote provider",
			"repo", repoName,
			"error", err,
		)
		return
	}

	uc.logger.Info("Fetched historical PRs for backfill",
		"repo", repoName,
		"count", len(prs),
	)

	for _, pr := range prs {
		pr.OrganizationID = orgID
		if pr.Repository.ID == "" {
			pr.Repository.ID = target.ID
		}
		if pr.Repository.Name == "" {
			pr.Repository.Name = target.Name
		}
		if pr.Repository.FullName == "" {
			pr.Repository.FullName = repoName
		}

		_, err := uc.repo.Create(ctx, pr)
		if err != nil {
			uc.logger.Warn("Failed persisting backfilled PR",
				"repo", repoName,
				"prNumber", pr.Number,
				"error", err,
			)
		}

		// Rate limiting sleep to stay within SCM API quotas
		select {
		case <-ctx.Done():
			return
		case <-time.After(uc.perPRDelay):
		}
	}
}
