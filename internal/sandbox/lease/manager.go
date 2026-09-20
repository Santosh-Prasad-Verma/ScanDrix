package lease

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/config"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/local"
	"github.com/scandrix/backend/internal/sandbox/null"
)

const (
	DefaultIdleTimeout = 5 * time.Minute      // 5 min default for interactive conversation flow
	ReviewIdleTimeout  = 30 * time.Second     // 30s for automated review pipeline
	DefaultLeaseTTL    = 30 * time.Minute     // 30 min default lease TTL before reaper cleans up
	PollInterval       = 500 * time.Millisecond
	MaxPollWait        = 30 * time.Second
	CreateMaxAttempts  = 3
)

// LeaseBoundSandboxInstance wraps a SandboxInstance and intercepts Cleanup to route through LeaseManager.Release.
type LeaseBoundSandboxInstance struct {
	contracts.SandboxInstance
	onCleanup func(ctx context.Context) error
}

func (l *LeaseBoundSandboxInstance) Cleanup(ctx context.Context) error {
	if l.onCleanup != nil {
		return l.onCleanup(ctx)
	}
	return l.SandboxInstance.Cleanup(ctx)
}

func (l *LeaseBoundSandboxInstance) Destroy(ctx context.Context) error {
	return l.Cleanup(ctx)
}

// SandboxLeaseManager coordinates shared sandbox microVMs and worktrees across reviews and chat flows.
type SandboxLeaseManager struct {
	provider          contracts.ISandboxProvider
	repo              ISandboxLeaseRepository
	cfg               *config.Config
	mu                sync.RWMutex
	leaseIDToPrKey    map[string]string
	connectExistingFn func(ctx context.Context, sandboxID string) (contracts.SandboxInstance, error)
}

// NewSandboxLeaseManager initializes a new lease manager.
func NewSandboxLeaseManager(
	provider contracts.ISandboxProvider,
	repo ISandboxLeaseRepository,
	cfg *config.Config,
) *SandboxLeaseManager {
	return &SandboxLeaseManager{
		provider:       provider,
		repo:           repo,
		cfg:            cfg,
		leaseIDToPrKey: make(map[string]string),
	}
}

// SetConnectExistingHook configures an optional hook for connecting to existing remote sandboxes (used in tests and E2B reconnect).
func (m *SandboxLeaseManager) SetConnectExistingHook(fn func(ctx context.Context, sandboxID string) (contracts.SandboxInstance, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectExistingFn = fn
}

// Acquire acquires a lease for the given prKey, creating or reusing the sandbox.
func (m *SandboxLeaseManager) Acquire(
	ctx context.Context,
	prKey, consumer string,
	leaseTTL time.Duration,
	cloneParams *contracts.CreateSandboxParams,
) (*contracts.AcquireResult, error) {
	if err := contracts.AssertValidPrKey(prKey); err != nil {
		return nil, err
	}

	if leaseTTL <= 0 {
		leaseTTL = DefaultLeaseTTL
	}

	slog.Info("SandboxLeaseManager: acquire requested", "pr_key", prKey, "consumer", consumer)

	doc, err := m.repo.UpsertAcquire(ctx, prKey, leaseTTL, consumer)
	if err != nil {
		return nil, fmt.Errorf("lease acquisition failed: %w", err)
	}

	// 1. If cleanup is in progress, wait for it to complete (doc will be deleted), then retry
	if doc.CleanupStatus == CleanupInProgress {
		_, _ = m.repo.DecrementLease(ctx, prKey)
		slog.Info("SandboxLeaseManager: cleanup in progress, waiting for completion", "pr_key", prKey)

		deadline := time.Now().Add(MaxPollWait)
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(PollInterval):
			}

			check, _ := m.repo.FindByPrKey(ctx, prKey)
			if check == nil {
				break // Cleanup finished and doc deleted
			}
		}
		return m.Acquire(ctx, prKey, consumer, leaseTTL, cloneParams)
	}

	if doc.CleanupStatus == CleanupFailed {
		_, _ = m.repo.DecrementLease(ctx, prKey)
		return nil, fmt.Errorf("SandboxLeaseManager: local cleanup previously failed for prKey=%q", prKey)
	}

	// 2. Acquire-vs-invalidated race: undo increment and return typed error
	if doc.State == StateInvalidated {
		_, _ = m.repo.DecrementLease(ctx, prKey)
		slog.Info("SandboxLeaseManager: acquired INVALIDATED lease, decremented count", "pr_key", prKey)
		return nil, &contracts.SandboxInvalidatedError{PrKey: prKey}
	}

	leaseID := uuid.New().String()

	// Atomically clear any scheduled idle-kill
	_ = m.repo.ClearKillAt(ctx, prKey)

	// Path A: Creator path (we inserted the doc)
	if doc.State == StateCreating && doc.LeaseCount == 1 {
		return m.handleCreatorPath(ctx, prKey, leaseID, consumer, cloneParams)
	}

	// Path B: Joiner path (doc already exists or another process is creating)
	result, err := m.handleJoinerPath(ctx, prKey, leaseID, consumer, doc.State, doc.SandboxID)
	if err != nil {
		var staleErr *contracts.SandboxStaleConnectionError
		if errors.As(err, &staleErr) {
			slog.Warn("SandboxLeaseManager: stale sandbox connection detected, retrying cold start", "pr_key", prKey, "sandbox_id", doc.SandboxID)
			return m.Acquire(ctx, prKey, consumer, leaseTTL, cloneParams)
		}
		return nil, err
	}

	return result, nil
}

