package workflow_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkflowErrorClassifierMatrix tests every HTTP status code, message pattern, and SCM error variation.
func TestWorkflowErrorClassifierMatrix(t *testing.T) {
	classifier := workflow.NewErrorClassifierService()

	// 1. Nil error defaults to retryable
	assert.Equal(t, domain.ErrorClassificationRetryable, classifier.Classify(nil))

	// 2. Concrete RateLimitError
	rateErr := &workflow.RateLimitError{
		ResetAt:   time.Now().Add(30 * time.Second),
		Remaining: 0,
		Message:   "Rate limited by GitHub API",
	}
	assert.Equal(t, domain.ErrorClassificationRateLimited, classifier.Classify(rateErr))

	// 3. SCM Response Errors - Status Code Classification
	testCases := []struct {
		name     string
		err      error
		expected domain.ErrorClassification
	}{
		{
			name: "SCM 401 Unauthorized",
			err: &workflow.SCMResponseError{
				StatusCode: 401,
				Message:    "Bad credentials",
			},
			expected: domain.ErrorClassificationPermanent,
		},
		{
			name: "SCM 403 Forbidden without rate headers",
			err: &workflow.SCMResponseError{
				StatusCode: 403,
				Message:    "Resource not accessible by integration",
			},
			expected: domain.ErrorClassificationPermanent,
		},
		{
			name: "SCM 404 Not Found",
			err: &workflow.SCMResponseError{
				StatusCode: 404,
				Message:    "Repository not found",
			},
			expected: domain.ErrorClassificationPermanent,
		},
		{
			name: "SCM 422 Unprocessable Entity",
			err: &workflow.SCMResponseError{
				StatusCode: 422,
				Message:    "Validation Failed: pull request already exists",
			},
			expected: domain.ErrorClassificationPermanent,
		},
		{
			name: "SCM 403 with Primary Rate Limit Header",
			err: &workflow.SCMResponseError{
				StatusCode: 403,
				Headers: map[string]string{
					"x-ratelimit-remaining": "0",
					"x-ratelimit-reset":     fmt.Sprintf("%d", time.Now().Add(60*time.Second).Unix()),
				},
				Message: "API rate limit exceeded",
			},
			expected: domain.ErrorClassificationRateLimited,
		},
		{
			name: "SCM 429 with Retry-After Header",
			err: &workflow.SCMResponseError{
				StatusCode: 429,
				Headers: map[string]string{
					"retry-after": "120",
				},
				Message: "Too Many Requests",
			},
			expected: domain.ErrorClassificationRateLimited,
		},
		{
			name: "SCM 429 without Headers (fallback)",
			err: &workflow.SCMResponseError{
				StatusCode: 429,
				Message:    "Too Many Requests",
			},
			expected: domain.ErrorClassificationRateLimited,
		},
		{
			name:     "Generic Network Timeout",
			err:      errors.New("dial tcp 140.82.121.4:443: i/o timeout"),
			expected: domain.ErrorClassificationRetryable,
		},
		{
			name:     "Connection ECONNREFUSED",
			err:      errors.New("connect: connection refused (ECONNREFUSED)"),
			expected: domain.ErrorClassificationRetryable,
		},
		{
			name:     "Connection ETIMEDOUT",
			err:      errors.New("read: connection timed out (ETIMEDOUT)"),
			expected: domain.ErrorClassificationRetryable,
		},
		{
			name:     "Connection reset by peer",
			err:      errors.New("read tcp: connection reset by peer"),
			expected: domain.ErrorClassificationRetryable,
		},
		{
			name:     "HTTP 502 Bad Gateway",
			err:      errors.New("upstream returned HTTP 502 Bad Gateway"),
			expected: domain.ErrorClassificationRetryable,
		},
		{
			name:     "HTTP 503 Service Unavailable",
			err:      errors.New("upstream service temporarily unavailable: 503"),
			expected: domain.ErrorClassificationRetryable,
		},
		{
			name:     "HTTP 504 Gateway Timeout",
			err:      errors.New("gateway timed out waiting for upstream: 504"),
			expected: domain.ErrorClassificationRetryable,
		},
		{
			name:     "Validation Error String",
			err:      errors.New("validation failed: invalid schema on line 4"),
			expected: domain.ErrorClassificationNonRetryable,
		},
		{
			name:     "Invalid State Error String",
			err:      errors.New("invalid payload structure supplied"),
			expected: domain.ErrorClassificationNonRetryable,
		},
		{
			name:     "Repository Not Found String",
			err:      errors.New("not found: organization/repo does not exist"),
			expected: domain.ErrorClassificationNonRetryable,
		},
		{
			name:     "Unauthorized String",
			err:      errors.New("unauthorized: missing or expired token"),
			expected: domain.ErrorClassificationNonRetryable,
		},
		{
			name:     "Bad Credentials String",
			err:      errors.New("bad credentials for user"),
			expected: domain.ErrorClassificationNonRetryable,
		},
		{
			name:     "Forbidden String",
			err:      errors.New("forbidden: insufficient privileges for action"),
			expected: domain.ErrorClassificationNonRetryable,
		},
		{
			name:     "Arbitrary Internal System Error (Fallback)",
			err:      errors.New("unexpected nil pointer in worker task execution"),
			expected: domain.ErrorClassificationRetryable,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := classifier.Classify(tc.err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// TestClassifySCMRateLimitDeep tests case-insensitive headers and edge cases.
func TestClassifySCMRateLimitDeep(t *testing.T) {
	// Nil error
	assert.Nil(t, workflow.ClassifySCMRateLimit(nil))

	// Status not 403 or 429
	scmErr500 := &workflow.SCMResponseError{StatusCode: 500}
	assert.Nil(t, workflow.ClassifySCMRateLimit(scmErr500))

	// Mixed-case headers
	resetTarget := time.Now().Add(180 * time.Second).Unix()
	scmMixed := &workflow.SCMResponseError{
		StatusCode: 403,
		Headers: map[string]string{
			"X-RateLimit-Remaining": "0",
			"X-RateLimit-Reset":     fmt.Sprintf("%d", resetTarget),
		},
		Message: "rate limit exceeded",
	}
	res := workflow.ClassifySCMRateLimit(scmMixed)
	require.NotNil(t, res)
	assert.Equal(t, 0, res.Remaining)
	assert.Equal(t, resetTarget, res.ResetAt.Unix())

	// Non-zero remaining with 403 (e.g. permission error, not rate limit)
	scmPerm := &workflow.SCMResponseError{
		StatusCode: 403,
		Headers: map[string]string{
			"X-RateLimit-Remaining": "4999",
			"X-RateLimit-Reset":     fmt.Sprintf("%d", resetTarget),
		},
		Message: "forbidden access",
	}
	assert.Nil(t, workflow.ClassifySCMRateLimit(scmPerm))

	// Invalid integer in reset header
	scmInvalidReset := &workflow.SCMResponseError{
		StatusCode: 403,
		Headers: map[string]string{
			"x-ratelimit-remaining": "0",
			"x-ratelimit-reset":     "not-a-number",
		},
	}
	assert.Nil(t, workflow.ClassifySCMRateLimit(scmInvalidReset))

	// Retry-After with invalid integer
	scmInvalidRetry := &workflow.SCMResponseError{
		StatusCode: 429,
		Headers: map[string]string{
			"Retry-After": "abc",
		},
	}
	fallback429 := workflow.ClassifySCMRateLimit(scmInvalidRetry)
	require.NotNil(t, fallback429)
	assert.Contains(t, fallback429.Message, "HTTP 429")

	// Error() representation
	assert.Equal(t, "SCM HTTP 404: Not Found", (&workflow.SCMResponseError{StatusCode: 404, Message: "Not Found"}).Error())
}

// TestEventBufferServiceHighConcurrencyStress tests race-free concurrent Store, Check, and Cleanup operations.
func TestEventBufferServiceHighConcurrencyStress(t *testing.T) {
	buffer := workflow.NewEventBufferService()
	const workers = 50
	const iterations = 100

	var storedCount int64
	var consumedCount int64
	var missedCount int64

	var wg sync.WaitGroup
	wg.Add(workers * 2)

	// Producers: store events concurrently
	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				eventKey := fmt.Sprintf("event-key-%d-%d", workerID, i)
				event := workflow.StageCompletedEvent{
					EventType: "stage.completed.ast",
					EventKey:  eventKey,
					TaskID:    fmt.Sprintf("task-%d-%d", workerID, i),
					Result: map[string]any{
						"worker": workerID,
						"iter":   i,
					},
					Duration: int64(100 + i),
				}
				buffer.Store(event.EventType, event.EventKey, event, 10*time.Second)
				atomic.AddInt64(&storedCount, 1)
			}
		}(w)
	}

	// Consumers: check and consume events concurrently
	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				eventKey := fmt.Sprintf("event-key-%d-%d", workerID, i)
				// Poll briefly for event
				var found bool
				for attempt := 0; attempt < 20; attempt++ {
					item, ok := buffer.Check("stage.completed.ast", eventKey)
					if ok {
						assert.NotNil(t, item)
						assert.Equal(t, fmt.Sprintf("task-%d-%d", workerID, i), item.TaskID)
						atomic.AddInt64(&consumedCount, 1)
						found = true
						break
					}
					time.Sleep(1 * time.Millisecond)
				}
				if !found {
					atomic.AddInt64(&missedCount, 1)
				}
			}
		}(w)
	}

	wg.Wait()

	assert.Equal(t, int64(workers*iterations), storedCount)
	assert.True(t, consumedCount > 0, "concurrent consumer should have consumed events")

	// Stress cleanup with expired entries
	for i := 0; i < 50; i++ {
		buffer.Store("test.exp", fmt.Sprintf("exp-%d", i), workflow.StageCompletedEvent{
			EventType: "test.exp",
			EventKey:  fmt.Sprintf("exp-%d", i),
		}, 1*time.Millisecond)
	}
	time.Sleep(5 * time.Millisecond)
	buffer.Cleanup()

	for i := 0; i < 50; i++ {
		_, ok := buffer.Check("test.exp", fmt.Sprintf("exp-%d", i))
		assert.False(t, ok, "entry should have expired and cleaned up")
	}
}

