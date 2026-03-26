# Design Document

## 1. Overview

Chromium-inspired task scheduling in Go — post tasks to virtual threads (TaskRunners) instead of managing goroutines directly. This library provides Sequenced, Parallel, and SingleThread task runners backed by a centralized scheduler and a goroutine thread pool. Educational and testing purposes only; not intended for production use.

For complete interface reference, see [API.md](API.md). For the Job subsystem, see [JOB_MANAGER.md](JOB_MANAGER.md).

```
                    ┌──────────────────┐
                    │  TaskScheduler   │  (Brain: scheduling decisions)
                    │  ┌────────────┐  │
                    │  │ TaskQueue  │  │  (FIFO or Priority)
                    │  └────────────┘  │
                    │  ┌────────────┐  │
                    │  │DelayManager│  │  (Timer Pump + min-heap)
                    │  └────────────┘  │
                    └────────┬─────────┘
                             │ GetWork() [pull model]
                    ┌────────▼─────────┐
                    │ GoroutineThreadPool│  (Muscles: execution only)
                    │  worker[0..N]    │
                    └────────┬─────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
     ┌────────▼──────┐ ┌────▼─────────┐ ┌──▼──────────────────┐
     │  Sequenced    │ │  Parallel    │ │  SingleThread       │
     │  TaskRunner   │ │  TaskRunner  │ │  TaskRunner         │
     │  (1-at-a-time)│ │  (bounded    │ │  (dedicated goroutine)
     │               │ │   concurrent)│ │  + QueuePolicy      │
     └───────────────┘ └──────────────┘ └─────────────────────┘
```

Package structure:
- Root package `taskrunner`: facade with type aliases (`type X = core.X`), `GoroutineThreadPool`, global singleton helpers
- `core/`: all implementation — TaskScheduler, queues, runners, DelayManager, interfaces
- `job/`: JobManager subsystem (see [JOB_MANAGER.md](JOB_MANAGER.md))

---

## 2. Core Philosophy

**Centralized Scheduling**: TaskScheduler is the sole decision-maker. It owns the queue, manages priorities, and controls the signal channel. Workers have no scheduling logic — they pull work via `GetWork()`.

**Priority & Fairness**: `TaskTraits.Priority` distinguishes `BestEffort` (prefetching), `UserVisible` (UI updates, default), and `UserBlocking` (input response, must run <16ms). Priority determines when a sequence gets scheduled, not the order within a sequence. `SequencedTaskRunner` processes one task then yields back to the scheduler, preventing any single sequence from starving others.

**Separation of Duty**: The TaskScheduler decides *what runs when*. The ThreadPool decides *how to execute*. This separation means scheduling logic is tested independently from execution mechanics.

**Plugin Extensibility**: `TaskSchedulerConfig` accepts `PanicHandler`, `Metrics`, and `RejectedTaskHandler` at construction time. These propagate through the system — workers retrieve them from the scheduler, runners access them via `GetThreadPool()`. Default no-op implementations mean zero configuration works out of the box.

---

## 3. The Brain — TaskScheduler

**Scheduling Model**: Single `TaskQueue` (either `FIFOTaskQueue` or `PriorityTaskQueue`) plus a buffered `signal` channel. Workers call `GetWork(stopCh)` which pops a `TaskItem` or blocks waiting on the signal. This pull model means no polling — workers sleep until notified.

**Why single queue**: A single queue with internal priority ordering (for `PriorityTaskQueue`) is simpler than three per-priority queues. It avoids priority-inversion complexity and the need for work-stealing between queues.

**DelayManager**: Uses a "Timer Pump" pattern — a min-heap of `DelayedTask` items sorted by `RunAt`, driven by a single `time.Timer` and a `wakeup` channel. The pump loop:
1. Peek at the earliest task
2. If expired, collect ALL expired tasks in one lock hold (batch processing)
3. Post them back to their target runners
4. If not expired, set timer to the remaining duration

The `wakeup` channel interrupts the timer when a new earlier task is inserted. Timer channels are drained properly with `select { case <-timer.C: default: }` to prevent goroutine leaks.

**Shutdown**: `Shutdown()` is immediate — stops accepting tasks, stops the DelayManager, and clears the queue. `ShutdownGraceful(timeout)` waits for queued and active tasks to finish within the timeout, then force-clears.

