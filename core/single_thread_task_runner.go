package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// SingleThreadTaskRunner binds a dedicated Goroutine to execute tasks sequentially.
// It guarantees that all tasks submitted to it run on the same Goroutine (Thread Affinity).
//
// Use cases:
// 1. Blocking IO operations (e.g., NetworkReceiver)
// 2. CGO calls that require Thread Local Storage
// 3. Simulating Main Thread / UI Thread behavior
//
// Key differences from SequencedTaskRunner:
// - SequencedTaskRunner: Tasks execute sequentially but may run on different worker goroutines
// - SingleThreadTaskRunner: Tasks execute sequentially AND always on the same dedicated goroutine
type SingleThreadTaskRunner struct {
	// Task queue: Buffered channel for tasks
	workQueue chan Task

	// Lifecycle control
	ctx    context.Context
	cancel context.CancelFunc

	// For graceful shutdown
	stopped      chan struct{}
	once         sync.Once
	closed       atomic.Bool
	shutdownChan chan struct{}
	shutdownOnce sync.Once

	// Metadata
	name     string
	metadata map[string]interface{}
	mu       sync.Mutex
}

// NewSingleThreadTaskRunner creates and starts a new SingleThreadTaskRunner.
// It immediately spawns a dedicated goroutine for task execution.
func NewSingleThreadTaskRunner() *SingleThreadTaskRunner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &SingleThreadTaskRunner{
		workQueue:    make(chan Task, 100), // Buffer to avoid blocking senders
		ctx:          ctx,
		cancel:       cancel,
		stopped:      make(chan struct{}),
		shutdownChan: make(chan struct{}),
		metadata:     make(map[string]interface{}),
	}

	// Start the dedicated message loop
	go r.runLoop()

	return r
}

// Name returns the name of the task runner
func (r *SingleThreadTaskRunner) Name() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.name
}

// SetName sets the name of the task runner
func (r *SingleThreadTaskRunner) SetName(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.name = name
}

// Metadata returns the metadata associated with the task runner
func (r *SingleThreadTaskRunner) Metadata() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make(map[string]interface{}, len(r.metadata))
	for k, v := range r.metadata {
		result[k] = v
	}
	return result
}

// SetMetadata sets a metadata key-value pair
func (r *SingleThreadTaskRunner) SetMetadata(key string, value interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metadata[key] = value
}

// GetThreadPool returns nil because SingleThreadTaskRunner doesn't use a thread pool
func (r *SingleThreadTaskRunner) GetThreadPool() ThreadPool {
	return nil
}

// PostTask submits a task for execution
func (r *SingleThreadTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

// PostTaskWithTraits submits a task with traits (traits are ignored for single-threaded execution)
func (r *SingleThreadTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	// Check if runner is closed to avoid panic on closed channel
	if r.closed.Load() {
		return
	}

	select {
	case <-r.ctx.Done():
		// Runner stopped, drop task
		return
	case r.workQueue <- task:
		// Successfully queued
	}
}

// PostDelayedTask submits a delayed task
func (r *SingleThreadTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraits(task, delay, DefaultTaskTraits())
}

// PostDelayedTaskWithTraits submits a delayed task with traits.
// Uses time.AfterFunc which is independent of the global TaskScheduler,
// ensuring IO-related timers are not affected by scheduler load.
func (r *SingleThreadTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	if r.closed.Load() {
		return
	}

	select {
	case <-r.ctx.Done():
		return
	default:
		// time.AfterFunc spawns a new goroutine when the timer fires,
		// we use PostTask to inject the task back into our main loop
		time.AfterFunc(delay, func() {
			r.PostTaskWithTraits(task, traits)
		})
	}
}

// PostRepeatingTask submits a task that repeats at a fixed interval
func (r *SingleThreadTaskRunner) PostRepeatingTask(task Task, interval time.Duration) RepeatingTaskHandle {
	return r.PostRepeatingTaskWithTraits(task, interval, DefaultTaskTraits())
}

// PostRepeatingTaskWithTraits submits a repeating task with traits
func (r *SingleThreadTaskRunner) PostRepeatingTaskWithTraits(
	task Task,
	interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle {
	return r.PostRepeatingTaskWithInitialDelay(task, 0, interval, traits)
}

// PostRepeatingTaskWithInitialDelay submits a repeating task with an initial delay
func (r *SingleThreadTaskRunner) PostRepeatingTaskWithInitialDelay(
	task Task,
	initialDelay, interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle {
	handle := &singleThreadRepeatingHandle{
		runner:   r,
		task:     task,
		interval: interval,
		traits:   traits,
	}

	// Create the repeating task
	repeatingTask := handle.createRepeatingTask()

	// Schedule first execution
	if initialDelay > 0 {
		r.PostDelayedTaskWithTraits(repeatingTask, initialDelay, traits)
	} else {
		r.PostTaskWithTraits(repeatingTask, traits)
	}

	return handle
}

// Shutdown marks the runner as closed and signals shutdown waiters.
// Unlike Stop(), this method does NOT immediately terminate the runLoop.
// This allows tasks to call Shutdown() from within themselves.
//
// After calling Shutdown():
// - WaitShutdown() will return
// - IsClosed() will return true
// - New tasks posted will be ignored
// - Existing queued tasks will still execute
// - Call Stop() to actually terminate the runLoop
func (r *SingleThreadTaskRunner) Shutdown() {
	r.shutdownOnce.Do(func() {
		// Mark as closed
		r.closed.Store(true)
		// Cancel context to stop accepting new tasks and unblock runLoop
		r.cancel()
		// Close shutdown channel to signal waiters
		close(r.shutdownChan)
	})
}

// IsClosed returns true if the runner has been stopped
func (r *SingleThreadTaskRunner) IsClosed() bool {
	return r.closed.Load()
}

// Stop stops the runner and releases resources
func (r *SingleThreadTaskRunner) Stop() {
	r.once.Do(func() {
		// 1. Mark as closed
		r.closed.Store(true)

		// 2. Cancel context to stop accepting new tasks
		r.cancel()

		// 3. Wait for runLoop to finish (ensures current task completes)
		<-r.stopped
	})
}

// runLoop is the core of this runner, it occupies a dedicated goroutine
func (r *SingleThreadTaskRunner) runLoop() {
	defer close(r.stopped) // Signal that Stop() can return

	// Create context with taskRunnerKey for GetCurrentTaskRunner
	runCtx := context.WithValue(r.ctx, taskRunnerKey, r)

	for {
		select {
		case task := <-r.workQueue:
			// Execute task and catch panics
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						fmt.Printf("[SingleThreadTaskRunner] Panic: %v\n", rec)
					}
				}()
				task(runCtx)
			}()

		case <-r.ctx.Done():
			// Received stop signal
			// Option: Could drain remaining workQueue here
			// Current implementation: exit immediately
			return
		}
	}
}

