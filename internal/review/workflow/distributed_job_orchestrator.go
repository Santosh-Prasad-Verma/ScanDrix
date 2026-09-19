package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// OrchestrationEvent represents an incoming message in the RabbitMQ/distributed queue.
type OrchestrationEvent struct {
	MessageID   string          `json:"message_id"`
	QueueName   string          `json:"queue_name"`
	RoutingKey  string          `json:"routing_key"`
	Payload     json.RawMessage `json:"payload"`
	ReceivedAt  time.Time       `json:"received_at"`
}

// OrchestratorWorkerConfig configures background worker pool and heartbeat parameters.
type OrchestratorWorkerConfig struct {
	WorkerID          string        `json:"worker_id"`
	WorkerCount       int           `json:"worker_count"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
	LeaseDuration     time.Duration `json:"lease_duration"`
	SweepInterval     time.Duration `json:"sweep_interval"`
}

// DefaultWorkerConfig provides production concurrency configuration.
func DefaultWorkerConfig(workerID string) OrchestratorWorkerConfig {
	return OrchestratorWorkerConfig{
		WorkerID:          workerID,
		WorkerCount:       8,
		HeartbeatInterval: 5 * time.Second,
		LeaseDuration:     30 * time.Second,
		SweepInterval:     1 * time.Minute,
	}
}

// DistributedJobOrchestrator manages consumption of code review jobs with distributed claims and fault tolerance.
type DistributedJobOrchestrator struct {
	mu           sync.RWMutex
	processor    *ReviewJobProcessor
	claimEngine  *InboxClaimEngine
	dlqEngine    *DeadLetterEngine
	config       OrchestratorWorkerConfig
	incomingChan chan OrchestrationEvent
	stopChan     chan struct{}
	wg           sync.WaitGroup
	isRunning    bool
}

// NewDistributedJobOrchestrator initializes the distributed job orchestrator.
func NewDistributedJobOrchestrator(
	processor *ReviewJobProcessor,
	claimEngine *InboxClaimEngine,
	dlqEngine *DeadLetterEngine,
	cfg ...OrchestratorWorkerConfig,
) *DistributedJobOrchestrator {
	config := DefaultWorkerConfig("scandrix-worker-1")
	if len(cfg) > 0 {
		config = cfg[0]
	}
	if claimEngine == nil {
		claimEngine = NewInboxClaimEngine(nil, config.LeaseDuration)
	}
	if dlqEngine == nil {
		dlqEngine = NewDeadLetterEngine()
	}

	return &DistributedJobOrchestrator{
		processor:    processor,
		claimEngine:  claimEngine,
		dlqEngine:    dlqEngine,
		config:       config,
		incomingChan: make(chan OrchestrationEvent, 256),
		stopChan:     make(chan struct{}),
	}
}

// SubmitEvent enqueues an orchestration event for asynchronous processing.
func (o *DistributedJobOrchestrator) SubmitEvent(ctx context.Context, event OrchestrationEvent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case o.incomingChan <- event:
		return nil
	default:
		return fmt.Errorf("worker incoming queue is full")
	}
}

// Start launches the distributed worker pool.
func (o *DistributedJobOrchestrator) Start(ctx context.Context) error {
	o.mu.Lock()
	if o.isRunning {
		o.mu.Unlock()
		return fmt.Errorf("distributed job orchestrator is already running")
	}
	o.isRunning = true
	o.mu.Unlock()

	for i := 0; i < o.config.WorkerCount; i++ {
		o.wg.Add(1)
		workerSubID := fmt.Sprintf("%s-thread-%d", o.config.WorkerID, i)
		go o.workerLoop(ctx, workerSubID)
	}

	// Launch expired lease sweeper loop
	o.wg.Add(1)
	go o.sweeperLoop(ctx)

	return nil
}

// Stop gracefully terminates worker threads.
func (o *DistributedJobOrchestrator) Stop() {
	o.mu.Lock()
	if !o.isRunning {
		o.mu.Unlock()
		return
	}
	o.isRunning = false
	close(o.stopChan)
	o.mu.Unlock()

	o.wg.Wait()
}

func (o *DistributedJobOrchestrator) workerLoop(ctx context.Context, workerID string) {
	defer o.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopChan:
			return
		case event, ok := <-o.incomingChan:
			if !ok {
				return
			}
			o.processEvent(ctx, workerID, event)
		}
	}
}

func (o *DistributedJobOrchestrator) processEvent(ctx context.Context, workerID string, event OrchestrationEvent) {
	// 1. Try to claim event exclusively
	claim, err := o.claimEngine.TryClaim(ctx, event.MessageID, workerID)
	if err != nil {
		// Event is already claimed, completed, or actively held by another worker
		return
	}

	// 2. Start periodic heartbeat loop for duration of execution
	doneHeartbeat := make(chan struct{})
	heartbeatErrChan := make(chan error, 1)

	go func() {
		ticker := time.NewTicker(o.config.HeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-doneHeartbeat:
				return
			case <-ticker.C:
				err := o.claimEngine.RenewHeartbeat(
					ctx, event.MessageID, workerID, claim.FencingToken, o.config.LeaseDuration,
				)
				if err != nil {
					select {
					case heartbeatErrChan <- err:
					default:
					}
					return
				}
			}
		}
	}()

	// 3. Execute review job
	execErr := o.processor.ProcessReviewJob(ctx, event.Payload)

	// Terminate heartbeat
	close(doneHeartbeat)

	// 4. Handle result
	if execErr != nil {
		// Log failure and evaluate retry policy / DLQ
		env := o.dlqEngine.HandleFailure(
			ctx, event.MessageID, event.QueueName, event.RoutingKey,
			event.Payload, claim.AttemptCount, execErr,
		)

		outcome := ClaimStatusReleased
		if env.IsDeadLettered {
			outcome = ClaimStatusDeadLetter
		}
		_ = o.claimEngine.ReleaseClaim(ctx, event.MessageID, workerID, claim.FencingToken, outcome)
	} else {
		// Success: release claim as completed and clear DLQ tracking
		_ = o.claimEngine.ReleaseClaim(ctx, event.MessageID, workerID, claim.FencingToken, ClaimStatusCompleted)
		o.dlqEngine.AcknowledgeSuccess(event.MessageID)
	}
}

func (o *DistributedJobOrchestrator) sweeperLoop(ctx context.Context) {
	defer o.wg.Done()

	ticker := time.NewTicker(o.config.SweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopChan:
			return
		case <-ticker.C:
			_, _ = o.claimEngine.SweepExpired(ctx)
		}
	}
}

// GetTelemetry returns runtime queue depths and DLQ statistics.
func (o *DistributedJobOrchestrator) GetTelemetry() map[string]interface{} {
	o.mu.RLock()
	defer o.mu.RUnlock()

	dlqMsgs := o.dlqEngine.GetDLQMessages()
	retries := o.dlqEngine.GetReadyRetries(time.Now().UTC())

	return map[string]interface{}{
		"queue_depth":       len(o.incomingChan),
		"worker_count":      o.config.WorkerCount,
		"dlq_message_count": len(dlqMsgs),
		"pending_retries":   len(retries),
		"is_running":        o.isRunning,
	}
}
