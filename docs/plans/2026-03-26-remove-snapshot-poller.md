# Remove SnapshotPoller and executionHistory — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove the snapshot polling metrics subsystem and task execution history, keeping only the event-driven `Metrics` interface and lightweight `Stats()` introspection.

**Architecture:** Delete `SnapshotPoller` (periodic Stats polling into Prometheus gauges) and `executionHistory` (ring buffer of `TaskExecutionRecord`). Simplify runners by removing history-related fields/methods and `wrapObservedTask` wrapping. `Stats()` remains as a simple struct without `LastTaskName`/`LastTaskAt`.

**Tech Stack:** Go, Prometheus client_golang

---

### Task 1: Delete SnapshotPoller files

**Files:**
- Delete: `observability/prometheus/snapshot_poller.go`
- Delete: `observability/prometheus/snapshot_poller_test.go`

**Step 1: Delete the files**

```bash
rm observability/prometheus/snapshot_poller.go observability/prometheus/snapshot_poller_test.go
```

**Step 2: Verify build still compiles (expect errors from example referencing SnapshotPoller)**

Run: `go build ./...`
Expected: Compile errors only in `examples/prometheus_metrics/main.go` referencing `obs.NewSnapshotPoller`, `poller.AddPool`, etc.

**Step 3: Commit**

```bash
git add -A
git commit -m "refactor(observability): remove SnapshotPoller and its tests"
```

---

### Task 2: Remove TaskExecutionRecord and simplify RunnerStats

**Files:**
- Modify: `core/observability.go`
- Modify: `types.go`

**Step 1: Remove `TaskExecutionRecord` and `LastTaskName`/`LastTaskAt` from `core/observability.go`**

Replace the entire file content with:

```go
package core

type RunnerStats struct {
	Name           string
	Type           string
	Pending        int
	Running        int
	Rejected       int64
	Closed         bool
	BarrierPending bool
}

type PoolStats struct {
	ID      string
	Workers int
	Queued  int
	Active  int
	Delayed int
	Running bool
}
```

Note: The `time` import is no longer needed.

**Step 2: Remove `TaskExecutionRecord` re-export from `types.go`**

In `types.go`, delete line 40:

```go
type TaskExecutionRecord = core.TaskExecutionRecord
```

And update the section comment on line 37 from `"// Observability snapshot types"` to `"// Observability stats types"`.

**Step 3: Verify build**

Run: `go build ./...`
Expected: Errors in runner files referencing `executionHistory`, `wrapObservedTask`, `recordTaskExecution`, `RecentTasks`, `TaskExecutionRecord`.

**Step 4: Commit**

```bash
git add -A
git commit -m "refactor(core): remove TaskExecutionRecord and simplify RunnerStats"
```

---

### Task 3: Remove executionHistory from SequencedTaskRunner

**Files:**
- Modify: `core/sequenced_task_runner.go`

**Step 1: Remove `history` field from struct (line 28)**

Delete this line from the struct:

```go
	history executionHistory
```

**Step 2: Remove `history` initialization from constructor (line 37)**

In `NewSequencedTaskRunner`, delete this line:

```go
		history:      newExecutionHistory(defaultTaskHistoryCapacity),
```

**Step 3: Simplify `Stats()` method (lines 95-109)**

Replace the `Stats()` method with:

```go
func (r *SequencedTaskRunner) Stats() RunnerStats {
	return RunnerStats{
		Name:    r.observabilityName(),
		Type:    "sequenced",
		Pending: r.PendingTaskCount(),
		Running: r.RunningTaskCount(),
		Closed:  r.IsClosed(),
	}
}
```

**Step 4: Remove `RecentTasks()` method (lines 111-114)**

Delete the entire `RecentTasks` method.

**Step 5: Remove `recordTaskExecution()` method (lines 124-126)**

Delete the entire `recordTaskExecution` method.

**Step 6: Remove `wrapObservedTask` call from `PostTaskWithTraitsNamed` (line 275)**

Replace:
```go
	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "sequenced", r.recordTaskExecution)
```
With:
```go
	wrapped := task
```

**Step 7: Remove `wrapObservedTask` call from `PostDelayedTaskWithTraitsNamed` (line 303)**

Replace:
```go
	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "sequenced", r.recordTaskExecution)
```
With:
```go
	wrapped := task
```

**Step 8: Verify build**

