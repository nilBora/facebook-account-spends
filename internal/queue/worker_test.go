package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testJob is a Job implementation used in tests.
type testJob struct {
	name string
	fn   func(ctx context.Context) error
}

func (j *testJob) Name() string                        { return j.name }
func (j *testJob) Execute(ctx context.Context) error   { return j.fn(ctx) }

func newJob(name string, fn func(ctx context.Context) error) *testJob {
	return &testJob{name: name, fn: fn}
}

func TestPool_AllJobsRun(t *testing.T) {
	const n = 20
	var count atomic.Int64

	pool := New(4)
	pool.Start(context.Background())
	for i := 0; i < n; i++ {
		pool.Submit(newJob("job", func(_ context.Context) error {
			count.Add(1)
			return nil
		}))
	}
	pool.Wait()
	pool.Close()

	if count.Load() != n {
		t.Errorf("ran %d jobs, want %d", count.Load(), n)
	}
}

func TestPool_FailedJobsDoNotBlockOthers(t *testing.T) {
	var successCount atomic.Int64

	pool := New(2)
	pool.Start(context.Background())

	for i := 0; i < 10; i++ {
		i := i
		pool.Submit(newJob("job", func(_ context.Context) error {
			if i%2 == 0 {
				return errors.New("even job fails")
			}
			successCount.Add(1)
			return nil
		}))
	}
	pool.Wait()
	pool.Close()

	if successCount.Load() != 5 {
		t.Errorf("expected 5 successful jobs, got %d", successCount.Load())
	}
}

func TestPool_BoundedConcurrency(t *testing.T) {
	const concurrency = 3
	var (
		mu      sync.Mutex
		current int
		maxSeen int
	)

	pool := New(concurrency)
	pool.Start(context.Background())

	for i := 0; i < 15; i++ {
		pool.Submit(newJob("job", func(_ context.Context) error {
			mu.Lock()
			current++
			if current > maxSeen {
				maxSeen = current
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()
			return nil
		}))
	}
	pool.Wait()
	pool.Close()

	if maxSeen > concurrency {
		t.Errorf("max concurrent jobs = %d, want <= %d", maxSeen, concurrency)
	}
	if maxSeen == 0 {
		t.Error("no jobs ran")
	}
}

func TestPool_WaitReturnsAfterAllDone(t *testing.T) {
	pool := New(2)
	pool.Start(context.Background())

	var done atomic.Bool
	pool.Submit(newJob("slow", func(_ context.Context) error {
		time.Sleep(30 * time.Millisecond)
		done.Store(true)
		return nil
	}))

	pool.Wait()
	pool.Close()

	if !done.Load() {
		t.Error("Wait() returned before job finished")
	}
}

func TestPool_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pool := New(2)
	pool.Start(ctx)

	var executed atomic.Int64
	for i := 0; i < 5; i++ {
		pool.Submit(newJob("job", func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
				executed.Add(1)
				return nil
			}
		}))
	}

	cancel()
	pool.Wait()
	pool.Close()
	// Jobs may or may not complete depending on race; just verify no panic/deadlock.
	t.Logf("completed %d jobs after cancel", executed.Load())
}

func TestPool_EmptyPool(t *testing.T) {
	pool := New(4)
	pool.Start(context.Background())
	pool.Wait()  // should return immediately
	pool.Close() // should not panic
}
