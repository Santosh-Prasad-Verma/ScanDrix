package benchmark_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/internal/queue/consumer"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)

func TestConcurrentWebhookIngestionStress(t *testing.T) {
	outbox := relay.NewOutboxStore()
	secret := "production_secret_webhook_key_stress"
	resolver := ingestion.NewStaticSecretResolver(map[string]string{
		"github": secret,
	})
	handler := ingestion.NewIngestionHandler(resolver, outbox)

	const totalRequests = 500
	const concurrency = 20

	var successCount int64
	var wg sync.WaitGroup

	reqChan := make(chan int, totalRequests)
	for i := 0; i < totalRequests; i++ {
		reqChan <- i
	}
	close(reqChan)

	start := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for num := range reqChan {
				payload := []byte(fmt.Sprintf(`{
					"action": "opened",
					"number": %d,
					"pull_request": {
						"title": "stress test PR %d",
						"head": {"sha": "head_%d"},
						"base": {"sha": "base_main"},
						"user": {"login": "dev"}
					},
					"repository": {
						"full_name": "acme/stress-repo"
					}
				}`, num, num, num))

				mac := hmac.New(sha256.New, []byte(secret))
				mac.Write(payload)
				sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

				req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
				req.Header.Set("X-GitHub-Event", "pull_request")
				req.Header.Set("X-Hub-Signature-256", sig)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				if rec.Code == http.StatusAccepted {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	if successCount != totalRequests {
		t.Fatalf("expected %d successful ingestions, got %d", totalRequests, successCount)
	}

	lag, err := outbox.GetLag(context.Background())
	if err != nil || lag.PendingCount != totalRequests {
		t.Fatalf("expected %d outbox messages, got %+v", totalRequests, lag)
	}

	rps := float64(totalRequests) / duration.Seconds()
	t.Logf("Webhook Ingestion Throughput: %0.2f req/sec (%d requests in %v)", rps, totalRequests, duration)
}

func TestConcurrentWorkerPoolExecutionStress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inbox := relay.NewInboxDeduplicator()

	var processedCount int64
	mockExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		atomic.AddInt64(&processedCount, 1)
		return []models.CodeFinding{
			{Title: "Stress Finding", Severity: models.SeverityLow},
		}, nil
	}

	cfg := consumer.ConsumerConfig{
		MaxRetries:  3,
		Concurrency: 8,
	}
	reviewConsumer := consumer.NewReviewConsumer(cfg, inbox, mockExecutor)
	pool := consumer.NewWorkerPool(8, reviewConsumer)
	pool.Start(ctx)

	const totalTasks = 200

	// Drain results concurrently to avoid deadlocking with the submit goroutine.
	// Workers push into p.results (buffer 100); if nobody drains it, workers block
	// and the submit loop can never push more into jobChan (buffer 100).
	var resultsReceived int64
	var successCount int64
	resultsDone := make(chan struct{})
	go func() {
		defer close(resultsDone)
		for res := range pool.ResultsChannel() {
			n := atomic.AddInt64(&resultsReceived, 1)
			if res.Status == consumer.TaskStatusSuccess {
				atomic.AddInt64(&successCount, 1)
			}
			if n >= totalTasks {
				return
			}
			_ = res // consume all statuses to prevent blocking workers
		}
	}()

	// Submit all tasks with backpressure retry.
	for i := 0; i < totalTasks; i++ {
		task := consumer.ReviewTaskPayload{
			TaskID:            uuid.New(),
			WorkspaceID:       uuid.New(),
			Provider:          models.ProviderGitHub,
			RepoNamespace:     "acme/pool-stress",
			PullRequestNumber: i + 1,
		}
		for !pool.Submit(task) {
			select {
			case <-ctx.Done():
				t.Fatalf("context expired while submitting task %d", i)
			default:
				time.Sleep(100 * time.Microsecond)
			}
		}
	}

	// Wait for all results to be collected or context to expire.
	select {
	case <-resultsDone:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for results: received %d/%d", atomic.LoadInt64(&resultsReceived), totalTasks)
	}

	pool.Stop()

	sc := atomic.LoadInt64(&successCount)
	pc := atomic.LoadInt64(&processedCount)
	t.Logf("Worker Pool Stress: %d submitted, %d results received, %d successes, %d processed by executor", totalTasks, atomic.LoadInt64(&resultsReceived), sc, pc)

	if sc == 0 {
		t.Fatalf("expected at least some successful tasks, got 0 successes out of %d results", atomic.LoadInt64(&resultsReceived))
	}
}

func TestConcurrentRateLimiterStress(t *testing.T) {
	tb := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:         500,
		RefillRatePerSec: 500,
	})
	ctx := context.Background()

	const routines = 25
	const iterations = 50

	var wg sync.WaitGroup
	var allowedCount int64
	var deniedCount int64

	for r := 0; r < routines; r++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				res, _ := tb.Allow(ctx, "stress-tenant", 1)
				if res != nil && res.Allowed {
					atomic.AddInt64(&allowedCount, 1)
				} else {
					atomic.AddInt64(&deniedCount, 1)
				}
			}
		}(r)
	}

	wg.Wait()

	total := allowedCount + deniedCount
	if total != routines*iterations {
		t.Fatalf("expected %d total checks, got %d", routines*iterations, total)
	}
	// Initial burst is 500, so exactly 500 should be allowed, and remaining denied
	if allowedCount > 505 {
		t.Fatalf("allowed count exceeded capacity: allowed=%d, denied=%d", allowedCount, deniedCount)
	}
}
