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

## Installation

```bash
go get github.com/Swind/go-task-runner
```

## Usage

Note: The snippets below focus on specific APIs. For complete runnable programs, see `examples/*`.

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

## Architecture

See [DESIGN.md](docs/DESIGN.md) for a deep dive into the internal architecture, including how the `TaskScheduler`, `DelayManager`, and `TaskQueue` interact.

## License

MIT License. See [LICENSE](LICENSE) for details.