func (m *SandboxLeaseManager) handleCreatorPath(
	ctx context.Context,
	prKey, leaseID, consumer string,
	cloneParams *contracts.CreateSandboxParams,
) (*contracts.AcquireResult, error) {
	slog.Info("SandboxLeaseManager: creator path initializing sandbox", "pr_key", prKey, "consumer", consumer)

	var sandbox contracts.SandboxInstance
	var sandboxID string

	cleanupCreated := func() {
		if sandbox != nil {
			_ = sandbox.Cleanup(ctx)
		}
		if sandboxID != "" && local.IsLocalSandboxPath(sandboxID) {
			_ = local.DeleteLocalSandbox(sandboxID)
		}
		_ = m.repo.Delete(ctx, prKey)
	}

	var err error
	if m.provider != nil && m.provider.IsAvailable() && cloneParams != nil {
		sandbox, err = m.createWithRetry(ctx, *cloneParams)
	} else {
		sandbox = null.NewNullSandboxInstance()
	}

	if err != nil {
		cleanupCreated()
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}

	sandboxID = sandbox.GetRepoDir()
	if sandboxID == "" {
		sandboxID = sandbox.GetID().String()
	}

	// Update lease to READY with registered sandbox ID
	if err := m.repo.UpdateReady(ctx, prKey, sandboxID); err != nil {
		cleanupCreated()
		return nil, fmt.Errorf("failed to transition lease to READY: %w", err)
	}

	// Check for mid-create invalidation race (PR closed or force pushed while creating)
	latestDoc, err := m.repo.FindByPrKey(ctx, prKey)
	if err == nil && latestDoc != nil && latestDoc.State == StateInvalidated {
		slog.Warn("SandboxLeaseManager: lease invalidated mid-create, killing orphaned sandbox", "pr_key", prKey, "sandbox_id", sandboxID)
		cleanupCreated()
		return nil, &contracts.SandboxInvalidatedError{PrKey: prKey, MidCreate: true}
	}

	m.mu.Lock()
	m.leaseIDToPrKey[leaseID] = prKey
	m.mu.Unlock()

	wrapped := &LeaseBoundSandboxInstance{
		SandboxInstance: sandbox,
		onCleanup: func(cleanCtx context.Context) error {
			return m.Release(cleanCtx, leaseID, nil)
		},
	}

	slog.Info("SandboxLeaseManager: sandbox READY (cold create)", "pr_key", prKey, "lease_id", leaseID, "sandbox_id", sandboxID)
	return &contracts.AcquireResult{
		Sandbox:    wrapped,
		LeaseID:    leaseID,
		SandboxID:  sandboxID,
		WasCreated: true,
	}, nil
}

