---
name: go-task-runner
description: Expert in using go-task-runner library for lock-free concurrent programming with virtual threads
tools: Read, Write, Edit, Bash, Glob, Grep
---

# go-task-runner Expert

Expert guidance for using the go-task-runner library - a Chromium-inspired task execution framework for lock-free concurrent programming.

## Overview

**Core Philosophy**: Post tasks to virtual threads (TaskRunners) instead of managing raw goroutines and channels.

**Key Benefits**:
- 🔒 **Lock-free patterns** - SequencedTaskRunner eliminates mutex needs
- 📋 **Sequential guarantees** - FIFO execution without race conditions
- 🎯 **Thread affinity** - SingleThreadTaskRunner for blocking operations
- ⚡ **Controlled parallelism** - ParallelTaskRunner with concurrency limits
- 🔄 **Task and Reply** - Clean UI/background work patterns

**⚠️ Educational Library**: This is for learning/experimentation only, NOT production use.

## Quick Decision Tree

```
What do you need?
├─ Lock-free state management          → SequencedTaskRunner ⭐
├─ Sequential tasks (FIFO, no locks)   → SequencedTaskRunner
├─ Thread affinity (blocking IO, CGO)  → SingleThreadTaskRunner
├─ Controlled parallelism (max N)      → ParallelTaskRunner
├─ UI + Background work                → Task and Reply pattern
└─ Periodic work                       → PostRepeatingTask
```

## Initialization Patterns

### Pattern 1: Global Thread Pool (Recommended)

✅ **Use when**: Building application with shared worker pool

```go
package main

import (
    "context"
    taskrunner "github.com/Swind/go-task-runner"
)

func main() {
    // Initialize with 4 workers
    taskrunner.InitGlobalThreadPool(4)
    defer taskrunner.ShutdownGlobalThreadPool()

    // Create runner
    runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
    defer runner.Shutdown()

    // Post tasks
    runner.PostTask(func(ctx context.Context) {
        // Your code here
    })
}
```

### Pattern 2: Custom Thread Pool

✅ **Use when**: Need isolated pools or custom configuration

```go
pool := taskrunner.NewGoroutineThreadPool("MyPool", 8)
pool.Start(context.Background())
defer pool.Stop()

runner := core.NewSequencedTaskRunner(pool)
defer runner.Shutdown()
```

## 🔒 Lock-Free Pattern with SequencedTaskRunner

**The killer feature**: Replace mutexes with sequential execution guarantees.

### ❌ Before: Using Mutexes

```go
type Counter struct {
    mu    sync.Mutex
    count int
}

func (c *Counter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

func (c *Counter) Get() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.count
}
```

### ✅ After: Using SequencedTaskRunner

```go
type Counter struct {
    runner *taskrunner.SequencedTaskRunner
    count  int  // No mutex needed!
}

func NewCounter() *Counter {
    return &Counter{
        runner: taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits()),
    }
}

func (c *Counter) Increment() {
    c.runner.PostTask(func(ctx context.Context) {
        c.count++  // Lock-free!
    })
}

func (c *Counter) Get(callback func(int)) {
    c.runner.PostTask(func(ctx context.Context) {
        callback(c.count)  // Lock-free read!
    })
}

func (c *Counter) Shutdown() {
    c.runner.Shutdown()
}
```

**Why it works**: SequencedTaskRunner guarantees FIFO execution - only ONE task runs at a time, eliminating race conditions.

**📚 For detailed lock-free patterns**, see [docs/lock-free-patterns.md](docs/lock-free-patterns.md)

## Runner Quick Reference

### SequencedTaskRunner

**When**: Need sequential execution without locks

**Guarantees**: FIFO order, one task at a time, no race conditions

**Use Cases**:
- Lock-free state management ⭐
- State machines
- Resource ownership
- Event processing

```go
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
defer runner.Shutdown()

runner.PostTask(func(ctx context.Context) {
    // Exclusive access to runner's owned state
})
```

### SingleThreadTaskRunner

**When**: Need thread affinity or blocking operations

**Guarantees**: All tasks on same dedicated goroutine

**Use Cases**:
- Blocking IO (network, file, database)
- CGO with Thread Local Storage (TLS)
- UI thread simulation

```go
runner := taskrunner.NewSingleThreadTaskRunner()
defer runner.Stop()

runner.PostTask(func(ctx context.Context) {
    // Blocking operation - safe on dedicated thread
    db.Query("SELECT ...")
})
```

### ParallelTaskRunner

**When**: Need controlled parallelism

**Guarantees**: Max N tasks run concurrently

**Use Cases**:
- Batch processing
- Rate limiting
- Resource pooling

