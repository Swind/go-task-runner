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

const (
	// maxAllowedConcurrency is the maximum allowed value for maxConcurrency parameter.
	// Values higher than this could lead to excessive goroutine creation and memory exhaustion.
	maxAllowedConcurrency = 10000
)

// ParallelTaskRunner executes up to maxConcurrency tasks simultaneously.
// Tasks are queued with priority support and executed as slots become available.
type ParallelTaskRunner struct {
	// Internal SingleThreadTaskRunner for serializing scheduling operations
	// Using SingleThreadTaskRunner ensures scheduling operations are never blocked
	// by thread pool congestion - the scheduler has its own dedicated goroutine.
	scheduler *SingleThreadTaskRunner

	threadPool     ThreadPool
	queue          TaskQueue
	maxConcurrency int
	runningCount   atomic.Int32
	closed         atomic.Bool
	shutdownChan   chan struct{}
	shutdownOnce   sync.Once

	// Barrier task tracking
	// barrierTaskIDs stores IDs of tasks that act as synchronization barriers
	// When tryScheduleInternal encounters a barrier task, it waits for all
	// currently running tasks to complete before executing the barrier callback.
	//
	// SAFETY CONTRACT: pendingBarrierTask and barrierTaskRunning (for flag updates)
	// are only accessed from tryScheduleInternal and postBarrierTaskInternal,
	// which are ALWAYS called from the internal scheduler's dedicated goroutine.
	// This provides implicit serialization without additional locks.
	// The scheduler's PostTask mechanism ensures the serialization guarantee.
	// The panic check at lines 303-307 enforces this contract.
	//
	// barrierTaskRunning uses atomic.Bool because it's also set from barrier
	// task callbacks (lines 426, 512) which run on thread pool goroutines.
	// barrierTaskIDs is protected by barrierMu as it's accessed from markAsBarrier
	// which may be called before tasks enter the scheduler context.
	pendingBarrierTask *TaskItem
	barrierTaskRunning atomic.Bool
	barrierTaskIDs     map[TaskID]bool
	barrierMu          sync.RWMutex

	// Metadata
	name       string
	metadata   map[string]any
	metadataMu sync.Mutex

	history executionHistory
}

// NewParallelTaskRunner creates a new ParallelTaskRunner with the specified concurrency limit.
// Panics if threadPool is nil or maxConcurrency is out of valid range [1, 10000].
func NewParallelTaskRunner(threadPool ThreadPool, maxConcurrency int) *ParallelTaskRunner {
	if threadPool == nil {
		panic("ParallelTaskRunner: threadPool must not be nil")
	}
	if maxConcurrency < 1 {
		panic("ParallelTaskRunner: maxConcurrency must be at least 1")
	}
	if maxConcurrency > maxAllowedConcurrency {
		panic(fmt.Sprintf("ParallelTaskRunner: maxConcurrency must not exceed %d", maxAllowedConcurrency))
	}

	// Create internal SingleThreadTaskRunner for serializing scheduling operations
	// SingleThreadTaskRunner has its own dedicated goroutine, ensuring scheduling
	// operations are never blocked by thread pool congestion.
	scheduler := NewSingleThreadTaskRunner()
	scheduler.SetName("parallel-scheduler")

	r := &ParallelTaskRunner{
		scheduler:      scheduler,
		threadPool:     threadPool,
		queue:          NewFIFOTaskQueue(),
		maxConcurrency: maxConcurrency,
		shutdownChan:   make(chan struct{}),
		barrierTaskIDs: make(map[TaskID]bool),
		metadata:       make(map[string]any),
		history:        newExecutionHistory(defaultTaskHistoryCapacity),
	}
	return r
}

// MaxConcurrency returns the maximum number of concurrent tasks.
func (r *ParallelTaskRunner) MaxConcurrency() int {
	return r.maxConcurrency
}

// PendingTaskCount returns the number of queued tasks waiting to run.
func (r *ParallelTaskRunner) PendingTaskCount() int {
	return r.queue.Len()
}

