package lease

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/config"
	"github.com/scandrix/backend/internal/sandbox/local"
)

const (
	CleanupConcurrency   = 5
	ReaperInterval       = 5 * time.Minute
	IdleKillInterval     = 30 * time.Second
	StaleCleanupDuration = 5 * time.Minute
)

// SandboxLeaseReaper manages background garbage collection of expired and idle sandboxes.
type SandboxLeaseReaper struct {
	repo       ISandboxLeaseRepository
	cfg        *config.Config
	httpClient *http.Client
	killFn     func(ctx context.Context, sandboxID string) error
}

// NewSandboxLeaseReaper creates a new lease reaper service.
func NewSandboxLeaseReaper(repo ISandboxLeaseRepository, cfg *config.Config) *SandboxLeaseReaper {
	return &SandboxLeaseReaper{
		repo: repo,
		cfg:  cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetKillHook allows tests or custom providers to override the remote microVM kill routine.
func (r *SandboxLeaseReaper) SetKillHook(fn func(ctx context.Context, sandboxID string) error) {
	r.killFn = fn
}

// ReapExpiredLeases reclaims resources for all leases whose expires_at timestamp has passed.
func (r *SandboxLeaseReaper) ReapExpiredLeases(ctx context.Context) error {
	now := time.Now().UTC()
	expired, err := r.repo.FindExpired(ctx, now)
	if err != nil {
		return fmt.Errorf("failed querying expired leases: %w", err)
	}

	if len(expired) == 0 {
		return nil
	}

	slog.Info("SandboxLeaseReaper: sweeping expired leases", "expired_count", len(expired))
	return r.processConcurrently(ctx, expired, func(lease *SandboxLease) error {
		if local.IsLocalSandboxPath(lease.SandboxID) {
			return r.cleanupLocalLease(ctx, lease, false)
		}

		if lease.SandboxID != "" && lease.State != StateInvalidated {
			if err := r.killRemoteSandbox(ctx, lease.SandboxID); err != nil {
				slog.Warn("SandboxLeaseReaper: failed killing remote sandbox", "sandbox_id", lease.SandboxID, "error", err)
			}
		}

		_ = r.repo.Delete(ctx, lease.PrKey)
		slog.Info("SandboxLeaseReaper: reaped expired lease", "pr_key", lease.PrKey, "sandbox_id", lease.SandboxID)
		return nil
	})
}

// KillIdleSandboxes terminates microVMs whose idle-kill timestamp (kill_at) has elapsed.
func (r *SandboxLeaseReaper) KillIdleSandboxes(ctx context.Context) error {
	now := time.Now().UTC()
	ready, err := r.repo.FindReadyToKill(ctx, now)
	if err != nil {
		return fmt.Errorf("failed querying idle leases: %w", err)
	}

	if len(ready) == 0 {
		return nil
	}

	slog.Info("SandboxLeaseReaper: sweeping idle sandboxes", "idle_count", len(ready))
	return r.processConcurrently(ctx, ready, func(lease *SandboxLease) error {
		if local.IsLocalSandboxPath(lease.SandboxID) {
			return r.cleanupLocalLease(ctx, lease, true)
		}

		if lease.SandboxID != "" {
			if err := r.killRemoteSandbox(ctx, lease.SandboxID); err != nil {
				slog.Warn("SandboxLeaseReaper: failed killing idle remote sandbox", "sandbox_id", lease.SandboxID, "error", err)
			}
		}

		_ = r.repo.Delete(ctx, lease.PrKey)
		slog.Info("SandboxLeaseReaper: killed idle sandbox and removed lease", "pr_key", lease.PrKey, "sandbox_id", lease.SandboxID)
		return nil
	})
}

func (r *SandboxLeaseReaper) cleanupLocalLease(ctx context.Context, lease *SandboxLease, requireLeaseCountZero bool) error {
	if lease.SandboxID == "" || !local.IsLocalSandboxPath(lease.SandboxID) {
		return nil
	}

	current, err := r.repo.FindByPrKey(ctx, lease.PrKey)
	if err != nil || current == nil {
		return nil
	}
	if requireLeaseCountZero && current.LeaseCount > 0 {
		return nil
	}

	// Reset stale in_progress claims older than 5m left by crashed workers
	staleThreshold := time.Now().UTC().Add(-StaleCleanupDuration)
	_ = r.repo.ResetStaleCleanup(ctx, lease.PrKey, staleThreshold)

	claimed, err := r.repo.ClaimCleanup(ctx, lease.PrKey, lease.SandboxID, requireLeaseCountZero)
	if err != nil || claimed == nil {
		return nil
	}

	if cleanErr := local.DeleteLocalSandbox(lease.SandboxID); cleanErr != nil {
		_, _ = r.repo.FailCleanup(ctx, lease.PrKey, lease.SandboxID, cleanErr.Error())
		slog.Warn("SandboxLeaseReaper: local cleanup failed", "pr_key", lease.PrKey, "sandbox_id", lease.SandboxID, "error", cleanErr)
		return cleanErr
	}

	_, _ = r.repo.CompleteCleanup(ctx, lease.PrKey, lease.SandboxID)
	slog.Info("SandboxLeaseReaper: cleaned local sandbox directory", "pr_key", lease.PrKey, "sandbox_id", lease.SandboxID)
	return nil
}

func (r *SandboxLeaseReaper) killRemoteSandbox(ctx context.Context, sandboxID string) error {
	if r.killFn != nil {
		return r.killFn(ctx, sandboxID)
	}

	apiKey := ""
	apiURL := "https://api.e2b.dev"
	if r.cfg != nil {
		apiKey = r.cfg.E2BAPIKey
		if r.cfg.E2BEndpoint != "" {
			apiURL = r.cfg.E2BEndpoint
		}
	}


	if apiKey == "" {
		return nil
	}

	url := fmt.Sprintf("%s/sandboxes/%s", strings.TrimRight(apiURL, "/"), sandboxID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}

	return fmt.Errorf("unexpected E2B kill status: %d", resp.StatusCode)
}

func (r *SandboxLeaseReaper) processConcurrently(
	ctx context.Context,
	leases []*SandboxLease,
	fn func(l *SandboxLease) error,
) error {
	sem := make(chan struct{}, CleanupConcurrency)
	var wg sync.WaitGroup

	for _, l := range leases {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(item *SandboxLease) {
			defer wg.Done()
			defer func() { <-sem }()
			_ = fn(item)
		}(l)
	}

	wg.Wait()
	return nil
}

// ═══════════════════════════════════════════════════════════════
// CRON JOB ADAPTERS FOR ScanDrix internal/cron/scheduler.go
// ═══════════════════════════════════════════════════════════════

type ReaperCronJob struct {
	reaper *SandboxLeaseReaper
}

func NewReaperCronJob(reaper *SandboxLeaseReaper) *ReaperCronJob {
	return &ReaperCronJob{reaper: reaper}
}

func (j *ReaperCronJob) Name() string {
	return "SandboxLeaseReaper"
}

func (j *ReaperCronJob) Interval() time.Duration {
	return ReaperInterval
}

func (j *ReaperCronJob) Run(ctx context.Context) error {
	return j.reaper.ReapExpiredLeases(ctx)
}

type IdleKillCronJob struct {
	reaper *SandboxLeaseReaper
}

func NewIdleKillCronJob(reaper *SandboxLeaseReaper) *IdleKillCronJob {
	return &IdleKillCronJob{reaper: reaper}
}

func (j *IdleKillCronJob) Name() string {
	return "SandboxIdleKill"
}

func (j *IdleKillCronJob) Interval() time.Duration {
	return IdleKillInterval
}

func (j *IdleKillCronJob) Run(ctx context.Context) error {
	return j.reaper.KillIdleSandboxes(ctx)
}
