package chaos_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/chaos"
)

// WebhookPlatform identifies the source code management system.
type WebhookPlatform string

const (
	PlatformGitHub    WebhookPlatform = "GITHUB"
	PlatformGitLab    WebhookPlatform = "GITLAB"
	PlatformBitbucket WebhookPlatform = "BITBUCKET"
	PlatformAzureDevOps WebhookPlatform = "AZURE_DEVOPS"
	PlatformForgejo   WebhookPlatform = "FORGEJO"
)

// WebhookPayload simulates an incoming SCM pull request event.
type WebhookPayload struct {
	Platform    WebhookPlatform `json:"platform"`
	DeliveryID  string          `json:"delivery_id"`
	RepoID      string          `json:"repo_id"`
	PullNumber  int             `json:"pull_number"`
	HeadSHA     string          `json:"head_sha"`
	BaseSHA     string          `json:"base_sha"`
	Action      string          `json:"action"` // "opened", "synchronize", "reopened"
	Author      string          `json:"author"`
	RawBody     []byte          `json:"raw_body"`
	Signature   string          `json:"signature"`
	Timestamp   time.Time       `json:"timestamp"`
}

// ComputeHMACSHA256 generates hex-encoded HMAC-SHA256 signature for payload validation.
func ComputeHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySCMWebhookSignature performs constant-time validation across all supported SCM platforms.
func VerifySCMWebhookSignature(platform WebhookPlatform, secret string, headerSig string, body []byte) bool {
	if secret == "" {
		return false
	}

	switch platform {
	case PlatformGitHub, PlatformBitbucket, PlatformForgejo:
		// GitHub & Forgejo send "sha256=<hex>"
		targetSig := headerSig
		if len(targetSig) > 7 && targetSig[:7] == "sha256=" {
			targetSig = targetSig[7:]
		}
		expectedSig := ComputeHMACSHA256(secret, body)
		return subtle.ConstantTimeCompare([]byte(targetSig), []byte(expectedSig)) == 1

	case PlatformGitLab:
		// GitLab sends secret directly in X-GitLab-Token header
		return subtle.ConstantTimeCompare([]byte(headerSig), []byte(secret)) == 1

	case PlatformAzureDevOps:
		// Azure DevOps sends HMAC signature or shared bearer secret
		expectedSig := ComputeHMACSHA256(secret, body)
		return subtle.ConstantTimeCompare([]byte(headerSig), []byte(expectedSig)) == 1

	default:
		return false
	}
}

// IdempotencyClaimState represents the processing lifecycle of a claimed webhook.
type IdempotencyClaimState string

const (
	ClaimPending   IdempotencyClaimState = "CLAIMED"
	ClaimExecuting IdempotencyClaimState = "PROCESSING"
	ClaimCompleted IdempotencyClaimState = "COMPLETED"
	ClaimFailed    IdempotencyClaimState = "FAILED"
)

// IdempotencyRecord stores deduplication metadata and lock state.
type IdempotencyRecord struct {
	Key         string
	State       IdempotencyClaimState
	OwnerWorker string
	ClaimedAt   time.Time
	ExpiresAt   time.Time
	Attempts    int
}

// ThreadSafeInboxStore provides memory-efficient, race-free webhook idempotency.
type ThreadSafeInboxStore struct {
	mu      sync.Mutex
	records map[string]*IdempotencyRecord
	ttl     time.Duration
}

func NewThreadSafeInboxStore(ttl time.Duration) *ThreadSafeInboxStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &ThreadSafeInboxStore{
		records: make(map[string]*IdempotencyRecord),
		ttl:     ttl,
	}
}