func (m *SandboxLeaseManager) createWithRetry(
	ctx context.Context,
	params contracts.CreateSandboxParams,
) (contracts.SandboxInstance, error) {
	var lastErr error
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second}

	for attempt := 0; attempt < CreateMaxAttempts; attempt++ {
		instance, err := m.provider.CreateSandboxWithRepo(ctx, params)
		if err == nil {
			return instance, nil
		}
		lastErr = err

		if attempt == CreateMaxAttempts-1 {
			break
		}

		wait := backoffs[attempt]
		slog.Warn("SandboxLeaseManager: sandbox creation attempt failed, retrying",
			"attempt", attempt+1,
			"max_attempts", CreateMaxAttempts,
			"wait_ms", wait.Milliseconds(),
			"error", err,
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	return nil, lastErr
}

func (m *SandboxLeaseManager) handleJoinerPath(
	ctx context.Context,
	prKey, leaseID, consumer string,
	state LeaseState,
	sandboxID string,
) (*contracts.AcquireResult, error) {
	if state == StateInvalidated {
		return nil, &contracts.SandboxInvalidatedError{PrKey: prKey}
	}

	if state == StateReady && sandboxID != "" {
		return m.connectToExisting(ctx, prKey, leaseID, consumer, sandboxID)
	}

	// State is CREATING: poll until READY
	slog.Info("SandboxLeaseManager: joiner path polling for READY state", "pr_key", prKey, "consumer", consumer)

	deadline := time.Now().Add(MaxPollWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(PollInterval):
		}

		doc, err := m.repo.FindByPrKey(ctx, prKey)
		if err != nil || doc == nil {
			return nil, fmt.Errorf("SandboxLeaseManager: lease disappeared while polling for READY for prKey=%q", prKey)
		}

		if doc.State == StateInvalidated {
			return nil, &contracts.SandboxInvalidatedError{PrKey: prKey}
		}

		if doc.State == StateReady && doc.SandboxID != "" {
			return m.connectToExisting(ctx, prKey, leaseID, consumer, doc.SandboxID)
		}
	}

	return nil, &contracts.SandboxCreateTimeoutError{PrKey: prKey}
}

func (m *SandboxLeaseManager) connectToExisting(
	ctx context.Context,
	prKey, leaseID, consumer, sandboxID string,
) (*contracts.AcquireResult, error) {
	slog.Info("SandboxLeaseManager: connecting to existing sandbox", "pr_key", prKey, "consumer", consumer, "sandbox_id", sandboxID)

	var sandbox contracts.SandboxInstance
	var err error

	m.mu.RLock()
	hook := m.connectExistingFn
	m.mu.RUnlock()

	if hook != nil {
		sandbox, err = hook(ctx, sandboxID)
	} else if local.IsLocalSandboxPath(sandboxID) {
		// Existing local directory worktree
		sandbox = local.NewLocalSandboxInstance(sandboxID, "main")
	} else {
		// Fallback to null sandbox for testing or when remote client not provided
		sandbox = null.NewNullSandboxInstance()
	}

	if err != nil {
		slog.Warn("SandboxLeaseManager: stale sandbox connection failed, cleaning stale lease", "pr_key", prKey, "sandbox_id", sandboxID, "error", err)
		m.mu.Lock()
		delete(m.leaseIDToPrKey, leaseID)
		m.mu.Unlock()
		_ = m.repo.Delete(ctx, prKey)
		return nil, &contracts.SandboxStaleConnectionError{PrKey: prKey, SandboxID: sandboxID}
	}

	m.mu.Lock()
	m.leaseIDToPrKey[leaseID] = prKey
	m.mu.Unlock()

	wrapped := &LeaseBoundSandboxInstance{
		SandboxInstance: sandbox,
		onCleanup: func(cleanCtx context.Context) error {
			return m.Release(cleanCtx, leaseID, nil)
		},
	}

	slog.Info("SandboxLeaseManager: connected to existing sandbox (warm resume)", "pr_key", prKey, "lease_id", leaseID, "sandbox_id", sandboxID)
	return &contracts.AcquireResult{
		Sandbox:    wrapped,
		LeaseID:    leaseID,
		SandboxID:  sandboxID,
		WasCreated: false,
	}, nil
}

