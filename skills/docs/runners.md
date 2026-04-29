# Runner Selection and Deep Dive

Complete guide to choosing and using the right TaskRunner for your use case.

## Quick Comparison Table

| Feature | SequencedTaskRunner | SingleThreadTaskRunner | ParallelTaskRunner |
|---------|---------------------|------------------------|---------------------|
| **Execution** | FIFO, sequential | Sequential on dedicated goroutine | Up to N concurrent |
| **Thread Pool** | Uses global pool | Independent goroutine | Uses global pool |
| **Use Case** | Lock-free state | Blocking IO, thread affinity | Batch processing, parallelism |
| **Lock-Free State** | ✅ Yes | ✅ Yes (but overkill) | ❌ No |
| **Blocking IO** | ❌ Blocks workers | ✅ Perfect | ⚠️ OK but wastes slots |
| **CPU Parallelism** | ❌ Sequential only | ❌ Sequential only | ✅ Perfect |
| **Delayed Tasks** | Via DelayManager | Via time.AfterFunc | Via DelayManager |
| **Repeating Tasks** | ✅ Yes | ✅ Yes | ✅ Yes |
| **Priority Support** | ✅ Yes | ❌ No | ✅ Yes |

---

## SequencedTaskRunner

### When to Use

✅ **Perfect for**:
- Lock-free state management (replace mutexes)
- State machines with sequential transitions
- Event processing with FIFO guarantee
- Resource ownership (cache, pool, etc.)
- Order-sensitive operations

❌ **Don't use for**:
- CPU-intensive parallel work
- Blocking IO operations
- Operations that need thread affinity

### How It Works

**FIFO Guarantee Mechanism**:

```
┌─────────────────┐
│  Task Queue     │
│  [T1, T2, T3]   │
└────────┬────────┘
         │
         ▼
    runningCount
    (atomic int32)
         │
         ├─ 0 = idle
         └─ 1 = running/scheduled
         │
         ▼
    ensureRunning()
    CompareAndSwap(0→1)
         │
         ▼
    runLoop()
    ├─ Execute ONE task
    ├─ Set runningCount=0
    └─ Double-check for new tasks
```

**Key Implementation Details**:
- `runningCount` atomic variable (0=idle, 1=running)
- `ensureRunning()` uses CAS to ensure only one runLoop active
- Processes exactly ONE task per runLoop invocation
- Always yields back to scheduler between tasks
- Double-check pattern prevents race conditions

### Priority Behavior

⚠️ **Important**: Priority affects WHEN the sequence gets scheduled by the thread pool, NOT task order within the sequence.

```go
runner := taskrunner.CreateTaskRunner(taskrunner.TraitsUserBlocking())

// All these run in FIFO order, regardless of priority
runner.PostTask(task1)  // 1st
runner.PostTask(task2)  // 2nd
runner.PostTask(task3)  // 3rd
```

**Priority only matters** when multiple runners compete for workers:

```go
lowRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())
highRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsUserBlocking())

// highRunner's runLoop gets scheduled before lowRunner's
lowRunner.PostTask(...)   // Might wait
highRunner.PostTask(...)  // Scheduled first
```

### Example Usage

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
```

---

## SingleThreadTaskRunner

### When to Use

✅ **Perfect for**:
- Blocking IO (database queries, network calls, file operations)
- CGO with Thread Local Storage (TLS)
- UI thread simulation (all updates on same thread)
- Operations requiring thread affinity
- Third-party libraries with thread requirements

❌ **Don't use for**:
- CPU-intensive work (wastes dedicated thread)
- High-throughput parallel operations

### How It Works

**Thread Affinity Design**:

```
┌──────────────────┐
│ Dedicated        │
│ Goroutine        │  ← ONLY this goroutine
│ (GID: 12345)     │    executes ALL tasks
└────────┬─────────┘
         │
         ▼
┌────────────────────┐
│  Task Queue        │
│  [T1, T2, T3, ...] │
└────────────────────┘
         │
         ▼
   Sequential Execution
   (same GID always)
```

**Independent Operation**:
- Does NOT use global thread pool
- Has own dedicated goroutine
- Uses `time.AfterFunc` for delays (not DelayManager)
- Stops via `Stop()` method (not `Shutdown()`)

### Blocking IO Pattern

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
    s.runner.Stop()  // Note: Stop(), not Shutdown()
}
```

### CGO with TLS Pattern

```go
// C library with thread-local state
type CLibWrapper struct {
    runner *taskrunner.SingleThreadTaskRunner
}

func NewCLibWrapper() *CLibWrapper {
    w := &CLibWrapper{
        runner: taskrunner.NewSingleThreadTaskRunner(),
    }

    // Initialize C library on dedicated thread
    w.runner.PostTask(func(ctx context.Context) {
        C.lib_init()  // Thread-local initialization
    })

    return w
}

func (w *CLibWrapper) CallCFunction(data []byte) {
    w.runner.PostTask(func(ctx context.Context) {
        // Always same thread - TLS preserved
        C.lib_process(unsafe.Pointer(&data[0]))
    })
}
```

