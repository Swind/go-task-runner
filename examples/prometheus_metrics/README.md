# Prometheus Metrics Example

This example shows how to:

- Wire `core.TaskSchedulerConfig.Metrics` to the Prometheus exporter.
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

## Metrics

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