// RunningTaskCount returns the number of currently executing tasks.
func (r *ParallelTaskRunner) RunningTaskCount() int {
	return int(r.runningCount.Load())
}

// Stats returns current observability data for this runner.
func (r *ParallelTaskRunner) Stats() RunnerStats {
	stats := RunnerStats{
		Name:           r.observabilityName(),
		Type:           "parallel",
		Pending:        r.PendingTaskCount(),
		Running:        r.RunningTaskCount(),
		Closed:         r.IsClosed(),
		BarrierPending: r.barrierTaskRunning.Load(),
	}
	if last, ok := r.history.Last(); ok {
		stats.LastTaskName = last.Name
		stats.LastTaskAt = last.FinishedAt
	}
	return stats
}

// RecentTasks returns completed task execution records in newest-first order.
func (r *ParallelTaskRunner) RecentTasks(limit int) []TaskExecutionRecord {
	return r.history.Recent(limit)
}

func (r *ParallelTaskRunner) observabilityName() string {
	name := r.Name()
	if name == "" {
		return "parallel"
	}
	return name
}

func (r *ParallelTaskRunner) recordTaskExecution(record TaskExecutionRecord) {
	r.history.Add(record)
}

func (r *ParallelTaskRunner) emitQueueDepth(depth int) {
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

// markAsBarrier marks a task ID as a barrier task.
// Barrier tasks will wait for all running tasks to complete before executing.
func (r *ParallelTaskRunner) markAsBarrier(id TaskID) {
	r.barrierMu.Lock()
	defer r.barrierMu.Unlock()
	r.barrierTaskIDs[id] = true
}

// isBarrierTaskID checks if a task ID is marked as a barrier task.
func (r *ParallelTaskRunner) isBarrierTaskID(id TaskID) bool {
	r.barrierMu.RLock()
	defer r.barrierMu.RUnlock()
	return r.barrierTaskIDs[id]
}

// removeBarrier removes a task ID from the barrier set.
// This should be called after a barrier task has been executed to clean up.
func (r *ParallelTaskRunner) removeBarrier(id TaskID) {
	r.barrierMu.Lock()
	defer r.barrierMu.Unlock()
	delete(r.barrierTaskIDs, id)
}

// IsClosed returns true if the runner has been shut down.
func (r *ParallelTaskRunner) IsClosed() bool {
	return r.closed.Load()
}

// Name returns the name of the task runner
func (r *ParallelTaskRunner) Name() string {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	return r.name
}

// SetName sets the name of the task runner
func (r *ParallelTaskRunner) SetName(name string) {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	r.name = name
}

// Metadata returns the metadata associated with the task runner
func (r *ParallelTaskRunner) Metadata() map[string]any {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	// Return a copy to prevent external modification
	metadata := make(map[string]any, len(r.metadata))
	maps.Copy(metadata, r.metadata)
	return metadata
}

// SetMetadata sets a metadata key-value pair
func (r *ParallelTaskRunner) SetMetadata(key string, value any) {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()
	r.metadata[key] = value
}

// GetThreadPool returns the underlying ThreadPool used by this runner
func (r *ParallelTaskRunner) GetThreadPool() ThreadPool {
	return r.threadPool
}

// PostTask submits a task with default traits.
func (r *ParallelTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

// PostTaskWithTraits submits a task with specified traits.
func (r *ParallelTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	r.PostTaskWithTraitsNamed("", task, traits)
}

// PostTaskNamed submits a task with a caller-provided display name.
func (r *ParallelTaskRunner) PostTaskNamed(name string, task Task) {
	r.PostTaskWithTraitsNamed(name, task, DefaultTaskTraits())
}

// PostTaskWithTraitsNamed submits a named task with specified traits.
func (r *ParallelTaskRunner) PostTaskWithTraitsNamed(name string, task Task, traits TaskTraits) {
	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "parallel", r.recordTaskExecution)

	// Submit scheduling operation to internal SingleThreadTaskRunner
	// This guarantees all queue/scheduling operations run sequentially
	// on the scheduler's dedicated goroutine.
	r.scheduler.PostTask(func(ctx context.Context) {
		if r.closed.Load() {
			return
		}
		// Queue and schedule (executed serially on scheduler)
		r.queue.Push(wrapped, traits)
		r.emitQueueDepth(r.queue.Len())
		r.tryScheduleInternal(ctx)
	})
}

// PostDelayedTask submits a task to execute after a delay.
func (r *ParallelTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraits(task, delay, DefaultTaskTraits())
}

// PostDelayedTaskWithTraits submits a delayed task with specified traits.
func (r *ParallelTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	r.PostDelayedTaskWithTraitsNamed("", task, delay, traits)
}

// PostDelayedTaskNamed submits a delayed named task.
func (r *ParallelTaskRunner) PostDelayedTaskNamed(name string, task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraitsNamed(name, task, delay, DefaultTaskTraits())
}

// PostDelayedTaskWithTraitsNamed submits a delayed named task with specified traits.
func (r *ParallelTaskRunner) PostDelayedTaskWithTraitsNamed(name string, task Task, delay time.Duration, traits TaskTraits) {
	// Check closed flag before submitting to thread pool
	if r.closed.Load() {
		return
	}
	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "parallel", r.recordTaskExecution)
	// Delegate to thread pool's delayed task mechanism
	// When delay expires, the task will be posted back to this runner via PostTask
	r.threadPool.PostDelayedInternal(wrapped, delay, traits, r)
}