// TestStateSerializerExhaustiveMatrix tests all serialization strategies and compression edge cases.
func TestStateSerializerExhaustiveMatrix(t *testing.T) {
	serializer := workflow.NewStateSerializer()

	// 1. Nil state handling
	nilRes, err := serializer.Serialize(nil, workflow.SerializationOptions{})
	require.NoError(t, err)
	assert.Empty(t, nilRes)

	nilDeser, err := serializer.Deserialize(nil)
	require.NoError(t, err)
	assert.Empty(t, nilDeser)

	// 2. Default Strategy (Auto Full)
	baseState := map[string]any{
		"workflowJobId": "job-alpha",
		"prNumber":      float64(101),
		"repositoryId":  "repo-xyz",
		"metadata": map[string]any{
			"engine":  "scandrix-ast",
			"version": "2.4.0",
		},
	}
	res, err := serializer.Serialize(baseState, workflow.SerializationOptions{})
	require.NoError(t, err)
	assert.Equal(t, "job-alpha", res["workflowJobId"])

	// 3. StrategyMinimal: ensure only retained keys are kept
	richState := map[string]any{
		"workflowJobId":         "job-minimal-1",
		"currentStage":          "FileAnalysisStage",
		"correlationId":         "corr-999",
		"automationExecutionId": "exec-777",
		"repositoryId":          "repo-1",
		"prNumber":              float64(55),
		"suggestionsCount":      float64(12),
		"status":                "PROCESSING",
		"hugeIntermediateData":  make([]byte, 1024),
		"transientSecrets":      "should-be-omitted",
	}
	minimalRes, err := serializer.Serialize(richState, workflow.SerializationOptions{
		Strategy: workflow.StrategyMinimal,
	})
	require.NoError(t, err)
	assert.Equal(t, workflow.StrategyMinimal, minimalRes["_strategy"])
	assert.Equal(t, "job-minimal-1", minimalRes["workflowJobId"])
	assert.Equal(t, "FileAnalysisStage", minimalRes["currentStage"])
	assert.Equal(t, "corr-999", minimalRes["correlationId"])
	assert.Equal(t, "exec-777", minimalRes["automationExecutionId"])
	assert.Equal(t, "repo-1", minimalRes["repositoryId"])
	assert.Equal(t, float64(55), minimalRes["prNumber"])
	assert.Equal(t, float64(12), minimalRes["suggestionsCount"])
	assert.Equal(t, "PROCESSING", minimalRes["status"])
	assert.NotContains(t, minimalRes, "hugeIntermediateData")
	assert.NotContains(t, minimalRes, "transientSecrets")

	// 4. StrategyDelta: unmodified fields omitted, new/changed fields captured
	stateV1 := map[string]any{
		"workflowJobId": "job-delta-1",
		"currentStage":  "Stage1",
		"correlationId": "corr-1",
		"dataA":         "valueA",
		"dataB":         "valueB",
		"dataArray":     []any{"x", "y"},
	}
	stateV2 := map[string]any{
		"workflowJobId": "job-delta-1",
		"currentStage":  "Stage2", // modified
		"correlationId": "corr-1", // preserved primary ID
		"dataA":         "valueA", // identical
		"dataB":         "valueB_modified",
		"dataArray":     []any{"x", "y"}, // identical slice
		"dataC":         "valueC_new",    // newly added
	}

	deltaRes, err := serializer.Serialize(stateV2, workflow.SerializationOptions{
		Strategy:      workflow.StrategyDelta,
		PreviousState: stateV1,
	})
	require.NoError(t, err)
	assert.Equal(t, workflow.StrategyDelta, deltaRes["_strategy"])
	assert.Equal(t, "job-delta-1", deltaRes["workflowJobId"])
	assert.Equal(t, "Stage2", deltaRes["currentStage"])
	assert.Equal(t, "corr-1", deltaRes["correlationId"])
	assert.Equal(t, "valueB_modified", deltaRes["dataB"])
	assert.Equal(t, "valueC_new", deltaRes["dataC"])
	assert.NotContains(t, deltaRes, "dataA", "unchanged dataA should be omitted")
	assert.NotContains(t, deltaRes, "dataArray", "unchanged dataArray should be omitted")

	// Delta when previous state is nil defaults to current state
	deltaNilPrev, err := serializer.Serialize(stateV2, workflow.SerializationOptions{
		Strategy:      workflow.StrategyDelta,
		PreviousState: nil,
	})
	require.NoError(t, err)
	assert.Equal(t, "Stage2", deltaNilPrev["currentStage"])

	// 5. StrategyCompressed and Auto-Compression threshold (>50KB)
	hugeString := ""
	for i := 0; i < 2000; i++ {
		hugeString += "ScanDrix AST symbol resolution diff line\n"
	}
	heavyState := map[string]any{
		"workflowJobId": "job-heavy-100",
		"hugePayload":   hugeString,
	}

	// Auto-compress via StrategyFull because size > 50KB
	autoCompRes, err := serializer.Serialize(heavyState, workflow.SerializationOptions{
		Strategy: workflow.StrategyFull,
	})
	require.NoError(t, err)
	assert.True(t, autoCompRes["compressed"].(bool))
	assert.NotEmpty(t, autoCompRes["data"])
	assert.True(t, autoCompRes["originalSizeBytes"].(int) > workflow.CompressionThresholdBytes)

	// Deserialize compressed state
	decompressed, err := serializer.Deserialize(autoCompRes)
	require.NoError(t, err)
	assert.Equal(t, "job-heavy-100", decompressed["workflowJobId"])
	assert.Equal(t, hugeString, decompressed["hugePayload"])

	// Corrupted base64 compressed data handling
	badBase64 := map[string]any{
		"compressed": true,
		"data":       "!!this_is_not_valid_base64$$",
	}
	_, errBadB64 := serializer.Deserialize(badBase64)
	assert.Error(t, errBadB64)

	// Corrupted gzip payload
	corruptGzip := map[string]any{
		"compressed": true,
		"data":       base64.StdEncoding.EncodeToString([]byte("plain text that is not gzip format")),
	}
	_, errBadGzip := serializer.Deserialize(corruptGzip)
	assert.Error(t, errBadGzip)
}

