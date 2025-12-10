package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type SequencedTaskRunner struct {
	threadPool    ThreadPool
	queue         TaskQueue
	mu            sync.Mutex
	isRunning     bool
	activeRunners int32      // atomic guard for concurrency assertion
	closed        atomic.Bool // indicates if the runner is closed
}

func NewSequencedTaskRunner(threadPool ThreadPool) *SequencedTaskRunner {
	return &SequencedTaskRunner{
		threadPool: threadPool,
		queue:      NewFIFOTaskQueue(),
	}
}

func (r *SequencedTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	r.threadPool.PostDelayedInternal(task, delay, traits, r)
}

func (r *SequencedTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraits(task, delay, DefaultTaskTraits())
}

func (r *SequencedTaskRunner) runLoop(ctx context.Context) {
	// Assertion: Ensure strictly one goroutine at a time
	if n := atomic.AddInt32(&r.activeRunners, 1); n > 1 {
		panic(fmt.Sprintf("SequencedTaskRunner: concurrent runLoop detected (count=%d)", n))
	}
	defer atomic.AddInt32(&r.activeRunners, -1)

	runCtx := context.WithValue(ctx, taskRunnerKey, r)

	// 1. Fetch SINGLE task
	item, ok := r.queue.Pop() // Changed from PopUpTo

	if !ok {
		// Queue completely empty
		r.mu.Lock()
		if r.queue.IsEmpty() {
			r.isRunning = false
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()

		var needRepost bool
		r.mu.Lock()
		if r.queue.IsEmpty() {
			r.isRunning = false
			needRepost = false
		} else {
			needRepost = true
		}
		r.mu.Unlock()

		if needRepost {
			nextTraits, _ := r.queue.PeekTraits() // Best effort peek
			r.rePostSelf(nextTraits)
		}
		return
	}

	// 2. Execute ONE task
	func() {
		defer func() { recover() }()
		item.Task(runCtx)
	}()

	// 3. Always repost if there are more tasks (Yield)
	// This ensures we yield to the Scheduler between every task
	var nextTraits TaskTraits
	var more bool

	r.mu.Lock()
	if r.queue.IsEmpty() {
		r.isRunning = false
		more = false
	} else {
		more = true
	}
	r.mu.Unlock()

	if more {
		nextTraits, _ = r.queue.PeekTraits()
		r.rePostSelf(nextTraits)
	}
}

// scheduleRunLoop starts runLoop (if not already running)
func (r *SequencedTaskRunner) scheduleRunLoop(traits TaskTraits) {
	r.mu.Lock()
	if !r.isRunning {
		r.isRunning = true
		r.mu.Unlock()
		r.threadPool.PostInternal(r.runLoop, traits)
	} else {
		r.mu.Unlock()
	}
}

// rePostSelf re-submits runLoop to Scheduler (for Yield)
func (r *SequencedTaskRunner) rePostSelf(traits TaskTraits) {
	r.threadPool.PostInternal(r.runLoop, traits)
}

// PostTask submits task (using default Traits)
func (r *SequencedTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

// PostTaskWithTraits submits task with traits
func (r *SequencedTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	r.queue.Push(task, traits)
	r.scheduleRunLoop(traits)
}

// =============================================================================
// Repeating Task Implementation
// =============================================================================

// repeatingTaskHandle implements RepeatingTaskHandle interface
type repeatingTaskHandle struct {
	task     Task
	interval time.Duration
	traits   TaskTraits
	stopped  atomic.Bool
}

func (h *repeatingTaskHandle) Stop() {
	h.stopped.Store(true)
}

func (h *repeatingTaskHandle) IsStopped() bool {
	return h.stopped.Load()
}

// createRepeatingTask creates a self-scheduling repeating task
func (h *repeatingTaskHandle) createRepeatingTask() Task {
	return func(ctx context.Context) {
		// Get the current runner from context
		runner := GetCurrentTaskRunner(ctx)

		// Check if runner is closed (automatic cleanup)
		if r, ok := runner.(*SequencedTaskRunner); ok && r.IsClosed() {
			return
		}

		// Check if handle is manually stopped
		if h.IsStopped() {
			return
		}

		// Execute the original task
		h.task(ctx)

		// After execution, reschedule if not stopped and runner is still open
		if !h.IsStopped() && runner != nil {
			// Check again before rescheduling
			if r, ok := runner.(*SequencedTaskRunner); ok && r.IsClosed() {
				return
			}
			// Reschedule itself
			runner.PostDelayedTaskWithTraits(h.createRepeatingTask(), h.interval, h.traits)
		}
	}
}

// PostRepeatingTask submits a task that repeats at a fixed interval
func (r *SequencedTaskRunner) PostRepeatingTask(task Task, interval time.Duration) RepeatingTaskHandle {
	return r.PostRepeatingTaskWithTraits(task, interval, DefaultTaskTraits())
}

// PostRepeatingTaskWithTraits submits a repeating task with specific traits
func (r *SequencedTaskRunner) PostRepeatingTaskWithTraits(
	task Task,
	interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle {
	return r.PostRepeatingTaskWithInitialDelay(task, 0, interval, traits)
}

// PostRepeatingTaskWithInitialDelay submits a repeating task with an initial delay
// The task will first execute after initialDelay, then repeat every interval.
func (r *SequencedTaskRunner) PostRepeatingTaskWithInitialDelay(
	task Task,
	initialDelay, interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle {
	handle := &repeatingTaskHandle{
		task:     task,
		interval: interval,
		traits:   traits,
	}

	// Create the self-scheduling repeating task
	repeatingTask := handle.createRepeatingTask()

	// Schedule first execution based on initialDelay
	if initialDelay > 0 {
		r.PostDelayedTaskWithTraits(repeatingTask, initialDelay, traits)
	} else {
		r.PostTaskWithTraits(repeatingTask, traits)
	}

	return handle
}

// =============================================================================
// Shutdown and Lifecycle Management
// =============================================================================

// Shutdown gracefully stops the runner by:
// 1. Marking it as closed (stops accepting new tasks)
// 2. Clearing all pending tasks in the queue
// 3. All repeating tasks will automatically stop on their next execution
//
// Note: This will not interrupt currently executing tasks.
func (r *SequencedTaskRunner) Shutdown() {
	// Mark as closed
	r.closed.Store(true)

	// Clear the queue
	r.mu.Lock()
	r.queue = NewFIFOTaskQueue()
	r.isRunning = false
	r.mu.Unlock()
}

// IsClosed returns true if the runner has been shut down.
func (r *SequencedTaskRunner) IsClosed() bool {
	return r.closed.Load()
}

// =============================================================================
// Task and Reply Pattern
// =============================================================================

// PostTaskAndReply executes task on this runner, then posts reply to replyRunner.
// If task panics, reply will not be executed.
func (r *SequencedTaskRunner) PostTaskAndReply(task Task, reply Task, replyRunner TaskRunner) {
	postTaskAndReplyInternal(r, task, reply, replyRunner, DefaultTaskTraits())
}

// PostTaskAndReplyWithTraits allows specifying different traits for task and reply.
// This is useful when task is background work (BestEffort) but reply is UI update (UserVisible).
func (r *SequencedTaskRunner) PostTaskAndReplyWithTraits(
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	postTaskAndReplyInternalWithTraits(r, task, taskTraits, reply, replyTraits, replyRunner)
}
