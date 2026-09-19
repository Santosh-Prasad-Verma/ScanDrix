package workflow

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

const (
	DefaultOutboxMaxAttempts      = 10
	DefaultOutboxPublishTimeoutMs = 15000
	DefaultStaleJobTimeoutMinutes = 180
	StaleJobHighReapThreshold     = 5
	OutboxBaseIntervalMs          = 2000    // 2 seconds
	OutboxMaxIntervalMs           = 3600000 // 1 hour
	OutboxJitterFactor            = 0.1     // ±10% jitter
	OutboxMultiplier              = 2.0     // Exponential
)

// InboxReaperConsumerTimeouts mirrors INBOX_REAPER_CONSUMER_TIMEOUTS from ScanDrix.
var InboxReaperConsumerTimeouts = map[string]time.Duration{
	"workflow-job-consumer.webhook":              20 * time.Minute,
	"workflow-job-consumer.check_implementation": 20 * time.Minute,
	"workflow-job-consumer.code_review":          150 * time.Minute, // 2.5 hours
	"workflow-job-consumer.ast_graph_build":      30 * time.Minute,
	"workflow-job-consumer.ast_graph_incremental": 15 * time.Minute,
	"workflow-events-stage-completed":            20 * time.Minute,
	"workflow-events-ast":                        20 * time.Minute,
}

// OutboxRepository defines persistence operations for outbox messages.
type OutboxRepository interface {
	FindReadyMessages(ctx context.Context, batchSize int) ([]*OutboxMessageModel, error)
	MarkSent(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, lastError string) error
	ScheduleRetry(ctx context.Context, id uuid.UUID, nextAttemptAt time.Time, attempts int, lastError string) error
}

// StaleJobReaper defines interface to sweep zombie jobs.
type StaleJobReaper interface {
	ReapStaleJobs(ctx context.Context, staleThreshold time.Duration) (int, error)
}

// InboxStuckReaper defines interface to reclaim stuck inbox messages.
type InboxStuckReaper interface {
	ReapStuckInbox(ctx context.Context, consumer string, timeout time.Duration) (int, error)
}

// IncidentAlerter triggers operational alarms.
type IncidentAlerter interface {
	TriggerIncident(ctx context.Context, title, message string, metadata map[string]any) error
}

// MessagePublisher abstracts RabbitMQ publishing with confirmation.
type MessagePublisher interface {
	Publish(ctx context.Context, exchange, routingKey string, payload map[string]any, options domain.BrokerPublishOptions) error
}

// OutboxRelayService mirrors ScanDrix OutboxRelayService for transactional message publishing.
type OutboxRelayService struct {
	outboxRepo     OutboxRepository
	publisher      MessagePublisher
	jobReaper      StaleJobReaper
	inboxReaper    InboxStuckReaper
	alerter        IncidentAlerter
	lockService    *DistributedLockService
	maxAttempts    int
	publishTimeout time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Telemetry
	relayedCount uint64
	failedCount  uint64
	retryCount   uint64
}

// NewOutboxRelayService instantiates an outbox relay service.
func NewOutboxRelayService(
	outboxRepo OutboxRepository,
	publisher MessagePublisher,
	jobReaper StaleJobReaper,
	inboxReaper InboxStuckReaper,
	alerter IncidentAlerter,
	lockService *DistributedLockService,
) *OutboxRelayService {
	ctx, cancel := context.WithCancel(context.Background())
	return &OutboxRelayService{
		outboxRepo:     outboxRepo,
		publisher:      publisher,
		jobReaper:      jobReaper,
		inboxReaper:    inboxReaper,
		alerter:        alerter,
		lockService:    lockService,
		maxAttempts:    DefaultOutboxMaxAttempts,
		publishTimeout: time.Duration(DefaultOutboxPublishTimeoutMs) * time.Millisecond,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start launches the periodic outbox relay loop, stale job reaper, and inbox lock cleaner.
func (s *OutboxRelayService) Start() {
	s.wg.Add(1)
	go s.relayLoop()

	if s.jobReaper != nil {
		s.wg.Add(1)
		go s.staleJobReaperLoop()
	}

	if s.inboxReaper != nil {
		s.wg.Add(1)
		go s.inboxReaperLoop()
	}
}

// Stop gracefully drains the outbox relay service.
func (s *OutboxRelayService) Stop() {
	s.cancel()
	s.wg.Wait()
}

func (s *OutboxRelayService) relayLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.RelayBatch(s.ctx, 50)
		}
	}
}

