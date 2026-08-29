package consumer

import (
	"context"
	"sync"
)

// WorkerPool manages parallel worker goroutines processing review queue tasks.
type WorkerPool struct {
	concurrency int
	consumer    *ReviewConsumer
	taskChan    chan ReviewTaskPayload
	results     chan TaskExecutionResult
	wg          sync.WaitGroup
	stopChan    chan struct{}
}

// NewWorkerPool initializes the concurrent worker pool.
func NewWorkerPool(concurrency int, consumer *ReviewConsumer) *WorkerPool {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &WorkerPool{
		concurrency: concurrency,
		consumer:    consumer,
		taskChan:    make(chan ReviewTaskPayload, 100),
		results:     make(chan TaskExecutionResult, 100),
		stopChan:    make(chan struct{}),
	}
}

// Start launches worker goroutines.
func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// Submit queues a task for immediate worker pickup.
func (p *WorkerPool) Submit(task ReviewTaskPayload) {
	p.taskChan <- task
}

// Stop gracefully terminates workers after flushing queued tasks.
func (p *WorkerPool) Stop() {
	close(p.taskChan)
	p.wg.Wait()
	close(p.results)
}

func (p *WorkerPool) worker(ctx context.Context) {
	defer p.wg.Done()

	for task := range p.taskChan {
		res := p.consumer.ProcessTask(ctx, task)
		p.results <- res
	}
}

// ResultsChannel returns the stream of execution results.
func (p *WorkerPool) ResultsChannel() <-chan TaskExecutionResult {
	return p.results
}