Run: `go build ./core/...`
Expected: Errors in parallel and single-thread runners, not in sequenced runner.

**Step 9: Commit**

```bash
git add -A
git commit -m "refactor(core): remove executionHistory from SequencedTaskRunner"
```

---

### Task 4: Remove executionHistory from ParallelTaskRunner

**Files:**
- Modify: `core/parallel_task_runner.go`

**Step 1: Remove `history` field from struct (line 62)**

Delete:
```go
	history executionHistory
```

**Step 2: Remove `history` initialization from constructor (line 92)**

Delete:
```go
		history:        newExecutionHistory(defaultTaskHistoryCapacity),
```

**Step 3: Simplify `Stats()` method (lines 113-127)**

Replace with:

```go
func (r *ParallelTaskRunner) Stats() RunnerStats {
	return RunnerStats{
		Name:           r.observabilityName(),
		Type:           "parallel",
		Pending:        r.PendingTaskCount(),
		Running:        r.RunningTaskCount(),
		Closed:         r.IsClosed(),
		BarrierPending: r.barrierTaskRunning.Load(),
	}
}
```

**Step 4: Remove `RecentTasks()` method (lines 129-132)**

Delete the entire method.

**Step 5: Remove `recordTaskExecution()` method (lines 142-144)**

Delete the entire method.

**Step 6: Remove `wrapObservedTask` call from `PostTaskWithTraitsNamed` (line 240)**

Replace:
```go
	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "parallel", r.recordTaskExecution)
```
With:
```go
	wrapped := task
```

**Step 7: Remove `wrapObservedTask` call from `PostDelayedTaskWithTraitsNamed` (line 277)**

Replace:
```go
	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "parallel", r.recordTaskExecution)
```
With:
```go
	wrapped := task
```

**Step 8: Verify build**

Run: `go build ./core/...`
Expected: Errors only in single-thread runner.

**Step 9: Commit**

```bash
git add -A
git commit -m "refactor(core): remove executionHistory from ParallelTaskRunner"
```

---

### Task 5: Remove executionHistory from SingleThreadTaskRunner

**Files:**
- Modify: `core/single_thread_task_runner.go`

**Step 1: Remove `history` field from struct (line 66)**

Delete:
```go
	history executionHistory
```

**Step 2: Remove `history` initialization from constructor (line 81)**

Delete:
```go
		history:      newExecutionHistory(defaultTaskHistoryCapacity),
```

**Step 3: Simplify `Stats()` method (lines 140-154)**

Replace with:

```go
func (r *SingleThreadTaskRunner) Stats() RunnerStats {
	return RunnerStats{
		Name:     r.observabilityName(),
		Type:     "single-thread",
		Pending:  r.PendingTaskCount(),
		Running:  r.RunningTaskCount(),
		Rejected: r.RejectedCount(),
		Closed:   r.IsClosed(),
	}
}
```

**Step 4: Remove `RecentTasks()` method (lines 156-159)**

Delete the entire method.

**Step 5: Remove `recordTaskExecution()` method (lines 169-171)**

Delete the entire method.

**Step 6: Remove `wrapObservedTask` call from `PostTaskWithTraitsNamed` (line 269)**

Replace:
```go
	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "single-thread", r.recordTaskExecution)
```
With:
```go
	wrapped := task
```

**Step 7: Remove `wrapObservedTask` call from `PostDelayedTaskWithTraitsNamed` (line 296)**

Replace:
```go
	wrapped := wrapObservedTask(task, name, traits, r.observabilityName(), "single-thread", r.recordTaskExecution)
```
With:
```go
	wrapped := task
```

**Step 8: Delete `core/task_history.go`**

```bash
rm core/task_history.go
```

**Step 9: Verify build**

Run: `go build ./...`
Expected: Clean build (only the example still needs fixing).

**Step 10: Commit**

```bash
git add -A
git commit -m "refactor(core): remove executionHistory from SingleThreadTaskRunner and delete task_history.go"
```

---

### Task 6: Update example and docs

**Files:**
- Modify: `examples/prometheus_metrics/main.go`
- Delete+Rewrite: `examples/prometheus_metrics/README.md`
- Modify: `README.md`

**Step 1: Update `examples/prometheus_metrics/main.go`**

