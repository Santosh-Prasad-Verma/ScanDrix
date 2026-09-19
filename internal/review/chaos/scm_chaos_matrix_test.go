package chaos_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/review/chaos"
)

func TestResilientSCMCaller_RecoversFromRateLimitBurst(t *testing.T) {
	ctx := context.Background()
	injector := chaos.NewSCMChaosInjector()
	caller := chaos.NewResilientSCMCaller()

	// Inject 3 consecutive 429 Too Many Requests with 20ms Retry-After
	injector.InjectFault("create_comment", chaos.ChaosFaultRule{
		FaultType:      chaos.FaultRateLimit429,
		TriggerCount:   3,
		RetryAfterWait: 20 * time.Millisecond,
	})

	var executionAttempts int64

	err := caller.ExecuteWithRetry(ctx, chaos.PlatformGitHub, "create_comment", func(callCtx context.Context) error {
		atomic.AddInt64(&executionAttempts, 1)
		return injector.Intercept(callCtx, chaos.PlatformGitHub, "create_comment")
	})

	if err != nil {
		t.Fatalf("expected successful recovery from 429 burst, got error: %v", err)
	}

	// 3 failures + 1 success = 4 total attempts
	if atomic.LoadInt64(&executionAttempts) != 4 {
		t.Fatalf("expected 4 attempts, got %d", atomic.LoadInt64(&executionAttempts))
	}

	totalInvocations, failures, successes := injector.Stats()
	if totalInvocations != 4 || failures != 3 || successes != 1 {
		t.Fatalf("unexpected chaos stats: invocations=%d, failures=%d, successes=%d", totalInvocations, failures, successes)
	}
}

func TestResilientSCMCaller_RecoversFromConnectionReset(t *testing.T) {
	ctx := context.Background()
	injector := chaos.NewSCMChaosInjector()
	caller := chaos.NewResilientSCMCaller()

	// Inject 2 socket connection resets
	injector.InjectFault("get_diff", chaos.ChaosFaultRule{
		FaultType:    chaos.FaultConnectionReset,
		TriggerCount: 2,
	})

	var executionAttempts int64

	err := caller.ExecuteWithRetry(ctx, chaos.PlatformGitLab, "get_diff", func(callCtx context.Context) error {
		atomic.AddInt64(&executionAttempts, 1)
		return injector.Intercept(callCtx, chaos.PlatformGitLab, "get_diff")
	})

	if err != nil {
		t.Fatalf("expected successful recovery from connection reset, got error: %v", err)
	}

	if atomic.LoadInt64(&executionAttempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", atomic.LoadInt64(&executionAttempts))
	}
}

func TestResilientSCMCaller_ExhaustedRetriesFailsCleanly(t *testing.T) {
	ctx := context.Background()
	injector := chaos.NewSCMChaosInjector()
	caller := chaos.NewResilientSCMCaller()

	// Inject 20 consecutive 429s (exceeding MaxRetries=5)
	injector.InjectFault("submit_review", chaos.ChaosFaultRule{
		FaultType:      chaos.FaultRateLimit429,
		TriggerCount:   20,
		RetryAfterWait: 5 * time.Millisecond,
	})

	err := caller.ExecuteWithRetry(ctx, chaos.PlatformGitHub, "submit_review", func(callCtx context.Context) error {
		return injector.Intercept(callCtx, chaos.PlatformGitHub, "submit_review")
	})

	if err == nil {
		t.Fatal("expected error on exhausted retries, got nil")
	}

	if !strings.Contains(err.Error(), "failed on github after 5 retries") {
		t.Fatalf("expected exhausted retries message, got: %v", err)
	}
}

func TestCommentPayloadChunker_SplitsLargeCommentPreservingFences(t *testing.T) {
	chunker := chaos.NewCommentPayloadChunker()

	var sb strings.Builder
	sb.WriteString("# Review Summary\n\n```go\n")
	// Generate 150,000 characters of code
	for i := 0; i < 5000; i++ {
		sb.WriteString("func ProcessTransaction(ctx context.Context, tx *Transaction) error { return nil }\n")
	}
	sb.WriteString("```\n\nFinal remarks.")

	raw := sb.String()
	chunks := chunker.SplitComment(raw, chaos.PlatformGitHub)

	if len(chunks) <= 1 {
		t.Fatalf("expected content to be split into multiple chunks, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		if len(chunk) > chaos.MaxGitHubCommentLength {
			t.Fatalf("chunk %d exceeded max GitHub limit (%d chars)", i, len(chunk))
		}

		// Ensure Part headers are formatted
		expectedHeader := "(Part "
		if !strings.Contains(chunk, expectedHeader) {
			t.Fatalf("chunk %d missing part header: %s", i, chunk[:50])
		}

		// Check code fence balance
		fenceCount := strings.Count(chunk, "```")
		if fenceCount%2 != 0 {
			t.Fatalf("chunk %d has unclosed code fence (count=%d)", i, fenceCount)
		}
	}
}

func TestValidateDiffHunkPosition(t *testing.T) {
	modifiedRanges := [][2]int{
		{10, 25},
		{50, 75},
		{100, 110},
	}

	// Inside modified range
	if !chaos.ValidateDiffHunkPosition(12, 15, modifiedRanges) {
		t.Fatal("expected lines 12-15 to be valid inside range [10, 25]")
	}

	// Overlapping modified range boundary
	if !chaos.ValidateDiffHunkPosition(20, 30, modifiedRanges) {
		t.Fatal("expected lines 20-30 to be valid overlapping range [10, 25]")
	}

	// Outside modified range
	if chaos.ValidateDiffHunkPosition(30, 45, modifiedRanges) {
		t.Fatal("expected lines 30-45 to be invalid (between 25 and 50)")
	}

	// Invalid line numbers
	if chaos.ValidateDiffHunkPosition(0, 10, modifiedRanges) {
		t.Fatal("expected start line 0 to be invalid")
	}
	if chaos.ValidateDiffHunkPosition(50, 40, modifiedRanges) {
		t.Fatal("expected startLine > endLine to be invalid")
	}
}

func TestResilientSCMCaller_ConcurrentStress(t *testing.T) {
	ctx := context.Background()
	injector := chaos.NewSCMChaosInjector()
	caller := chaos.NewResilientSCMCaller()

	// Inject transient socket timeouts with 10% probability
	injector.InjectFault("fetch_blob", chaos.ChaosFaultRule{
		FaultType:    chaos.FaultConnectionReset,
		TriggerCount: 5,
	})

	var wg sync.WaitGroup
	workers := 25

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := caller.ExecuteWithRetry(ctx, chaos.PlatformBitbucket, "fetch_blob", func(callCtx context.Context) error {
				return injector.Intercept(callCtx, chaos.PlatformBitbucket, "fetch_blob")
			})
			if err != nil {
				t.Errorf("worker %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()
}
