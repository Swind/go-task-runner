# Remove SnapshotPoller and executionHistory

## Problem

The project has two distinct metrics subsystems:

1. **Event-driven** (`Metrics` interface + `MetricsExporter`) — tasks record metrics as they execute
2. **Snapshot polling** (`SnapshotPoller`) — a separate goroutine periodically polls `Stats()` into Prometheus gauges

This dual model is confusing. The snapshot polling mechanism adds complexity with minimal value over the event-driven approach.

## Decision

Remove `SnapshotPoller` and the `executionHistory` ring buffer. Keep `Stats()` / `RunnerStats` / `PoolStats` as a lightweight introspection API.

## Scope

### Delete

- `observability/prometheus/snapshot_poller.go` — SnapshotPoller, RunnerSnapshotProvider, PoolSnapshotProvider
- `observability/prometheus/snapshot_poller_test.go`
- `core/task_history.go` — executionHistory, wrapObservedTask, resolveTaskName
- `core/runner_task_history_test.go`
- `examples/prometheus_metrics/README.md` — rewrite

### Modify

- `core/observability.go` — remove `TaskExecutionRecord`; remove `LastTaskName`, `LastTaskAt` from `RunnerStats`
- `core/sequenced_task_runner.go` — remove `history` field, `recordTaskExecution()`, `RecentTasks()`; simplify `Stats()`; remove `wrapObservedTask` call in `runLoop`
- `core/parallel_task_runner.go` — same as above
- `core/single_thread_task_runner.go` — same as above
- `types.go` — remove `TaskExecutionRecord` re-export
- `examples/prometheus_metrics/main.go` — remove SnapshotPoller wiring
- `README.md` — remove SnapshotPoller mention
- `core/runner_observability_test.go` — remove `LastTaskName`/`LastTaskAt` assertions

### Keep unchanged

- `RunnerStats` — Name, Type, Pending, Running, Rejected, Closed, BarrierPending
- `PoolStats` — all fields
- `Stats()` on all three runners and pool
- `MetricsExporter` and event-driven metrics
- `pool.go`

## Affected files

```
DELETE  observability/prometheus/snapshot_poller.go
DELETE  observability/prometheus/snapshot_poller_test.go
DELETE  core/task_history.go
DELETE  core/runner_task_history_test.go
MODIFY  core/observability.go
MODIFY  core/sequenced_task_runner.go
MODIFY  core/parallel_task_runner.go
MODIFY  core/single_thread_task_runner.go
MODIFY  types.go
MODIFY  examples/prometheus_metrics/main.go
DELETE  examples/prometheus_metrics/README.md (rewrite)
MODIFY  README.md
MODIFY  core/runner_observability_test.go
```
