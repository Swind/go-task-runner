# API Reference

> Auto-generated from source. Do not edit manually.

## Package `taskrunner`

Package taskrunner provides a Chromium-inspired task scheduling architecture for Go.

This library implements a threading model where developers post tasks to virtual threads
(TaskRunners) rather than managing goroutines directly. The core design is inspired by
Chromium's Threading and Tasks system.

# Quick Start

Initialize the global thread pool at application startup:

	taskrunner.InitGlobalThreadPool(4) // 4 workers
	defer taskrunner.ShutdownGlobalThreadPool()

Create a SequencedTaskRunner for sequential task execution:

	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
	runner.PostTask(func(ctx context.Context) {
		// Your code here - guaranteed sequential execution
	})

# Key Concepts

TaskRunner: Interface for posting tasks. Tasks posted to a SequencedTaskRunner
execute sequentially, eliminating the need for locks on resources owned by that runner.

TaskTraits: Describes task attributes including priority (BestEffort, UserVisible, UserBlocking).
Priority determines when the sequence gets scheduled, not the order within a sequence.

GoroutineThreadPool: The execution engine managing worker goroutines that pull
and execute tasks from the scheduler.

# Thread Safety

SequencedTaskRunner provides strict FIFO execution guarantees with runtime assertions.
Tasks within a sequence never run concurrently, allowing lock-free programming
for resources owned by that sequence.

# Example

	import (
		"context"
		taskrunner "github.com/Swind/go-task-runner"
	)

	func main() {
		taskrunner.InitGlobalThreadPool(4)
		defer taskrunner.ShutdownGlobalThreadPool()

		runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

		// Tasks execute sequentially
		runner.PostTask(func(ctx context.Context) {
			println("Task 1")
		})
		runner.PostTask(func(ctx context.Context) {
			println("Task 2")
		})

		// Delayed task
		runner.PostDelayedTask(func(ctx context.Context) {
			println("Task 3 - delayed")
		}, 1*time.Second)
	}

For more details, see https://github.com/Swind/go-task-runner

### Structs

#### `GoroutineThreadPool`

GoroutineThreadPool manages a set of worker goroutines
Responsible for pulling tasks from WorkSource and executing them

```go
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
```

**Exported methods:**

- `ActiveTaskCount` func (tg *GoroutineThreadPool) ActiveTaskCount() int
- `DelayedTaskCount` func (tg *GoroutineThreadPool) DelayedTaskCount() int
- `GetScheduler` func (tg *GoroutineThreadPool) GetScheduler() *core.TaskScheduler
  > GetScheduler returns the underlying TaskScheduler for advanced configuration

- `ID` func (tg *GoroutineThreadPool) ID() string
  > ID returns the ID of the thread pool

- `IsRunning` func (tg *GoroutineThreadPool) IsRunning() bool
  > IsRunning returns whether the thread pool is running

- `Join` func (tg *GoroutineThreadPool) Join()
  > Join waits for all worker goroutines to finish

- `PostDelayedInternal` func (tg *GoroutineThreadPool) PostDelayedInternal(task core.Task, delay time.Duration, traits core.TaskTraits, target core.TaskRunner)
- `PostInternal` func (tg *GoroutineThreadPool) PostInternal(task core.Task, traits core.TaskTraits)
- `QueuedTaskCount` func (tg *GoroutineThreadPool) QueuedTaskCount() int
- `Start` func (tg *GoroutineThreadPool) Start(ctx context.Context)
  > Start starts all worker goroutines

- `Stats` func (tg *GoroutineThreadPool) Stats() core.PoolStats
  > Stats returns current observability data for this pool.

- `Stop` func (tg *GoroutineThreadPool) Stop()
  > Stop stops the thread pool

- `StopGraceful` func (tg *GoroutineThreadPool) StopGraceful(timeout time.Duration) error
  > StopGraceful stops the thread pool gracefully, waiting for queued tasks to complete
  > Returns error if timeout is exceeded before tasks complete

- `WorkerCount` func (tg *GoroutineThreadPool) WorkerCount() int
  > WorkerCount returns the number of workers

### Types

#### `ParallelTaskRunner`

ParallelTaskRunner executes up to maxConcurrency tasks simultaneously

```go
type ParallelTaskRunner = core.ParallelTaskRunner
```

#### `PoolStats`

```go
type PoolStats = core.PoolStats
```

#### `RepeatingTaskHandle`

RepeatingTaskHandle controls the lifecycle of a repeating task

```go
type RepeatingTaskHandle = core.RepeatingTaskHandle
```

#### `ReplyWithResult`

```go
type ReplyWithResult[T any] = core.ReplyWithResult[T]
```

#### `RunnerStats`

Observability stats types

```go
type RunnerStats = core.RunnerStats
```

#### `SequencedTaskRunner`

SequencedTaskRunner ensures sequential execution of tasks

```go
type SequencedTaskRunner = core.SequencedTaskRunner
```

#### `SingleThreadTaskRunner`

SingleThreadTaskRunner ensures all tasks execute on the same dedicated goroutine

```go
type SingleThreadTaskRunner = core.SingleThreadTaskRunner
```

#### `Task`

Task is the unit of work (Closure)

```go
type Task = core.Task
```

#### `TaskID`

TaskID is a unique identifier for tasks

```go
type TaskID = core.TaskID
```

#### `TaskPriority`

TaskPriority defines the priority levels for tasks

```go
type TaskPriority = core.TaskPriority
```

#### `TaskRunner`

TaskRunner is the interface for posting tasks

```go
type TaskRunner = core.TaskRunner
```

#### `TaskTraits`

TaskTraits defines task attributes (priority, blocking behavior, etc.)

```go
type TaskTraits = core.TaskTraits
```

#### `TaskWithResult`

TaskWithResult and ReplyWithResult for generic PostTaskAndReply pattern

```go
type TaskWithResult[T any] = core.TaskWithResult[T]
```

#### `ThreadPool`

ThreadPool is re-exported for type compatibility

```go
type ThreadPool = core.ThreadPool
```

### Constructors

#### `CreateTaskRunner`

CreateTaskRunner creates a new SequencedTaskRunner using the global thread pool.
This is the recommended way to get a new TaskRunner.

```go
func CreateTaskRunner(traits TaskTraits) *SequencedTaskRunner
```

#### `NewGoroutineThreadPool`

NewGoroutineThreadPool creates a new GoroutineThreadPool

```go
func NewGoroutineThreadPool(id string, workers int) *GoroutineThreadPool
```

#### `NewGoroutineThreadPoolWithConfig`

NewGoroutineThreadPoolWithConfig creates a new GoroutineThreadPool with custom config

```go
func NewGoroutineThreadPoolWithConfig(id string, workers int, config *core.TaskSchedulerConfig) *GoroutineThreadPool
```

#### `NewParallelTaskRunner`