**Atomics for metrics**: `QueuedTaskCount`, `ActiveTaskCount`, `DelayedTaskCount` use `atomic.Int32`/`atomic.LoadInt32`. This avoids mutex contention on the hot path where every task start/end increments/decrements counters.

---

## 4. The Muscles — GoroutineThreadPool

**Worker Loop**: Each worker goroutine runs a simple loop: `GetWork(stopCh)` → `OnTaskStart()` → execute task → `OnTaskEnd()`. Panic recovery wraps every task execution using the `PanicHandler` from the scheduler. No scheduling logic whatsoever.

**ThreadPool as Interface**: `ThreadPool` is an interface (10 methods) rather than a concrete type. This enables testing with mock implementations, different backends without changing runners, and ensures runners depend on abstraction, not implementation.

**Global Singleton**: `InitGlobalThreadPool(workers)` / `GetGlobalThreadPool()` / `ShutdownGlobalThreadPool()` provide a convenience pattern for applications that need a single shared pool. `CreateTaskRunner(traits)` uses the global pool to create a `SequencedTaskRunner`.

**Graceful Stop**: `Stop()` cancels the worker context immediately. `StopGraceful(timeout)` waits for active tasks to finish within the timeout before stopping. `Join()` blocks until all workers have exited.

---

## 5. Virtual Threads — TaskRunners

All three runners implement the `TaskRunner` interface (20 methods). They provide sequential or bounded-concurrent execution guarantees without requiring users to manage goroutines or channels directly.

### 5.1 SequencedTaskRunner

**Design intent**: Sequential execution with fairness. Tasks within a sequence never run concurrently, eliminating the need for locks on shared resources.

**Lock-free CAS pattern**: `runningCount` is an `int32` atomic — 0 means idle, 1 means a runLoop is active or scheduled. `ensureRunning()` uses `CompareAndSwapInt32(0, 1)` to start a runLoop only if idle.

**Execution flow**: `runLoop` pops one task, executes it, checks for more. If more tasks exist, it re-posts `runLoop` to the thread pool. If empty, it sets `runningCount` to 0 and calls `scheduleNextIfNeeded()` to handle the race where a task was enqueued between the empty check and the CAS.

**Why CAS instead of mutex**: The CAS is a single atomic operation that determines whether to schedule work. A mutex would need to be held across the `PostInternal` call, creating potential deadlocks with the scheduler's own locks.

**Why yield after each task**: Processing one task and re-posting allows the scheduler to re-evaluate priorities between tasks from different sequences, providing fairness.

### 5.2 ParallelTaskRunner

**Design intent**: Bounded concurrent execution with coordination. Up to `maxConcurrency` tasks run simultaneously.

**Internal scheduler goroutine**: A `SingleThreadTaskRunner` serializes all scheduling decisions — dispatching tasks from the internal queue to the thread pool. This avoids locks on the pending queue while allowing concurrent task execution.

**Barrier task system**: `pendingBarrierTask` + `barrierTaskIDs` track in-flight tasks for `WaitIdle()` and `FlushAsync()`. When `WaitIdle` is called, a barrier task is queued. It only executes after all previously dispatched tasks have completed, providing true drain semantics.

**Why SingleThreadTaskRunner as scheduler**: Simple serialization without additional locking. The scheduler goroutine has exclusive access to the pending queue, and tasks are dispatched to the thread pool for actual execution.

### 5.3 SingleThreadTaskRunner

**Design intent**: Dedicated goroutine for blocking IO or tasks requiring thread affinity. Bypasses the global scheduler entirely.

**Own channel + own goroutine**: Uses a buffered channel (`workQueue`) and a single long-lived goroutine. All operations (post, delayed, repeating) are serialized through this channel.

**QueuePolicy**: Controls behavior when the channel is full:
- `QueuePolicyDrop` (default): silently discard the task
- `QueuePolicyReject`: discard and increment rejected counter
- `QueuePolicyWait`: block until space is available

**Delayed tasks**: Uses `time.AfterFunc` directly instead of the DelayManager. This is intentional — blocking IO tasks need timer affinity with their dedicated goroutine, not the global scheduler's timer pump.