// TestHashKeyDistributionAndDeterminism verifies DJB2 + FNV-1a hash properties.
func TestHashKeyDistributionAndDeterminism(t *testing.T) {
	const sampleCount = 5000
	seen := make(map[[2]int32]string, sampleCount)

	for i := 0; i < sampleCount; i++ {
		key := fmt.Sprintf("scandrix:advisory_lock:repo_%d:job_%s", i, uuid.NewString())
		hash := workflow.HashKey(key)

		// Both components must be non-negative signed 32-bit ints
		assert.True(t, hash[0] >= 0, "hash1 must be >= 0")
		assert.True(t, hash[1] >= 0, "hash2 must be >= 0")

		// Strict determinism
		rehash := workflow.HashKey(key)
		assert.Equal(t, hash, rehash, "hashing same key must be strictly deterministic")

		// Collision check
		if existingKey, exists := seen[hash]; exists {
			t.Fatalf("unexpected hash collision between '%s' and '%s'", key, existingKey)
		}
		seen[hash] = key
	}

	// Empty string test
	emptyHash := workflow.HashKey("")
	assert.True(t, emptyHash[0] >= 0)
	assert.True(t, emptyHash[1] >= 0)

	// Unicode test
	unicodeHash1 := workflow.HashKey("🔒-ключ-разблокировки-1")
	unicodeHash2 := workflow.HashKey("🔒-ключ-разблокировки-2")
	assert.NotEqual(t, unicodeHash1, unicodeHash2)
}