```go
runner := taskrunner.NewParallelTaskRunner(
    taskrunner.GlobalThreadPool(),
    50,  // Max 50 concurrent tasks
)
defer runner.Shutdown()

for _, item := range items {
    item := item
    runner.PostTask(func(ctx context.Context) {
        processItem(item)
    })
}

runner.WaitIdle(context.Background())
```

## Common Task Operations

### PostTask - Basic Execution

```go
runner.PostTask(func(ctx context.Context) {
    // Your code here
})
```

### PostDelayedTask - Delayed Execution

```go
runner.PostDelayedTask(func(ctx context.Context) {
    // Runs after 1 second
}, 1*time.Second)
```

### PostRepeatingTask - Periodic Execution

```go
handle := runner.PostRepeatingTask(func(ctx context.Context) {
    // Runs every 100ms
}, 100*time.Millisecond)

// Stop when done
defer handle.Stop()
```

**With initial delay**:
```go
handle := runner.PostRepeatingTaskWithInitialDelay(
    func(ctx context.Context) { /* periodic work */ },
    1*time.Second,    // Initial delay
    100*time.Millisecond,  // Interval
)
```

### PostTaskAndReply - UI/Background Pattern

```go
uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
bgRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

uiRunner.PostTask(func(ctx context.Context) {
    me := taskrunner.GetCurrentTaskRunner(ctx)

    bgRunner.PostTaskAndReply(
        func(ctx context.Context) {
            // Heavy work on background runner
            fetchDataFromNetwork()
        },
        func(ctx context.Context) {
            // Update UI on UI runner
            updateUI()
        },
        me,  // Reply to UI runner
    )
})
```

**With result passing**:
```go
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (*Data, error) {
        return fetchData(), nil
    },
    func(ctx context.Context, data *Data, err error) {
        if err != nil {
            showError(err)
            return
        }
        updateUI(data)
    },
    uiRunner,
)
```

## Lifecycle Management

### Shutdown Pattern

```go
func main() {
    taskrunner.InitGlobalThreadPool(4)
    defer taskrunner.ShutdownGlobalThreadPool()  // Always shutdown!

    runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
    defer runner.Shutdown()  // Always shutdown!

    // Your code
}
```

### IsClosed Checking

```go
runner.PostTask(func(ctx context.Context) {
    if runner.IsClosed() {
        return  // Don't process if shutdown
    }
    // Your code
})
```

### Repeating Task Lifecycle

```go
handle := runner.PostRepeatingTask(fn, interval)

// Stop manually
handle.Stop()

// Or shutdown runner - auto-stops all repeating tasks
runner.Shutdown()
```

**📚 For detailed lifecycle patterns**, see [docs/lifecycle.md](docs/lifecycle.md)

## Common Pitfalls

### ❌ Pitfall 1: Forgetting to Initialize Thread Pool

```go
// ❌ Bad - no thread pool initialized
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
runner.PostTask(fn)  // Will block forever!
```

```go
// ✅ Good
taskrunner.InitGlobalThreadPool(4)
defer taskrunner.ShutdownGlobalThreadPool()

runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
```

### ❌ Pitfall 2: Using Mutex with SequencedTaskRunner

```go
// ❌ Bad - mutex is unnecessary and defeats the purpose
type State struct {
    runner *taskrunner.SequencedTaskRunner
    mu     sync.Mutex  // Don't do this!
    value  int
}
```

```go
// ✅ Good - no mutex needed
type State struct {
    runner *taskrunner.SequencedTaskRunner
    value  int  // Protected by sequential execution
}
```

### ❌ Pitfall 3: Not Calling Shutdown

```go
// ❌ Bad - resource leak
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
// Forgot to call Shutdown()
```

```go
// ✅ Good - always cleanup
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
defer runner.Shutdown()
```

### ❌ Pitfall 4: Closure Variable Capture Bug

```go
// ❌ Bad - all tasks see final value of i
for i := 0; i < 10; i++ {
    runner.PostTask(func(ctx context.Context) {
        fmt.Println(i)  // Prints 10 ten times!
    })
}
```

```go
// ✅ Good - capture loop variable
for i := 0; i < 10; i++ {
    i := i  // Capture
    runner.PostTask(func(ctx context.Context) {
        fmt.Println(i)  // Prints 0, 1, 2, ..., 9
    })
}
```

### ❌ Pitfall 5: Wrong Runner Choice

```go
// ❌ Bad - using SingleThread for CPU work
runner := taskrunner.NewSingleThreadTaskRunner()
for _, item := range millionItems {
    runner.PostTask(func(ctx context.Context) {
        cpuIntensiveWork(item)  // Blocks single thread!
    })
}
```

```go
// ✅ Good - use Parallel for CPU work
runner := taskrunner.NewParallelTaskRunner(pool, 50)
for _, item := range millionItems {
    item := item
    runner.PostTask(func(ctx context.Context) {
        cpuIntensiveWork(item)  // 50 concurrent!
    })
}
```