NewParallelTaskRunner creates a new ParallelTaskRunner with the specified concurrency limit.
This runner executes up to maxConcurrency tasks simultaneously.
Panics if maxConcurrency is less than 1.

```go
func NewParallelTaskRunner(threadPool ThreadPool, maxConcurrency int) *ParallelTaskRunner
```

#### `NewPriorityGoroutineThreadPool`

```go
func NewPriorityGoroutineThreadPool(id string, workers int) *GoroutineThreadPool
```

#### `NewPriorityGoroutineThreadPoolWithConfig`

NewPriorityGoroutineThreadPoolWithConfig creates a new priority-based GoroutineThreadPool with custom config

```go
func NewPriorityGoroutineThreadPoolWithConfig(id string, workers int, config *core.TaskSchedulerConfig) *GoroutineThreadPool
```

#### `NewSequencedTaskRunner`

NewSequencedTaskRunner creates a new SequencedTaskRunner with the given thread pool.
This is re-exported for advanced users who want to create runners with custom pools.

```go
func NewSequencedTaskRunner(pool ThreadPool) *SequencedTaskRunner
```

#### `NewSingleThreadTaskRunner`

NewSingleThreadTaskRunner creates a new SingleThreadTaskRunner with a dedicated goroutine.
Use this for blocking IO operations, CGO calls with thread-local storage, or UI thread simulation.

```go
func NewSingleThreadTaskRunner() *SingleThreadTaskRunner
```

### Functions

#### `GetGlobalThreadPool`

GetGlobalThreadPool returns the global thread pool instance.
It panics if InitGlobalThreadPool has not been called.

```go
func GetGlobalThreadPool() *GoroutineThreadPool
```

#### `GlobalThreadPool`

GlobalThreadPool returns the global thread pool instance as a ThreadPool interface.
This is a convenience wrapper for use with ParallelTaskRunner.
It panics if InitGlobalThreadPool has not been called.

```go
func GlobalThreadPool() ThreadPool
```

#### `InitGlobalThreadPool`

InitGlobalThreadPool initializes the global thread pool with specified number of workers.
It starts the pool immediately.

```go
func InitGlobalThreadPool(workers int)
```

#### `PostDelayedTaskAndReplyWithResult`

```go
func PostDelayedTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	delay time.Duration,
	reply ReplyWithResult[T],
	replyRunner TaskRunner,
)
```

#### `PostDelayedTaskAndReplyWithResultAndTraits`

```go
func PostDelayedTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	delay time.Duration,
	taskTraits TaskTraits,
	reply ReplyWithResult[T],
	replyTraits TaskTraits,
	replyRunner TaskRunner,
)
```

#### `PostTaskAndReplyWithResult`

```go
func PostTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	reply ReplyWithResult[T],
	replyRunner TaskRunner,
)
```

#### `PostTaskAndReplyWithResultAndTraits`

```go
func PostTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	taskTraits TaskTraits,
	reply ReplyWithResult[T],
	replyTraits TaskTraits,
	replyRunner TaskRunner,
)
```

#### `ShutdownGlobalThreadPool`

ShutdownGlobalThreadPool stops the global thread pool.

```go
func ShutdownGlobalThreadPool()
```

### Constants

- `TaskPriorityBestEffort`, `TaskPriorityUserVisible`, `TaskPriorityUserBlocking` = core.TaskPriorityBestEffort = core.TaskPriorityUserVisible = core.TaskPriorityUserBlocking
  > Priority constants

### Variables

- `DefaultTaskTraits`, `TraitsUserBlocking`, `TraitsBestEffort`, `TraitsUserVisible` = core.DefaultTaskTraits = core.TraitsUserBlocking = core.TraitsBestEffort = core.TraitsUserVisible
  > Convenience functions for creating TaskTraits

- `GetCurrentTaskRunner` = core.GetCurrentTaskRunner
  > GetCurrentTaskRunner retrieves the current TaskRunner from context

## Package `core`

### Interfaces

#### `Metrics`

Metrics defines the interface for collecting task execution metrics.
Implementations can send metrics to monitoring systems (Prometheus, StatsD, etc.).

All methods are optional; implementations should handle nil receivers gracefully.
Methods should be non-blocking and fast to avoid impacting task execution performance.

```go
type Metrics interface {
	// RecordTaskDuration records how long a task took to execute.
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - priority: The task priority
	// - duration: How long the task took to execute
	RecordTaskDuration(runnerName string, priority TaskPriority, duration time.Duration)

	// RecordTaskPanic records that a task panicked during execution.
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - panicInfo: The panic value recovered from the task
	RecordTaskPanic(runnerName string, panicInfo any)

	// RecordQueueDepth records the current queue depth.
	// This can be called periodically to track queue growth/shrinkage.
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - depth: The current number of tasks in the queue
	RecordQueueDepth(runnerName string, depth int)

	// RecordTaskRejected records that a task was rejected (e.g., during shutdown).
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - reason: Why the task was rejected
	RecordTaskRejected(runnerName string, reason string)
}
```

#### `PanicHandler`

PanicHandler is called when a task panics during execution.
This allows custom panic handling, logging, and recovery strategies.

Implementations should be thread-safe as they may be called concurrently.

```go
type PanicHandler interface {
	// HandlePanic is called when a task panics.
	//
	// Parameters:
	// - ctx: The context from the panicked task (may contain task runner info)
	// - runnerName: The name of the task runner where the panic occurred
	// - workerID: The ID of the worker (for thread pool workers, -1 for single-threaded runners)
	// - panicInfo: The panic value recovered from the task
	// - stackTrace: The stack trace at the time of panic
	HandlePanic(ctx context.Context, runnerName string, workerID int, panicInfo any, stackTrace []byte)
}
```

#### `RejectedTaskHandler`

RejectedTaskHandler is called when a task is rejected by the scheduler.
This can happen when:
- The scheduler is shutting down
- The signal channel is full (backpressure)
- The task queue is full (if bounded queues are implemented in the future)

Implementations should be thread-safe as they may be called concurrently.

```go
type RejectedTaskHandler interface {
	// HandleRejectedTask is called when a task is rejected.
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - reason: Why the task was rejected (e.g., "shutdown", "backpressure")
	HandleRejectedTask(runnerName string, reason string)
}
```

#### `RepeatingTaskHandle`

RepeatingTaskHandle controls the lifecycle of a repeating task.

```go
type RepeatingTaskHandle interface {
	// Stop stops the repeating task. It will not interrupt a currently executing task,
	// but will prevent future executions from being scheduled.
	Stop()

	// IsStopped returns true if the task has been stopped.
	IsStopped() bool
}
```

#### `TaskQueue`

TaskQueue defines the interface for different queue implementations

