package prometheus

import (
	"errors"
	"fmt"
	"time"

	"github.com/Swind/go-task-runner/core"
	prom "github.com/prometheus/client_golang/prometheus"
)

// ExporterOptions controls collector configuration.
type ExporterOptions struct {
	DurationBuckets []float64
}

// MetricsExporter adapts core.Metrics to Prometheus collectors.
type MetricsExporter struct {
	taskDurationSeconds *prom.HistogramVec
	taskPanicTotal      *prom.CounterVec
	taskRejectedTotal   *prom.CounterVec
	queueDepth          *prom.GaugeVec
}

var _ core.Metrics = (*MetricsExporter)(nil)

// NewMetricsExporter creates and registers Prometheus collectors for core.Metrics.
func NewMetricsExporter(namespace string, reg prom.Registerer, opts ExporterOptions) (*MetricsExporter, error) {
	if namespace == "" {
		namespace = "taskrunner"
	}
	if reg == nil {
		reg = prom.DefaultRegisterer
	}
	buckets := opts.DurationBuckets
	if len(buckets) == 0 {
		buckets = prom.DefBuckets
	}

	durationVec := prom.NewHistogramVec(prom.HistogramOpts{
		Namespace: namespace,
		Name:      "task_duration_seconds",
		Help:      "Task execution duration in seconds.",
		Buckets:   buckets,
	}, []string{"runner", "priority"})
	panicVec := prom.NewCounterVec(prom.CounterOpts{
		Namespace: namespace,
		Name:      "task_panic_total",
		Help:      "Total number of task panics.",
	}, []string{"runner"})
	rejectedVec := prom.NewCounterVec(prom.CounterOpts{
		Namespace: namespace,
		Name:      "task_rejected_total",
		Help:      "Total number of rejected tasks.",
	}, []string{"runner", "reason"})
	queueDepthVec := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: namespace,
		Name:      "queue_depth",
		Help:      "Current queue depth.",
	}, []string{"runner"})

	var err error
	if durationVec, err = registerCollector(reg, durationVec); err != nil {
		return nil, err
	}
	if panicVec, err = registerCollector(reg, panicVec); err != nil {
		return nil, err
	}
	if rejectedVec, err = registerCollector(reg, rejectedVec); err != nil {
		return nil, err
	}
	if queueDepthVec, err = registerCollector(reg, queueDepthVec); err != nil {
		return nil, err
	}

	return &MetricsExporter{
		taskDurationSeconds: durationVec,
		taskPanicTotal:      panicVec,
		taskRejectedTotal:   rejectedVec,
		queueDepth:          queueDepthVec,
	}, nil
}

// RecordTaskDuration records task execution duration.
func (m *MetricsExporter) RecordTaskDuration(runnerName string, priority core.TaskPriority, duration time.Duration) {
	if m == nil {
		return
	}
	m.taskDurationSeconds.WithLabelValues(normalizeLabel(runnerName, "unknown"), priorityLabel(priority)).Observe(duration.Seconds())
}

// RecordTaskPanic records task panic events.
func (m *MetricsExporter) RecordTaskPanic(runnerName string, panicInfo any) {
	if m == nil {
		return
	}
	m.taskPanicTotal.WithLabelValues(normalizeLabel(runnerName, "unknown")).Inc()
}

// RecordQueueDepth records queue depth.
func (m *MetricsExporter) RecordQueueDepth(runnerName string, depth int) {
	if m == nil {
		return
	}
	m.queueDepth.WithLabelValues(normalizeLabel(runnerName, "unknown")).Set(float64(depth))
}

// RecordTaskRejected records task rejection events.
func (m *MetricsExporter) RecordTaskRejected(runnerName string, reason string) {
	if m == nil {
		return
	}
	m.taskRejectedTotal.WithLabelValues(normalizeLabel(runnerName, "unknown"), normalizeLabel(reason, "unknown")).Inc()
}

func normalizeLabel(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func priorityLabel(priority core.TaskPriority) string {
	switch priority {
	case core.TaskPriorityUserBlocking:
		return "user_blocking"
	case core.TaskPriorityUserVisible:
		return "user_visible"
	case core.TaskPriorityBestEffort:
		return "best_effort"
	default:
		return "unknown"
	}
}

func registerCollector[T prom.Collector](reg prom.Registerer, collector T) (T, error) {
	err := reg.Register(collector)
	if err == nil {
		return collector, nil
	}

	var alreadyRegisteredErr prom.AlreadyRegisteredError
	if errors.As(err, &alreadyRegisteredErr) {
		existing, ok := alreadyRegisteredErr.ExistingCollector.(T)
		if !ok {
			return collector, fmt.Errorf("collector type mismatch for %T", collector)
		}
		return existing, nil
	}

	return collector, err
}
