# Observability and Custom Handlers

Configure custom panic handling, metrics collection, and task rejection policies via `TaskSchedulerConfig`.

## Import

```go
import (
    taskrunner "github.com/Swind/go-task-runner"
    "github.com/Swind/go-task-runner/core"
)
```

---

## TaskSchedulerConfig

Pass a config when creating a thread pool:

```go
config := &core.TaskSchedulerConfig{
    PanicHandler:        myPanicHandler,     // optional
    Metrics:             myMetrics,          // optional
    RejectedTaskHandler: myRejectedHandler,  // optional
}

pool := taskrunner.NewGoroutineThreadPoolWithConfig("my-pool", 4, config)
// or for priority-based scheduling:
pool := taskrunner.NewPriorityGoroutineThreadPoolWithConfig("my-pool", 4, config)
```

All fields are optional — omitted fields use the default no-op behavior.

---

## Panic Handler

Implement `core.PanicHandler` to receive panic notifications:

```go
type core.PanicHandler interface {
    HandlePanic(ctx context.Context, runnerName string, workerID int, panicInfo any, stackTrace []byte)
}
```

`workerID >= 0` means the panic came from a thread pool worker; `workerID == -1` means it came directly from the runner.

### Example

```go
type AlertingPanicHandler struct{}

func (h *AlertingPanicHandler) HandlePanic(
    ctx context.Context,
    runnerName string,
    workerID int,
    panicInfo any,
    stackTrace []byte,
) {
    log.Printf("PANIC in %s (worker %d): %v\n%s", runnerName, workerID, panicInfo, stackTrace)
    alerting.Send(fmt.Sprintf("panic in %s: %v", runnerName, panicInfo))
}
```

Default: `core.DefaultPanicHandler{}` — logs to stderr.

---

## Metrics

Implement `core.Metrics` to hook into task execution events:

```go
type core.Metrics interface {
    RecordTaskDuration(runnerName string, priority core.TaskPriority, duration time.Duration)
    RecordTaskPanic(runnerName string, panicInfo any)
    RecordQueueDepth(runnerName string, depth int)
    RecordTaskRejected(runnerName string, reason string)
}
```

### Example: In-Memory Metrics

```go
type InMemoryMetrics struct {
    mu         sync.Mutex
    taskCount  int64
    panicCount int64
}

func (m *InMemoryMetrics) RecordTaskDuration(runnerName string, _ core.TaskPriority, duration time.Duration) {
    m.mu.Lock()
    m.taskCount++
    m.mu.Unlock()
    if duration > 100*time.Millisecond {
        log.Printf("slow task in %s: %v", runnerName, duration)
    }
}

func (m *InMemoryMetrics) RecordTaskPanic(runnerName string, panicInfo any) {
    m.mu.Lock()
    m.panicCount++
    m.mu.Unlock()
}

func (m *InMemoryMetrics) RecordQueueDepth(runnerName string, depth int) {
    if depth > 100 {
        log.Printf("high queue depth in %s: %d", runnerName, depth)
    }
}

func (m *InMemoryMetrics) RecordTaskRejected(runnerName string, reason string) {
    log.Printf("task rejected in %s: %s", runnerName, reason)
}
```

---

## Prometheus Metrics

```go
import (
    obs "github.com/Swind/go-task-runner/observability/prometheus"
    prom "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

reg := prom.NewRegistry()

exporter, err := obs.NewMetricsExporter("taskrunner", reg, obs.ExporterOptions{})
if err != nil { panic(err) }

config := &core.TaskSchedulerConfig{
    Metrics: exporter,
}

pool := taskrunner.NewPriorityGoroutineThreadPoolWithConfig("app-pool", 4, config)
pool.Start(context.Background())
defer pool.Stop()

// Expose metrics
http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
go http.ListenAndServe(":2112", nil)
```

Exposed metrics (prefixed with your namespace, e.g. `taskrunner_`):
- `taskrunner_task_duration_seconds` — histogram of task execution time
- `taskrunner_task_panic_total` — counter of panics by runner
- `taskrunner_queue_depth` — gauge of queue depth by runner
- `taskrunner_task_rejected_total` — counter of rejected tasks

---

## Rejected Task Handler

Called when a task is rejected (e.g. posted to a closed runner):

```go
type core.RejectedTaskHandler interface {
    HandleRejectedTask(runnerName string, reason string)
}
```

### Example

```go
type DeadLetterHandler struct {
    queue chan string
}

func (h *DeadLetterHandler) HandleRejectedTask(runnerName string, reason string) {
    h.queue <- fmt.Sprintf("%s: %s", runnerName, reason)
}
```

Default: `core.DefaultRejectedTaskHandler{}` — logs to stderr.

---

## Named Tasks

Set a display name on a runner or use named task posting for better observability:

```go
runner := taskrunner.NewSequencedTaskRunner(pool)
runner.SetName("auth-runner")

// Named task — visible in metrics and logs
runner.PostTaskNamed("validate-token", func(ctx context.Context) {
    validateToken(token)
})
```

---

## Queue Depth Monitoring

```go
// Check current queue depth on the pool
depth := pool.QueuedTaskCount()
fmt.Printf("Queued tasks: %d\n", depth)
```

---

## Complete Example

```go
func main() {
    metrics := &InMemoryMetrics{}
    panicHandler := &AlertingPanicHandler{}

    config := &core.TaskSchedulerConfig{
        PanicHandler:        panicHandler,
        Metrics:             metrics,
        RejectedTaskHandler: &core.DefaultRejectedTaskHandler{},
    }

    pool := taskrunner.NewGoroutineThreadPoolWithConfig("app-pool", 4, config)
    pool.Start(context.Background())
    defer pool.Stop()

    runner := taskrunner.NewSequencedTaskRunner(pool)
    runner.SetName("my-runner")

    runner.PostTaskNamed("heavy-work", func(ctx context.Context) {
        doHeavyWork()
    })

    if err := runner.WaitIdle(context.Background()); err != nil {
        log.Printf("WaitIdle: %v", err)
    }
}
```

---

## See Also

- [`runners.md`](runners.md) — runner types and selection
- [`lifecycle.md`](lifecycle.md) — shutdown patterns
- [Prometheus example](../../examples/prometheus_metrics/main.go)
- [Custom handlers example](../../examples/custom_handlers/main.go)
