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
	stopped chan struct{}
	once    sync.Once
	closed  atomic.Bool
}

// NewSingleThreadTaskRunner creates and starts a new SingleThreadTaskRunner.
// It immediately spawns a dedicated goroutine for task execution.
func NewSingleThreadTaskRunner() *SingleThreadTaskRunner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &SingleThreadTaskRunner{
		workQueue: make(chan Task, 100), // Buffer to avoid blocking senders
		ctx:       ctx,
		cancel:    cancel,
		stopped:   make(chan struct{}),
	}

	// Start the dedicated message loop
	go r.runLoop()

	return r
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

// Shutdown gracefully stops the runner
func (r *SingleThreadTaskRunner) Shutdown() {
	r.Stop()
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
				task(r.ctx)
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