```go
type TaskQueue interface {
	Push(t Task, traits TaskTraits)
	PushWithID(t Task, traits TaskTraits) TaskID // Push with specific TaskID
	Pop() (TaskItem, bool)
	PopUpTo(max int) []TaskItem
	PeekTraits() (TaskTraits, bool)
	Len() int
	IsEmpty() bool
	MaybeCompact()
	Clear() // Clear all tasks from the queue
}
```

#### `TaskRunner`

```go
type TaskRunner interface {
	PostTask(task Task)
	PostTaskWithTraits(task Task, traits TaskTraits)
	PostDelayedTask(task Task, delay time.Duration)

	// [v2.1 New] Support delayed tasks with specific traits
	PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits)

	// PostRepeatingTask submits a task that repeats at a fixed interval.
	// The interval is measured from the end of one execution to the start of the next
	// (fixed-delay semantics), not from start-to-start (fixed-rate).
	PostRepeatingTask(task Task, interval time.Duration) RepeatingTaskHandle
	PostRepeatingTaskWithTraits(task Task, interval time.Duration, traits TaskTraits) RepeatingTaskHandle
	PostRepeatingTaskWithInitialDelay(task Task, initialDelay, interval time.Duration, traits TaskTraits) RepeatingTaskHandle

	// [v2.3 New] Support task and reply pattern
	// PostTaskAndReply executes task on this runner, then posts reply to replyRunner
	PostTaskAndReply(task Task, reply Task, replyRunner TaskRunner)
	// PostTaskAndReplyWithTraits allows specifying traits for both task and reply
	PostTaskAndReplyWithTraits(task Task, taskTraits TaskTraits, reply Task, replyTraits TaskTraits, replyRunner TaskRunner)

	// [v2.4 New] Synchronization and lifecycle management
	// WaitIdle blocks until all currently queued tasks have completed execution
	// Tasks posted after WaitIdle is called are not waited for
	// Returns error if context is cancelled or runner is closed
	WaitIdle(ctx context.Context) error

	// FlushAsync posts a barrier task that executes callback when all prior tasks complete
	// This is a non-blocking alternative to WaitIdle
	FlushAsync(callback func())

	// WaitShutdown blocks until Shutdown() is called on this runner
	// Returns error if context is cancelled
	WaitShutdown(ctx context.Context) error

	// Shutdown marks the runner as closed and clears all pending tasks
	// This method is non-blocking and can be safely called from within a task
	Shutdown()

	// IsClosed returns true if the runner has been shut down
	IsClosed() bool

	// [v2.5 New] Identification and Metadata
	// Name returns the name of the task runner
	Name() string
	// Metadata returns the metadata associated with the task runner
	Metadata() map[string]any

	// [v2.6 New] Thread Pool Access
	// GetThreadPool returns the underlying ThreadPool used by this runner
	// Returns nil for runners that don't use a thread pool (e.g., SingleThreadTaskRunner)
	GetThreadPool() ThreadPool
}
```

#### `ThreadPool`

=============================================================================
ThreadPool: Define task execution interface
=============================================================================

```go
type ThreadPool interface {
	PostInternal(task Task, traits TaskTraits)
	PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner)

	Start(ctx context.Context)
	Stop()

	ID() string
	IsRunning() bool

	WorkerCount() int
	QueuedTaskCount() int  // In queue
	ActiveTaskCount() int  // Executing
	DelayedTaskCount() int // Delayed
}
```

#### `WorkSource`

```go
type WorkSource interface {
	GetWork(stopCh <-chan struct{}) (TaskItem, bool)
}
```

### Structs

#### `DefaultPanicHandler`

DefaultPanicHandler provides a basic panic handler that logs to stdout.

```go
type DefaultPanicHandler struct{}
```

**Exported methods:**

- `HandlePanic` func (h *DefaultPanicHandler) HandlePanic(ctx context.Context, runnerName string, workerID int, panicInfo any, stackTrace []byte)
  > HandlePanic prints panic information to stdout.

#### `DefaultRejectedTaskHandler`

DefaultRejectedTaskHandler provides a basic handler that logs rejected tasks.

```go
type DefaultRejectedTaskHandler struct{}
```

**Exported methods:**

- `HandleRejectedTask` func (h *DefaultRejectedTaskHandler) HandleRejectedTask(runnerName string, reason string)
  > HandleRejectedTask logs the rejected task.

#### `DelayManager`

```go
type DelayManager struct {
	pq     DelayedTaskHeap
	mu     sync.Mutex
	wakeup chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}
```

**Exported methods:**

- `AddDelayedTask` func (dm *DelayManager) AddDelayedTask(task Task, delay time.Duration, traits TaskTraits, target TaskRunner)
- `Stop` func (dm *DelayManager) Stop()
- `TaskCount` func (dm *DelayManager) TaskCount() int
#### `DelayedTask`

DelayedTask represents a task scheduled for the future

```go
type DelayedTask struct {
	RunAt  time.Time
	Task   Task
	Traits TaskTraits
	Target TaskRunner
	index  int // for heap interface
}
```

#### `FIFOTaskQueue`

```go
type FIFOTaskQueue struct {
	mu    sync.Mutex
	tasks []TaskItem
}
```

**Exported methods:**

- `Clear` func (q *FIFOTaskQueue) Clear()
  > Clear removes all tasks from the queue and releases references

- `IsEmpty` func (q *FIFOTaskQueue) IsEmpty() bool
- `Len` func (q *FIFOTaskQueue) Len() int
- `MaybeCompact` func (q *FIFOTaskQueue) MaybeCompact()
- `PeekTraits` func (q *FIFOTaskQueue) PeekTraits() (TaskTraits, bool)
- `Pop` func (q *FIFOTaskQueue) Pop() (TaskItem, bool)
- `PopUpTo` func (q *FIFOTaskQueue) PopUpTo(max int) []TaskItem
- `Push` func (q *FIFOTaskQueue) Push(t Task, traits TaskTraits)
- `PushWithID` func (q *FIFOTaskQueue) PushWithID(t Task, traits TaskTraits) TaskID
#### `NilMetrics`

NilMetrics provides a no-op metrics implementation that does nothing.
This is the default when no metrics interface is provided.

```go
type NilMetrics struct{}
```

**Exported methods:**

- `RecordQueueDepth` func (m *NilMetrics) RecordQueueDepth(runnerName string, depth int)
  > RecordQueueDepth is a no-op.

- `RecordTaskDuration` func (m *NilMetrics) RecordTaskDuration(runnerName string, priority TaskPriority, duration time.Duration)
  > RecordTaskDuration is a no-op.

- `RecordTaskPanic` func (m *NilMetrics) RecordTaskPanic(runnerName string, panicInfo any)
  > RecordTaskPanic is a no-op.

- `RecordTaskRejected` func (m *NilMetrics) RecordTaskRejected(runnerName string, reason string)
  > RecordTaskRejected is a no-op.

#### `ParallelTaskRunner`

