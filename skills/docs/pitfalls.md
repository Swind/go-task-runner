# Common Pitfalls and Anti-Patterns

Learn from common mistakes to avoid them in your code.

## Table of Contents
1. [Initialization Mistakes](#initialization-mistakes)
2. [Lock Usage Mistakes](#lock-usage-mistakes)
3. [Shutdown Mistakes](#shutdown-mistakes)
4. [Runner Choice Mistakes](#runner-choice-mistakes)
5. [Task Posting Mistakes](#task-posting-mistakes)

---

## Initialization Mistakes {#initialization-mistakes}

### ❌ Pitfall 1: Forgetting InitGlobalThreadPool

```go
// ❌ Bad - no thread pool initialized
func main() {
    runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

    runner.PostTask(func(ctx context.Context) {
        fmt.Println("This will never execute!")
    })

    time.Sleep(1 * time.Second)  // Waiting forever...
}
```

**Problem**: Tasks posted to runners using global pool will never execute.

**Symptom**: Program hangs, tasks never run.

```go
// ✅ Good
func main() {
    taskrunner.InitGlobalThreadPool(4)  // Initialize!
    defer taskrunner.ShutdownGlobalThreadPool()

    runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTraits())
    runner.PostTask(fn)  // Works
}
```

### ❌ Pitfall 2: Wrong Worker Count

```go
// ❌ Bad - too few workers
taskrunner.InitGlobalThreadPool(1)  // Only 1 worker!

// Create 100 runners, but only 1 can run at a time
for i := 0; i < 100; i++ {
    runner := taskrunner.CreateTaskRunner(traits)
    runner.PostTask(...)  // All contend for 1 worker
}
```

**Problem**: Severe underutilization, poor performance.

```go
// ✅ Good - match workload
numCPUs := runtime.NumCPU()
taskrunner.InitGlobalThreadPool(numCPUs)  // Or higher for IO-bound
```

### ❌ Pitfall 3: Multiple InitGlobalThreadPool Calls

```go
// ❌ Bad - re-initializing pool
taskrunner.InitGlobalThreadPool(4)
// ... later ...
taskrunner.InitGlobalThreadPool(8)  // Error! Pool already initialized
```

**Problem**: Panics on second initialization.

```go
// ✅ Good - initialize once
func main() {
    taskrunner.InitGlobalThreadPool(4)
    defer taskrunner.ShutdownGlobalThreadPool()

    // All code uses this pool
}
```

---

## Lock Usage Mistakes {#lock-usage-mistakes}

### ❌ Pitfall 4: Using Mutex with SequencedTaskRunner

```go
// ❌ Bad - mutex defeats the purpose
type Counter struct {
    runner *taskrunner.SequencedTaskRunner
    mu     sync.Mutex  // Unnecessary!
    count  int
}

func (c *Counter) Increment() {
    c.runner.PostTask(func(ctx context.Context) {
        c.mu.Lock()         // Don't need this!
        defer c.mu.Unlock()
        c.count++
    })
}
```

**Problem**: Mutex is unnecessary - SequencedTaskRunner guarantees sequential execution.

```go
// ✅ Good - no mutex needed
type Counter struct {
    runner *taskrunner.SequencedTaskRunner
    count  int  // Protected by sequential execution
}

func (c *Counter) Increment() {
    c.runner.PostTask(func(ctx context.Context) {
        c.count++  // Lock-free!
    })
}
```

### ❌ Pitfall 5: Cross-Runner State Without Locks

```go
// ❌ Bad - state accessed by multiple runners (RACE!)
type App struct {
    runner1 *taskrunner.SequencedTaskRunner
    runner2 *taskrunner.SequencedTaskRunner
    counter int  // RACE CONDITION!
}

func (app *App) IncrementFrom1() {
    app.runner1.PostTask(func(ctx context.Context) {
        app.counter++  // Race!
    })
}

func (app *App) IncrementFrom2() {
    app.runner2.PostTask(func(ctx context.Context) {
        app.counter++  // Race!
    })
}
```

**Problem**: Two different runners accessing same state = race condition.

```go
// ✅ Good - state owned by ONE runner
type App struct {
    stateRunner *taskrunner.SequencedTaskRunner
    runner2     *taskrunner.SequencedTaskRunner
    counter     int  // Owned by stateRunner only
}

func (app *App) IncrementFrom1() {
    app.stateRunner.PostTask(func(ctx context.Context) {
        app.counter++  // Safe
    })
}

func (app *App) IncrementFrom2() {
    app.stateRunner.PostTask(func(ctx context.Context) {
        app.counter++  // Safe - same runner
    })
}
```

### ❌ Pitfall 6: Shared State in Parallel Tasks

```go
// ❌ Bad - parallel tasks accessing shared state
runner := taskrunner.NewParallelTaskRunner(pool, 50)

counter := 0  // Shared state

for i := 0; i < 1000; i++ {
    runner.PostTask(func(ctx context.Context) {
        counter++  // RACE! 50 tasks run concurrently
    })
}
```

**Problem**: ParallelTaskRunner allows concurrent execution = race conditions.

```go
// ✅ Good - use atomic or mutex for parallel access
runner := taskrunner.NewParallelTaskRunner(pool, 50)

var counter atomic.Int32

for i := 0; i < 1000; i++ {
    runner.PostTask(func(ctx context.Context) {
        counter.Add(1)  // Atomic operation - safe
    })
}
```

---

## Shutdown Mistakes {#shutdown-mistakes}

### ❌ Pitfall 7: Forgetting to Call Shutdown

```go
// ❌ Bad - resource leak
func processData() {
    runner := taskrunner.CreateTaskRunner(traits)
    // Forgot defer runner.Shutdown()

    runner.PostTask(...)
}  // Runner leaks
```

**Problem**: Runner and associated resources leak.

```go
// ✅ Good
func processData() {
    runner := taskrunner.CreateTaskRunner(traits)
    defer runner.Shutdown()  // Always cleanup

    runner.PostTask(...)
}
```

### ❌ Pitfall 8: Posting After Shutdown

```go
// ❌ Bad - posting to closed runner
runner := taskrunner.CreateTaskRunner(traits)
runner.Shutdown()

runner.PostTask(func(ctx context.Context) {
    // This never executes - silently ignored!
})
```

**Problem**: Tasks posted after shutdown are silently dropped.

```go
// ✅ Good - check before posting
if runner.IsClosed() {
    return fmt.Errorf("runner is closed")
}
runner.PostTask(fn)
```

### ❌ Pitfall 9: Not Stopping Repeating Tasks

```go
// ❌ Bad - repeating task leaks
func startService() {
    runner := taskrunner.CreateTaskRunner(traits)

    handle := runner.PostRepeatingTask(fn, interval)
    // Forgot handle.Stop() or runner.Shutdown()

}  // Repeating task keeps running!
```

**Problem**: Repeating task continues even after function returns.

```go
// ✅ Good - always stop
func startService() {
    runner := taskrunner.CreateTaskRunner(traits)
    defer runner.Shutdown()  // Auto-stops repeating tasks

    handle := runner.PostRepeatingTask(fn, interval)
    // Will stop on shutdown
}
```

---

## Runner Choice Mistakes {#runner-choice-mistakes}

### ❌ Pitfall 10: Using SingleThread for CPU Work

```go
// ❌ Bad - wasting dedicated thread
runner := taskrunner.NewSingleThreadTaskRunner()

for _, item := range millionItems {
    runner.PostTask(func(ctx context.Context) {
        cpuIntensiveWork(item)  // Only 1 at a time!
    })
}
```

**Problem**: SingleThread is sequential - no parallelism for CPU work.

```go
// ✅ Good - use ParallelTaskRunner for CPU work
runner := taskrunner.NewParallelTaskRunner(pool, 50)

for _, item := range millionItems {
    item := item
    runner.PostTask(func(ctx context.Context) {
        cpuIntensiveWork(item)  // 50 concurrent!
    })
}
```

### ❌ Pitfall 11: Using Sequenced for Parallel Work

```go
// ❌ Bad - sequential when you need parallel
runner := taskrunner.CreateTaskRunner(traits)

// Want these to run in parallel, but they won't!
for _, item := range items {
    runner.PostTask(func(ctx context.Context) {
        expensiveOperation(item)  // One at a time
    })
}
```

**Problem**: SequencedTaskRunner is sequential by design.

```go
// ✅ Good - use ParallelTaskRunner
runner := taskrunner.NewParallelTaskRunner(pool, NumCPU)

for _, item := range items {
    item := item
    runner.PostTask(func(ctx context.Context) {
        expensiveOperation(item)  // Parallel execution
    })
}
```

### ❌ Pitfall 12: Blocking the Thread Pool

```go
// ❌ Bad - blocking worker threads
runner := taskrunner.CreateTaskRunner(traits)

runner.PostTask(func(ctx context.Context) {
    // Blocking IO on thread pool worker!
    resp, _ := http.Get("http://slow-api.com")
    // ...
})
```

**Problem**: Blocks thread pool worker, reducing available workers.

```go
// ✅ Good - use SingleThreadTaskRunner for blocking IO
runner := taskrunner.NewSingleThreadTaskRunner()

runner.PostTask(func(ctx context.Context) {
    // Blocking IO on dedicated thread - OK
    resp, _ := http.Get("http://slow-api.com")
    // ...
})
```

---

## Task Posting Mistakes {#task-posting-mistakes}

### ❌ Pitfall 13: Closure Variable Capture Bug

```go
// ❌ Bad - all tasks see final value
for i := 0; i < 10; i++ {
    runner.PostTask(func(ctx context.Context) {
        fmt.Println(i)  // Prints 10 ten times!
    })
}
```

**Problem**: All closures capture the same `i` variable.

```go
// ✅ Good - capture loop variable
for i := 0; i < 10; i++ {
    i := i  // Shadow variable
    runner.PostTask(func(ctx context.Context) {
        fmt.Println(i)  // Prints 0, 1, 2, ..., 9
    })
}
```

### ❌ Pitfall 14: Panic in Task

```go
// ❌ Bad - unchecked panic
runner.PostTask(func(ctx context.Context) {
    panic("something went wrong!")  // Recovered, but no handling
})
```

**Problem**: Panic is recovered but you have no error handling.

```go
// ✅ Good - handle errors properly
runner.PostTask(func(ctx context.Context) {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("Task panicked: %v", r)
            // Handle recovery
        }
    }()

    // Your code
})
```

**Note**: Repeating tasks STOP on panic.

### ❌ Pitfall 15: Long-Running Task Without Cancellation

```go
// ❌ Bad - no way to cancel long task
runner.PostTask(func(ctx context.Context) {
    for i := 0; i < 1000000; i++ {
        expensiveOperation(i)  // Can't interrupt!
    }
})
```

**Problem**: Task can't be interrupted if shutdown is requested.

```go
// ✅ Good - check IsClosed periodically
runner.PostTask(func(ctx context.Context) {
    for i := 0; i < 1000000; i++ {
        if runner.IsClosed() {
            return  // Abort on shutdown
        }
        expensiveOperation(i)
    }
})
```

### ❌ Pitfall 16: Returning Value from Task

```go
// ❌ Bad - can't return value
runner.PostTask(func(ctx context.Context) int {  // Wrong signature!
    return 42  // Can't return
})
```

**Problem**: Tasks have signature `func(context.Context)` - no return value.

```go
// ✅ Good - use callback or channel
resultChan := make(chan int)

runner.PostTask(func(ctx context.Context) {
    result := compute()
    resultChan <- result
})

result := <-resultChan
```

Or use `PostTaskAndReplyWithResult`:

```go
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (int, error) {
        return compute(), nil  // Can return!
    },
    func(ctx context.Context, result int, err error) {
        fmt.Printf("Result: %d\n", result)
    },
    replyRunner,
)
```

---

## Summary

**Top 5 Most Common Mistakes**:

1. ❌ **Forgetting InitGlobalThreadPool** → Program hangs
2. ❌ **Using mutex with SequencedTaskRunner** → Defeats purpose
3. ❌ **Forgetting Shutdown** → Resource leaks
4. ❌ **Closure variable capture** → Wrong values in tasks
5. ❌ **Wrong runner choice** → Poor performance

**Quick Checks**:
- [ ] Did you call `InitGlobalThreadPool()`?
- [ ] Did you use `defer runner.Shutdown()`?
- [ ] Did you capture loop variables (`i := i`)?
- [ ] Did you choose the right runner type?
- [ ] Are you using mutex with SequencedTaskRunner? (Don't!)

**See Also**:
- [../SKILL.md](../SKILL.md) - Quick reference
- [lock-free-patterns.md](lock-free-patterns.md) - Correct patterns
- [runners.md](runners.md) - Runner selection guide
- [lifecycle.md](lifecycle.md) - Shutdown patterns