// =============================================================================
// Repeating Task Implementation
// =============================================================================

// parallelRepeatingTaskHandle implements RepeatingTaskHandle interface for ParallelTaskRunner
type parallelRepeatingTaskHandle struct {
	task     Task
	interval time.Duration
	traits   TaskTraits
	stopped  atomic.Bool
}

func (h *parallelRepeatingTaskHandle) Stop() {
	h.stopped.Store(true)
}

func (h *parallelRepeatingTaskHandle) IsStopped() bool {
	return h.stopped.Load()
}

// createRepeatingTask creates a self-scheduling repeating task
func (h *parallelRepeatingTaskHandle) createRepeatingTask() Task {
	return func(ctx context.Context) {
		// Get the current runner from context
		runner := GetCurrentTaskRunner(ctx)

		// Check if runner is closed (automatic cleanup)
		if r, ok := runner.(*ParallelTaskRunner); ok && r.IsClosed() {
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
			if r, ok := runner.(*ParallelTaskRunner); ok && r.IsClosed() {
				return
			}
			// Reschedule itself
			runner.PostDelayedTaskWithTraits(h.createRepeatingTask(), h.interval, h.traits)
		}
	}
}

// PostRepeatingTask submits a repeating task
func (r *ParallelTaskRunner) PostRepeatingTask(task Task, interval time.Duration) RepeatingTaskHandle {
	return r.PostRepeatingTaskWithTraits(task, interval, DefaultTaskTraits())
}

