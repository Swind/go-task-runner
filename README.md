# Go Task Runner

[![CI](https://github.com/Swind/go-task-runner/workflows/CI/badge.svg)](https://github.com/Swind/go-task-runner/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Swind/go-task-runner)](https://goreportcard.com/report/github.com/Swind/go-task-runner)
![Go Version](https://img.shields.io/badge/Go-1.24%2B-%2300ADD8?logo=go)
[![License](https://img.shields.io/github/license/Swind/go-task-runner)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Swind/go-task-runner)](https://github.com/Swind/go-task-runner/releases/latest)

[繁體中文 README](README.zh-TW.md)

### ⚠️ **Disclaimer: Experimental / Educational Use Only**

This library is an **experimental implementation** for educational and testing purposes. It is **NOT** intended for production environments.

A multi-threaded programming architecture for Go, inspired by **Chromium's Threading and Tasks design**.

Reference: [Threading and Tasks in Chrome](https://chromium.googlesource.com/chromium/src/+/main/docs/threading_and_tasks.md)

## Design Philosophy

This library implements a **threading model** where developers **post tasks to virtual threads (Task Runners)** rather than managing raw goroutines or channels manually. This decoupling allows the underlying system to manage concurrency details while application code focuses on logic.

The core concepts derived from Chromium are:

-   **Task Runners over Threads**: You rarely create raw goroutines. Instead, you ask for a `TaskRunner` and post tasks to it.
-   **Sequential Consistency (Strands)**: The `SequencedTaskRunner` acts like a single logical thread. Tasks posted to it are guaranteed to run sequentially, allowing you to write **lock-free** code for resources owned by that sequence.
-   **Task Traits**: You describe **what** the task is (e.g., `UserBlocking`, `BestEffort`) rather than **how** to run it. The system decides the appropriate priority.
-   **Concurrency Safety**: Runtime assertions ensure that sequential rules are strictly followed, preventing common race conditions.

## Features

-   **Goroutine Thread Pool**: Efficient worker pool backing the execution model.
-   **Sequenced Task Runner**: Strict FIFO execution order for tasks within a stream.
-   **Single Thread Task Runner**: Guaranteed thread affinity for blocking IO and TLS.
-   **Delayed Tasks**: Scheduling tasks in the future.
-   **Repeating Tasks**: Execute tasks repeatedly at fixed intervals with easy stop control.
-   **Task and Reply Pattern**: Execute task on one runner, reply on another with type-safe return values.
-   **Task Traits**: Priority-aware task scheduling.
-   **JobManager (core package)**: Three-layer execution model with durable-ack submission and pluggable persistence.

## Installation

```bash
go get github.com/Swind/go-task-runner
```

## Usage

Note: The snippets below focus on specific APIs. For complete runnable programs, see `examples/*`.

Important synchronization note:
- Lock-free state updates are safe when the state is owned and accessed by a single `SequencedTaskRunner`/`SingleThreadTaskRunner`.
- When data crosses goroutine or runner boundaries, use explicit synchronization (`WaitIdle`, `WaitShutdown`, channels, or `sync/atomic`).

### 1. Initialize the Global Thread Pool

```go
package main

import (
    "context"

    taskrunner "github.com/Swind/go-task-runner"
)

func main() {
    // Initialize the global thread pool with 4 workers
    taskrunner.InitGlobalThreadPool(4)
    defer taskrunner.ShutdownGlobalThreadPool()

    // Create a sequenced runner (like a logical thread)
    runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

    // ...
}
```

### 2. Using SequencedTaskRunner

The `SequencedTaskRunner` is the recommended way to execute tasks. It ensures that tasks posted to the same runner are executed sequentially, removing the need for mutexes for state protected by that runner.

```go
    // Task 1
    runner.PostTask(func(ctx context.Context) {
        // This runs safely without locks relative to other tasks on this runner
        println("Doing work...")
    })

    // Task 2 (Delayed)
    runner.PostDelayedTask(func(ctx context.Context) {
        println("Runs 1 second later...")
    }, 1 * time.Second)
```

### 2.1 Using SingleThreadTaskRunner

The `SingleThreadTaskRunner` guarantees that all tasks execute on the same dedicated goroutine (thread affinity). This is useful for:

- **Blocking IO operations** (e.g., NetworkReceiver with blocking reads)
- **CGO calls** that require Thread Local Storage
- **UI Thread simulation** where tasks must run on a specific thread

```go
    // Create a single-threaded runner
    runner := taskrunner.NewSingleThreadTaskRunner()
    defer runner.Stop()

    // All tasks run on the same dedicated goroutine
    runner.PostTask(func(ctx context.Context) {
        // Blocking IO operation - safe on dedicated thread
        data := blockingRead()
        process(data)
    })

    // Delayed task - still on same goroutine
    runner.PostDelayedTask(func(ctx context.Context) {
        println("Runs on same goroutine after delay")
    }, 1 * time.Second)
```

See [examples/single_thread](examples/single_thread/main.go) for more examples.

### 2.2 Using PostTaskAndReply Pattern

The `PostTaskAndReply` pattern allows you to execute a task on one runner, then automatically post a reply to another runner when the task completes. This is perfect for UI/background work patterns.

```go
    uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
    bgRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

    uiRunner.PostTask(func(ctx context.Context) {
        me := taskrunner.GetCurrentTaskRunner(ctx)

        // Execute task on background runner, reply back to UI runner
        bgRunner.PostTaskAndReply(
            func(ctx context.Context) {
                // Heavy work on background thread
                loadDataFromServer()
            },
            func(ctx context.Context) {
                // Update UI on UI thread
                updateUI()
            },
            me, // Reply back to UI runner
        )
    })
```

**With Return Values (Generic):**

Use `PostTaskAndReplyWithResult` to pass data from task to reply.
This helper currently lives in the `core` package, so import:

```go
import "github.com/Swind/go-task-runner/core"
```

Then call:

```go
    core.PostTaskAndReplyWithResult(
        bgRunner,
        func(ctx context.Context) (*UserData, error) {
            // Returns data and error
            return fetchUserFromDB(ctx)
        },
        func(ctx context.Context, user *UserData, err error) {
            // Receives data and error
            if err != nil {
                showError(err)
                return
            }
            updateUserUI(user)
        },
        uiRunner,
    )
```

See [examples/task_and_reply](examples/task_and_reply/main.go) for more examples.

### 3. Using Task Traits (Priorities)

Priority effects are most visible when multiple runners compete for the same worker pool.
See [examples/mixed_priority](examples/mixed_priority/main.go) for a focused demonstration.

```go
    runner.PostTaskWithTraits(func(ctx context.Context) {
        println("High priority work!")
    }, taskrunner.TaskTraits{
        Priority: taskrunner.TaskPriorityUserBlocking,
    })
```

### 4. Repeating Tasks

Execute tasks repeatedly at fixed intervals:

```go
    // Simple repeating task
    handle := runner.PostRepeatingTask(func(ctx context.Context) {
        println("Runs every second")
    }, 1*time.Second)

    // Stop when done
    defer handle.Stop()

    // With initial delay
    handle2 := runner.PostRepeatingTaskWithInitialDelay(
        task,
        2*time.Second,  // Start after 2 seconds
        1*time.Second,  // Then repeat every second
        taskrunner.DefaultTaskTraits(),
    )
    defer handle2.Stop()
```

See [examples/repeating_task](examples/repeating_task/main.go) for more examples.

### 5. Shutdown and Cleanup

Gracefully shutdown a runner to stop all tasks:

```go
    runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

    // Add tasks and repeating tasks...

    // Shutdown when done
    runner.Shutdown()  // Prevents new tasks, clears pending queue, stops all repeating tasks

    // Check if closed
    if runner.IsClosed() {
        println("Runner is closed")
    }
```

See [examples/shutdown](examples/shutdown/main.go) for more examples.

### 6. Observability with Prometheus

You can bridge built-in runtime metrics to Prometheus without changing task APIs:

```go
import (
    obs "github.com/Swind/go-task-runner/observability/prometheus"
    prom "github.com/prometheus/client_golang/prometheus"
)

reg := prom.NewRegistry()
exporter, _ := obs.NewMetricsExporter("taskrunner", reg, obs.ExporterOptions{})
```

Use `TaskSchedulerConfig.Metrics = exporter` to export:
- task duration histogram
- task panic/rejection counters
- queue depth gauges

For runner/pool `Stats()` snapshots, attach `SnapshotPoller`.
See [examples/prometheus_metrics](examples/prometheus_metrics/main.go).

### 7. JobManager (Durable Ack + Pluggable Store)

`JobManager` is implemented in the `core` package for durable job workflows.

```go
import (
    "context"

    taskrunner "github.com/Swind/go-task-runner"
    "github.com/Swind/go-task-runner/core"
)

func setupJobManager() *core.JobManager {
    controlRunner := taskrunner.NewSequencedTaskRunner(taskrunner.GlobalThreadPool())
    ioRunner := taskrunner.NewSequencedTaskRunner(taskrunner.GlobalThreadPool())
    executionRunner := taskrunner.NewParallelTaskRunner(taskrunner.GlobalThreadPool(), 4)

    // Default in-memory implementation supports DurableJobStore.CreateJob.
    store := core.NewMemoryJobStore()
    serializer := core.NewJSONSerializer()

    return core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)
}

func submit(ctx context.Context, jm *core.JobManager) error {
    if err := core.RegisterHandler(jm, "send_email", func(ctx context.Context, args struct {
        To      string `json:"to"`
        Subject string `json:"subject"`
    }) error {
        return nil
    }); err != nil {
        return err
    }

    return jm.SubmitJob(ctx, "job-1", "send_email", struct {
        To      string `json:"to"`
        Subject string `json:"subject"`
    }{
        To:      "user@example.com",
        Subject: "Hello",
    }, taskrunner.DefaultTaskTraits())
}
```

Key behavior:
- `SubmitJob` is durable-ack: it returns success only after persistence confirms durability.
- If the store implements `DurableJobStore`, JobManager uses `CreateJob` for atomic create semantics.
- Otherwise, JobManager falls back to compatibility mode (`GetJob` + `SaveJob`).

See [docs/JOB_MANAGER.md](docs/JOB_MANAGER.md) for architecture details.

## Example Programs

- [examples/basic_sequence](examples/basic_sequence/main.go): basic sequenced execution and delayed task.
- [examples/delayed_task](examples/delayed_task/main.go): delayed task scheduling.
- [examples/repeating_task](examples/repeating_task/main.go): repeating tasks and stop semantics.
- [examples/task_and_reply](examples/task_and_reply/main.go): task-reply patterns (including generic result helpers in `core`).
- [examples/single_thread](examples/single_thread/main.go): thread affinity and lock-free state on single runner ownership.
- [examples/mixed_priority](examples/mixed_priority/main.go): priority behavior across competing runners.
- [examples/shutdown](examples/shutdown/main.go): runner lifecycle and shutdown behavior.
- [examples/custom_handlers](examples/custom_handlers/main.go): custom panic/metrics/rejection handlers.
- [examples/event_bus](examples/event_bus/main.go): sequenced-runner event bus pattern.
- [examples/parallel_tasks](examples/parallel_tasks/main.go): parallel runner features and concurrency limits.
- [examples/prometheus_metrics](examples/prometheus_metrics/main.go): Prometheus exporter and snapshot polling.

## Architecture

See [DESIGN.md](docs/DESIGN.md) for a deep dive into the internal architecture, including how the `TaskScheduler`, `DelayManager`, and `TaskQueue` interact.
For JobManager specifics, see [JOB_MANAGER.md](docs/JOB_MANAGER.md).

## License

MIT License. See [LICENSE](LICENSE) for details.
