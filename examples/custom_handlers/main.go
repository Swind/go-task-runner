package main

import (
	"context"
	"fmt"
	"log"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// CustomPanicHandler demonstrates a custom panic handler that logs to a file
type CustomPanicHandler struct{}

func (h *CustomPanicHandler) HandlePanic(ctx context.Context, runnerName string, workerID int, panicInfo any, stackTrace []byte) {
	if workerID >= 0 {
		log.Printf("[CUSTOM HANDLER] Worker %d @ %s panicked: %v\nStack: %s\n",
			workerID, runnerName, panicInfo, string(stackTrace))
	} else {
		log.Printf("[CUSTOM HANDLER] Runner %s panicked: %v\nStack: %s\n",
			runnerName, panicInfo, string(stackTrace))
	}
}

// CustomMetrics demonstrates a custom metrics collector
type CustomMetrics struct {
	taskCount      int
	panicCount     int
	rejectionCount int
}

func (m *CustomMetrics) RecordTaskDuration(runnerName string, priority core.TaskPriority, duration time.Duration) {
	m.taskCount++
	// In a real implementation, you would send this to Prometheus, StatsD, etc.
	if duration > 100*time.Millisecond {
		log.Printf("[METRICS] Slow task detected: %s took %v\n", runnerName, duration)
	}
}

func (m *CustomMetrics) RecordTaskPanic(runnerName string, panicInfo any) {
	m.panicCount++
	log.Printf("[METRICS] Panic recorded in %s: %v (total panics: %d)\n",
		runnerName, panicInfo, m.panicCount)
}

func (m *CustomMetrics) RecordQueueDepth(runnerName string, depth int) {
	// Could be called periodically to monitor queue depth
	if depth > 100 {
		log.Printf("[METRICS] High queue depth in %s: %d\n", runnerName, depth)
	}
}

func (m *CustomMetrics) RecordTaskRejected(runnerName string, reason string) {
	m.rejectionCount++
	log.Printf("[METRICS] Task rejected in %s: %s (total rejections: %d)\n",
		runnerName, reason, m.rejectionCount)
}

// CustomRejectedTaskHandler demonstrates custom rejection handling
type CustomRejectedTaskHandler struct{}

func (h *CustomRejectedTaskHandler) HandleRejectedTask(runnerName string, reason string) {
	log.Printf("[REJECTION] Task rejected in %s: %s\n", runnerName, reason)
	// In a real implementation, you might:
	// - Send an alert
	// - Retry with backoff
	// - Store in a dead letter queue
}

func main() {
	// Example 1: Using default handlers
	fmt.Println("=== Example 1: Default Handlers ===")
	defaultConfig()

	// Example 2: Using custom handlers
	fmt.Println("\n=== Example 2: Custom Handlers ===")
	customConfig()

	// Example 3: Monitoring queue depth
	fmt.Println("\n=== Example 3: Monitoring Queue Depth ===")
	monitorQueueDepth()
}

func defaultConfig() {
	// Create a thread pool with default handlers
	pool := taskrunner.NewGoroutineThreadPool("default-pool", 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)

	// Create a runner
	runner := core.NewSequencedTaskRunner(pool)
	runner.SetName("default-runner")

	// Post some tasks
	for i := range 3 {
		i := i
		runner.PostTask(func(_ context.Context) {
			fmt.Printf("Task %d executing\n", i)
		})
	}

	// Wait a bit for tasks to complete
	time.Sleep(100 * time.Millisecond)

	// Shutdown
	pool.Stop()
	fmt.Println("Default pool stopped")
}

func customConfig() {
	// Create custom handlers
	panicHandler := &CustomPanicHandler{}
	metrics := &CustomMetrics{}
	rejectedHandler := &CustomRejectedTaskHandler{}

	// Create config with custom handlers
	config := &core.TaskSchedulerConfig{
		PanicHandler:        panicHandler,
		Metrics:             metrics,
		RejectedTaskHandler: rejectedHandler,
	}

	// Create a thread pool with custom config
	pool := taskrunner.NewGoroutineThreadPoolWithConfig("custom-pool", 2, config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)

	// Create a runner
	runner := core.NewSequencedTaskRunner(pool)
	runner.SetName("custom-runner")

	// Post a normal task
	runner.PostTask(func(_ context.Context) {
		fmt.Println("Normal task executing")
	})

	// Post a task that will panic
	runner.PostTask(func(_ context.Context) {
		fmt.Println("About to panic...")
		panic("intentional panic for demo")
	})

	// Post another task after panic
	runner.PostTask(func(_ context.Context) {
		fmt.Println("Task after panic executing")
	})

	// Wait for tasks to complete
	time.Sleep(200 * time.Millisecond)

	// Shutdown
	pool.Stop()

	// Print metrics summary
	fmt.Printf("\nMetrics Summary:\n")
	fmt.Printf("  Tasks executed: %d\n", metrics.taskCount)
	fmt.Printf("  Panics: %d\n", metrics.panicCount)
	fmt.Printf("  Rejections: %d\n", metrics.rejectionCount)
}

func monitorQueueDepth() {
	// Create custom metrics that monitor queue depth
	type MonitoringMetrics struct {
		*CustomMetrics
		runnerName string
	}

	metrics := &MonitoringMetrics{
		CustomMetrics: &CustomMetrics{},
		runnerName:    "monitored-runner",
	}

	config := &core.TaskSchedulerConfig{
		Metrics: metrics,
	}

	pool := taskrunner.NewGoroutineThreadPoolWithConfig("monitored-pool", 1, config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)

	runner := core.NewSequencedTaskRunner(pool)
	runner.SetName(metrics.runnerName)

	// Post many tasks to create queue depth
	for i := range 10 {
		i := i
		runner.PostTask(func(_ context.Context) {
			time.Sleep(10 * time.Millisecond)
			fmt.Printf("Task %d completed\n", i)
		})
	}

	// Monitor queue depth periodically
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range 10 {
			<-ticker.C
			metrics.RecordQueueDepth(metrics.runnerName, pool.QueuedTaskCount())
		}
		close(done)
	}()

	// Wait for monitoring to complete
	<-done

	// Shutdown
	pool.Stop()
	fmt.Println("Monitoring completed")
}
