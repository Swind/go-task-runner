package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

type event struct {
	At     time.Time `json:"at"`
	Type   string    `json:"type"`
	Runner string    `json:"runner"`
	Data   string    `json:"data"`
}

type metricsSnapshot struct {
	TaskCount      int64   `json:"task_count"`
	PanicCount     int64   `json:"panic_count"`
	RejectedCount  int64   `json:"rejected_count"`
	QueueDepth     int     `json:"queue_depth"`
	LastRunner     string  `json:"last_runner"`
	LastDurationMS float64 `json:"last_duration_ms"`
	Events         []event `json:"events"`
}

// InMemoryMetrics is a custom collector that stores metrics for in-process dumps.
type InMemoryMetrics struct {
	mu sync.Mutex

	taskCount      int64
	panicCount     int64
	rejectedCount  int64
	queueDepth     int
	lastRunner     string
	lastDurationMS float64
	events         []event
}

func (m *InMemoryMetrics) RecordTaskDuration(runnerName string, priority core.TaskPriority, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taskCount++
	m.lastRunner = runnerName
	m.lastDurationMS = float64(duration.Microseconds()) / 1000.0
	m.pushEvent(event{
		At:     time.Now(),
		Type:   "duration",
		Runner: runnerName,
		Data:   fmt.Sprintf("priority=%d duration=%s", priority, duration),
	})
}

func (m *InMemoryMetrics) RecordTaskPanic(runnerName string, panicInfo any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.panicCount++
	m.pushEvent(event{
		At:     time.Now(),
		Type:   "panic",
		Runner: runnerName,
		Data:   fmt.Sprintf("panic=%v", panicInfo),
	})
}

func (m *InMemoryMetrics) RecordQueueDepth(runnerName string, depth int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueDepth = depth
	m.pushEvent(event{
		At:     time.Now(),
		Type:   "queue_depth",
		Runner: runnerName,
		Data:   fmt.Sprintf("depth=%d", depth),
	})
}

func (m *InMemoryMetrics) RecordTaskRejected(runnerName string, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rejectedCount++
	m.pushEvent(event{
		At:     time.Now(),
		Type:   "rejected",
		Runner: runnerName,
		Data:   fmt.Sprintf("reason=%s", reason),
	})
}

func (m *InMemoryMetrics) Snapshot() metricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	copiedEvents := make([]event, len(m.events))
	copy(copiedEvents, m.events)
	return metricsSnapshot{
		TaskCount:      m.taskCount,
		PanicCount:     m.panicCount,
		RejectedCount:  m.rejectedCount,
		QueueDepth:     m.queueDepth,
		LastRunner:     m.lastRunner,
		LastDurationMS: m.lastDurationMS,
		Events:         copiedEvents,
	}
}

func (m *InMemoryMetrics) pushEvent(e event) {
	const maxEvents = 20
	m.events = append(m.events, e)
	if len(m.events) > maxEvents {
		m.events = m.events[len(m.events)-maxEvents:]
	}
}

func main() {
	metrics := &InMemoryMetrics{}
	config := &core.TaskSchedulerConfig{
		Metrics: metrics,
	}

	pool := taskrunner.NewPriorityGoroutineThreadPoolWithConfig("dump-pool", 2, config)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := taskrunner.NewSequencedTaskRunner(pool)
	runner.SetName("dump-runner")

	go func() {
		for i := range 8 {
			runner.PostTaskNamed(fmt.Sprintf("work-%d", i), func(ctx context.Context) {
				time.Sleep(30 * time.Millisecond)
			})
		}
		runner.PostTaskNamed("panic-demo", func(ctx context.Context) {
			panic("demo panic")
		})
	}()

	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for range 5 {
		<-ticker.C
		snap := metrics.Snapshot()
		b, _ := json.MarshalIndent(snap, "", "  ")
		fmt.Println(string(b))
	}

	_ = runner.WaitIdle(context.Background())
}
