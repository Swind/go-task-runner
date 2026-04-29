# Lifecycle Management

Complete guide to proper shutdown, cleanup, and lifecycle patterns.

## Overview

**Critical**: Always clean up runners to prevent resource leaks.

**Key Methods**:
- `Shutdown()` - SequencedTaskRunner, ParallelTaskRunner
- `Stop()` - SingleThreadTaskRunner
- `ShutdownGlobalThreadPool()` - Global pool cleanup
- `IsClosed()` - Check if runner is closed

---

## Shutdown Patterns

### SequencedTaskRunner Shutdown

```go
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
defer runner.Shutdown()  // Always cleanup

// Post tasks...
runner.PostTask(...)

// Shutdown:
// 1. Stops accepting new tasks
// 2. Clears pending queue
// 3. Stops all repeating tasks
// 4. Does NOT interrupt currently executing task
```

### SingleThreadTaskRunner Stop

```go
runner := taskrunner.NewSingleThreadTaskRunner()
defer runner.Stop()  // Note: Stop(), not Shutdown()

// Post tasks...
runner.PostTask(...)

// Stop:
// 1. Stops accepting new tasks
// 2. Waits for current task to complete
// 3. Terminates dedicated goroutine
```

### ParallelTaskRunner Shutdown

```go
runner := taskrunner.NewParallelTaskRunner(pool, 50)
defer runner.Shutdown()

// Post tasks...
runner.PostTask(...)

// Shutdown:
// 1. Stops accepting new tasks
// 2. Clears pending queue
// 3. Stops repeating tasks
// 4. Does NOT wait for active tasks
```

### Global Thread Pool Shutdown

```go
func main() {
    taskrunner.InitGlobalThreadPool(4)
    defer taskrunner.ShutdownGlobalThreadPool()  // Cleanup pool

    // Create runners and post tasks...

    // Shutdown:
    // 1. Stops all workers
    // 2. Waits for active tasks to complete
    // 3. Prevents new task posts
}
```

---

## IsClosed Checking

### Check Before Posting

```go
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

// Check if closed
if runner.IsClosed() {
    fmt.Println("Runner is closed, cannot post task")
    return
}

runner.PostTask(fn)
```

### Inside Task

```go
runner.PostTask(func(ctx context.Context) {
    // Check if shutdown was requested
    if runner.IsClosed() {
        fmt.Println("Shutdown requested, aborting")
        return
    }

    // Continue work...
})
```

---

## Repeating Task Lifecycle

### Manual Stop

```go
handle := runner.PostRepeatingTask(func(ctx context.Context) {
    // Periodic work
    fmt.Println("Tick")
}, 100*time.Millisecond)

// Later...
handle.Stop()  // Stops repeating task
```

### Auto-Stop on Shutdown

```go
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

handle := runner.PostRepeatingTask(fn, interval)

// Shutdown auto-stops all repeating tasks
runner.Shutdown()  // handle.Stop() called automatically
```

### Self-Stopping Task

```go
count := 0
handle := runner.PostRepeatingTask(func(ctx context.Context) {
    count++
    fmt.Printf("Execution #%d\n", count)

    if count >= 10 {
        handle.Stop()  // Stop self after 10 executions
    }
}, 100*time.Millisecond)
```

---

## WaitIdle and FlushAsync

### WaitIdle (ParallelTaskRunner)

```go
runner := taskrunner.NewParallelTaskRunner(pool, 50)

// Post many tasks
for i := 0; i < 1000; i++ {
    runner.PostTask(...)
}

// Wait until all tasks complete
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := runner.WaitIdle(ctx); err != nil {
    fmt.Printf("Timeout waiting for idle: %v\n", err)
}

fmt.Println("All tasks completed")
```

### FlushAsync (Barrier Pattern)

```go
runner := taskrunner.NewParallelTaskRunner(pool, 10)

// Batch 1
for i := 0; i < 100; i++ {
    runner.PostTask(batch1Task)
}

// Barrier - callback runs after batch 1 completes
runner.FlushAsync(func() {
    fmt.Println("Batch 1 done, starting batch 2")
})

// Batch 2
for i := 0; i < 100; i++ {
    runner.PostTask(batch2Task)
}
```

---

## Module Lifecycle Pattern

### Start/Stop Pattern

```go
type Module struct {
    runner *taskrunner.SequencedTaskRunner
    handle *taskrunner.RepeatingTaskHandle
}

func (m *Module) Start() {
    m.runner = taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

    // Start periodic work
    m.handle = m.runner.PostRepeatingTask(func(ctx context.Context) {
        m.periodicWork()
    }, 1*time.Second)
}

func (m *Module) Stop() {
    // Stop repeating task
    if m.handle != nil {
        m.handle.Stop()
    }

    // Shutdown runner
    if m.runner != nil {
        m.runner.Shutdown()
    }
}
```

