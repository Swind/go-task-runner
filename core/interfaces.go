package core

import (
	"context"
	"fmt"
	"time"
)

// =============================================================================
// PanicHandler: Interface for handling task panics
// =============================================================================

// PanicHandler is called when a task panics during execution.
// This allows custom panic handling, logging, and recovery strategies.
//
// Implementations should be thread-safe as they may be called concurrently.
type PanicHandler interface {
	// HandlePanic is called when a task panics.
	//
	// Parameters:
	// - ctx: The context from the panicked task (may contain task runner info)
	// - runnerName: The name of the task runner where the panic occurred
	// - workerID: The ID of the worker (for thread pool workers, -1 for single-threaded runners)
	// - panicInfo: The panic value recovered from the task
	// - stackTrace: The stack trace at the time of panic
	HandlePanic(ctx context.Context, runnerName string, workerID int, panicInfo any, stackTrace []byte)
}

// DefaultPanicHandler provides a basic panic handler that logs to stdout.
type DefaultPanicHandler struct{}

// HandlePanic prints panic information to stdout.
func (h *DefaultPanicHandler) HandlePanic(ctx context.Context, runnerName string, workerID int, panicInfo any, stackTrace []byte) {
	if workerID >= 0 {
		fmt.Printf("[Worker %d @ %s] Panic: %v\nStack trace:\n%s",
			workerID, runnerName, panicInfo, stackTrace)
	} else {
		fmt.Printf("[Runner %s] Panic: %v\nStack trace:\n%s",
			runnerName, panicInfo, stackTrace)
	}
}

// =============================================================================
// Metrics: Interface for observability and monitoring
// =============================================================================

// Metrics defines the interface for collecting task execution metrics.
// Implementations can send metrics to monitoring systems (Prometheus, StatsD, etc.).
//
// All methods are optional; implementations should handle nil receivers gracefully.
// Methods should be non-blocking and fast to avoid impacting task execution performance.
type Metrics interface {
	// RecordTaskDuration records how long a task took to execute.
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - priority: The task priority
	// - duration: How long the task took to execute
	RecordTaskDuration(runnerName string, priority TaskPriority, duration time.Duration)

	// RecordTaskPanic records that a task panicked during execution.
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - panicInfo: The panic value recovered from the task
	RecordTaskPanic(runnerName string, panicInfo any)

	// RecordQueueDepth records the current queue depth.
	// This can be called periodically to track queue growth/shrinkage.
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - depth: The current number of tasks in the queue
	RecordQueueDepth(runnerName string, depth int)

	// RecordTaskRejected records that a task was rejected (e.g., during shutdown).
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - reason: Why the task was rejected
	RecordTaskRejected(runnerName string, reason string)
}

// NilMetrics provides a no-op metrics implementation that does nothing.
// This is the default when no metrics interface is provided.
type NilMetrics struct{}

// RecordTaskDuration is a no-op.
func (m *NilMetrics) RecordTaskDuration(runnerName string, priority TaskPriority, duration time.Duration) {
}

// RecordTaskPanic is a no-op.
func (m *NilMetrics) RecordTaskPanic(runnerName string, panicInfo any) {
}

// RecordQueueDepth is a no-op.
func (m *NilMetrics) RecordQueueDepth(runnerName string, depth int) {
}

// RecordTaskRejected is a no-op.
func (m *NilMetrics) RecordTaskRejected(runnerName string, reason string) {
}

// =============================================================================
// RejectedTaskHandler: Interface for handling rejected tasks
// =============================================================================

// RejectedTaskHandler is called when a task is rejected by the scheduler.
// This can happen when:
// - The scheduler is shutting down
// - The signal channel is full (backpressure)
// - The task queue is full (if bounded queues are implemented in the future)
//
// Implementations should be thread-safe as they may be called concurrently.
type RejectedTaskHandler interface {
	// HandleRejectedTask is called when a task is rejected.
	//
	// Parameters:
	// - runnerName: The name of the task runner
	// - reason: Why the task was rejected (e.g., "shutdown", "backpressure")
	HandleRejectedTask(runnerName string, reason string)
}

// DefaultRejectedTaskHandler provides a basic handler that logs rejected tasks.
type DefaultRejectedTaskHandler struct{}

// HandleRejectedTask logs the rejected task.
func (h *DefaultRejectedTaskHandler) HandleRejectedTask(runnerName string, reason string) {
	fmt.Printf("[Runner %s] Task rejected: %s", runnerName, reason)
}

// =============================================================================
// TaskSchedulerConfig: Configuration for TaskScheduler
// =============================================================================

// TaskSchedulerConfig holds configuration options for TaskScheduler.
// All handlers are optional; if not provided, default implementations will be used.
type TaskSchedulerConfig struct {
	// PanicHandler is called when a task panics. Defaults to DefaultPanicHandler.
	PanicHandler PanicHandler

	// Metrics is called to record task execution metrics. Defaults to NilMetrics.
	Metrics Metrics

	// RejectedTaskHandler is called when a task is rejected. Defaults to DefaultRejectedTaskHandler.
	RejectedTaskHandler RejectedTaskHandler
}

// DefaultTaskSchedulerConfig returns a config with default handlers.
func DefaultTaskSchedulerConfig() *TaskSchedulerConfig {
	return &TaskSchedulerConfig{
		PanicHandler:        &DefaultPanicHandler{},
		Metrics:             &NilMetrics{},
		RejectedTaskHandler: &DefaultRejectedTaskHandler{},
	}
}
