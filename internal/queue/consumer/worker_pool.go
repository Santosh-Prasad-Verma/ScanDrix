package consumer

import (
	"context"
	"sync"
	"sync/atomic"
)

// WorkerPool manages parallel worker goroutines processing review queue tasks.
type WorkerPool struct {
	concurrency      int
	consumer         *ReviewConsumer
	jobChan          chan WorkerJob
	results          chan TaskExecutionResult
	hasResultsReader atomic.Bool
	wg               sync.WaitGroup
	mu               sync.RWMutex
	isClosing        bool
}

// NewWorkerPool initializes the concurrent worker pool.
func NewWorkerPool(concurrency int, consumer *ReviewConsumer) *WorkerPool {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &WorkerPool{
		concurrency: concurrency,
		consumer:    consumer,
		jobChan:     make(chan WorkerJob, 100),
		results:     make(chan TaskExecutionResult, 100),
	}
}

// Start launches worker goroutines.
//
// Callers that consume ResultsChannel MUST obtain the channel via
// ResultsChannel BEFORE calling Start. Results produced for jobs submitted with
// no OnComplete callback are delivered to the results channel only while a
// reader is registered; anything produced before registration is discarded so
// that a pool with no result consumer can never deadlock its own workers.
// Registering after Start therefore silently loses the results produced in
// between, so the subscription must be established up front.
func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// Submit queues a task for immediate worker pickup. Returns false if the pool is stopping.
func (p *WorkerPool) Submit(task ReviewTaskPayload) bool {
	return p.SubmitJob(WorkerJob{Task: task})
}

// SubmitJob queues a task with an optional completion callback. Returns false if the pool is stopping or buffer is full.
func (p *WorkerPool) SubmitJob(job WorkerJob) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.isClosing {
		return false
	}

	select {
	case p.jobChan <- job:
		return true
	default:
		return false
	}
}

// Stop gracefully terminates workers after draining and completing all queued tasks.
func (p *WorkerPool) Stop() {
	p.mu.Lock()
	if p.isClosing {
		p.mu.Unlock()
		return
	}
	p.isClosing = true
	close(p.jobChan)
	p.mu.Unlock()

	p.wg.Wait()
	close(p.results)
}

func (p *WorkerPool) worker(ctx context.Context) {
	defer p.wg.Done()

	for job := range p.jobChan {
		res := p.consumer.ProcessTask(ctx, job.Task)
		if job.OnComplete != nil {
			job.OnComplete(res)
		} else if p.hasResultsReader.Load() {
			// A reader is registered: block until the result is delivered (or the
			// pool's context is cancelled) so delivery is never lossy.
			select {
			case p.results <- res:
			case <-ctx.Done():
				return
			}
		} else {
			// No reader: deliver if there is room, otherwise discard. Blocking here
			// would deadlock the workers once the buffer fills and nothing drains it.
			select {
			case p.results <- res:
			default:
			}
		}
	}
}

// ResultsChannel returns the stream of execution results and registers this
// pool as having a result reader, which switches workers from discarding
// results to delivering them. Call it before Start to avoid losing results.
func (p *WorkerPool) ResultsChannel() <-chan TaskExecutionResult {
	p.hasResultsReader.Store(true)
	return p.results
}
