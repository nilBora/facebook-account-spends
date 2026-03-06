package queue

import (
	"context"
	"log/slog"
	"sync"
)

// Job is a unit of work executed by the pool.
type Job interface {
	Execute(ctx context.Context) error
	// Name returns a human-readable job identifier for logging.
	Name() string
}

// Pool runs jobs with a bounded number of concurrent workers.
// Each token should have its own Pool to enforce per-token concurrency limits.
type Pool struct {
	concurrency int
	jobs        chan Job
	wg          sync.WaitGroup
}

// New creates a Pool with the given concurrency limit.
func New(concurrency int) *Pool {
	return &Pool{
		concurrency: concurrency,
		jobs:        make(chan Job, 1000),
	}
}

// Start begins processing jobs from the queue until ctx is cancelled.
func (p *Pool) Start(ctx context.Context) {
	sem := make(chan struct{}, p.concurrency)

	go func() {
		for {
			select {
			case job, ok := <-p.jobs:
				if !ok {
					return
				}
				sem <- struct{}{} // acquire slot
				p.wg.Add(1)
				go func(j Job) {
					defer func() {
						<-sem // release slot
						p.wg.Done()
					}()
					if err := j.Execute(ctx); err != nil {
						slog.Error("job failed", "job", j.Name(), "err", err)
					}
				}(job)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Submit enqueues a job. Blocks if the queue is full.
func (p *Pool) Submit(job Job) {
	p.jobs <- job
}

// Wait blocks until all submitted jobs complete.
func (p *Pool) Wait() {
	p.wg.Wait()
}

// Close drains and closes the job channel.
func (p *Pool) Close() {
	close(p.jobs)
}