ParallelTaskRunner executes up to maxConcurrency tasks simultaneously.
Tasks are queued with priority support and executed as slots become available.

```go
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
}
```

**Exported methods:**

- `FlushAsync` func (r *ParallelTaskRunner) FlushAsync(callback func())
  > FlushAsync posts a barrier task that executes callback when all prior tasks complete.
  > This is a non-blocking alternative to WaitIdle.
  > 
  > The callback will execute after all tasks posted before FlushAsync() have completed.
  > Tasks posted after FlushAsync() will not run before the callback.
  > 
  > Implementation note: This posts a special barrier task to the queue. When the
  > scheduler encounters the barrier, it waits for all currently running tasks to
  > complete before executing the callback. This provides true barrier semantics.
  > 
  > Example:
  > 
  > 	runner.PostTask(task1)
  > 	runner.PostTask(task2)
  > 	runner.FlushAsync(func() {
  > 	    // This runs after task1 and task2 complete
  > 	    fmt.Println("task1 and task2 completed!")
  > 	})
  > 	runner.PostTask(task3)  // Will NOT run before the callback

- `GetThreadPool` func (r *ParallelTaskRunner) GetThreadPool() ThreadPool
  > GetThreadPool returns the underlying ThreadPool used by this runner

- `IsClosed` func (r *ParallelTaskRunner) IsClosed() bool
  > IsClosed returns true if the runner has been shut down.

- `MaxConcurrency` func (r *ParallelTaskRunner) MaxConcurrency() int
  > MaxConcurrency returns the maximum number of concurrent tasks.

- `Metadata` func (r *ParallelTaskRunner) Metadata() map[string]any
  > Metadata returns the metadata associated with the task runner

- `Name` func (r *ParallelTaskRunner) Name() string
  > Name returns the name of the task runner

- `PendingTaskCount` func (r *ParallelTaskRunner) PendingTaskCount() int
  > PendingTaskCount returns the number of queued tasks waiting to run.

- `PostDelayedTask` func (r *ParallelTaskRunner) PostDelayedTask(task Task, delay time.Duration)
  > PostDelayedTask submits a task to execute after a delay.

- `PostDelayedTaskNamed` func (r *ParallelTaskRunner) PostDelayedTaskNamed(name string, task Task, delay time.Duration)
  > PostDelayedTaskNamed submits a delayed named task.

- `PostDelayedTaskWithTraits` func (r *ParallelTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits)
  > PostDelayedTaskWithTraits submits a delayed task with specified traits.

- `PostDelayedTaskWithTraitsNamed` func (r *ParallelTaskRunner) PostDelayedTaskWithTraitsNamed(name string, task Task, delay time.Duration, traits TaskTraits)
  > PostDelayedTaskWithTraitsNamed submits a delayed named task with specified traits.

- `PostRepeatingTask` func (r *ParallelTaskRunner) PostRepeatingTask(task Task, interval time.Duration) RepeatingTaskHandle
  > PostRepeatingTask submits a repeating task

