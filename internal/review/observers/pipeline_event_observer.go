// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package observers

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// PipelineEventType categorizes the lifecycle milestones of a code review run.
type PipelineEventType string

const (
	EventReviewStarted   PipelineEventType = "REVIEW_STARTED"
	EventStageStarted    PipelineEventType = "STAGE_STARTED"
	EventStageCompleted  PipelineEventType = "STAGE_COMPLETED"
	EventFindingEmitted  PipelineEventType = "FINDING_EMITTED"
	EventReviewCompleted PipelineEventType = "REVIEW_COMPLETED"
	EventReviewFailed    PipelineEventType = "REVIEW_FAILED"
)

// PipelineEvent carries structured lifecycle details across review execution.
type PipelineEvent struct {
	Type          PipelineEventType      `json:"type"`
	ReviewID      uuid.UUID              `json:"review_id"`
	PullNumber    int                    `json:"pull_number"`
	RepoID        string                 `json:"repo_id"`
	StageName     string                 `json:"stage_name,omitempty"`
	Duration      time.Duration          `json:"duration,omitempty"`
	Finding       *models.CodeFinding    `json:"finding,omitempty"`
	TotalFindings int                    `json:"total_findings,omitempty"`
	CriticalCount int                    `json:"critical_count,omitempty"`
	PassedReview  bool                   `json:"passed_review"`
	Error         error                  `json:"error,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
}

// PipelineObserver defines the listener contract for code review events.
type PipelineObserver interface {
	Name() string
	OnEvent(ctx context.Context, event PipelineEvent) error
}

// PipelineEventDispatcher coordinates asynchronous, non-blocking fan-out of review events.
type PipelineEventDispatcher struct {
	mu          sync.RWMutex
	observers   []PipelineObserver
	eventQueue  chan PipelineEvent
	workers     int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	closed      bool
	droppedCount int64
}

// NewPipelineEventDispatcher constructs an event dispatcher with a worker pool.
func NewPipelineEventDispatcher(bufferSize, workers int) *PipelineEventDispatcher {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	if workers <= 0 {
		workers = 4
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &PipelineEventDispatcher{
		observers:  make([]PipelineObserver, 0),
		eventQueue: make(chan PipelineEvent, bufferSize),
		workers:    workers,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Start worker pool
	for i := 0; i < workers; i++ {
		d.wg.Add(1)
		go d.workerLoop()
	}

	return d
}

// RegisterObserver attaches a new listener.
func (d *PipelineEventDispatcher) RegisterObserver(obs PipelineObserver) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.observers = append(d.observers, obs)
}

// Emit broadcasts an event to all registered observers asynchronously.
// If the buffer is full, it drops the event or records a dropped counter to avoid stalling the pipeline.
func (d *PipelineEventDispatcher) Emit(event PipelineEvent) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return false
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	select {
	case d.eventQueue <- event:
		return true
	default:
		// Queue full: non-blocking drop to protect review pipeline latency
		d.droppedCount++
		return false
	}
}

func (d *PipelineEventDispatcher) workerLoop() {
	defer d.wg.Done()

	for {
		select {
		case <-d.ctx.Done():
			// Drain remaining events in queue
			for {
				select {
				case ev := <-d.eventQueue:
					d.dispatchToObservers(ev)
				default:
					return
				}
			}
		case ev := <-d.eventQueue:
			d.dispatchToObservers(ev)
		}
	}
}

func (d *PipelineEventDispatcher) dispatchToObservers(ev PipelineEvent) {
	d.mu.RLock()
	observersCopy := make([]PipelineObserver, len(d.observers))
	copy(observersCopy, d.observers)
	d.mu.RUnlock()

	for _, obs := range observersCopy {
		_ = obs.OnEvent(d.ctx, ev)
	}
}

// Close gracefully stops workers and drains pending events up to timeout.
func (d *PipelineEventDispatcher) Close(timeout time.Duration) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	d.mu.Unlock()

	d.cancel()

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return context.DeadlineExceeded
	}
}

// In-Memory Telemetry Observer for testing and metrics aggregation.
type MetricsObserver struct {
	mu           sync.Mutex
	EventsByType map[PipelineEventType]int
	TotalEvents  int
}

func NewMetricsObserver() *MetricsObserver {
	return &MetricsObserver{
		EventsByType: make(map[PipelineEventType]int),
	}
}

func (m *MetricsObserver) Name() string {
	return "MetricsObserver"
}

func (m *MetricsObserver) OnEvent(ctx context.Context, ev PipelineEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EventsByType[ev.Type]++
	m.TotalEvents++
	return nil
}

func (m *MetricsObserver) GetCount(t PipelineEventType) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.EventsByType[t]
}