// Claim attempts to acquire a lock on the webhook event. Returns true if acquired.
func (s *ThreadSafeInboxStore) Claim(key, workerID string) (bool, IdempotencyClaimState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	rec, exists := s.records[key]

	if exists {
		// If completed, reject duplicate delivery
		if rec.State == ClaimCompleted {
			return false, ClaimCompleted
		}

		// If currently claimed or processing, check lease expiry
		if rec.State == ClaimPending || rec.State == ClaimExecuting {
			if now.Before(rec.ExpiresAt) {
				return false, rec.State
			}
			// Lease expired -> worker crashed or timed out: allow re-claim
			rec.State = ClaimPending
			rec.OwnerWorker = workerID
			rec.ClaimedAt = now
			rec.ExpiresAt = now.Add(s.ttl)
			rec.Attempts++
			return true, ClaimPending
		}

		// Failed state: allow retry up to 5 attempts
		if rec.State == ClaimFailed {
			if rec.Attempts >= 5 {
				return false, ClaimFailed
			}
			rec.State = ClaimPending
			rec.OwnerWorker = workerID
			rec.ClaimedAt = now
			rec.ExpiresAt = now.Add(s.ttl)
			rec.Attempts++
			return true, ClaimPending
		}
	}

	// First time seeing this webhook event
	s.records[key] = &IdempotencyRecord{
		Key:         key,
		State:       ClaimPending,
		OwnerWorker: workerID,
		ClaimedAt:   now,
		ExpiresAt:   now.Add(s.ttl),
		Attempts:    1,
	}
	return true, ClaimPending
}

// Complete transitions the claim to COMPLETED state.
func (s *ThreadSafeInboxStore) Complete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.records[key]; ok {
		rec.State = ClaimCompleted
	}
}

// Release marks the claim as FAILED for subsequent retry.
func (s *ThreadSafeInboxStore) Release(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.records[key]; ok {
		rec.State = ClaimFailed
	}
}

// OutboxMessage simulates an asynchronous queue task in RabbitMQ or PostgreSQL outbox table.
type OutboxMessage struct {
	ID          uuid.UUID
	EventKey    string
	ReviewID    uuid.UUID
	Payload     []byte
	Status      string // "PENDING", "CLAIMED", "COMPLETED", "DEAD_LETTER"
	Attempts    int
	MaxAttempts int
	WorkerID    string
	ClaimedAt   time.Time
	LastError   string
}

// ThreadSafeOutboxRelay simulates the background outbox publisher and consumer loop.
type ThreadSafeOutboxRelay struct {
	mu       sync.Mutex
	messages map[uuid.UUID]*OutboxMessage
	dlq      []*OutboxMessage
}

func NewThreadSafeOutboxRelay() *ThreadSafeOutboxRelay {
	return &ThreadSafeOutboxRelay{
		messages: make(map[uuid.UUID]*OutboxMessage),
		dlq:      make([]*OutboxMessage, 0),
	}
}

func (r *ThreadSafeOutboxRelay) Enqueue(eventKey string, reviewID uuid.UUID, payload []byte) uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := uuid.New()
	r.messages[id] = &OutboxMessage{
		ID:          id,
		EventKey:    eventKey,
		ReviewID:    reviewID,
		Payload:     payload,
		Status:      "PENDING",
		Attempts:    0,
		MaxAttempts: 5,
	}
	return id
}

func (r *ThreadSafeOutboxRelay) ClaimNext(workerID string) *OutboxMessage {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, msg := range r.messages {
		if msg.Status == "PENDING" {
			msg.Status = "CLAIMED"
			msg.WorkerID = workerID
			msg.ClaimedAt = time.Now().UTC()
			msg.Attempts++
			return msg
		}
	}
	return nil
}

func (r *ThreadSafeOutboxRelay) Acknowledge(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg, ok := r.messages[id]; ok {
		msg.Status = "COMPLETED"
	}
}

func (r *ThreadSafeOutboxRelay) Fail(id uuid.UUID, errStr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg, ok := r.messages[id]; ok {
		msg.LastError = errStr
		if msg.Attempts >= msg.MaxAttempts {
			msg.Status = "DEAD_LETTER"
			r.dlq = append(r.dlq, msg)
		} else {
			msg.Status = "PENDING" // return to queue for retry
		}
	}
}

