package core

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"
)

type SequencedTaskRunner struct {
	threadPool   ThreadPool
	queue        TaskQueue
	queueMu      sync.Mutex  // Protects queue operations
	runningCount int32       // atomic: 0 (idle) or 1 (running/scheduled)
	closed       atomic.Bool // indicates if the runner is closed
	shutdownChan chan struct{}
	shutdownOnce sync.Once

	// Metadata
	name       string
	metadata   map[string]any
	metadataMu sync.Mutex // Protects name and metadata
}

func NewSequencedTaskRunner(threadPool ThreadPool) *SequencedTaskRunner {
	return &SequencedTaskRunner{
		threadPool:   threadPool,
		queue:        NewFIFOTaskQueue(),
		shutdownChan: make(chan struct{}),
		metadata:     make(map[string]any),
	}
}

// =============================================================================
// Metadata Methods
// =============================================================================

// Name returns the name of the task runner
func (r *SequencedTaskRunner) Name() string {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	return r.name
}

// SetName sets the name of the task runner
func (r *SequencedTaskRunner) SetName(name string) {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	r.name = name
}

// Metadata returns the metadata associated with the task runner
func (r *SequencedTaskRunner) Metadata() map[string]any {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()

	// Return a copy to avoid race conditions
	result := make(map[string]any, len(r.metadata))
	maps.Copy(result, r.metadata)
	return result
}

// SetMetadata sets a metadata key-value pair
func (r *SequencedTaskRunner) SetMetadata(key string, value interface{}) {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	r.metadata[key] = value
}

// =============================================================================
// Core Task Execution
// =============================================================================

// ensureRunning ensures there is a runLoop running.
// If no runLoop is currently running or scheduled, it starts one.
// This method is safe to call concurrently from multiple goroutines.
func (r *SequencedTaskRunner) ensureRunning(traits TaskTraits) {
	// CAS: Only start runLoop if runningCount is 0 (idle)
	// If runningCount is already 1, a runLoop is running/scheduled, so we don't need to start another
	if atomic.CompareAndSwapInt32(&r.runningCount, 0, 1) {
		r.threadPool.PostInternal(r.runLoop, traits)
	}
}

// runLoop processes exactly one task, then either:
// - Posts itself again if there are more tasks (yields to scheduler between tasks)
// - Sets runningCount to 0 if no more tasks
//
// This design ensures:
// 1. Only one runLoop is active at a time (protected by runningCount CAS)
// 2. Each task gets a chance to be re-prioritized by the scheduler
// 3. High-priority tasks can interleave with sequences
func (r *SequencedTaskRunner) runLoop(ctx context.Context) {
	// Sanity check: runningCount should be 1 when runLoop executes
	if atomic.LoadInt32(&r.runningCount) != 1 {
		panic(fmt.Sprintf("SequencedTaskRunner: runLoop started with runningCount=%d (expected 1)",
			atomic.LoadInt32(&r.runningCount)))
	}

	runCtx := context.WithValue(ctx, taskRunnerKey, r)

	// 1. Fetch one task
	r.queueMu.Lock()
	item, ok := r.queue.Pop()
	r.queueMu.Unlock()

	if !ok {
		// Queue is empty, prepare to go idle
		atomic.StoreInt32(&r.runningCount, 0)

		// Double-check: a task might have been posted after Pop but before Store
		r.queueMu.Lock()
		hasMore := !r.queue.IsEmpty()
		var nextTraits TaskTraits
		if hasMore {
			nextTraits, _ = r.queue.PeekTraits()
		}
		r.queueMu.Unlock()

		if hasMore {
			// New task arrived, ensure a runLoop is running
			// This will CAS(0, 1) and start a new runLoop
			r.ensureRunning(nextTraits)
		}
		return
	}

	// 2. Execute the task
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				// Task panicked, but we continue processing
				// (repeating tasks and PostTaskAndReply have their own panic handling)
				_ = rec
			}
		}()
		item.Task(runCtx)
	}()

	// 3. Check if there are more tasks
	r.queueMu.Lock()
	hasMore := !r.queue.IsEmpty()
	var nextTraits TaskTraits
	if hasMore {
		nextTraits, _ = r.queue.PeekTraits()
	}
	r.queueMu.Unlock()

	if hasMore {
		// More tasks to process, directly post next runLoop
		// runningCount stays at 1 (we're passing the "running" state to the next runLoop)
		r.threadPool.PostInternal(r.runLoop, nextTraits)
	} else {
		// No more tasks, go idle
		atomic.StoreInt32(&r.runningCount, 0)

		// Double-check again: a task might have been posted during the check
		r.queueMu.Lock()
		hasMore = !r.queue.IsEmpty()
		if hasMore {
			nextTraits, _ = r.queue.PeekTraits()
		}
		r.queueMu.Unlock()

		if hasMore {
			r.ensureRunning(nextTraits)
		}
	}
}

// PostTask submits a task with default traits
func (r *SequencedTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

// PostTaskWithTraits submits a task with specified traits
func (r *SequencedTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	if r.closed.Load() {
		return
	}

	r.queueMu.Lock()
	r.queue.Push(task, traits)
	r.queueMu.Unlock()

	r.ensureRunning(traits)
}

// PostDelayedTask submits a task to execute after a delay
func (r *SequencedTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraits(task, delay, DefaultTaskTraits())
}

// PostDelayedTaskWithTraits submits a delayed task with specified traits
func (r *SequencedTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	r.threadPool.PostDelayedInternal(task, delay, traits, r)
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
// 4. Signaling all WaitShutdown() waiters
//
// Note: This method is non-blocking and can be safely called from within a task.
// Note: This will not interrupt currently executing tasks.
func (r *SequencedTaskRunner) Shutdown() {
	r.shutdownOnce.Do(func() {
		// Mark as closed
		r.closed.Store(true)
		// Close shutdown channel to signal waiters
		close(r.shutdownChan)

		// Clear the queue
		r.queueMu.Lock()
		r.queue = NewFIFOTaskQueue()
		r.queueMu.Unlock()
	})
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

// =============================================================================
// Synchronization Methods
// =============================================================================

// WaitIdle blocks until all currently queued tasks have completed execution.
// This is implemented by posting a barrier task and waiting for it to execute.
//
// Due to the sequential nature of SequencedTaskRunner, when the barrier task
// executes, all tasks posted before WaitIdle are guaranteed to have completed.
//
// Returns error if:
// - Context is cancelled or deadline exceeded
// - Runner is closed when WaitIdle is called
//
// Note: Tasks posted after WaitIdle is called are not waited for.
// Note: Repeating tasks will continue to repeat and are not waited for.
func (r *SequencedTaskRunner) WaitIdle(ctx context.Context) error {
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
// The callback will be executed on this runner's thread, after all tasks posted
// before FlushAsync have completed.
//
// Example:
//
//	runner.PostTask(task1)
//	runner.PostTask(task2)
//	runner.FlushAsync(func() {
//	    fmt.Println("task1 and task2 completed!")
//	})
func (r *SequencedTaskRunner) FlushAsync(callback func()) {
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
//	// Task shuts down the runner when condition is met
//	runner.PostTask(func(ctx context.Context) {
//	    if conditionMet() {
//	        me := GetCurrentTaskRunner(ctx)
//	        me.Shutdown()
//	    }
//	})
//
//	// Main thread waits for shutdown
//	runner.WaitShutdown(context.Background())
func (r *SequencedTaskRunner) WaitShutdown(ctx context.Context) error {
	select {
	case <-r.shutdownChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