func (s *OutboxRelayService) staleJobReaperLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			reaped, err := s.ReapStaleJobs(s.ctx)
			if err == nil && reaped >= StaleJobHighReapThreshold && s.alerter != nil {
				_ = s.alerter.TriggerIncident(
					s.ctx,
					"High Orphaned Workflow Jobs Spike",
					fmt.Sprintf("Reaped %d orphaned workflow jobs beyond %d minutes threshold", reaped, DefaultStaleJobTimeoutMinutes),
					map[string]any{"reapedCount": reaped},
				)
			}
		}
	}
}

func (s *OutboxRelayService) inboxReaperLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			for consumer, timeout := range InboxReaperConsumerTimeouts {
				_, _ = s.inboxReaper.ReapStuckInbox(s.ctx, consumer, timeout)
			}
		}
	}
}

// RelayBatch processes a batch of ready outbox messages with exponential backoff on failure.
func (s *OutboxRelayService) RelayBatch(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	messages, err := s.outboxRepo.FindReadyMessages(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("failed fetching ready outbox messages: %w", err)
	}

	relayedCount := 0
	for _, msg := range messages {
		pubCtx, cancel := context.WithTimeout(ctx, s.publishTimeout)
		pubErr := s.publisher.Publish(
			pubCtx,
			msg.Exchange,
			msg.RoutingKey,
			msg.Payload,
			domain.BrokerPublishOptions{
				Persistent: true,
			},
		)
		cancel()

		if pubErr != nil {
			attempts := msg.Attempts + 1
			if attempts >= s.maxAttempts {
				_ = s.outboxRepo.MarkFailed(ctx, msg.UUID, pubErr.Error())
				atomic.AddUint64(&s.failedCount, 1)
			} else {
				backoff := CalculateBackoff(attempts)
				nextAttempt := time.Now().UTC().Add(backoff)
				_ = s.outboxRepo.ScheduleRetry(ctx, msg.UUID, nextAttempt, attempts, pubErr.Error())
				atomic.AddUint64(&s.retryCount, 1)
			}
		} else {
			if err := s.outboxRepo.MarkSent(ctx, msg.UUID); err == nil {
				relayedCount++
				atomic.AddUint64(&s.relayedCount, 1)
			}
		}
	}

	return relayedCount, nil
}

// ReapStaleJobs triggers reaping of abandoned PROCESSING jobs.
func (s *OutboxRelayService) ReapStaleJobs(ctx context.Context) (int, error) {
	if s.jobReaper == nil {
		return 0, nil
	}
	threshold := time.Duration(DefaultStaleJobTimeoutMinutes) * time.Minute
	return s.jobReaper.ReapStaleJobs(ctx, threshold)
}

// CalculateBackoff computes exponential backoff with full jitter matching ScanDrix polling utility.
func CalculateBackoff(attempt int) time.Duration {
	intervalMs := float64(OutboxBaseIntervalMs) * math.Pow(OutboxMultiplier, float64(attempt-1))
	if intervalMs > OutboxMaxIntervalMs {
		intervalMs = OutboxMaxIntervalMs
	}

	// Full jitter calculation: ±10%
	jitterRange := intervalMs * OutboxJitterFactor
	minInterval := intervalMs - jitterRange
	maxInterval := intervalMs + jitterRange

	jittered := minInterval + rand.Float64()*(maxInterval-minInterval)
	return time.Duration(jittered) * time.Millisecond
}

// GetMetrics returns snapshot of relay performance.
func (s *OutboxRelayService) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"relayed": atomic.LoadUint64(&s.relayedCount),
		"failed":  atomic.LoadUint64(&s.failedCount),
		"retrying": atomic.LoadUint64(&s.retryCount),
	}
}