**📚 For complete pitfalls list**, see [docs/pitfalls.md](docs/pitfalls.md)

## Quick Checklist

### Before Using go-task-runner:
- [ ] Called `InitGlobalThreadPool()` or created custom pool
- [ ] Chosen correct runner type for use case
- [ ] Understand shutdown semantics
- [ ] Using `defer` for cleanup

### Before Committing Code:
- [ ] No goroutine leaks (check defer statements)
- [ ] No data races (use SequencedTaskRunner for shared state)
- [ ] Proper shutdown handling
- [ ] Tests pass with `go test -race`
- [ ] No mutexes used with SequencedTaskRunner

## Templates and Code Examples

### Complete Templates (Ready to Copy):
- [`templates/basic-setup.go`](templates/basic-setup.go) - Global thread pool initialization
- [`templates/lock-free-state.go`](templates/lock-free-state.go) ⭐ - Lock-free state management
- [`templates/sequenced-runner.go`](templates/sequenced-runner.go) - SequencedTaskRunner basics
- [`templates/single-thread-runner.go`](templates/single-thread-runner.go) - Thread affinity pattern
- [`templates/parallel-runner.go`](templates/parallel-runner.go) - Controlled parallelism
- [`templates/ui-background.go`](templates/ui-background.go) - UI/Background work pattern

### Detailed Documentation:
- [`docs/lock-free-patterns.md`](docs/lock-free-patterns.md) ⭐ - **How to replace mutexes**
- [`docs/runners.md`](docs/runners.md) - Runner selection and deep dive
- [`docs/task-and-reply.md`](docs/task-and-reply.md) - Task and Reply pattern details
- [`docs/lifecycle.md`](docs/lifecycle.md) - Shutdown and lifecycle management
- [`docs/pitfalls.md`](docs/pitfalls.md) - Common mistakes and anti-patterns

## Integration with Other Skills

- **go-test-doc**: Document tests using go-task-runner patterns
- **golangci-lint**: Catch common mistakes (unused runners, race conditions)
- **go-clean-architecture**: Where runners fit in layered architecture
- **go-cli-architecture**: Using runners in CLI applications

## Quick Examples

### Example 1: Simple Sequential State

```go
type Cache struct {
    runner *taskrunner.SequencedTaskRunner
    data   map[string]string  // No mutex needed!
}

func NewCache() *Cache {
    return &Cache{
        runner: taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits()),
        data:   make(map[string]string),
    }
}

func (c *Cache) Set(key, value string) {
    c.runner.PostTask(func(ctx context.Context) {
        c.data[key] = value  // Lock-free!
    })
}

func (c *Cache) Get(key string, callback func(string, bool)) {
    c.runner.PostTask(func(ctx context.Context) {
        val, ok := c.data[key]  // Lock-free read!
        callback(val, ok)
    })
}
```

### Example 2: Blocking IO with Thread Affinity

```go
type DBService struct {
    runner *taskrunner.SingleThreadTaskRunner
    db     *sql.DB
}

func NewDBService(db *sql.DB) *DBService {
    return &DBService{
        runner: taskrunner.NewSingleThreadTaskRunner(),
        db:     db,
    }
}

func (s *DBService) Query(sql string, callback func(*sql.Rows, error)) {
    s.runner.PostTask(func(ctx context.Context) {
        // Blocking query - safe on dedicated thread
        rows, err := s.db.Query(sql)
        callback(rows, err)
    })
}

func (s *DBService) Stop() {
    s.runner.Stop()
}
```

### Example 3: Batch Processing with Parallelism

```go
func ProcessBatch(items []Item) {
    taskrunner.InitGlobalThreadPool(50)
    defer taskrunner.ShutdownGlobalThreadPool()

    runner := taskrunner.NewParallelTaskRunner(
        taskrunner.GlobalThreadPool(),
        50,  // 50 concurrent tasks
    )
    defer runner.Shutdown()

    var wg sync.WaitGroup
    for _, item := range items {
        item := item
        wg.Add(1)
        runner.PostTask(func(ctx context.Context) {
            defer wg.Done()
            processItem(item)
        })
    }

    wg.Wait()
}
```

## When to Read More

**Basic usage?** → Use templates above ✓

**Need lock-free patterns?** → Read [docs/lock-free-patterns.md](docs/lock-free-patterns.md) ⭐

**Choosing runner?** → Read [docs/runners.md](docs/runners.md)

**Task and Reply?** → Read [docs/task-and-reply.md](docs/task-and-reply.md)

**Lifecycle issues?** → Read [docs/lifecycle.md](docs/lifecycle.md)

**Something wrong?** → Read [docs/pitfalls.md](docs/pitfalls.md)
