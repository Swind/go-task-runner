package core

import (
	"context"
	"fmt"
	"log"
	"maps"
	"runtime/debug"
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

	history executionHistory
}

func NewSequencedTaskRunner(threadPool ThreadPool) *SequencedTaskRunner {
	return &SequencedTaskRunner{
		threadPool:   threadPool,
		queue:        NewFIFOTaskQueue(),
		shutdownChan: make(chan struct{}),
		metadata:     make(map[string]any),
		history:      newExecutionHistory(defaultTaskHistoryCapacity),
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
func (r *SequencedTaskRunner) SetMetadata(key string, value any) {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	r.metadata[key] = value
}

// GetThreadPool returns the underlying ThreadPool used by this runner
func (r *SequencedTaskRunner) GetThreadPool() ThreadPool {
	return r.threadPool
}

// PendingTaskCount returns the number of queued tasks waiting to run.
func (r *SequencedTaskRunner) PendingTaskCount() int {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	return r.queue.Len()
}

// RunningTaskCount returns 1 if runLoop is running/scheduled, otherwise 0.
func (r *SequencedTaskRunner) RunningTaskCount() int {
	return int(atomic.LoadInt32(&r.runningCount))
}

// Stats returns current observability data for this runner.
func (r *SequencedTaskRunner) Stats() RunnerStats {
	name := r.observabilityName()
	stats := RunnerStats{
		Name:    name,
		Type:    "sequenced",
		Pending: r.PendingTaskCount(),
		Running: r.RunningTaskCount(),
		Closed:  r.IsClosed(),
	}
	if last, ok := r.history.Last(); ok {
		stats.LastTaskName = last.Name
		stats.LastTaskAt = last.FinishedAt
	}
	return stats
}

// RecentTasks returns completed task execution records in newest-first order.
func (r *SequencedTaskRunner) RecentTasks(limit int) []TaskExecutionRecord {
	return r.history.Recent(limit)
}

func (r *SequencedTaskRunner) observabilityName() string {
	name := r.Name()
	if name == "" {
		return "sequenced"
	}
	return name
}

func (r *SequencedTaskRunner) recordTaskExecution(record TaskExecutionRecord) {
	r.history.Add(record)
}

func (r *SequencedTaskRunner) emitQueueDepth(depth int) {
	type schedulerGetter interface {
		GetScheduler() *TaskScheduler
	}
	if tp, ok := r.threadPool.(schedulerGetter); ok {
		if scheduler := tp.GetScheduler(); scheduler != nil {
			if metrics := scheduler.GetMetrics(); metrics != nil {
				metrics.RecordQueueDepth(r.observabilityName(), depth)
			}
		}
	}
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

// scheduleNextIfNeeded checks if there are more tasks and schedules a runLoop if needed.
// This is used after setting runningCount to 0 to handle the race condition where
// a new task arrives between checking the queue and setting runningCount to 0.
//
// Returns true if a runLoop was scheduled, false otherwise.
func (r *SequencedTaskRunner) scheduleNextIfNeeded() bool {
	r.queueMu.Lock()
	hasMore := !r.queue.IsEmpty()
	var nextTraits TaskTraits
	if hasMore {
		nextTraits, _ = r.queue.PeekTraits()
	}
	r.queueMu.Unlock()

	if hasMore {
		r.ensureRunning(nextTraits)
		return true
	}
	return false
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
	depth := r.queue.Len()
	r.queueMu.Unlock()
	if ok {
		r.emitQueueDepth(depth)
	}

	if !ok {
		// Queue is empty, go idle
		atomic.StoreInt32(&r.runningCount, 0)
		// Double-check: a task might have been posted after Pop but before Store
		r.scheduleNextIfNeeded()
		return
	}

	// 2. Execute the task
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				// Get panic handler from thread pool if available
				if tp, ok := r.threadPool.(interface{ GetScheduler() *TaskScheduler }); ok {
					if handler := tp.GetScheduler().GetPanicHandler(); handler != nil {
						handler.HandlePanic(runCtx, r.Name(), -1, rec, debug.Stack())
					}
					if metrics := tp.GetScheduler().GetMetrics(); metrics != nil {
						metrics.RecordTaskPanic(r.Name(), rec)
					}
				} else {
					// Fallback to basic logging
					log.Printf("[SequencedTaskRunner] Task panic recovered: %v\nStack trace:\n%s",
						rec, debug.Stack())
				}
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
		// Double-check: a task might have been posted during the check
		r.scheduleNextIfNeeded()
	}
}

// PostTask submits a task with default traits
func (r *SequencedTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

// PostTaskWithTraits submits a task with specified traits
func (r *SequencedTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	r.PostTaskWithTraitsNamed("", task, traits)
}

// PostTaskNamed submits a task with a caller-provided display name.
func (r *SequencedTaskRunner) PostTaskNamed(name string, task Task) {
	r.PostTaskWithTraitsNamed(name, task, DefaultTaskTraits())
}

// PostTaskWithTraitsNamed submits a named task with specified traits.
func (r *SequencedTaskRunner) PostTaskWithTraitsNamed(name string, task Task, traits TaskTraits) {
	if r.closed.Load() {
		return
	}

	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "sequenced", r.recordTaskExecution)

	r.queueMu.Lock()
	r.queue.Push(wrapped, traits)
	depth := r.queue.Len()
	r.queueMu.Unlock()
	r.emitQueueDepth(depth)

	r.ensureRunning(traits)
}

// PostDelayedTask submits a task to execute after a delay
func (r *SequencedTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraits(task, delay, DefaultTaskTraits())
}

// PostDelayedTaskWithTraits submits a delayed task with specified traits
func (r *SequencedTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	r.PostDelayedTaskWithTraitsNamed("", task, delay, traits)
}

// PostDelayedTaskNamed submits a delayed named task.
func (r *SequencedTaskRunner) PostDelayedTaskNamed(name string, task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraitsNamed(name, task, delay, DefaultTaskTraits())
}

// PostDelayedTaskWithTraitsNamed submits a delayed named task with specified traits.
func (r *SequencedTaskRunner) PostDelayedTaskWithTraitsNamed(name string, task Task, delay time.Duration, traits TaskTraits) {
	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "sequenced", r.recordTaskExecution)
	r.threadPool.PostDelayedInternal(wrapped, delay, traits, r)
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
		r.emitQueueDepth(0)
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