// PostRepeatingTaskWithTraits submits a repeating task with specific traits
func (r *ParallelTaskRunner) PostRepeatingTaskWithTraits(
	task Task,
	interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle {
	return r.PostRepeatingTaskWithInitialDelay(task, 0, interval, traits)
}

// PostRepeatingTaskWithInitialDelay submits a repeating task with an initial delay
func (r *ParallelTaskRunner) PostRepeatingTaskWithInitialDelay(
	task Task,
	initialDelay, interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle {
	handle := &parallelRepeatingTaskHandle{
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
// Task and Reply Pattern
// =============================================================================

// PostTaskAndReply executes task on this runner, then posts reply to replyRunner.
func (r *ParallelTaskRunner) PostTaskAndReply(task Task, reply Task, replyRunner TaskRunner) {
	postTaskAndReplyInternal(r, task, reply, replyRunner, DefaultTaskTraits())
}

// PostTaskAndReplyWithTraits allows specifying different traits for task and reply.
func (r *ParallelTaskRunner) PostTaskAndReplyWithTraits(
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	postTaskAndReplyInternalWithTraits(r, task, taskTraits, reply, replyTraits, replyRunner)
}

func (r *ParallelTaskRunner) postBarrierTaskInternal(barrierTaskItem *TaskItem) {
	r.removeBarrier(barrierTaskItem.ID)
	r.pendingBarrierTask = nil
	r.barrierTaskRunning.Store(true)
	r.runningCount.Add(1)
	r.threadPool.PostInternal(r.runLoop(barrierTaskItem.Task), barrierTaskItem.Traits)
}

// tryScheduleInternal attempts to start tasks from the queue if slots are available.
// IMPORTANT: This must only be called from the internal scheduler (serial execution).
// The scheduler's dedicated goroutine ensures this is never blocked by thread pool congestion.
func (r *ParallelTaskRunner) tryScheduleInternal(ctx context.Context) {
	// tryScheduleInternal must be called from the internal scheduler
	currentTaskRunner := GetCurrentTaskRunner(ctx)
	if currentTaskRunner != r.scheduler {
		// Wrong context - must be called from internal scheduler
		panic("ParallelTaskRunner: tryScheduleInternal must be called from internal scheduler")
	}

	if r.barrierTaskRunning.Load() {
		// Currently executing a barrier task - wait for it to complete
		return
	}

	// If there is a pending barrier task and no running tasks, execute it now
	if r.pendingBarrierTask != nil {
		if r.runningCount.Load() == 0 {
			r.postBarrierTaskInternal(r.pendingBarrierTask)
			return
		} else {
			// Still waiting for running tasks to complete
			return
		}
	}

	// This runs on scheduler which guarantees serial execution
	for r.runningCount.Load() < int32(r.maxConcurrency) {
		if r.queue.IsEmpty() && r.pendingBarrierTask == nil {
			return
		}

		item, ok := r.queue.Pop()
		if !ok {
			return
		}
		r.emitQueueDepth(r.queue.Len())

		// If this is a barrier task, we need to wait for all running tasks to complete
		// before executing it. Store it in pendingBarrierTask for later execution.
		if r.isBarrierTaskID(item.ID) {
			r.removeBarrier(item.ID)
			// If there are no running tasks, post to thread pool immediately
			if r.runningCount.Load() == 0 {
				r.runningCount.Add(1)
				r.threadPool.PostInternal(r.runLoop(item.Task), item.Traits)
			} else {
				// Else store as pending barrier task, it will be pushed to thread pool
				// when all running tasks complete.
				// This ensures true barrier semantics.
				// The pending barrier task will be executed in the next scheduling cycle.
				r.pendingBarrierTask = &item
			}

			return
		}

		// Normal task - submit to thread pool
		r.runningCount.Add(1)
		r.threadPool.PostInternal(r.runLoop(item.Task), item.Traits)
	}
}

// runLoop wraps a task with cleanup logic.
func (r *ParallelTaskRunner) runLoop(task Task) Task {
	return func(ctx context.Context) {
		defer r.onTaskComplete()

		// Inject current runner into context
		runCtx := context.WithValue(ctx, taskRunnerKey, r)

		// Execute with panic recovery
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					// Try to get panic handler from thread pool
					if tp, ok := r.threadPool.(interface{ GetScheduler() *TaskScheduler }); ok {
						if handler := tp.GetScheduler().GetPanicHandler(); handler != nil {
							handler.HandlePanic(runCtx, r.Name(), -1, rec, debug.Stack())
						}
						if metrics := tp.GetScheduler().GetMetrics(); metrics != nil {
							metrics.RecordTaskPanic(r.Name(), rec)
						}
					} else {
						// Fallback to basic logging
						log.Printf("[ParallelTaskRunner] Task panic recovered: %v\nStack trace:\n%s",
							rec, debug.Stack())
					}
				}
			}()
			task(runCtx)
		}()
	}
}

// onTaskComplete is called when a task finishes.
func (r *ParallelTaskRunner) onTaskComplete() {
	r.runningCount.Add(-1)

	// Trigger next scheduling via internal SingleThreadTaskRunner
	r.scheduler.PostTask(func(ctx context.Context) {
		r.tryScheduleInternal(ctx)
	})
}

// WaitIdle blocks until all currently queued tasks have completed execution.
//
// This method waits until both the queue is empty AND no tasks are currently
// executing (runningCount == 0).
//
// Returns error if:
// - Context is cancelled or deadline exceeded
// - Runner is closed when WaitIdle is called
//
// Note: Tasks posted after WaitIdle is called are not waited for.
func (r *ParallelTaskRunner) WaitIdle(ctx context.Context) error {
	if r.IsClosed() {
		return fmt.Errorf("runner is closed")
	}

	done := make(chan struct{})

	// Post a barrier task that will signal completion when all prior tasks are done
	r.scheduler.PostTask(func(ctx context.Context) {
		if r.IsClosed() {
			return
		}

		// Create a barrier task that signals completion
		barrierTask := func(ctx context.Context) {
			defer func() {
				r.scheduler.PostTask(func(ctx context.Context) {
					r.barrierTaskRunning.Store(false)
				})
			}()
			close(done)
		}

		// Push the barrier task with its special ID
		// Use UserBlocking priority as barriers represent synchronization points
		barrierID := r.queue.PushWithID(barrierTask, TaskTraits{Priority: TaskPriorityUserBlocking})
		r.markAsBarrier(barrierID)
		r.emitQueueDepth(r.queue.Len())
		r.tryScheduleInternal(ctx)
	})

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-r.shutdownChan:
		return fmt.Errorf("runner shutdown during WaitIdle")
	}
}