// TestSCMWebhook_HMACSignatureVerification tests constant-time verification across all platforms.
func TestSCMWebhook_HMACSignatureVerification(t *testing.T) {
	secret := "secret-key-998877"
	payload := []byte(`{"action":"synchronize","pull_request":{"number":101,"head":{"sha":"abc123"}}}`)

	// 1. GitHub format
	ghSig := "sha256=" + ComputeHMACSHA256(secret, payload)
	if !VerifySCMWebhookSignature(PlatformGitHub, secret, ghSig, payload) {
		t.Fatalf("GitHub signature verification failed")
	}

	// 2. Tampered payload fails
	tampered := []byte(`{"action":"synchronize","pull_request":{"number":101,"head":{"sha":"tampered"}}}`)
	if VerifySCMWebhookSignature(PlatformGitHub, secret, ghSig, tampered) {
		t.Fatalf("expected tampered payload to fail signature check")
	}

	// 3. Wrong secret fails
	if VerifySCMWebhookSignature(PlatformGitHub, "wrong-secret", ghSig, payload) {
		t.Fatalf("expected wrong secret to fail signature check")
	}

	// 4. GitLab token format
	if !VerifySCMWebhookSignature(PlatformGitLab, secret, secret, payload) {
		t.Fatalf("GitLab token verification failed")
	}
	if VerifySCMWebhookSignature(PlatformGitLab, secret, "invalid-token", payload) {
		t.Fatalf("expected invalid GitLab token to fail")
	}

	// 5. Bitbucket format
	bbSig := "sha256=" + ComputeHMACSHA256(secret, payload)
	if !VerifySCMWebhookSignature(PlatformBitbucket, secret, bbSig, payload) {
		t.Fatalf("Bitbucket signature verification failed")
	}

	// 6. Forgejo format
	fjSig := "sha256=" + ComputeHMACSHA256(secret, payload)
	if !VerifySCMWebhookSignature(PlatformForgejo, secret, fjSig, payload) {
		t.Fatalf("Forgejo signature verification failed")
	}
}

