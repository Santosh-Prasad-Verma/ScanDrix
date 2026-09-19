package stages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/sandbox/contracts"
)

// ReviewIdleTimeout defines the warm-resume window after review completes.
const ReviewIdleTimeout = 30 * time.Second

// leasedSandboxInstance decorates a SandboxInstance so its Cleanup delegates to the lease manager with warm-release.
type leasedSandboxInstance struct {
	contracts.SandboxInstance
	cleanupFn func(ctx context.Context) error
}

func (l *leasedSandboxInstance) Cleanup(ctx context.Context) error {
	if l.cleanupFn != nil {
		return l.cleanupFn(ctx)
	}
	return l.SandboxInstance.Cleanup(ctx)
}

// CreateSandboxStage acquires an ephemeral sandbox lease for the review pipeline.
// If the sandbox cannot be acquired or was superseded, the review continues in self-contained mode.
type CreateSandboxStage struct {
	leaseManager contracts.ISandboxLeaseManager
}

// NewCreateSandboxStage initializes a new CreateSandboxStage.
func NewCreateSandboxStage(leaseManager contracts.ISandboxLeaseManager) *CreateSandboxStage {
	return &CreateSandboxStage{
		leaseManager: leaseManager,
	}
}

func (s *CreateSandboxStage) Name() string {
	return "create_sandbox"
}

func (s *CreateSandboxStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	// Guard 1: Skip if sandbox already exists in pipeline context
	if pCtx.SandboxHandle != nil {
		slog.Info("Sandbox already exists in pipeline context, skipping creation",
			"review_id", pCtx.ReviewID,
		)
		return nil
	}

	// Guard 2: Skip if lease manager is not configured (e.g. static-only tests)
	if s.leaseManager == nil {
		slog.Info("No sandbox lease manager configured, continuing review self-contained",
			"review_id", pCtx.ReviewID,
		)
		return nil
	}

	// Guard 3: Skip if no changed files or diff content
	if len(pCtx.FilteredPatches) == 0 && len(pCtx.ParsedPatches) == 0 && pCtx.RawDiff == "" {
		slog.Info("Skipping sandbox creation: no changed files in PR diff",
			"review_id", pCtx.ReviewID,
		)
		return nil
	}

	// Build stable per-PR key for lease coordination
	var prKey string
	var err error
	if pCtx.IsCLI {
		branch := pCtx.Branch
		if branch == "" {
			branch = "unknown"
		}
		prKey = fmt.Sprintf("%s:%s:cli:%s", pCtx.WorkspaceID.String(), pCtx.RepositoryID.String(), branch)
	} else {
		prKey, err = contracts.BuildPrKey(pCtx.WorkspaceID.String(), pCtx.RepositoryID.String(), pCtx.PullNumber)
		if err != nil {
			slog.Warn("Failed to construct valid prKey for sandbox lease; continuing self-contained",
				"error", err,
				"workspace_id", pCtx.WorkspaceID,
				"repo_id", pCtx.RepositoryID,
				"pr_number", pCtx.PullNumber,
			)
			return nil
		}
	}

	cloneURL := pCtx.CloneURL
	if cloneURL == "" && pCtx.RepoNamespace != "" {
		cloneURL = fmt.Sprintf("https://github.com/%s.git", pCtx.RepoNamespace)
	}

	cloneParams := contracts.CreateSandboxParams{
		CloneURL:        cloneURL,
		AuthToken:       pCtx.AuthToken,
		AuthUsername:    pCtx.AuthUsername,
		Branch:          pCtx.Branch,
		BaseBranch:      pCtx.BaseBranch,
		PRNumber:        pCtx.PullNumber,
		Platform:        pCtx.Provider,
		CheckoutSHA:     pCtx.HeadSHA,
		UnifiedDiff:     pCtx.RawDiff,
		SandboxMetadata: map[string]string{"stage": "review"},
	}

	slog.Info("Acquiring sandbox lease for code review",
		"pr_key", prKey,
		"review_id", pCtx.ReviewID,
		"pr_number", pCtx.PullNumber,
		"platform", cloneParams.Platform,
	)

	acq, err := s.leaseManager.Acquire(ctx, prKey, "review", 0, &cloneParams)
	if err != nil {
		var invErr *contracts.SandboxInvalidatedError
		if errors.As(err, &invErr) {
			slog.Warn("Sandbox lease superseded (PR closed or force-pushed) — continuing self-contained",
				"pr_key", prKey,
				"error", err,
			)
			return nil
		}
		slog.Error("Failed to acquire sandbox lease (all retries exhausted), continuing self-contained",
			"pr_key", prKey,
			"error", err,
		)
		return nil
	}

	sandbox := acq.Sandbox
	leaseID := acq.LeaseID

	// Wrap Cleanup so pipeline finalizer releases lease with warm idle window rather than destroying immediately
	wrapped := &leasedSandboxInstance{
		SandboxInstance: sandbox,
		cleanupFn: func(cleanCtx context.Context) error {
			_ = sandbox.Cleanup(cleanCtx)
			return s.leaseManager.Release(cleanCtx, leaseID, &contracts.ReleaseOptions{
				IdleTimeout: ReviewIdleTimeout,
			})
		},
	}

	pCtx.SandboxHandle = wrapped
	pCtx.SandboxLeaseID = leaseID
	pCtx.SandboxPlatform = string(sandbox.GetTier())

	slog.Info("Sandbox lease successfully acquired for code review",
		"pr_key", prKey,
		"lease_id", leaseID,
		"sandbox_type", sandbox.GetTier(),
		"base_branch", sandbox.GetBaseBranch(),
		"was_created", acq.WasCreated,
	)

	return nil
}
