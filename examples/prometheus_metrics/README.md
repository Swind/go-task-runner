# Prometheus Metrics Example

This example shows how to:

- Wire `core.TaskSchedulerConfig.Metrics` to the Prometheus exporter.
- Poll runner/pool `Stats()` into Prometheus gauges.
- Expose `/metrics` for scraping.

## Run

```bash
go run ./examples/prometheus_metrics
```

While it is running, scrape metrics:

```bash
curl -s http://127.0.0.1:2112/metrics | grep '^taskrunner_'
```

The demo keeps the server alive for about 2 seconds.

## What to look at

- `taskrunner_task_duration_seconds`: task execution latency histogram.
- `taskrunner_task_panic_total`: count of panics during task execution.
- `taskrunner_task_rejected_total`: count of rejected tasks.
- `taskrunner_queue_depth`: scheduler/runner queue depth.
- `taskrunner_runner_pending`: pending tasks from `RunnerStats`.
- `taskrunner_runner_running`: running tasks from `RunnerStats`.
- `taskrunner_pool_queued`: queued tasks from `PoolStats`.
- `taskrunner_pool_active`: active tasks from `PoolStats`.
- `taskrunner_pool_delayed`: delayed tasks from `PoolStats`.

## PromQL examples

```promql
sum(rate(taskrunner_task_duration_seconds_count[5m])) by (runner)
```

```promql
max(taskrunner_queue_depth) by (runner)
```

```promql
taskrunner_pool_active{pool="metrics-pool"}
```