**Shutdown vs Stop**: `Shutdown()` marks the runner as closed and signals waiters, but does NOT cancel the context (in-flight tasks continue). `Stop()` cancels the context and waits for the goroutine to exit. Use `Shutdown()` for graceful drain, `Stop()` for immediate teardown.

---

## 6. Task Patterns

### 6.1 Task-and-Reply

Generic result passing via `TaskWithResult[T] func(ctx context.Context) (T, error)` and `ReplyWithResult[T] func(ctx context.Context, result T, err error)`. Variants: `PostTaskAndReplyWithResult`, `PostTaskAndReplyWithResultAndTraits`, `PostDelayedTaskAndReplyWithResult`, `PostDelayedTaskAndReplyWithResultAndTraits`.

**Panic semantics**: If the task panics, the reply is NOT executed. The panic is recovered by the runner's panic handler, but the reply callback is skipped entirely. This prevents the reply from operating on incomplete state.

Root package `reply.go` provides convenience wrappers that don't require importing `core`.

### 6.2 Repeating Tasks

Fixed-delay semantics (not fixed-rate): the interval is measured from the end of one execution to the start of the next. If a task takes 200ms and the interval is 100ms, the next execution starts 100ms after the previous one finishes (300ms from the previous start).

Three constructors: `PostRepeatingTask` (no initial delay), `PostRepeatingTaskWithTraits`, `PostRepeatingTaskWithInitialDelay`. All return `RepeatingTaskHandle` with `Stop()` (prevent future executions) and `IsStopped()` (check state). All three runners implement repeating task variants.

### 6.3 Named Tasks

`PostTaskNamed`, `PostTaskWithTraitsNamed`, `PostDelayedTaskNamed`, `PostDelayedTaskWithTraitsNamed` accept a name string for debug and metadata purposes. The name is stored alongside the task and can be useful for tracing and logging.

### 6.4 Context Propagation

All three runners inject `taskRunnerKey` into the context before executing each task: `context.WithValue(ctx, taskRunnerKey, runner)`. `GetCurrentTaskRunner(ctx)` retrieves the runner that is executing the current task.

This enables tasks to post back to their own runner without holding a direct reference — useful for callbacks and chained operations.

---

## 7. Observability & Lifecycle

### 7.1 Stats

`RunnerStats{Name, Type, Pending, Running, Rejected, Closed, BarrierPending}` — snapshot of a runner's state. `PoolStats{ID, Workers, Queued, Active, Delayed, Running}` — snapshot of the thread pool. Both have a `Stats()` method for point-in-time inspection.

### 7.2 Synchronization

- `WaitIdle(ctx)`: blocks until the queue is empty AND no tasks are executing. Tasks posted after `WaitIdle` is called are not waited for. Returns error if context is cancelled or runner is closed.
- `FlushAsync(callback)`: posts a barrier callback that runs after all currently in-flight tasks complete. Non-blocking alternative to `WaitIdle`. On `SequencedTaskRunner` and `ParallelTaskRunner`, this uses the barrier task system.
- `WaitShutdown(ctx)`: blocks until `Shutdown()` has been called and the runner has fully stopped.

### 7.3 Shutdown Semantics

- **SequencedTaskRunner**: `Shutdown()` — marks closed, drains queue via runLoop, signals waiters
- **ParallelTaskRunner**: `Shutdown()` — marks closed, drains pending queue via scheduler goroutine, waits for all dispatched tasks to finish
- **SingleThreadTaskRunner**: Two distinct operations — `Shutdown()` (marks closed, signals waiters, in-flight tasks continue) vs `Stop()` (cancels context, waits for goroutine to exit)

### 7.4 Plugin Architecture

`TaskSchedulerConfig` accepts three plugin interfaces:
- `PanicHandler(ctx, runnerName, workerID, panicInfo, stackTrace)` — called when a task panics during execution. `DefaultPanicHandler` logs to stderr.
- `Metrics` — `RecordTaskDuration`, `RecordTaskPanic`, `RecordQueueDepth`, `RecordTaskRejected`. `NilMetrics` is the default no-op.
- `RejectedTaskHandler(runnerName, reason)` — called when a task is rejected (e.g., runner is closed). `DefaultRejectedTaskHandler` logs to stderr.

Extension point: `observability/prometheus/` provides a `MetricsExporter` that implements `Metrics` using Prometheus HistogramVec, CounterVec, and GaugeVec.