Remove all SnapshotPoller-related code. The final file should look like:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
	obs "github.com/Swind/go-task-runner/observability/prometheus"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	reg := prom.NewRegistry()

	exporter, err := obs.NewMetricsExporter("taskrunner", reg, obs.ExporterOptions{})
	if err != nil {
		panic(err)
	}

	config := &core.TaskSchedulerConfig{
		PanicHandler:        &core.DefaultPanicHandler{},
		Metrics:             exporter,
		RejectedTaskHandler: &core.DefaultRejectedTaskHandler{},
	}

	pool := taskrunner.NewPriorityGoroutineThreadPoolWithConfig("metrics-pool", 2, config)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := taskrunner.NewSequencedTaskRunner(pool)
	runner.SetName("metrics-seq")

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	server := &http.Server{Addr: ":2112", Handler: mux}

	go func() {
		_ = server.ListenAndServe()
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	for i := range 8 {
		runner.PostTaskNamed(fmt.Sprintf("demo-task-%d", i), func(ctx context.Context) {
			time.Sleep(5 * time.Millisecond)
		})
	}

	if err := runner.WaitIdle(context.Background()); err != nil {
		panic(err)
	}

	fmt.Println("Prometheus endpoint is up at http://127.0.0.1:2112/metrics")
	fmt.Println("Try: curl -s http://127.0.0.1:2112/metrics | grep '^taskrunner_'")

	time.Sleep(2 * time.Second)
}
```

**Step 2: Rewrite `examples/prometheus_metrics/README.md`**

Replace with:

```markdown
# Prometheus Metrics Example

This example shows how to wire `core.TaskSchedulerConfig.Metrics` to the Prometheus exporter and expose `/metrics` for scraping.

## Run

```bash
go run ./examples/prometheus_metrics
```

While it is running, scrape metrics:

```bash
curl -s http://127.0.0.1:2112/metrics | grep '^taskrunner_'
```

The demo keeps the server alive for about 2 seconds.

## Available metrics

- `taskrunner_task_duration_seconds`: task execution latency histogram.
- `taskrunner_task_panic_total`: count of panics during task execution.
- `taskrunner_task_rejected_total`: count of rejected tasks.
- `taskrunner_queue_depth`: scheduler/runner queue depth.

## PromQL examples

```promql
sum(rate(taskrunner_task_duration_seconds_count[5m])) by (runner)
```

```promql
max(taskrunner_queue_depth) by (runner)
```
```

**Step 3: Update `README.md` line 258**

Replace:
```
For runner/pool `Stats()` snapshots, attach `SnapshotPoller`.
See [examples/prometheus_metrics](examples/prometheus_metrics/main.go).
```
With:
```
See [examples/prometheus_metrics](examples/prometheus_metrics/main.go) for a full wiring example.
```

**Step 4: Verify example runs**

Run: `go run ./examples/prometheus_metrics`
Expected: Prints "Prometheus endpoint is up at http://127.0.0.1:2112/metrics" and exits after ~2s.

**Step 5: Commit**

```bash
git add -A
git commit -m "docs(examples): update prometheus_metrics example after SnapshotPoller removal"
```

---

### Task 7: Update and fix tests

**Files:**
- Delete: `core/runner_task_history_test.go`
- Modify: `core/runner_observability_test.go`

**Step 1: Delete history test file**

```bash
rm core/runner_task_history_test.go
```

**Step 2: Verify runner observability tests still pass**

Run: `go test -v -run "TestSequencedTaskRunner_Stats|TestParallelTaskRunner_Stats|TestSingleThreadTaskRunner_Stats" ./core/`
Expected: All PASS. No changes needed to `runner_observability_test.go` since it only checks `Name`, `Type`, `Pending`, `Running`, `Closed`, `Rejected` — all of which are preserved.

**Step 3: Run full test suite**

Run: `go test ./...`
Expected: All PASS.

**Step 4: Run CI-mode tests**

Run: `go test -tags=ci -v -coverprofile=coverage.out ./...`
Expected: All PASS.

**Step 5: Commit**

```bash
git add -A
git commit -m "test(core): remove runner_task_history_test.go, verify remaining tests pass"
```

---

### Task 8: Final verification

**Step 1: Lint**

Run: `golangci-lint run --timeout=5m core/... .`
Expected: No errors.

**Step 2: Run all examples**

Run: `find examples -name "main.go" | sort | xargs -n1 go run`
Expected: All examples run without error.

**Step 3: Final commit if any lint fixes needed**

```bash
git add -A
git commit -m "chore: lint fixes after SnapshotPoller removal"
```
