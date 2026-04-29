---
name: go-task-runner
description: Guidance for go-task-runner lock-free virtual-thread patterns in educational/experimental contexts.
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
├─ Periodic work                       → PostRepeatingTask
├─ Typed event publish/subscribe       → EventBus ⭐
├─ Durable background jobs             → JobManager + JobStore
└─ Metrics / panic hooks               → TaskSchedulerConfig
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

runner := taskrunner.NewSequencedTaskRunner(pool)
defer runner.Shutdown()
```

### Pattern 3: Thread Pool with Custom Config (Metrics / Panic Handler)

✅ **Use when**: Need observability, custom panic handling, or rejection hooks

```go
config := &core.TaskSchedulerConfig{
    PanicHandler:        &MyPanicHandler{},
    Metrics:             &MyMetrics{},
    RejectedTaskHandler: &core.DefaultRejectedTaskHandler{},
}

pool := taskrunner.NewGoroutineThreadPoolWithConfig("MyPool", 8, config)
// or priority-based:
pool := taskrunner.NewPriorityGoroutineThreadPoolWithConfig("MyPool", 8, config)
pool.Start(context.Background())
defer pool.Stop()

runner := taskrunner.NewSequencedTaskRunner(pool)
runner.SetName("my-runner")
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

### PostTaskNamed - Named Task (for observability)

```go
runner.PostTaskNamed("validate-token", func(ctx context.Context) {
    // Task name appears in metrics and logs
    validateToken(token)
})
```

### SetName - Runner Name (for observability)

```go
runner := taskrunner.NewSequencedTaskRunner(pool)
runner.SetName("auth-runner")
```

### WaitIdle - Wait for Queue to Drain

Blocks until all tasks posted before this call have completed.

```go
runner.PostTask(task1)
runner.PostTask(task2)

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := runner.WaitIdle(ctx); err != nil {
    log.Printf("WaitIdle: %v", err)
}
// task1 and task2 have completed here
```

Works on `SequencedTaskRunner`, `SingleThreadTaskRunner`, and `ParallelTaskRunner`.

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

## EventBus — Typed Publish/Subscribe

**Import**: `github.com/Swind/go-task-runner/eventbus`

**Key properties**: type-safe, lock-free (built on SequencedTaskRunner), sequential handler delivery.

```go
bus := eventbus.NewEventBus(taskrunner.GlobalThreadPool())
defer bus.Close()

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Subscribe — returns ID for unsubscribing
type UserCreated struct{ ID int; Name string }

subID := eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
    fmt.Printf("User created: %s\n", event.Name)
})

// Subscribe to a second type
type OrderPlaced struct{ UserID int }
eventbus.Subscribe(bus, func(ctx context.Context, event OrderPlaced) {
    fmt.Printf("Order placed by user %d\n", event.UserID)
})

// Publish — non-blocking, enqueues immediately
bus.Publish(context.Background(), UserCreated{ID: 1, Name: "Alice"})

// Reentrant publish from inside a handler is safe
eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
    bus.Publish(ctx, OrderPlaced{UserID: event.ID})  // safe — enqueues without deadlock
})

// Wait until all handlers finish
bus.WaitIdle(ctx)

// Unsubscribe by ID
bus.Unsubscribe(subID)
```

**Handler state is lock-free** — handlers run sequentially, so no mutexes needed:

```go
var count int  // No mutex needed
eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
    count++  // Sequential execution = no race condition
})
```

**📚 Full guide**: [docs/eventbus.md](docs/eventbus.md) | **Template**: [templates/eventbus.go](templates/eventbus.go)

---

## Job Manager — Durable Background Jobs

**Import**: `github.com/Swind/go-task-runner/job`

Persist jobs to memory or SQLite; survive crashes; retry on failure.

