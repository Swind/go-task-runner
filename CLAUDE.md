# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is an experimental Go library implementing a multi-threaded programming architecture inspired by Chromium's Threading and Tasks design. It's for educational/testing purposes only, not production use.

**Core Philosophy**: Post tasks to virtual threads (Task Runners) instead of managing raw goroutines or channels. This decouples application logic from concurrency details.

## Build and Test Commands

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests in specific package
go test ./core/...

# Run a specific test
go test -v ./core -run TestSequencedTaskRunner

# Run examples
go run examples/basic_sequence/main.go
go run examples/single_thread/main.go
go run examples/task_and_reply/main.go
go run examples/repeating_task/main.go
go run examples/shutdown/main.go

# Build (no binary output - this is a library)
go build ./...
```

## Architecture Overview

### Core Components (in core/ package)

1. **TaskScheduler** (task_scheduler.go) - The "Brain"
   - Manages three priority queues: High, Normal, Low
   - Delegates work to thread pool workers
   - Handles delayed tasks via DelayManager
   - Tracks metrics (queued, active, delayed task counts)
   - Implements graceful shutdown

2. **GoroutineThreadPool** (pool.go) - The "Muscles"
   - Manages worker goroutines that pull from TaskScheduler
   - Worker loop: GetWork → OnTaskStart → Execute → OnTaskEnd
   - Global singleton pattern via InitGlobalThreadPool()

3. **SequencedTaskRunner** (sequenced_task_runner.go) - Virtual Thread
   - **CRITICAL**: Guarantees FIFO execution order (tasks run sequentially)
   - Allows lock-free code for resources owned by the sequence
   - Uses atomic `runningCount` (0 or 1) to ensure only one runLoop is active
   - CompareAndSwap ensures no duplicate runLoop instances
   - Runs ONE task at a time, then yields back to scheduler
   - Priority affects when the sequence gets scheduled, not task order within sequence

4. **SingleThreadTaskRunner** (single_thread_task_runner.go) - Thread Affinity
   - All tasks execute on the same dedicated goroutine
   - Use cases: blocking IO, CGO with TLS, UI thread simulation
   - Independent of global thread pool
   - Uses time.AfterFunc for delays (not TaskScheduler)

5. **TaskQueue** (queue.go) - Zero-allocation FIFO
   - Optimized with slice slicing and memory compaction
   - PopUpTo() for batch retrieval
   - MaybeCompact() to reduce memory footprint

6. **DelayManager** (delay_manager.go)
   - Built into TaskScheduler
   - Heap-based time management
   - Updates delayed task metrics

### Task and Reply Pattern (task_and_reply.go)

Two flavors:
- **PostTaskAndReply**: Execute task on one runner, reply on another
- **PostTaskAndReplyWithResult[T]**: Generic version that passes data from task to reply

Key implementation detail: Uses closure capture with heap escape to safely pass results across goroutines. Happens-before guarantee ensures task completes before reply starts.

### Package Structure

- `core/` - Internal implementation (TaskScheduler, runners, queue, etc.)
- Root package - Public API (types.go re-exports from core/)
- `examples/` - Usage examples for each feature

## Key Concepts

### 1. Task Priorities
```go
TaskPriorityBestEffort   // Lowest
TaskPriorityUserVisible  // Default
TaskPriorityUserBlocking // Highest
```

Priority determines when a task/sequence gets scheduled by the thread pool, NOT execution order within a SequencedTaskRunner.

### 2. Sequential Execution Guarantee

SequencedTaskRunner's `runLoop` (sequenced_task_runner.go:95-171):
- **Single atomic state variable**: `runningCount` (0=idle, 1=running)
- **CompareAndSwap protection**: `ensureRunning()` uses CAS to prevent duplicate runLoops
- Processes exactly ONE task per runLoop invocation
- Always yields back to scheduler between tasks (allows priority re-evaluation)
- **Double-check pattern**: After setting runningCount=0, checks if new tasks arrived
- **Sanity check**: runLoop panics if started with runningCount != 1

### 3. Lifecycle Management

All runners support:
- `Shutdown()` - Stops accepting tasks, clears queue
- `IsClosed()` - Check if runner is closed
- Repeating tasks auto-stop when runner is closed

### 4. Context Pattern

Tasks can retrieve their current runner:
```go
runner := core.GetCurrentTaskRunner(ctx)
```

This is stored in context during runLoop execution.

## Common Patterns

### Global Thread Pool Pattern
```go
taskrunner.InitGlobalThreadPool(4)
defer taskrunner.ShutdownGlobalThreadPool()
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
```

### UI/Background Work Pattern
```go
uiRunner.PostTask(func(ctx context.Context) {
    me := taskrunner.GetCurrentTaskRunner(ctx)
    bgRunner.PostTaskAndReply(
        func(ctx context.Context) { /* heavy work */ },
        func(ctx context.Context) { /* update UI */ },
        me,
    )
})
```

### Generic Result Passing
```go
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (*Data, error) { return fetchData() },
    func(ctx context.Context, data *Data, err error) { handleData(data, err) },
    uiRunner,
)
```

## Important Implementation Details

1. **Concurrency Safety**: SequencedTaskRunner uses a simplified atomic design:
   - **`runningCount` (int32)**: Single source of truth (0=idle, 1=running/scheduled)
   - **`ensureRunning()`**: Uses `CompareAndSwapInt32(&runningCount, 0, 1)` to atomically check and start
   - **Sanity check**: runLoop verifies runningCount==1 at start, panics if violated
   - **No complex state synchronization**: Eliminates the need for mutex-protected dual variables
   - See sequenced_task_runner.go:79-85 (ensureRunning) and 97-100 (sanity check)

2. **Memory Optimization**: TaskQueue uses zero-allocation techniques with slice reuse and compaction (queue.go)

3. **Error Handling**: Tasks that panic are recovered, but repeating tasks and PostTaskAndReply handle panics specially:
   - Repeating tasks: panic stops the repetition
   - PostTaskAndReply: task panic prevents reply from executing

4. **Shutdown Semantics**:
   - SequencedTaskRunner.Shutdown(): clears queue, stops repeating tasks
   - Does NOT interrupt currently executing task
   - Repeating tasks check IsClosed() on each iteration

5. **Delayed Tasks**: SequencedTaskRunner uses global TaskScheduler's DelayManager, SingleThreadTaskRunner uses independent time.AfterFunc

## Testing Patterns

Test files are co-located with implementation:
- core/*_test.go for unit tests
- example_test.go for package-level examples

Key test areas:
- Sequential execution verification
- Priority queue ordering
- Shutdown and lifecycle
- Task and reply pattern
- Repeating task cancellation