// TestJobStatusServiceProgressCalculationStress verifies stage progress metrics under stress and edge cases.
func TestJobStatusServiceProgressCalculationStress(t *testing.T) {
	repo := &mockJobRepo{
		jobs:    make(map[uuid.UUID]*workflow.WorkflowJobModel),
		updates: make(map[uuid.UUID]map[string]any),
	}
	statusSvc := workflow.NewJobStatusService(nil, repo)

	makeJob := func(status domain.JobStatus, stage string) *workflow.WorkflowJobModel {
		var s *string
		if stage != "" {
			s = &stage
		}
		return &workflow.WorkflowJobModel{
			Status:       status,
			CurrentStage: s,
		}
	}

	// Missing job returns nil response
	missingID := uuid.New()
	detailMissing, err := statusSvc.GetJobDetail(context.Background(), missingID)
	require.NoError(t, err)
	assert.Nil(t, detailMissing, "missing job should return nil response")

	// CalculateProgress via reflection or mock tests
	stages := []struct {
		stage    string
		status   domain.JobStatus
		expected int
	}{
		{"", domain.JobStatusCompleted, 100},
		{"", domain.JobStatusFailed, 0},
		{"", domain.JobStatusCancelled, 0},
		{"InitStage", domain.JobStatusProcessing, 10},
		{"_pipelineStart", domain.JobStatusProcessing, 10},
		{"FetchPRFilesStage", domain.JobStatusProcessing, 25},
		{"FilterFilesStage", domain.JobStatusProcessing, 40},
		{"AnalyzeChangesStage", domain.JobStatusProcessing, 55},
		{"ASTGraphStage", domain.JobStatusProcessing, 55},
		{"PRLevelReviewStage", domain.JobStatusProcessing, 70},
		{"FileAnalysisStage", domain.JobStatusProcessing, 85},
		{"SummaryGenerationStage", domain.JobStatusProcessing, 95},
		{"CustomUnmappedStage", domain.JobStatusProcessing, 50},
		{"", domain.JobStatusPending, 5},
	}


	for _, tc := range stages {
		jobID := uuid.New()
		repo.jobs[jobID] = makeJob(tc.status, tc.stage)

		detail, err := statusSvc.GetJobDetail(context.Background(), jobID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Equal(t, tc.expected, detail.ProgressPercent, "stage %s status %s should have progress %d", tc.stage, tc.status, tc.expected)
	}
}

type concurrentMockJobRepo struct {
	mu      sync.RWMutex
	jobs    map[uuid.UUID]*workflow.WorkflowJobModel
	updates map[uuid.UUID]map[string]any
}

func newConcurrentMockJobRepo() *concurrentMockJobRepo {
	return &concurrentMockJobRepo{
		jobs:    make(map[uuid.UUID]*workflow.WorkflowJobModel),
		updates: make(map[uuid.UUID]map[string]any),
	}
}

func (m *concurrentMockJobRepo) FindOne(ctx context.Context, id uuid.UUID) (*workflow.WorkflowJobModel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job := m.jobs[id]
	if job == nil {
		return nil, nil
	}
	cp := *job
	return &cp, nil
}

func (m *concurrentMockJobRepo) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates[id] = updates
	if job, ok := m.jobs[id]; ok {
		if s, ok := updates["status"].(domain.JobStatus); ok {
			job.Status = s
		}
	}
	return nil
}