```go
// 1. Choose a store
store := job.NewMemoryJobStore()
// or persistent:
// store, _ := job.NewSQLiteJobStore(db)  // import _ "modernc.org/sqlite"

// 2. Create runners
controlRunner  := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{Priority: taskrunner.TaskPriorityUserBlocking})
ioRunner       := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{Priority: taskrunner.TaskPriorityUserVisible})
executionRunner := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{Priority: taskrunner.TaskPriorityBestEffort})

// 3. Build manager
manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, job.NewJSONSerializer())
manager.SetShutdownRunners(true)
manager.SetLogger(job.NewDefaultLogger())

// 4. Register handlers BEFORE Start()
type SendEmailArgs struct{ To, Subject string }
job.RegisterHandler(manager, ctx, "send_email",
    func(ctx context.Context, args SendEmailArgs) error {
        return sendEmail(args.To, args.Subject)
    },
)

// 5. Start (recovers PENDING jobs from store on restart)
manager.Start(ctx)

// 6. Submit jobs
manager.SubmitJob(ctx, "email-001", "send_email",
    SendEmailArgs{To: "alice@example.com", Subject: "Hi"},
    taskrunner.DefaultTaskTraits(),
)

// 7. List / monitor
jobs, _ := store.ListJobs(ctx, job.JobFilter{})
for _, j := range jobs {
    fmt.Printf("id=%s status=%s\n", j.ID, j.Status)
}

// 8. Shutdown
manager.Shutdown(ctx)
```

**Job statuses**: `PENDING → RUNNING → COMPLETED | FAILED | CANCELLED`

**📚 Full guide**: [docs/job-manager.md](docs/job-manager.md) | **Template**: [templates/job-manager.go](templates/job-manager.go)

---

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

### EventBus:
- [ ] `defer bus.Close()` called
- [ ] `bus.WaitIdle(ctx)` used when synchronization is needed
- [ ] No mutexes inside handlers (handlers are sequential)

### Job Manager:
- [ ] Handlers registered BEFORE `manager.Start(ctx)`
- [ ] `manager.SetShutdownRunners(true)` if manager owns runners
- [ ] Job IDs are unique and deterministic (for idempotency)
- [ ] `manager.Start(ctx)` called to recover PENDING jobs on restart

## Templates and Code Examples

### Complete Templates (Ready to Copy):
- [`templates/basic-setup.go`](templates/basic-setup.go) - Global thread pool initialization
- [`templates/lock-free-state.go`](templates/lock-free-state.go) ⭐ - Lock-free state management
- [`templates/sequenced-runner.go`](templates/sequenced-runner.go) - SequencedTaskRunner basics
- [`templates/single-thread-runner.go`](templates/single-thread-runner.go) - Thread affinity pattern
- [`templates/parallel-runner.go`](templates/parallel-runner.go) - Controlled parallelism
- [`templates/ui-background.go`](templates/ui-background.go) - UI/Background work pattern
- [`templates/eventbus.go`](templates/eventbus.go) ⭐ - EventBus publish/subscribe
- [`templates/job-manager.go`](templates/job-manager.go) - Durable background jobs

### Detailed Documentation:
- [`docs/lock-free-patterns.md`](docs/lock-free-patterns.md) ⭐ - **How to replace mutexes**
- [`docs/runners.md`](docs/runners.md) - Runner selection and deep dive
- [`docs/task-and-reply.md`](docs/task-and-reply.md) - Task and Reply pattern details
- [`docs/lifecycle.md`](docs/lifecycle.md) - Shutdown and lifecycle management
- [`docs/pitfalls.md`](docs/pitfalls.md) - Common mistakes and anti-patterns
- [`docs/eventbus.md`](docs/eventbus.md) ⭐ - EventBus typed pub/sub
- [`docs/job-manager.md`](docs/job-manager.md) - Durable job processing with persistence
- [`docs/observability.md`](docs/observability.md) - Metrics, panic handlers, Prometheus

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

**Event pub/sub?** → Read [docs/eventbus.md](docs/eventbus.md) ⭐

**Background jobs / persistence?** → Read [docs/job-manager.md](docs/job-manager.md)

**Metrics / observability?** → Read [docs/observability.md](docs/observability.md)