### Application Lifecycle

```go
type App struct {
    pool       *taskrunner.GoroutineThreadPool
    uiRunner   *taskrunner.SequencedTaskRunner
    bgRunner   *taskrunner.SequencedTaskRunner
    modules    []*Module
}

func (app *App) Start() error {
    // Initialize thread pool
    app.pool = taskrunner.NewGoroutineThreadPool("AppPool", 8)
    app.pool.Start(context.Background())

    // Create runners
    app.uiRunner = taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
    app.bgRunner = taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

    // Start modules
    for _, module := range app.modules {
        module.Start()
    }

    return nil
}

func (app *App) Stop() {
    // Stop modules (reverse order)
    for i := len(app.modules) - 1; i >= 0; i-- {
        app.modules[i].Stop()
    }

    // Shutdown runners
    if app.bgRunner != nil {
        app.bgRunner.Shutdown()
    }
    if app.uiRunner != nil {
        app.uiRunner.Shutdown()
    }

    // Stop thread pool
    if app.pool != nil {
        app.pool.Stop()
    }
}
```

---

## Graceful Shutdown Sequence

### Best Practice Pattern

```go
func main() {
    // 1. Initialize thread pool FIRST
    taskrunner.InitGlobalThreadPool(4)
    defer taskrunner.ShutdownGlobalThreadPool()  // Cleanup LAST

    // 2. Create runners
    uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTraits())
    defer uiRunner.Shutdown()  // Cleanup before pool

    bgRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())
    defer bgRunner.Shutdown()  // Cleanup before pool

    // 3. Post tasks...

    // 4. Cleanup order (via defer):
    //    a. bgRunner.Shutdown()
    //    b. uiRunner.Shutdown()
    //    c. taskrunner.ShutdownGlobalThreadPool()
}
```

**Cleanup Order** (via defer stack):
1. Runners shutdown (reverse creation order)
2. Thread pool shutdown (last)

---

## Context Cancellation

### With Context

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
defer runner.Shutdown()

runner.PostTask(func(taskCtx context.Context) {
    select {
    case <-ctx.Done():
        fmt.Println("Context cancelled, aborting")
        return
    default:
        // Continue work
    }
})

// Later...
cancel()  // Cancel context
```

### With Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

runner.PostTask(func(taskCtx context.Context) {
    select {
    case <-ctx.Done():
        fmt.Println("Timeout reached")
        return
    case <-time.After(10 * time.Second):
        // Would timeout
    }
})
```

---

## Common Patterns

### Pattern 1: Defer Cleanup

```go
func processData() {
    runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
    defer runner.Shutdown()  // ✅ Always cleanup

    // Work...
}
```

### Pattern 2: Multiple Runners

```go
func main() {
    taskrunner.InitGlobalThreadPool(8)
    defer taskrunner.ShutdownGlobalThreadPool()

    r1 := taskrunner.CreateTaskRunner(traits1)
    defer r1.Shutdown()

    r2 := taskrunner.CreateTaskRunner(traits2)
    defer r2.Shutdown()

    // Cleanup order: r2, r1, pool
}
```

### Pattern 3: Stop Repeating Task on Error

```go
var errorCount int32

handle := runner.PostRepeatingTask(func(ctx context.Context) {
    if err := doWork(); err != nil {
        count := atomic.AddInt32(&errorCount, 1)
        if count > 5 {
            fmt.Println("Too many errors, stopping")
            handle.Stop()
        }
    }
}, interval)
```

---

## Common Mistakes

### ❌ Forgetting Shutdown

```go
// ❌ Bad - resource leak
runner := taskrunner.CreateTaskRunner(traits)
// Forgot defer runner.Shutdown()
```

### ❌ Posting After Shutdown

```go
// ❌ Bad - post after shutdown
runner.Shutdown()
runner.PostTask(fn)  // Silently ignored
```

### ❌ Not Stopping Repeating Tasks

```go
// ❌ Bad - repeating task leaks
handle := runner.PostRepeatingTask(fn, interval)
// Forgot handle.Stop() or runner.Shutdown()
```

---

## Summary

**Always Cleanup**:
- Use `defer runner.Shutdown()` for Sequenced/Parallel runners
- Use `defer runner.Stop()` for SingleThread runners
- Use `defer taskrunner.ShutdownGlobalThreadPool()`

**Repeating Tasks**:
- Manual stop: `handle.Stop()`
- Auto-stop: `runner.Shutdown()` stops all

**Graceful Shutdown**:
1. Runners shutdown (reverse order)
2. Thread pool shutdown (last)

**See Also**:
- [pitfalls.md](pitfalls.md) - Common lifecycle mistakes
- [../SKILL.md](../SKILL.md) - Quick reference