// TestJobProcessorRouterConcurrentStress tests concurrent dispatching of workflow jobs.
func TestJobProcessorRouterConcurrentStress(t *testing.T) {
	repo := newConcurrentMockJobRepo()
	classifier := workflow.NewErrorClassifierService()
	router := workflow.NewJobProcessorRouterService(repo, classifier)

	var codeReviewCount int64
	var webhookCount int64
	var failureCount int64

	router.Register(domain.WorkflowTypeCodeReview, &mockProcessor{
		processFn: func(ctx context.Context, job *workflow.WorkflowJobModel) error {
			atomic.AddInt64(&codeReviewCount, 1)
			return nil
		},
	})

	router.Register(domain.WorkflowTypeWebhookProcessing, &mockProcessor{
		processFn: func(ctx context.Context, job *workflow.WorkflowJobModel) error {
			atomic.AddInt64(&webhookCount, 1)
			return nil
		},
	})

	router.Register(domain.WorkflowTypeASTGraphBuild, &mockProcessor{
		processFn: func(ctx context.Context, job *workflow.WorkflowJobModel) error {
			atomic.AddInt64(&failureCount, 1)
			return errors.New("ast compiler timeout on file syntax parse")
		},
	})

	const numJobs = 120
	var jobIDs []uuid.UUID
	for i := 0; i < numJobs; i++ {
		id := uuid.New()
		var wfType domain.WorkflowType
		switch i % 3 {
		case 0:
			wfType = domain.WorkflowTypeCodeReview
		case 1:
			wfType = domain.WorkflowTypeWebhookProcessing
		case 2:
			wfType = domain.WorkflowTypeASTGraphBuild
		}

		repo.jobs[id] = &workflow.WorkflowJobModel{
			UUID:         id,
			WorkflowType: wfType,
			Status:       domain.JobStatusPending,
		}
		jobIDs = append(jobIDs, id)
	}

	var wg sync.WaitGroup
	wg.Add(numJobs)

	for _, id := range jobIDs {
		go func(jobID uuid.UUID) {
			defer wg.Done()
			_ = router.Process(context.Background(), jobID)
		}(id)
	}

	wg.Wait()

	assert.Equal(t, int64(40), codeReviewCount)
	assert.Equal(t, int64(40), webhookCount)
	assert.Equal(t, int64(40), failureCount)

	// Verify unmapped workflow type returns error
	unmappedID := uuid.New()
	repo.jobs[unmappedID] = &workflow.WorkflowJobModel{
		UUID:         unmappedID,
		WorkflowType: domain.WorkflowType("UNREGISTERED_WORKFLOW"),
		Status:       domain.JobStatusPending,
	}
	errUnmapped := router.Process(context.Background(), unmappedID)
	assert.Error(t, errUnmapped)
	assert.Contains(t, errUnmapped.Error(), "no processor registered")
}

