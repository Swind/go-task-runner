package taskrunner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Swind/go-task-runner/core"
)

// GoroutineThreadPool manages a set of worker goroutines
// Responsible for pulling tasks from WorkSource and executing them
type GoroutineThreadPool struct {
	id        string
	workers   int
	scheduler *core.TaskScheduler
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.RWMutex
}

// NewGoroutineThreadPool creates a new GoroutineThreadPool
func NewGoroutineThreadPool(id string, workers int) *GoroutineThreadPool {
	return &GoroutineThreadPool{
		id:        id,
		workers:   workers,
		scheduler: core.NewFIFOTaskScheduler(workers),
	}
}

func NewPriorityGoroutineThreadPool(id string, workers int) *GoroutineThreadPool {
	return &GoroutineThreadPool{
		id:        id,
		workers:   workers,
		scheduler: core.NewPriorityTaskScheduler(workers),
	}
}

// Start starts all worker goroutines
func (tg *GoroutineThreadPool) Start(ctx context.Context) {
	tg.runningMu.Lock()
	defer tg.runningMu.Unlock()

	if tg.running {
		return // Already running
	}

	tg.ctx, tg.cancel = context.WithCancel(ctx)
	tg.running = true

	for i := 0; i < tg.workers; i++ {
		tg.wg.Add(1)
		go tg.workerLoop(i, tg.ctx)
	}
}

// Stop stops the thread pool
func (tg *GoroutineThreadPool) Stop() {
	// Always shutdown scheduler to clean up resources (queue, delayed tasks)
	// even if pool was never started
	tg.scheduler.Shutdown()

	tg.runningMu.Lock()
	if !tg.running {
		tg.runningMu.Unlock()
		return
	}
	tg.runningMu.Unlock()

	if tg.cancel != nil {
		tg.cancel()
	}
	tg.Join()

	tg.runningMu.Lock()
	tg.running = false
	tg.runningMu.Unlock()
}

// StopGraceful stops the thread pool gracefully, waiting for queued tasks to complete
// Returns error if timeout is exceeded before tasks complete
func (tg *GoroutineThreadPool) StopGraceful(timeout time.Duration) error {
	tg.runningMu.Lock()
	if !tg.running {
		// Not running, nothing to do
		tg.runningMu.Unlock()
		return nil
	}
	tg.runningMu.Unlock()

	// First, gracefully shutdown the scheduler (waits for queues to drain)
	if err := tg.scheduler.ShutdownGraceful(timeout); err != nil {
		// Timeout occurred, but we still need to cancel workers
		if tg.cancel != nil {
			tg.cancel()
		}
		tg.Join()

		// Set running to false even on timeout path
		tg.runningMu.Lock()
		tg.running = false
		tg.runningMu.Unlock()

		return err
	}

	// Scheduler drained successfully, now cancel workers
	if tg.cancel != nil {
		tg.cancel()
	}
	tg.Join()

	tg.runningMu.Lock()
	tg.running = false
	tg.runningMu.Unlock()

	return nil
}

// ID returns the ID of the thread pool
func (tg *GoroutineThreadPool) ID() string {
	return tg.id
}

// IsRunning returns whether the thread pool is running
func (tg *GoroutineThreadPool) IsRunning() bool {
	tg.runningMu.RLock()
	defer tg.runningMu.RUnlock()
	return tg.running
}

// workerLoop is the main loop for each worker
func (tg *GoroutineThreadPool) workerLoop(id int, ctx context.Context) {
	defer tg.wg.Done()
	stopCh := ctx.Done()

	for {
		// Pull tasks from WorkSource
		task, ok := tg.scheduler.GetWork(stopCh)
		if !ok {
			// WorkSource closed or context canceled
			return
		}

		// Update Active Metrics via interface
		tg.scheduler.OnTaskStart()

		// Execute task and capture panic
		func() {
			defer func() {
				tg.scheduler.OnTaskEnd()
				if r := recover(); r != nil {
					// TODO: Add better error handling, e.g. callback
					fmt.Printf("[Worker %d] Panic: %v\n", id, r)
				}
			}()
			task(ctx)
		}()
	}
}

// Join waits for all worker goroutines to finish
func (tg *GoroutineThreadPool) Join() {
	tg.wg.Wait()
}

// WorkerCount returns the number of workers
func (tg *GoroutineThreadPool) WorkerCount() int {
	return tg.workers
}

func (tg *GoroutineThreadPool) QueuedTaskCount() int {
	return tg.scheduler.QueuedTaskCount()
}

func (tg *GoroutineThreadPool) ActiveTaskCount() int {
	return tg.scheduler.ActiveTaskCount()
}

func (tg *GoroutineThreadPool) DelayedTaskCount() int {
	return tg.scheduler.DelayedTaskCount()
}

func (tg *GoroutineThreadPool) PostInternal(task core.Task, traits core.TaskTraits) {
	tg.scheduler.PostInternal(task, traits)
}

func (tg *GoroutineThreadPool) PostDelayedInternal(task core.Task, delay time.Duration, traits core.TaskTraits, target core.TaskRunner) {
	tg.scheduler.PostDelayedInternal(task, delay, traits, target)
}

// =============================================================================
// Global Thread Pool Helper (Singleton)
// =============================================================================

var (
	globalThreadPool *GoroutineThreadPool
	globalMu         sync.Mutex
)

// InitGlobalThreadPool initializes the global thread pool with specified number of workers.
// It starts the pool immediately.
func InitGlobalThreadPool(workers int) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalThreadPool != nil {
		return // Already initialized
	}

	globalThreadPool = NewGoroutineThreadPool("global-pool", workers)
	globalThreadPool.Start(context.Background())
}

// GetGlobalThreadPool returns the global thread pool instance.
// It panics if InitGlobalThreadPool has not been called.
func GetGlobalThreadPool() *GoroutineThreadPool {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalThreadPool == nil {
		panic("GlobalThreadPool not initialized. Call InitGlobalThreadPool() first.")
	}
	return globalThreadPool
}

// ShutdownGlobalThreadPool stops the global thread pool.
func ShutdownGlobalThreadPool() {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalThreadPool != nil {
		globalThreadPool.Stop()
		globalThreadPool = nil
	}
}

// CreateTaskRunner creates a new SequencedTaskRunner using the global thread pool.
// This is the recommended way to get a new TaskRunner.
func CreateTaskRunner(traits TaskTraits) *SequencedTaskRunner {
	pool := GetGlobalThreadPool()
	// Note: Currently SequencedTaskRunner ignores traits for the runner itself (it attaches traits to tasks).
	// But in the future we might want to configure the runner with default traits.
	// For now, we return a standard SequencedTaskRunner backed by the global pool.
	return core.NewSequencedTaskRunner(pool)
}