- `PostRepeatingTaskWithInitialDelay` func (r *ParallelTaskRunner) PostRepeatingTaskWithInitialDelay(
	task Task,
	initialDelay, interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle
  > PostRepeatingTaskWithInitialDelay submits a repeating task with an initial delay

- `PostRepeatingTaskWithTraits` func (r *ParallelTaskRunner) PostRepeatingTaskWithTraits(
	task Task,
	interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle
  > PostRepeatingTaskWithTraits submits a repeating task with specific traits

- `PostTask` func (r *ParallelTaskRunner) PostTask(task Task)
  > PostTask submits a task with default traits.

- `PostTaskAndReply` func (r *ParallelTaskRunner) PostTaskAndReply(task Task, reply Task, replyRunner TaskRunner)
  > PostTaskAndReply executes task on this runner, then posts reply to replyRunner.

- `PostTaskAndReplyWithTraits` func (r *ParallelTaskRunner) PostTaskAndReplyWithTraits(
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyTraits TaskTraits,
	replyRunner TaskRunner,
)
  > PostTaskAndReplyWithTraits allows specifying different traits for task and reply.

- `PostTaskNamed` func (r *ParallelTaskRunner) PostTaskNamed(name string, task Task)
  > PostTaskNamed submits a task with a caller-provided display name.

- `PostTaskWithTraits` func (r *ParallelTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits)
  > PostTaskWithTraits submits a task with specified traits.

- `PostTaskWithTraitsNamed` func (r *ParallelTaskRunner) PostTaskWithTraitsNamed(name string, task Task, traits TaskTraits)
  > PostTaskWithTraitsNamed submits a named task with specified traits.

- `RunningTaskCount` func (r *ParallelTaskRunner) RunningTaskCount() int
  > RunningTaskCount returns the number of currently executing tasks.

- `SetMetadata` func (r *ParallelTaskRunner) SetMetadata(key string, value any)
  > SetMetadata sets a metadata key-value pair

- `SetName` func (r *ParallelTaskRunner) SetName(name string)
  > SetName sets the name of the task runner

- `Shutdown` func (r *ParallelTaskRunner) Shutdown()
  > Shutdown marks the runner as closed and clears all pending tasks.
  > This method is non-blocking and can be safely called from within a task.
  > 
  > Shutdown does NOT interrupt currently executing tasks - they will run to completion.
  > However, no new tasks will be started from the queue after Shutdown is called.

- `Stats` func (r *ParallelTaskRunner) Stats() RunnerStats
  > Stats returns current observability data for this runner.

- `WaitIdle` func (r *ParallelTaskRunner) WaitIdle(ctx context.Context) error
  > WaitIdle blocks until all currently queued tasks have completed execution.
  > 
  > This method waits until both the queue is empty AND no tasks are currently
  > executing (runningCount == 0).
  > 
  > Returns error if:
  > - Context is cancelled or deadline exceeded
  > - Runner is closed when WaitIdle is called
  > 
  > Note: Tasks posted after WaitIdle is called are not waited for.

- `WaitShutdown` func (r *ParallelTaskRunner) WaitShutdown(ctx context.Context) error
  > WaitShutdown blocks until Shutdown() is called on this runner.
  > Returns error if context is cancelled.

#### `PoolStats`

PoolStats represents runtime observability state for a thread pool.

```go
type PoolStats struct {
	ID      string
	Workers int
	Queued  int
	Active  int
	Delayed int
	Running bool
}
```

#### `PriorityTaskQueue`

```go
type PriorityTaskQueue struct {
	mu           sync.Mutex
	pq           priorityHeap
	nextSequence uint64
}
```

**Exported methods:**

- `Clear` func (q *PriorityTaskQueue) Clear()
  > Clear removes all tasks from the queue and releases references

- `IsEmpty` func (q *PriorityTaskQueue) IsEmpty() bool
- `Len` func (q *PriorityTaskQueue) Len() int
- `MaybeCompact` func (q *PriorityTaskQueue) MaybeCompact()
- `PeekTraits` func (q *PriorityTaskQueue) PeekTraits() (TaskTraits, bool)
- `Pop` func (q *PriorityTaskQueue) Pop() (TaskItem, bool)
- `PopUpTo` func (q *PriorityTaskQueue) PopUpTo(max int) []TaskItem
- `Push` func (q *PriorityTaskQueue) Push(t Task, traits TaskTraits)
- `PushWithID` func (q *PriorityTaskQueue) PushWithID(t Task, traits TaskTraits) TaskID
#### `RunnerStats`

RunnerStats represents runtime observability state for a task runner.

```go
type RunnerStats struct {
	Name           string
	Type           string
	Pending        int
	Running        int
	Rejected       int64
	Closed         bool
	BarrierPending bool
}
```

#### `SequencedTaskRunner`

```go
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
```

**Exported methods:**

- `FlushAsync` func (r *SequencedTaskRunner) FlushAsync(callback func())
  > FlushAsync posts a barrier task that executes the callback when all prior tasks complete.
  > This is a non-blocking alternative to WaitIdle.
  > 
  > The callback will be executed on this runner's thread, after all tasks posted
  > before FlushAsync have completed.
  > 
  > Example:
  > 
  > 	runner.PostTask(task1)
  > 	runner.PostTask(task2)
  > 	runner.FlushAsync(func() {
  > 	    fmt.Println("task1 and task2 completed!")
  > 	})

- `GetThreadPool` func (r *SequencedTaskRunner) GetThreadPool() ThreadPool
  > GetThreadPool returns the underlying ThreadPool used by this runner

- `IsClosed` func (r *SequencedTaskRunner) IsClosed() bool
  > IsClosed returns true if the runner has been shut down.

- `Metadata` func (r *SequencedTaskRunner) Metadata() map[string]any
  > Metadata returns the metadata associated with the task runner

- `Name` func (r *SequencedTaskRunner) Name() string
  > Name returns the name of the task runner

- `PendingTaskCount` func (r *SequencedTaskRunner) PendingTaskCount() int
  > PendingTaskCount returns the number of queued tasks waiting to run.

- `PostDelayedTask` func (r *SequencedTaskRunner) PostDelayedTask(task Task, delay time.Duration)
  > PostDelayedTask submits a task to execute after a delay

- `PostDelayedTaskNamed` func (r *SequencedTaskRunner) PostDelayedTaskNamed(name string, task Task, delay time.Duration)
  > PostDelayedTaskNamed submits a delayed named task.

- `PostDelayedTaskWithTraits` func (r *SequencedTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits)
  > PostDelayedTaskWithTraits submits a delayed task with specified traits

- `PostDelayedTaskWithTraitsNamed` func (r *SequencedTaskRunner) PostDelayedTaskWithTraitsNamed(name string, task Task, delay time.Duration, traits TaskTraits)
  > PostDelayedTaskWithTraitsNamed submits a delayed named task with specified traits.

- `PostRepeatingTask` func (r *SequencedTaskRunner) PostRepeatingTask(task Task, interval time.Duration) RepeatingTaskHandle
  > PostRepeatingTask submits a task that repeats at a fixed interval

- `PostRepeatingTaskWithInitialDelay` func (r *SequencedTaskRunner) PostRepeatingTaskWithInitialDelay(
	task Task,
	initialDelay, interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle
  > PostRepeatingTaskWithInitialDelay submits a repeating task with an initial delay
  > The task will first execute after initialDelay, then repeat every interval.

- `PostRepeatingTaskWithTraits` func (r *SequencedTaskRunner) PostRepeatingTaskWithTraits(
	task Task,
	interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle
  > PostRepeatingTaskWithTraits submits a repeating task with specific traits

- `PostTask` func (r *SequencedTaskRunner) PostTask(task Task)
  > PostTask submits a task with default traits

- `PostTaskAndReply` func (r *SequencedTaskRunner) PostTaskAndReply(task Task, reply Task, replyRunner TaskRunner)
  > PostTaskAndReply executes task on this runner, then posts reply to replyRunner.
  > If task panics, reply will not be executed.

- `PostTaskAndReplyWithTraits` func (r *SequencedTaskRunner) PostTaskAndReplyWithTraits(
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyTraits TaskTraits,
	replyRunner TaskRunner,
)
  > PostTaskAndReplyWithTraits allows specifying different traits for task and reply.
  > This is useful when task is background work (BestEffort) but reply is UI update (UserVisible).

- `PostTaskNamed` func (r *SequencedTaskRunner) PostTaskNamed(name string, task Task)
  > PostTaskNamed submits a task with a caller-provided display name.

- `PostTaskWithTraits` func (r *SequencedTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits)
  > PostTaskWithTraits submits a task with specified traits

- `PostTaskWithTraitsNamed` func (r *SequencedTaskRunner) PostTaskWithTraitsNamed(name string, task Task, traits TaskTraits)
  > PostTaskWithTraitsNamed submits a named task with specified traits.

- `RunningTaskCount` func (r *SequencedTaskRunner) RunningTaskCount() int
  > RunningTaskCount returns 1 if runLoop is running/scheduled, otherwise 0.

- `SetMetadata` func (r *SequencedTaskRunner) SetMetadata(key string, value any)
  > SetMetadata sets a metadata key-value pair

- `SetName` func (r *SequencedTaskRunner) SetName(name string)
  > SetName sets the name of the task runner

- `Shutdown` func (r *SequencedTaskRunner) Shutdown()
  > Shutdown gracefully stops the runner by:
  > 1. Marking it as closed (stops accepting new tasks)
  > 2. Clearing all pending tasks in the queue
  > 3. All repeating tasks will automatically stop on their next execution
  > 4. Signaling all WaitShutdown() waiters
  > 
  > Note: This method is non-blocking and can be safely called from within a task.
  > Note: This will not interrupt currently executing tasks.

- `Stats` func (r *SequencedTaskRunner) Stats() RunnerStats
  > Stats returns current observability data for this runner.

- `WaitIdle` func (r *SequencedTaskRunner) WaitIdle(ctx context.Context) error
  > WaitIdle blocks until all currently queued tasks have completed execution.
  > This is implemented by posting a barrier task and waiting for it to execute.
  > 
  > Due to the sequential nature of SequencedTaskRunner, when the barrier task
  > executes, all tasks posted before WaitIdle are guaranteed to have completed.
  > 
  > Returns error if:
  > - Context is cancelled or deadline exceeded
  > - Runner is closed when WaitIdle is called
  > 
  > Note: Tasks posted after WaitIdle is called are not waited for.
  > Note: Repeating tasks will continue to repeat and are not waited for.

- `WaitShutdown` func (r *SequencedTaskRunner) WaitShutdown(ctx context.Context) error
  > WaitShutdown blocks until Shutdown() is called on this runner.
  > 
  > This is useful for waiting for the runner to be shut down, either by
  > an external caller or by a task running on the runner itself.
  > 
  > Returns error if context is cancelled or deadline exceeded.
  > 
  > Example:
  > 
  > 	// Task shuts down the runner when condition is met
  > 	runner.PostTask(func(ctx context.Context) {
  > 	    if conditionMet() {
  > 	        me := GetCurrentTaskRunner(ctx)
  > 	        me.Shutdown()
  > 	    }
  > 	})
  > 
  > 	// Main thread waits for shutdown
  > 	runner.WaitShutdown(context.Background())

#### `SingleThreadTaskRunner`

SingleThreadTaskRunner binds a dedicated Goroutine to execute tasks sequentially.
It guarantees that all tasks submitted to it run on the same Goroutine (Thread Affinity).

Use cases:
1. Blocking IO operations (e.g., NetworkReceiver)
2. CGO calls that require Thread Local Storage
3. Simulating Main Thread / UI Thread behavior

Key differences from SequencedTaskRunner:
- SequencedTaskRunner: Tasks execute sequentially but may run on different worker goroutines
- SingleThreadTaskRunner: Tasks execute sequentially AND always on the same dedicated goroutine

```go
type SingleThreadTaskRunner struct {
	// Task queue: Buffered channel for tasks
	workQueue chan Task

	// Queue policy for handling full queue
	queuePolicyMu     sync.RWMutex
	queuePolicy       QueuePolicy
	rejectionCallback RejectionCallback
	rejectedCount     atomic.Int64
	executingCount    atomic.Int32

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
	metadata map[string]any
	mu       sync.Mutex
}
```

**Exported methods:**

- `FlushAsync` func (r *SingleThreadTaskRunner) FlushAsync(callback func())
  > FlushAsync posts a barrier task that executes the callback when all prior tasks complete.
  > This is a non-blocking alternative to WaitIdle.
  > 
  > The callback will be executed on this runner's dedicated goroutine, after all tasks
  > posted before FlushAsync have completed.
  > 
  > Example:
  > 
  > 	runner.PostTask(task1)
  > 	runner.PostTask(task2)
  > 	runner.FlushAsync(func() {
  > 	    fmt.Println("task1 and task2 completed!")
  > 	})

- `GetQueuePolicy` func (r *SingleThreadTaskRunner) GetQueuePolicy() QueuePolicy
  > GetQueuePolicy returns the current queue policy

- `GetThreadPool` func (r *SingleThreadTaskRunner) GetThreadPool() ThreadPool
  > GetThreadPool returns nil because SingleThreadTaskRunner doesn't use a thread pool

- `IsClosed` func (r *SingleThreadTaskRunner) IsClosed() bool
  > IsClosed returns true if the runner has been stopped

- `Metadata` func (r *SingleThreadTaskRunner) Metadata() map[string]any
  > Metadata returns the metadata associated with the task runner

- `Name` func (r *SingleThreadTaskRunner) Name() string
  > Name returns the name of the task runner

- `PendingTaskCount` func (r *SingleThreadTaskRunner) PendingTaskCount() int
  > PendingTaskCount returns the number of queued tasks waiting to run.

- `PostDelayedTask` func (r *SingleThreadTaskRunner) PostDelayedTask(task Task, delay time.Duration)
  > PostDelayedTask submits a delayed task

- `PostDelayedTaskWithTraits` func (r *SingleThreadTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits)
  > PostDelayedTaskWithTraits submits a delayed task with traits.
  > Uses time.AfterFunc which is independent of the global TaskScheduler,
  > ensuring IO-related timers are not affected by scheduler load.

- `PostRepeatingTask` func (r *SingleThreadTaskRunner) PostRepeatingTask(task Task, interval time.Duration) RepeatingTaskHandle
  > PostRepeatingTask submits a task that repeats at a fixed interval

- `PostRepeatingTaskWithInitialDelay` func (r *SingleThreadTaskRunner) PostRepeatingTaskWithInitialDelay(
	task Task,
	initialDelay, interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle
  > PostRepeatingTaskWithInitialDelay submits a repeating task with an initial delay

- `PostRepeatingTaskWithTraits` func (r *SingleThreadTaskRunner) PostRepeatingTaskWithTraits(
	task Task,
	interval time.Duration,
	traits TaskTraits,
) RepeatingTaskHandle
  > PostRepeatingTaskWithTraits submits a repeating task with traits

- `PostTask` func (r *SingleThreadTaskRunner) PostTask(task Task)
  > PostTask submits a task for execution

- `PostTaskAndReply` func (r *SingleThreadTaskRunner) PostTaskAndReply(task Task, reply Task, replyRunner TaskRunner)
  > PostTaskAndReply executes task on this runner, then posts reply to replyRunner.
  > If task panics, reply will not be executed.
  > Both task and reply will execute on the same dedicated goroutine if replyRunner is this runner.

- `PostTaskAndReplyWithTraits` func (r *SingleThreadTaskRunner) PostTaskAndReplyWithTraits(
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyTraits TaskTraits,
	replyRunner TaskRunner,
)
  > PostTaskAndReplyWithTraits allows specifying different traits for task and reply.
  > This is useful when task is background work (BestEffort) but reply is UI update (UserVisible).
  > Note: For SingleThreadTaskRunner, traits don't affect execution order since all tasks
  > run sequentially on the same goroutine, but they may be used for logging or metrics.

- `PostTaskNamed` func (r *SingleThreadTaskRunner) PostTaskNamed(name string, task Task)
  > PostTaskNamed submits a task with a caller-provided display name.

- `PostTaskWithTraits` func (r *SingleThreadTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits)
  > PostTaskWithTraits submits a task with traits (traits are ignored for single-threaded execution)
  > The behavior when the queue is full depends on the configured QueuePolicy:
  > - QueuePolicyDrop: Silently drops the task (default)
  > - QueuePolicyReject: Calls the rejection callback if set
  > - QueuePolicyWait: Blocks until queue has space or context is done

- `PostTaskWithTraitsNamed` func (r *SingleThreadTaskRunner) PostTaskWithTraitsNamed(name string, task Task, traits TaskTraits)
  > PostTaskWithTraitsNamed submits a named task with traits.

- `RejectedCount` func (r *SingleThreadTaskRunner) RejectedCount() int64
  > RejectedCount returns the number of tasks that have been rejected due to full queue
  > Only incremented when QueuePolicy is QueuePolicyReject

- `RunningTaskCount` func (r *SingleThreadTaskRunner) RunningTaskCount() int
  > RunningTaskCount returns the number of tasks currently executing.

- `SetMetadata` func (r *SingleThreadTaskRunner) SetMetadata(key string, value any)
  > SetMetadata sets a metadata key-value pair

- `SetName` func (r *SingleThreadTaskRunner) SetName(name string)
  > SetName sets the name of the task runner

- `SetQueuePolicy` func (r *SingleThreadTaskRunner) SetQueuePolicy(policy QueuePolicy)
  > SetQueuePolicy sets the policy for handling full queue situations

- `SetRejectionCallback` func (r *SingleThreadTaskRunner) SetRejectionCallback(callback RejectionCallback)
  > SetRejectionCallback sets the callback to be called when a task is rejected
  > Only used when QueuePolicy is set to QueuePolicyReject

- `Shutdown` func (r *SingleThreadTaskRunner) Shutdown()
  > Shutdown marks the runner as closed and signals shutdown waiters.
  > Unlike Stop(), this method does NOT immediately terminate the runLoop.
  > This allows tasks to call Shutdown() from within themselves.
  > 
  > After calling Shutdown():
  > - WaitShutdown() will return
  > - IsClosed() will return true
  > - New tasks posted will be ignored
  > - Existing queued tasks will still execute
  > - Call Stop() to actually terminate the runLoop

- `Stats` func (r *SingleThreadTaskRunner) Stats() RunnerStats
  > Stats returns current observability data for this runner.

- `Stop` func (r *SingleThreadTaskRunner) Stop()
  > Stop stops the runner and releases resources

- `WaitIdle` func (r *SingleThreadTaskRunner) WaitIdle(ctx context.Context) error
  > WaitIdle blocks until all currently queued tasks have completed execution.
  > This is implemented by posting a barrier task and waiting for it to execute.
  > 
  > Since SingleThreadTaskRunner executes tasks sequentially on a dedicated goroutine,
  > when the barrier task executes, all tasks posted before WaitIdle are guaranteed
  > to have completed.
  > 
  > Returns error if:
  > - Context is cancelled or deadline exceeded
  > - Runner is closed when WaitIdle is called
  > 
  > Note: Tasks posted after WaitIdle is called are not waited for.
  > Note: Repeating tasks will continue to repeat and are not waited for.

- `WaitShutdown` func (r *SingleThreadTaskRunner) WaitShutdown(ctx context.Context) error
  > WaitShutdown blocks until Shutdown() is called on this runner.
  > 
  > This is useful for waiting for the runner to be shut down, either by
  > an external caller or by a task running on the runner itself.
  > 
  > Returns error if context is cancelled or deadline exceeded.
  > 
  > Example:
  > 
  > 	// IO thread: receives messages and posts shutdown when condition met
  > 	ioRunner.PostTask(func(ctx context.Context) {
  > 	    for {
  > 	        msg := receiveMessage()
  > 	        mainRunner.PostTask(func(ctx context.Context) {
  > 	            if shouldShutdown(msg) {
  > 	                me := GetCurrentTaskRunner(ctx)
  > 	                me.Shutdown()
  > 	            }
  > 	        })
  > 	    }
  > 	})
  > 
  > 	// Main thread waits for shutdown
  > 	mainRunner.WaitShutdown(context.Background())

#### `TaskItem`

```go
type TaskItem struct {
	Task   Task
	Traits TaskTraits
	ID     TaskID // Unique identifier for the task
}
```

#### `TaskScheduler`

```go
type TaskScheduler struct {
	queue       TaskQueue
	signal      chan struct{}
	workerCount int

	delayManager *DelayManager

	metricQueued int32 // Waiting in ReadyQueue
	metricActive int32 // Executing in Worker

	// Handlers and Metrics
	panicHandler        PanicHandler
	metrics             Metrics
	rejectedTaskHandler RejectedTaskHandler

	// Lifecycle
	shuttingDown int32 // atomic flag
}
```

**Exported methods:**

- `ActiveTaskCount` func (s *TaskScheduler) ActiveTaskCount() int
- `DelayedTaskCount` func (s *TaskScheduler) DelayedTaskCount() int
- `GetMetrics` func (s *TaskScheduler) GetMetrics() Metrics
  > GetMetrics returns the metrics collector for this scheduler

- `GetPanicHandler` func (s *TaskScheduler) GetPanicHandler() PanicHandler
  > GetPanicHandler returns the panic handler for this scheduler

- `GetWork` func (s *TaskScheduler) GetWork(stopCh <-chan struct{}) (TaskItem, bool)
  > GetWork (Called by Worker)

- `OnTaskEnd` func (s *TaskScheduler) OnTaskEnd()
- `OnTaskStart` func (s *TaskScheduler) OnTaskStart()
- `PostDelayedInternal` func (s *TaskScheduler) PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner)
  > PostDelayedInternal

- `PostInternal` func (s *TaskScheduler) PostInternal(task Task, traits TaskTraits)
  > PostInternal

- `QueuedTaskCount` func (s *TaskScheduler) QueuedTaskCount() int
- `Shutdown` func (s *TaskScheduler) Shutdown()
- `ShutdownGraceful` func (s *TaskScheduler) ShutdownGraceful(timeout time.Duration) error
  > ShutdownGraceful waits for all queued and active tasks to complete
  > Returns error if timeout is exceeded before tasks complete

- `WorkerCount` func (s *TaskScheduler) WorkerCount() int
  > Metrics

#### `TaskSchedulerConfig`

TaskSchedulerConfig holds configuration options for TaskScheduler.
All handlers are optional; if not provided, default implementations will be used.

```go
type TaskSchedulerConfig struct {
	// PanicHandler is called when a task panics. Defaults to DefaultPanicHandler.
	PanicHandler PanicHandler

	// Metrics is called to record task execution metrics. Defaults to NilMetrics.
	Metrics Metrics

	// RejectedTaskHandler is called when a task is rejected. Defaults to DefaultRejectedTaskHandler.
	RejectedTaskHandler RejectedTaskHandler
}
```

#### `TaskTraits`

```go
type TaskTraits struct {
	Priority TaskPriority
}
```

### Types

#### `DelayedTaskHeap`

DelayedTaskHeap implements heap.Interface

```go
type DelayedTaskHeap []*DelayedTask
```

**Exported methods:**

- `Len` func (h DelayedTaskHeap) Len() int
- `Less` func (h DelayedTaskHeap) Less(i, j int) bool
- `Peek` func (h *DelayedTaskHeap) Peek() *DelayedTask
- `Pop` func (h *DelayedTaskHeap) Pop() any
- `Push` func (h *DelayedTaskHeap) Push(x any)
- `Swap` func (h DelayedTaskHeap) Swap(i, j int)
#### `QueuePolicy`

QueuePolicy defines how to handle full queue situations

```go
type QueuePolicy int
```

#### `RejectionCallback`

RejectionCallback is called when a task is rejected (QueuePolicyReject mode)

```go
type RejectionCallback func(task Task, traits TaskTraits)
```

#### `ReplyWithResult`

ReplyWithResult defines a reply callback that receives a result of type T and an error.
This is the counterpart to TaskWithResult, receiving the values returned by the task.

```go
type ReplyWithResult[T any] func(ctx context.Context, result T, err error)
```

#### `Task`

Task is the unit of work (Closure)

```go
type Task func(ctx context.Context)
```

#### `TaskID`

TaskID is a unique identifier for tasks, using UUID for guaranteed uniqueness.

```go
type TaskID uuid.UUID
```

**Exported methods:**

- `IsZero` func (id TaskID) IsZero() bool
  > IsZero returns true if the TaskID is the zero UUID.

- `String` func (id TaskID) String() string
  > String returns the string representation of the TaskID.

#### `TaskPriority`

```go
type TaskPriority int
```

#### `TaskWithResult`

TaskWithResult defines a task that returns a result of type T and an error.
This is used with PostTaskAndReplyWithResult to pass data from task to reply.

```go
type TaskWithResult[T any] func(ctx context.Context) (T, error)
```

### Constructors

#### `DefaultTaskSchedulerConfig`

DefaultTaskSchedulerConfig returns a config with default handlers.

```go
func DefaultTaskSchedulerConfig() *TaskSchedulerConfig
```

#### `DefaultTaskTraits`

```go
func DefaultTaskTraits() TaskTraits
```

#### `NewDelayManager`

```go
func NewDelayManager() *DelayManager
```

#### `NewFIFOTaskQueue`

```go
func NewFIFOTaskQueue() *FIFOTaskQueue
```

#### `NewFIFOTaskScheduler`

```go
func NewFIFOTaskScheduler(workerCount int) *TaskScheduler
```

#### `NewFIFOTaskSchedulerWithConfig`

```go
func NewFIFOTaskSchedulerWithConfig(workerCount int, config *TaskSchedulerConfig) *TaskScheduler
```

#### `NewParallelTaskRunner`

NewParallelTaskRunner creates a new ParallelTaskRunner with the specified concurrency limit.
Panics if threadPool is nil or maxConcurrency is out of valid range [1, 10000].

```go
func NewParallelTaskRunner(threadPool ThreadPool, maxConcurrency int) *ParallelTaskRunner
```

#### `NewPriorityTaskQueue`

```go
func NewPriorityTaskQueue() *PriorityTaskQueue
```

#### `NewPriorityTaskScheduler`

```go
func NewPriorityTaskScheduler(workerCount int) *TaskScheduler
```

#### `NewPriorityTaskSchedulerWithConfig`

```go
func NewPriorityTaskSchedulerWithConfig(workerCount int, config *TaskSchedulerConfig) *TaskScheduler
```

#### `NewSequencedTaskRunner`

```go
func NewSequencedTaskRunner(threadPool ThreadPool) *SequencedTaskRunner
```

#### `NewSingleThreadTaskRunner`

NewSingleThreadTaskRunner creates and starts a new SingleThreadTaskRunner.
It immediately spawns a dedicated goroutine for task execution.

```go
func NewSingleThreadTaskRunner() *SingleThreadTaskRunner
```

### Functions

#### `GenerateTaskID`

GenerateTaskID creates a new unique TaskID.

```go
func GenerateTaskID() TaskID
```

#### `GetCurrentTaskRunner`

```go
func GetCurrentTaskRunner(ctx context.Context) TaskRunner
```

#### `PostDelayedTaskAndReplyWithResult`

PostDelayedTaskAndReplyWithResult is similar to PostTaskAndReplyWithResult,
but delays the execution of the task.

The reply is NOT delayed - it executes immediately after the task completes.
Only the initial task execution is delayed by the specified duration.

Example:

	PostDelayedTaskAndReplyWithResult(
	    runner,
	    func(ctx context.Context) (string, error) {
	        return "delayed result", nil
	    },
	    2*time.Second,  // Wait 2 seconds before starting task
	    func(ctx context.Context, result string, err error) {
	        fmt.Println(result)  // Executes immediately after task completes
	    },
	    replyRunner,
	)

```go
func PostDelayedTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	delay time.Duration,
	reply ReplyWithResult[T],
	replyRunner TaskRunner,
)
```

#### `PostDelayedTaskAndReplyWithResultAndTraits`

PostDelayedTaskAndReplyWithResultAndTraits is the full-featured delayed version
with separate traits for task and reply.

```go
func PostDelayedTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	delay time.Duration,
	taskTraits TaskTraits,
	reply ReplyWithResult[T],
	replyTraits TaskTraits,
	replyRunner TaskRunner,
)
```

#### `PostTaskAndReplyWithResult`

PostTaskAndReplyWithResult executes a task that returns a result of type T and an error,
then passes that result to a reply callback on the replyRunner.

This function uses closure capture to safely pass the result across goroutines.
The captured variables (result and err) will escape to the heap, ensuring thread safety.

Execution guarantee (Happens-Before):
- The task ALWAYS completes before the reply starts
- The reply ALWAYS sees the final values written by the task
- This is guaranteed by the sequential execution in wrappedTask

Example:

	PostTaskAndReplyWithResult(
	    backgroundRunner,
	    func(ctx context.Context) (int, error) {
	        return len("Hello"), nil
	    },
	    func(ctx context.Context, length int, err error) {
	        fmt.Printf("Length: %d\n", length)
	    },
	    uiRunner,
	)

```go
func PostTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	reply ReplyWithResult[T],
	replyRunner TaskRunner,
)
```

#### `PostTaskAndReplyWithResultAndTraits`

PostTaskAndReplyWithResultAndTraits is the full-featured version that allows specifying
different traits for the task and reply separately.

This is useful when:
- Task is background work (BestEffort) but reply is UI update (UserVisible/UserBlocking)
- Task has different priority requirements than the reply

Example:

	PostTaskAndReplyWithResultAndTraits(
	    backgroundRunner,
	    func(ctx context.Context) (*UserData, error) {
	        return fetchUserFromDB(ctx)
	    },
	    TraitsBestEffort(),        // Background work, low priority
	    func(ctx context.Context, user *UserData, err error) {
	        updateUI(user)
	    },
	    TraitsUserVisible(),       // UI update, higher priority
	    uiRunner,
	)

```go
func PostTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	taskTraits TaskTraits,
	reply ReplyWithResult[T],
	replyTraits TaskTraits,
	replyRunner TaskRunner,
)
```

#### `TraitsBestEffort`

```go
func TraitsBestEffort() TaskTraits
```

#### `TraitsUserBlocking`

```go
func TraitsUserBlocking() TaskTraits
```

#### `TraitsUserVisible`

```go
func TraitsUserVisible() TaskTraits
```

### Constants

- `QueuePolicyDrop`, `QueuePolicyReject`, `QueuePolicyWait` = iota
- `TaskPriorityBestEffort`, `TaskPriorityUserVisible`, `TaskPriorityUserBlocking` = iota