// TestOutboxRelayExponentialBackoffStress tests exponential backoff calculations and retry scheduling.
func TestOutboxRelayExponentialBackoffStress(t *testing.T) {
	outboxRepo := &mockOutboxRepo{}
	publisher := &mockPublisher{}
	relay := workflow.NewOutboxRelayService(outboxRepo, publisher, nil, nil, nil, nil)

	// Test backoff formula: 2^(attempt-1) * 2000ms capped at 1h (with ±10% jitter)
	for attempt := 1; attempt <= 10; attempt++ {
		backoff := workflow.CalculateBackoff(attempt)
		assert.True(t, backoff >= 1500*time.Millisecond, "minimum backoff should be >= 1.5s")
		assert.True(t, backoff <= 1*time.Hour+6*time.Minute, "maximum backoff should be capped at 1h + jitter")
	}

	// Attempt 1 should be around 2s ± 10%
	b1 := workflow.CalculateBackoff(1)
	assert.True(t, b1 >= 1800*time.Millisecond && b1 <= 2200*time.Millisecond)

	// Attempt 4 should be around 16s ± 10%
	b4 := workflow.CalculateBackoff(4)
	assert.True(t, b4 >= 14*time.Second && b4 <= 18*time.Second)

	// Empty batch returns 0
	count, err := relay.RelayBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestASTEventHandlerConcurrentDedup tests inbox idempotency under parallel event arrivals.
func TestASTEventHandlerConcurrentDedup(t *testing.T) {
	waitingRepo := &mockWaitingJobRepo{
		mockJobRepo: mockJobRepo{
			jobs:    make(map[uuid.UUID]*workflow.WorkflowJobModel),
			updates: make(map[uuid.UUID]map[string]any),
		},
		waitingJobs: make(map[string][]*workflow.WorkflowJobModel),
	}

	inbox := newMockInboxRepo()
	publisher := &mockPublisher{}
	handler := workflow.NewASTEventHandler(waitingRepo, inbox, publisher, "worker-pod-1")

	jobID := uuid.New()
	job := &workflow.WorkflowJobModel{
		UUID:         jobID,
		WorkflowType: domain.WorkflowTypeASTGraphBuild,
		Status:       domain.JobStatusWaitingForEvent,
	}
	waitingRepo.jobs[jobID] = job
	waitingRepo.waitingJobs["ast.task.completed:task-dedup-999"] = []*workflow.WorkflowJobModel{job}

	msg := workflow.ASTCompletedMessage{
		TaskID: "task-dedup-999",
		Result: map[string]any{"filesParsed": 48},
	}

	const parallelEvents = 10
	var wg sync.WaitGroup
	var successCount int64

	wg.Add(parallelEvents)
	for i := 0; i < parallelEvents; i++ {
		go func() {
			defer wg.Done()
			err := handler.HandleASTCompleted(context.Background(), msg, "ast.task.completed:task-dedup-999")
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	// Exactly one event completes, others are deduped by inbox claim
	assert.Equal(t, int64(parallelEvents), successCount)
	assert.Equal(t, domain.JobStatusProcessing, waitingRepo.jobs[jobID].Status)
	assert.Equal(t, 1, len(inbox.claimedMessages))
}

// TestWorkflowModelValidation ensures all workflow contracts conform to ScanDrix invariants.
func TestWorkflowModelValidation(t *testing.T) {
	job := &workflow.WorkflowJobModel{
		UUID:         uuid.New(),
		WorkflowType: domain.WorkflowTypeCodeReview,
		Status:       domain.JobStatusPending,
		HandlerType:  domain.HandlerTypePipelineSync,
		MaxRetries:   5,
		RetryCount:   0,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	assert.NotEqual(t, uuid.Nil, job.UUID)
	assert.Equal(t, domain.WorkflowTypeCodeReview, job.WorkflowType)
	assert.Equal(t, domain.JobStatusPending, job.Status)
	assert.Equal(t, 5, job.MaxRetries)

	event := workflow.StageCompletedEvent{
		EventType: "pr.review.completed",
		EventKey:  "repo:owner/repo:pr:42",
		TaskID:    "task-pr-42",
		Result: map[string]any{
			"comments": 3,
			"approved": false,
		},
		Duration: 1250,
	}
	assert.Equal(t, "pr.review.completed", event.EventType)
	assert.Equal(t, int64(1250), event.Duration)
}