// =============================================================================
// Repeating Task Handle for SingleThreadTaskRunner
// =============================================================================

type singleThreadRepeatingHandle struct {
	runner   *SingleThreadTaskRunner
	task     Task
	interval time.Duration
	traits   TaskTraits
	stopped  atomic.Bool
}

func (h *singleThreadRepeatingHandle) Stop() {
	h.stopped.Store(true)
}

func (h *singleThreadRepeatingHandle) IsStopped() bool {
	return h.stopped.Load()
}

func (h *singleThreadRepeatingHandle) createRepeatingTask() Task {
	return func(ctx context.Context) {
		// Check if runner is closed
		if h.runner.IsClosed() {
			return
		}

		// Check if handle is stopped
		if h.IsStopped() {
			return
		}

		// Execute the task
		h.task(ctx)

		// Reschedule if not stopped and runner is still open
		if !h.IsStopped() && !h.runner.IsClosed() {
			h.runner.PostDelayedTaskWithTraits(h.createRepeatingTask(), h.interval, h.traits)
		}
	}
}

// =============================================================================
// Task and Reply Pattern
// =============================================================================

// PostTaskAndReply executes task on this runner, then posts reply to replyRunner.
// If task panics, reply will not be executed.
// Both task and reply will execute on the same dedicated goroutine if replyRunner is this runner.
func (r *SingleThreadTaskRunner) PostTaskAndReply(task Task, reply Task, replyRunner TaskRunner) {
	postTaskAndReplyInternal(r, task, reply, replyRunner, DefaultTaskTraits())
}

// PostTaskAndReplyWithTraits allows specifying different traits for task and reply.
// This is useful when task is background work (BestEffort) but reply is UI update (UserVisible).
// Note: For SingleThreadTaskRunner, traits don't affect execution order since all tasks
// run sequentially on the same goroutine, but they may be used for logging or metrics.
func (r *SingleThreadTaskRunner) PostTaskAndReplyWithTraits(
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	postTaskAndReplyInternalWithTraits(r, task, taskTraits, reply, replyTraits, replyRunner)
}

// =============================================================================
// Synchronization Methods
// =============================================================================

// WaitIdle blocks until all currently queued tasks have completed execution.
// This is implemented by posting a barrier task and waiting for it to execute.
//
// Since SingleThreadTaskRunner executes tasks sequentially on a dedicated goroutine,
// when the barrier task executes, all tasks posted before WaitIdle are guaranteed
// to have completed.
//
// Returns error if:
// - Context is cancelled or deadline exceeded
// - Runner is closed when WaitIdle is called
//
// Note: Tasks posted after WaitIdle is called are not waited for.
// Note: Repeating tasks will continue to repeat and are not waited for.
func (r *SingleThreadTaskRunner) WaitIdle(ctx context.Context) error {
	if r.IsClosed() {
		return fmt.Errorf("runner is closed")
	}

	done := make(chan struct{})

	// Post a barrier task that closes the done channel
	r.PostTask(func(taskCtx context.Context) {
		close(done)
	})

	// Wait for barrier task or context cancellation
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FlushAsync posts a barrier task that executes the callback when all prior tasks complete.
// This is a non-blocking alternative to WaitIdle.
//
// The callback will be executed on this runner's dedicated goroutine, after all tasks
// posted before FlushAsync have completed.
//
// Example:
//
//	runner.PostTask(task1)
//	runner.PostTask(task2)
//	runner.FlushAsync(func() {
//	    fmt.Println("task1 and task2 completed!")
//	})
func (r *SingleThreadTaskRunner) FlushAsync(callback func()) {
	r.PostTask(func(ctx context.Context) {
		callback()
	})
}

// WaitShutdown blocks until Shutdown() is called on this runner.
//
// This is useful for waiting for the runner to be shut down, either by
// an external caller or by a task running on the runner itself.
//
// Returns error if context is cancelled or deadline exceeded.
//
// Example:
//
//	// IO thread: receives messages and posts shutdown when condition met
//	ioRunner.PostTask(func(ctx context.Context) {
//	    for {
//	        msg := receiveMessage()
//	        mainRunner.PostTask(func(ctx context.Context) {
//	            if shouldShutdown(msg) {
//	                me := GetCurrentTaskRunner(ctx)
//	                me.Shutdown()
//	            }
//	        })
//	    }
//	})
//
//	// Main thread waits for shutdown
//	mainRunner.WaitShutdown(context.Background())
func (r *SingleThreadTaskRunner) WaitShutdown(ctx context.Context) error {
	select {
	case <-r.shutdownChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