// TestSCMWebhook_ConcurrentReplayDeduplication simulates 100 simultaneous webhook deliveries
// of the exact same event. Exactly 1 must acquire the claim, and 99 must be suppressed.
func TestSCMWebhook_ConcurrentReplayDeduplication(t *testing.T) {
	inbox := NewThreadSafeInboxStore(5 * time.Minute)
	concurrency := 100
	eventKey := "github:repo-99:pr-42:sha-e2e4f6"

	var acquiredCount int32
	var rejectedCount int32
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerIdx int) {
			defer wg.Done()
			workerID := fmt.Sprintf("worker-%03d", workerIdx)

			acquired, _ := inbox.Claim(eventKey, workerID)
			if acquired {
				atomic.AddInt32(&acquiredCount, 1)
				// Simulate fast-path processing
				time.Sleep(10 * time.Millisecond)
				inbox.Complete(eventKey)
			} else {
				atomic.AddInt32(&rejectedCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if acquiredCount != 1 {
		t.Fatalf("expected exactly 1 worker to claim event, got %d", acquiredCount)
	}
	if rejectedCount != int32(concurrency-1) {
		t.Fatalf("expected %d duplicate webhooks rejected, got %d", concurrency-1, rejectedCount)
	}
}

// TestSCMWebhook_OutboxRetryAndDeadLetterQueue tests the outbox retry state machine
// ensuring max 5 retries before transitioning to DEAD_LETTER.
func TestSCMWebhook_OutboxRetryAndDeadLetterQueue(t *testing.T) {
	relay := NewThreadSafeOutboxRelay()
	reviewID := uuid.New()
	msgID := relay.Enqueue("github:pr-101", reviewID, []byte("test-payload"))

	// Fail 4 times -> should remain retryable
	for attempt := 1; attempt <= 4; attempt++ {
		msg := relay.ClaimNext("worker-1")
		if msg == nil || msg.ID != msgID {
			t.Fatalf("attempt %d: expected to claim message %s", attempt, msgID)
		}
		if msg.Attempts != attempt {
			t.Fatalf("expected attempts count %d, got %d", attempt, msg.Attempts)
		}
		relay.Fail(msgID, fmt.Sprintf("network timeout error attempt %d", attempt))
	}

	// 5th attempt: should transition to DEAD_LETTER
	msg := relay.ClaimNext("worker-1")
	if msg == nil || msg.ID != msgID {
		t.Fatalf("expected 5th claim for message %s", msgID)
	}
	relay.Fail(msgID, "permanent error: upstream 500")

	// Verify no more pending messages
	next := relay.ClaimNext("worker-1")
	if next != nil {
		t.Fatalf("expected no pending messages after DLQ transition, got %v", next)
	}

	// Verify message in DLQ
	relay.mu.Lock()
	dlqLen := len(relay.dlq)
	relay.mu.Unlock()
	if dlqLen != 1 {
		t.Fatalf("expected 1 message in DLQ, got %d", dlqLen)
	}
}

// TestSCMWebhook_OutOfOrderCommitSuppression simulates race where a newer commit (c3)
// is received and processed, and an older commit (c2) arrives late.
func TestSCMWebhook_OutOfOrderCommitSuppression(t *testing.T) {
	type CommitRecord struct {
		SHA       string
		Timestamp time.Time
	}

	// Pull request commit history
	t0 := time.Now().Add(-10 * time.Minute)
	t1 := t0.Add(2 * time.Minute)
	t2 := t1.Add(2 * time.Minute)

	prCommits := map[string]CommitRecord{
		"c1-initial": {SHA: "c1-initial", Timestamp: t0},
		"c2-rebase":  {SHA: "c2-rebase", Timestamp: t1},
		"c3-latest":  {SHA: "c3-latest", Timestamp: t2},
	}

	var latestReviewedCommit atomic.Pointer[CommitRecord]
	latestReviewedCommit.Store(&CommitRecord{SHA: "c1-initial", Timestamp: t0})

	// Process commit helper: drops older commits
	processCommitEvent := func(sha string) (bool, string) {
		incoming, ok := prCommits[sha]
		if !ok {
			return false, "unknown commit"
		}

		current := latestReviewedCommit.Load()
		if incoming.Timestamp.Before(current.Timestamp) {
			return false, "DROPPED_STALE_COMMIT: incoming commit is older than active review"
		}

		latestReviewedCommit.Store(&incoming)
		return true, "ACCEPTED"
	}

	// 1. Commit 3 arrives first (fast network path)
	accepted, status := processCommitEvent("c3-latest")
	if !accepted || status != "ACCEPTED" {
		t.Fatalf("expected c3-latest accepted, got %v: %s", accepted, status)
	}

	// 2. Commit 2 arrives late (out of order delivery)
	accepted, status = processCommitEvent("c2-rebase")
	if accepted {
		t.Fatalf("expected out-of-order commit c2-rebase to be dropped, got accepted")
	}
	if status != "DROPPED_STALE_COMMIT: incoming commit is older than active review" {
		t.Fatalf("unexpected drop status: %s", status)
	}
}

// TestSCMWebhook_RateLimitBackoffJitterStress tests exponential backoff with jitter
// under sustained upstream 429 rate limit pressure.
func TestSCMWebhook_RateLimitBackoffJitterStress(t *testing.T) {
	ctx := context.Background()
	caller := chaos.NewResilientSCMCaller()

	workers := 10
	var totalRetries int64
	var wg sync.WaitGroup

	scmErr := &chaos.SCMError{
		Platform:   chaos.PlatformGitHub,
		StatusCode: 429,
		Message:    "rate limit exceeded",
		RetryAfter: 1 * time.Second,
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				// Simulate 429 response with Retry-After header
				backoffDuration := caller.CalculateWait(i, chaos.PlatformGitHub, scmErr)
				if backoffDuration < 100*time.Millisecond || backoffDuration > 5*time.Second {
					t.Errorf("worker %d unexpected backoff duration %v", workerID, backoffDuration)
				}
				atomic.AddInt64(&totalRetries, 1)
			}
		}(w)
	}

	wg.Wait()

	if totalRetries != int64(workers*5) {
		t.Fatalf("expected %d total retry evaluations, got %d", workers*5, totalRetries)
	}

	// Verify resilient caller recovers after simulated rate limit window
	caller = chaos.NewResilientSCMCaller()
	attempts := 0
	err := caller.ExecuteWithRetry(ctx, chaos.PlatformGitHub, "get_pull_request", func(callCtx context.Context) error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("HTTP 429 rate limit exceeded")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected caller to recover after retry: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected exactly 3 attempts before success, got %d", attempts)
	}
}