---

## ParallelTaskRunner

### When to Use

✅ **Perfect for**:
- Batch processing with controlled concurrency
- CPU-intensive parallel work
- Rate limiting (max N operations concurrently)
- Resource pooling (max N connections)
- High-throughput scenarios

❌ **Don't use for**:
- Lock-free state management (use SequencedTaskRunner)
- Thread affinity needs (use SingleThreadTaskRunner)

### How It Works

**Concurrency Control**:

```
┌────────────────────┐
│  Task Queue        │
│  [T1..T100]        │
└──────────┬─────────┘
           │
           ▼
    ┌─────────────────┐
    │ maxConcurrency  │
    │     = 50        │
    └────────┬────────┘
             │
    ┌────────▼────────┐
    │  Active Tasks   │
    │  [T1..T50]      │  ← Max 50 running
    └─────────────────┘
             │
    ┌────────▼────────┐
    │  Thread Pool    │
    │  (50 workers)   │
    └─────────────────┘
```

**Features**:
- `WaitIdle(ctx)` - waits until all tasks complete
- `FlushAsync(callback)` - barrier semantics
- Concurrency limit enforced
- Work-stealing from global pool

### Batch Processing Pattern

```go
func ProcessBatch(items []Item) error {
    runner := taskrunner.NewParallelTaskRunner(
        taskrunner.GlobalThreadPool(),
        50,  // Max 50 concurrent
    )
    defer runner.Shutdown()

    var wg sync.WaitGroup
    var errors atomic.Int32

    for _, item := range items {
        item := item
        wg.Add(1)
        runner.PostTask(func(ctx context.Context) {
            defer wg.Done()
            if err := processItem(item); err != nil {
                errors.Add(1)
            }
        })
    }

    wg.Wait()

    if count := errors.Load(); count > 0 {
        return fmt.Errorf("%d items failed", count)
    }
    return nil
}
```

### FlushAsync Barrier Pattern

```go
runner := taskrunner.NewParallelTaskRunner(pool, 10)

// Batch 1
for i := 0; i < 100; i++ {
    runner.PostTask(batch1Task)
}

// Barrier - callback runs after batch 1 completes
runner.FlushAsync(func() {
    fmt.Println("Batch 1 complete, starting batch 2")
})

// Batch 2
for i := 0; i < 100; i++ {
    runner.PostTask(batch2Task)
}
```

---

## Decision Tree

```
Start
  │
  ├─ Need lock-free state?
  │    └─ Yes → SequencedTaskRunner
  │
  ├─ Blocking IO or thread affinity?
  │    └─ Yes → SingleThreadTaskRunner
  │
  ├─ CPU parallelism with limit?
  │    └─ Yes → ParallelTaskRunner
  │
  └─ Just simple async tasks?
       └─ SequencedTaskRunner (default)
```

---

## Advanced Topics

### Combining Runners

```go
type Service struct {
    uiRunner *taskrunner.SequencedTaskRunner   // UI updates
    dbRunner *taskrunner.SingleThreadTaskRunner // DB queries
    cpuRunner *taskrunner.ParallelTaskRunner    // Heavy computation
}

func NewService(db *sql.DB) *Service {
    return &Service{
        uiRunner:  taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits()),
        dbRunner:  taskrunner.NewSingleThreadTaskRunner(),
        cpuRunner: taskrunner.NewParallelTaskRunner(pool, 50),
    }
}
```

### Runner Lifecycle

```go
// SequencedTaskRunner
runner := taskrunner.CreateTaskRunner(traits)
defer runner.Shutdown()  // Clears queue, stops repeating tasks

// SingleThreadTaskRunner
runner := taskrunner.NewSingleThreadTaskRunner()
defer runner.Stop()  // Stops dedicated goroutine

// ParallelTaskRunner
runner := taskrunner.NewParallelTaskRunner(pool, N)
defer runner.Shutdown()  // Clears queue
```

---

## Summary

| Your Need | Choose |
|-----------|--------|
| Lock-free state | SequencedTaskRunner |
| Blocking IO | SingleThreadTaskRunner |
| CPU parallelism | ParallelTaskRunner |
| Simple async | SequencedTaskRunner |
| Thread affinity | SingleThreadTaskRunner |
| Rate limiting | ParallelTaskRunner |

**See Also**:
- [lock-free-patterns.md](lock-free-patterns.md) - SequencedTaskRunner patterns
- [../templates/](../templates/) - Complete examples
- [pitfalls.md](pitfalls.md) - Common mistakes
