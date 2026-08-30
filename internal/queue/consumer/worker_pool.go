package consumer

import (
	"context"
	"sync"
)

// WorkerPool manages parallel worker goroutines processing review queue tasks.
type WorkerPool struct {
	concurrency int
	consumer    *ReviewConsumer
	jobChan     chan WorkerJob
	results     chan TaskExecutionResult
	wg          sync.WaitGroup
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
func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// Submit queues a task for immediate worker pickup.
func (p *WorkerPool) Submit(task ReviewTaskPayload) {
	p.SubmitJob(WorkerJob{Task: task})
}

// SubmitJob queues a task with an optional completion callback.
func (p *WorkerPool) SubmitJob(job WorkerJob) {
	p.jobChan <- job
}

// Stop gracefully terminates workers after draining and completing all queued tasks.
func (p *WorkerPool) Stop() {
	close(p.jobChan)
	p.wg.Wait()
	close(p.results)
}

func (p *WorkerPool) worker(ctx context.Context) {
	defer p.wg.Done()

	for job := range p.jobChan {
		res := p.consumer.ProcessTask(ctx, job.Task)
		if job.OnComplete != nil {
			job.OnComplete(res)
		} else {
			p.results <- res
		}
	}
}

// ResultsChannel returns the stream of execution results.
func (p *WorkerPool) ResultsChannel() <-chan TaskExecutionResult {
	return p.results
}