// Release releases a held lease. When lease_count reaches 0, schedules an idle-kill or cleans local worktree.
func (m *SandboxLeaseManager) Release(ctx context.Context, leaseID string, opts *contracts.ReleaseOptions) error {
	m.mu.Lock()
	prKey, ok := m.leaseIDToPrKey[leaseID]
	if ok {
		delete(m.leaseIDToPrKey, leaseID)
	}
	m.mu.Unlock()

	if !ok {
		slog.Warn("SandboxLeaseManager: release called with unknown leaseId", "lease_id", leaseID)
		return nil
	}

	updated, err := m.repo.DecrementLease(ctx, prKey)
	if err != nil {
		return fmt.Errorf("failed to decrement lease: %w", err)
	}

	remainingLeases := 0
	if updated != nil {
		remainingLeases = updated.LeaseCount
	}
	slog.Info("SandboxLeaseManager: released lease", "lease_id", leaseID, "pr_key", prKey, "remaining_leases", remainingLeases)

	if updated != nil && updated.LeaseCount <= 0 && updated.SandboxID != "" {
		// Local sandbox path: immediate cleanup
		if local.IsLocalSandboxPath(updated.SandboxID) {
			claimed, err := m.repo.ClaimCleanup(ctx, prKey, updated.SandboxID, true)
			if err == nil && claimed != nil {
				if cleanErr := local.DeleteLocalSandbox(updated.SandboxID); cleanErr != nil {
					_, _ = m.repo.FailCleanup(ctx, prKey, updated.SandboxID, cleanErr.Error())
					slog.Warn("SandboxLeaseManager: local sandbox directory cleanup failed", "sandbox_id", updated.SandboxID, "error", cleanErr)
				} else {
					_, _ = m.repo.CompleteCleanup(ctx, prKey, updated.SandboxID)
					slog.Info("SandboxLeaseManager: local sandbox cleaned immediately", "sandbox_id", updated.SandboxID, "pr_key", prKey)
				}
			}
			return nil
		}

		// E2B microVM path: schedule idle-kill
		idleTimeout := DefaultIdleTimeout
		if opts != nil && opts.IdleTimeout > 0 {
			idleTimeout = opts.IdleTimeout
		}

		killAt := time.Now().UTC().Add(idleTimeout)
		_ = m.repo.SetKillAt(ctx, prKey, killAt)
		slog.Info("SandboxLeaseManager: scheduled idle-kill for microVM", "pr_key", prKey, "sandbox_id", updated.SandboxID, "kill_at", killAt)
	}

	return nil
}

// Invalidate invalidates a sandbox for the given prKey, called on PR close or force-push.
func (m *SandboxLeaseManager) Invalidate(ctx context.Context, prKey string) error {
	slog.Info("SandboxLeaseManager: invalidating sandbox lease", "pr_key", prKey)

	_ = m.repo.ClearKillAt(ctx, prKey)

	doc, err := m.repo.FindByPrKey(ctx, prKey)
	if err != nil || doc == nil {
		slog.Info("SandboxLeaseManager: invalidate no-op (lease not found)", "pr_key", prKey)
		return nil
	}

	if doc.State == StateCreating {
		// Mid-create race: mark as INVALIDATED so the creator path terminates upon finish
		return m.repo.MarkInvalidated(ctx, prKey)
	}

	// READY or PAUSED state: cleanup
	if local.IsLocalSandboxPath(doc.SandboxID) {
		if doc.LeaseCount <= 0 {
			claimed, err := m.repo.ClaimCleanup(ctx, prKey, doc.SandboxID, true)
			if err == nil && claimed != nil {
				_ = local.DeleteLocalSandbox(doc.SandboxID)
				_, _ = m.repo.CompleteCleanup(ctx, prKey, doc.SandboxID)
			}
		} else {
			_ = m.repo.MarkInvalidated(ctx, prKey)
		}
		return nil
	}

	// E2B path: delete lease record
	return m.repo.Delete(ctx, prKey)
}
