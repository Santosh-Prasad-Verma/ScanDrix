package stages_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/null"
	"github.com/scandrix/backend/pkg/models"
)

type mockLeaseManager struct {
	acquireFn    func(ctx context.Context, prKey, consumer string, leaseTTL time.Duration, cloneParams *contracts.CreateSandboxParams) (*contracts.AcquireResult, error)
	releaseFn    func(ctx context.Context, leaseID string, opts *contracts.ReleaseOptions) error
	invalidateFn func(ctx context.Context, prKey string) error

	acquiredCount int
	releasedCount int
	lastReleaseID string
	lastIdleSec   time.Duration
}

func (m *mockLeaseManager) Acquire(ctx context.Context, prKey, consumer string, leaseTTL time.Duration, cloneParams *contracts.CreateSandboxParams) (*contracts.AcquireResult, error) {
	m.acquiredCount++
	if m.acquireFn != nil {
		return m.acquireFn(ctx, prKey, consumer, leaseTTL, cloneParams)
	}
	sb := null.NewNullSandboxInstance()
	return &contracts.AcquireResult{
		Sandbox:    sb,
		LeaseID:    "lease-test-123",
		SandboxID:  "sb-null-123",
		WasCreated: true,
	}, nil
}

func (m *mockLeaseManager) Release(ctx context.Context, leaseID string, opts *contracts.ReleaseOptions) error {
	m.releasedCount++
	m.lastReleaseID = leaseID
	if opts != nil {
		m.lastIdleSec = opts.IdleTimeout
	}
	if m.releaseFn != nil {
		return m.releaseFn(ctx, leaseID, opts)
	}
	return nil
}

func (m *mockLeaseManager) Invalidate(ctx context.Context, prKey string) error {
	if m.invalidateFn != nil {
		return m.invalidateFn(ctx, prKey)
	}
	return nil
}

func TestCreateSandboxStage_SuccessfulAcquire(t *testing.T) {
	lm := &mockLeaseManager{}
	stage := stages.NewCreateSandboxStage(lm)

	wsID := uuid.New()
	repoID := uuid.New()
	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   wsID,
		RepositoryID:  repoID,
		RepoNamespace: "acme/payment-service",
		Provider:      models.ProviderGitHub,
		PullNumber:    42,
		HeadSHA:       "abcdef123456",
		RawDiff:       "diff --git a/main.go b/main.go\n+package main\n",
		FilteredPatches: []*diff.FilePatch{
			{NewPath: "main.go"},
		},
	}

	ctx := context.Background()
	err := stage.Execute(ctx, pCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pCtx.SandboxHandle == nil {
		t.Fatalf("expected SandboxHandle to be populated")
	}
	if pCtx.SandboxLeaseID != "lease-test-123" {
		t.Fatalf("expected leaseID lease-test-123, got %s", pCtx.SandboxLeaseID)
	}
	if lm.acquiredCount != 1 {
		t.Fatalf("expected 1 acquire call, got %d", lm.acquiredCount)
	}

	// Trigger cleanup and assert release called with 30s idle timeout
	err = pCtx.SandboxHandle.Cleanup(ctx)
	if err != nil {
		t.Fatalf("unexpected cleanup error: %v", err)
	}
	if lm.releasedCount != 1 {
		t.Fatalf("expected 1 release call, got %d", lm.releasedCount)
	}
	if lm.lastReleaseID != "lease-test-123" {
		t.Fatalf("expected lastReleaseID lease-test-123, got %s", lm.lastReleaseID)
	}
	if lm.lastIdleSec != stages.ReviewIdleTimeout {
		t.Fatalf("expected idle timeout %v, got %v", stages.ReviewIdleTimeout, lm.lastIdleSec)
	}
}

func TestCreateSandboxStage_SupersededPR(t *testing.T) {
	lm := &mockLeaseManager{
		acquireFn: func(ctx context.Context, prKey, consumer string, leaseTTL time.Duration, cloneParams *contracts.CreateSandboxParams) (*contracts.AcquireResult, error) {
			return nil, &contracts.SandboxInvalidatedError{PrKey: prKey}
		},
	}
	stage := stages.NewCreateSandboxStage(lm)

	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		RepoNamespace: "acme/service",
		PullNumber:    10,
		RawDiff:       "diff --git a/a.go b/a.go\n",
		FilteredPatches: []*diff.FilePatch{
			{NewPath: "a.go"},
		},
	}

	ctx := context.Background()
	err := stage.Execute(ctx, pCtx)
	if err != nil {
		t.Fatalf("superseded sandbox acquisition should not fail pipeline, got: %v", err)
	}

	if pCtx.SandboxHandle != nil {
		t.Fatalf("expected SandboxHandle to remain nil on supersede")
	}
}

func TestCreateSandboxStage_SkipWhenNoDiff(t *testing.T) {
	lm := &mockLeaseManager{}
	stage := stages.NewCreateSandboxStage(lm)

	pCtx := &pipeline.PipelineContext{
		ReviewID:     uuid.New(),
		WorkspaceID:  uuid.New(),
		RepositoryID: uuid.New(),
		PullNumber:   10,
	}

	ctx := context.Background()
	err := stage.Execute(ctx, pCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if lm.acquiredCount != 0 {
		t.Fatalf("expected 0 acquire calls when no diff, got %d", lm.acquiredCount)
	}
}

func TestCreateSandboxStage_GracefulFallbackOnError(t *testing.T) {
	lm := &mockLeaseManager{
		acquireFn: func(ctx context.Context, prKey, consumer string, leaseTTL time.Duration, cloneParams *contracts.CreateSandboxParams) (*contracts.AcquireResult, error) {
			return nil, errors.New("e2b cluster out of capacity")
		},
	}
	stage := stages.NewCreateSandboxStage(lm)

	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		RepoNamespace: "acme/service",
		PullNumber:    10,
		RawDiff:       "diff --git a/a.go b/a.go\n",
		FilteredPatches: []*diff.FilePatch{
			{NewPath: "a.go"},
		},
	}

	ctx := context.Background()
	err := stage.Execute(ctx, pCtx)
	if err != nil {
		t.Fatalf("exhausted sandbox retry error should not crash review, got: %v", err)
	}
	if pCtx.SandboxHandle != nil {
		t.Fatalf("expected SandboxHandle to remain nil on error")
	}
}
