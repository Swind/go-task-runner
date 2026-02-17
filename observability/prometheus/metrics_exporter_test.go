package prometheus

import (
	"testing"
	"time"

	"github.com/Swind/go-task-runner/core"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestMetricsExporter_RecordMethods(t *testing.T) {
	reg := prom.NewRegistry()
	exporter, err := NewMetricsExporter("taskrunner", reg, ExporterOptions{})
	if err != nil {
		t.Fatalf("NewMetricsExporter failed: %v", err)
	}

	exporter.RecordTaskDuration("runner-a", core.TaskPriorityUserVisible, 250*time.Millisecond)
	exporter.RecordTaskPanic("runner-a", "panic")
	exporter.RecordQueueDepth("runner-a", 7)
	exporter.RecordTaskRejected("runner-a", "shutdown")

	panicTotal := testutil.ToFloat64(exporter.taskPanicTotal.WithLabelValues("runner-a"))
	if panicTotal != 1 {
		t.Fatalf("panic total = %v, want 1", panicTotal)
	}

	queueDepth := testutil.ToFloat64(exporter.queueDepth.WithLabelValues("runner-a"))
	if queueDepth != 7 {
		t.Fatalf("queue depth = %v, want 7", queueDepth)
	}

	rejected := testutil.ToFloat64(exporter.taskRejectedTotal.WithLabelValues("runner-a", "shutdown"))
	if rejected != 1 {
		t.Fatalf("rejected total = %v, want 1", rejected)
	}

	histCount, err := histogramSampleCount(exporter.taskDurationSeconds.WithLabelValues("runner-a", "user_visible"))
	if err != nil {
		t.Fatalf("histogramSampleCount failed: %v", err)
	}
	if histCount != 1 {
		t.Fatalf("duration sample count = %d, want 1", histCount)
	}
}

func TestMetricsExporter_AlreadyRegisteredReuse(t *testing.T) {
	reg := prom.NewRegistry()
	first, err := NewMetricsExporter("taskrunner", reg, ExporterOptions{})
	if err != nil {
		t.Fatalf("first NewMetricsExporter failed: %v", err)
	}
	second, err := NewMetricsExporter("taskrunner", reg, ExporterOptions{})
	if err != nil {
		t.Fatalf("second NewMetricsExporter failed: %v", err)
	}

	first.RecordTaskPanic("runner-a", nil)
	second.RecordTaskPanic("runner-a", nil)

	got := testutil.ToFloat64(first.taskPanicTotal.WithLabelValues("runner-a"))
	if got != 2 {
		t.Fatalf("shared panic counter = %v, want 2", got)
	}
}

func histogramSampleCount(observer prom.Observer) (uint64, error) {
	collector, ok := observer.(prom.Collector)
	if !ok {
		return 0, nil
	}

	metricCh := make(chan prom.Metric, 1)
	collector.Collect(metricCh)
	close(metricCh)
	for metric := range metricCh {
		msg := &dto.Metric{}
		if err := metric.Write(msg); err != nil {
			return 0, err
		}
		if msg.Histogram != nil {
			return msg.Histogram.GetSampleCount(), nil
		}
	}
	return 0, nil
}