// Shutdown marks the runner as closed and clears all pending tasks.
// This method is non-blocking and can be safely called from within a task.
//
// Shutdown does NOT interrupt currently executing tasks - they will run to completion.
// However, no new tasks will be started from the queue after Shutdown is called.
func (r *ParallelTaskRunner) Shutdown() {
	r.shutdownOnce.Do(func() {
		// Mark as closed first to stop accepting new tasks
		r.closed.Store(true)

		// Clear the queue
		r.queue.Clear()
		r.emitQueueDepth(0)

		close(r.shutdownChan)

		// Shutdown the internal scheduler
		r.scheduler.Shutdown()
	})
}

// WaitShutdown blocks until Shutdown() is called on this runner.
// Returns error if context is cancelled.
func (r *ParallelTaskRunner) WaitShutdown(ctx context.Context) error {
	select {
	case <-r.shutdownChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FlushAsync posts a barrier task that executes callback when all prior tasks complete.
// This is a non-blocking alternative to WaitIdle.
//
// The callback will execute after all tasks posted before FlushAsync() have completed.
// Tasks posted after FlushAsync() will not run before the callback.
//
// Implementation note: This posts a special barrier task to the queue. When the
// scheduler encounters the barrier, it waits for all currently running tasks to
// complete before executing the callback. This provides true barrier semantics.
//
// Example:
//
//	runner.PostTask(task1)
//	runner.PostTask(task2)
//	runner.FlushAsync(func() {
//	    // This runs after task1 and task2 complete
//	    fmt.Println("task1 and task2 completed!")
//	})
//	runner.PostTask(task3)  // Will NOT run before the callback
func (r *ParallelTaskRunner) FlushAsync(callback func()) {
	if callback == nil {
		return
	}
	// Post the barrier task to the scheduler
	// It will be queued after all previously posted tasks
	r.scheduler.PostTask(func(ctx context.Context) {
		if r.IsClosed() {
			return
		}
		// Create a barrier task (closure that captures the callback)

		// Push the barrier task with its special ID
		// Use UserBlocking priority as barriers represent synchronization points
		barrierID := r.queue.PushWithID(func(ctx context.Context) {
			defer func() {
				r.scheduler.PostTask(func(ctx context.Context) {
					r.barrierTaskRunning.Store(false)
				})
			}()
			callback()
		}, TaskTraits{Priority: TaskPriorityUserBlocking})
		r.markAsBarrier(barrierID)
		r.emitQueueDepth(r.queue.Len())

		r.tryScheduleInternal(ctx)
	})
}
