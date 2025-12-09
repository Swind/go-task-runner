# Go Task Runner

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
-   **Delayed Tasks**: Scheduling tasks in the future.
-   **Task Traits**: Priority-aware task scheduling.

## Installation

```bash
go get github.com/Swind/go-task-runner
```

## Usage

### 1. Initialize the Global Thread Pool

```go
package main

import (
    "context"
    "time"

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

### 3. Using Task Traits (Priorities)

```go
    runner.PostTaskWithTraits(func(ctx context.Context) {
        println("High priority work!")
    }, taskrunner.TaskTraits{
        Priority: taskrunner.TaskPriorityUserBlocking,
    })
```

## Architecture

See [DESIGN.md](DESIGN.md) for a deep dive into the internal architecture, including how the `TaskScheduler`, `DelayManager`, and `TaskQueue` interact.

## License

MIT License. See [LICENSE](LICENSE) for details.
