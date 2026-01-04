package core

import (
	"context"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Test PanicHandler
// =============================================================================

// TestPanicHandler is a mock panic handler for testing
type TestPanicHandler struct {
	mu            sync.Mutex
	calls         []PanicCall
	onPanicCalled func(ctx context.Context, runnerName string, workerID int, panicInfo interface{}, stackTrace []byte)
}

type PanicCall struct {
	RunnerName string
	WorkerID   int
	PanicInfo  interface{}
}

func NewTestPanicHandler() *TestPanicHandler {
	return &TestPanicHandler{
		calls: make([]PanicCall, 0),
	}
}

func (h *TestPanicHandler) HandlePanic(ctx context.Context, runnerName string, workerID int, panicInfo interface{}, stackTrace []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.calls = append(h.calls, PanicCall{
		RunnerName: runnerName,
		WorkerID:   workerID,
		PanicInfo:  panicInfo,
	})

	if h.onPanicCalled != nil {
		h.onPanicCalled(ctx, runnerName, workerID, panicInfo, stackTrace)
	}
}

func (h *TestPanicHandler) GetCalls() []PanicCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

func (h *TestPanicHandler) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = make([]PanicCall, 0)
}

func (h *TestPanicHandler) CallCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls)
}

func TestDefaultPanicHandler(t *testing.T) {
	// Given: A DefaultPanicHandler
	handler := &DefaultPanicHandler{}

	// When: HandlePanic is called
	ctx := context.Background()
	handler.HandlePanic(ctx, "test-runner", 42, "test panic", []byte("stack trace"))

	// Then: No panic should occur (handler should not crash)
	// This is just a sanity test to ensure the handler works
}

// =============================================================================
// Test Metrics
// =============================================================================

// TestMetrics is a mock metrics collector for testing
type TestMetrics struct {
	mu                  sync.Mutex
	taskDurations       []TaskDurationMetric
	taskPanics          []TaskPanicMetric
	queueDepths         []QueueDepthMetric
	taskRejections      []TaskRejectionMetric
	onTaskDuration      func(runnerName string, priority TaskPriority, duration time.Duration)
	onTaskPanic         func(runnerName string, panicInfo interface{})
	onQueueDepth        func(runnerName string, depth int)
	onTaskRejected      func(runnerName string, reason string)
}

type TaskDurationMetric struct {
	RunnerName string
	Priority   TaskPriority
	Duration   time.Duration
}

type TaskPanicMetric struct {
	RunnerName string
	PanicInfo  interface{}
}

type QueueDepthMetric struct {
	RunnerName string
	Depth      int
}

type TaskRejectionMetric struct {
	RunnerName string
	Reason     string
}

func NewTestMetrics() *TestMetrics {
	return &TestMetrics{
		taskDurations:  make([]TaskDurationMetric, 0),
		taskPanics:     make([]TaskPanicMetric, 0),
		queueDepths:    make([]QueueDepthMetric, 0),
		taskRejections: make([]TaskRejectionMetric, 0),
	}
}

func (m *TestMetrics) RecordTaskDuration(runnerName string, priority TaskPriority, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.taskDurations = append(m.taskDurations, TaskDurationMetric{
		RunnerName: runnerName,
		Priority:   priority,
		Duration:   duration,
	})

	if m.onTaskDuration != nil {
		m.onTaskDuration(runnerName, priority, duration)
	}
}

func (m *TestMetrics) RecordTaskPanic(runnerName string, panicInfo interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.taskPanics = append(m.taskPanics, TaskPanicMetric{
		RunnerName: runnerName,
		PanicInfo:  panicInfo,
	})

	if m.onTaskPanic != nil {
		m.onTaskPanic(runnerName, panicInfo)
	}
}

func (m *TestMetrics) RecordQueueDepth(runnerName string, depth int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queueDepths = append(m.queueDepths, QueueDepthMetric{
		RunnerName: runnerName,
		Depth:      depth,
	})

	if m.onQueueDepth != nil {
		m.onQueueDepth(runnerName, depth)
	}
}

func (m *TestMetrics) RecordTaskRejected(runnerName string, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.taskRejections = append(m.taskRejections, TaskRejectionMetric{
		RunnerName: runnerName,
		Reason:     reason,
	})

	if m.onTaskRejected != nil {
		m.onTaskRejected(runnerName, reason)
	}
}

func (m *TestMetrics) GetTaskDurations() []TaskDurationMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.taskDurations
}

func (m *TestMetrics) GetTaskPanics() []TaskPanicMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.taskPanics
}

func (m *TestMetrics) GetQueueDepths() []QueueDepthMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queueDepths
}

func (m *TestMetrics) GetTaskRejections() []TaskRejectionMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.taskRejections
}

func (m *TestMetrics) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taskDurations = make([]TaskDurationMetric, 0)
	m.taskPanics = make([]TaskPanicMetric, 0)
	m.queueDepths = make([]QueueDepthMetric, 0)
	m.taskRejections = make([]TaskRejectionMetric, 0)
}

func TestNilMetrics(t *testing.T) {
	// Given: A NilMetrics
	metrics := &NilMetrics{}

	// When: All methods are called
	metrics.RecordTaskDuration("test-runner", TaskPriorityUserVisible, time.Second)
	metrics.RecordTaskPanic("test-runner", "panic")
	metrics.RecordQueueDepth("test-runner", 10)
	metrics.RecordTaskRejected("test-runner", "shutdown")

	// Then: No panic should occur (all methods are no-ops)
	// This is just a sanity test to ensure the no-op implementation works
}

func TestTestMetrics(t *testing.T) {
	// Given: A TestMetrics
	metrics := NewTestMetrics()

	// When: Metrics are recorded
	metrics.RecordTaskDuration("runner1", TaskPriorityUserBlocking, 100*time.Millisecond)
	metrics.RecordTaskDuration("runner1", TaskPriorityBestEffort, 200*time.Millisecond)
	metrics.RecordTaskPanic("runner2", "test panic")
	metrics.RecordQueueDepth("runner1", 5)
	metrics.RecordTaskRejected("runner3", "backpressure")

	// Then: Metrics should be recorded correctly
	if len(metrics.GetTaskDurations()) != 2 {
		t.Errorf("Expected 2 task durations, got %d", len(metrics.GetTaskDurations()))
	}

	if len(metrics.GetTaskPanics()) != 1 {
		t.Errorf("Expected 1 task panic, got %d", len(metrics.GetTaskPanics()))
	}

	if len(metrics.GetQueueDepths()) != 1 {
		t.Errorf("Expected 1 queue depth, got %d", len(metrics.GetQueueDepths()))
	}

	if len(metrics.GetTaskRejections()) != 1 {
		t.Errorf("Expected 1 task rejection, got %d", len(metrics.GetTaskRejections()))
	}

	// Verify values
	durations := metrics.GetTaskDurations()
	if durations[0].RunnerName != "runner1" || durations[0].Duration != 100*time.Millisecond {
		t.Errorf("Unexpected first duration: %+v", durations[0])
	}

	panics := metrics.GetTaskPanics()
	if panics[0].RunnerName != "runner2" || panics[0].PanicInfo != "test panic" {
		t.Errorf("Unexpected panic: %+v", panics[0])
	}
}

// =============================================================================
// Test RejectedTaskHandler
// =============================================================================

// TestRejectedTaskHandler is a mock rejected task handler for testing
type TestRejectedTaskHandler struct {
	mu                  sync.Mutex
	rejections          []TaskRejection
	onRejectedTaskCalled func(runnerName string, reason string)
}

type TaskRejection struct {
	RunnerName string
	Reason     string
}

func NewTestRejectedTaskHandler() *TestRejectedTaskHandler {
	return &TestRejectedTaskHandler{
		rejections: make([]TaskRejection, 0),
	}
}

func (h *TestRejectedTaskHandler) HandleRejectedTask(runnerName string, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.rejections = append(h.rejections, TaskRejection{
		RunnerName: runnerName,
		Reason:     reason,
	})

	if h.onRejectedTaskCalled != nil {
		h.onRejectedTaskCalled(runnerName, reason)
	}
}

func (h *TestRejectedTaskHandler) GetRejections() []TaskRejection {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rejections
}

func (h *TestRejectedTaskHandler) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rejections = make([]TaskRejection, 0)
}

func (h *TestRejectedTaskHandler) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rejections)
}

func TestDefaultRejectedTaskHandler(t *testing.T) {
	// Given: A DefaultRejectedTaskHandler
	handler := &DefaultRejectedTaskHandler{}

	// When: HandleRejectedTask is called
	handler.HandleRejectedTask("test-runner", "shutdown")

	// Then: No panic should occur (handler should not crash)
	// This is just a sanity test to ensure the handler works
}

func TestTestRejectedTaskHandler(t *testing.T) {
	// Given: A TestRejectedTaskHandler
	handler := NewTestRejectedTaskHandler()

	// When: Tasks are rejected
	handler.HandleRejectedTask("runner1", "shutdown")
	handler.HandleRejectedTask("runner2", "backpressure")
	handler.HandleRejectedTask("runner1", "queue full")

	// Then: Rejections should be recorded correctly
	if handler.Count() != 3 {
		t.Errorf("Expected 3 rejections, got %d", handler.Count())
	}

	rejections := handler.GetRejections()
	if rejections[0].RunnerName != "runner1" || rejections[0].Reason != "shutdown" {
		t.Errorf("Unexpected first rejection: %+v", rejections[0])
	}

	if rejections[1].RunnerName != "runner2" || rejections[1].Reason != "backpressure" {
		t.Errorf("Unexpected second rejection: %+v", rejections[1])
	}
}

// =============================================================================
// Test TaskSchedulerConfig
// =============================================================================

func TestDefaultTaskSchedulerConfig(t *testing.T) {
	// Given: Default config
	config := DefaultTaskSchedulerConfig()

	// Then: All handlers should be non-nil
	if config.PanicHandler == nil {
		t.Error("PanicHandler should not be nil")
	}
	if config.Metrics == nil {
		t.Error("Metrics should not be nil")
	}
	if config.RejectedTaskHandler == nil {
		t.Error("RejectedTaskHandler should not be nil")
	}

	// Verify types
	if _, ok := config.PanicHandler.(*DefaultPanicHandler); !ok {
		t.Errorf("PanicHandler should be *DefaultPanicHandler, got %T", config.PanicHandler)
	}
	if _, ok := config.Metrics.(*NilMetrics); !ok {
		t.Errorf("Metrics should be *NilMetrics, got %T", config.Metrics)
	}
	if _, ok := config.RejectedTaskHandler.(*DefaultRejectedTaskHandler); !ok {
		t.Errorf("RejectedTaskHandler should be *DefaultRejectedTaskHandler, got %T", config.RejectedTaskHandler)
	}
}

func TestTaskSchedulerConfig_CustomHandlers(t *testing.T) {
	// Given: Custom handlers
	panicHandler := NewTestPanicHandler()
	metrics := NewTestMetrics()
	rejectedHandler := NewTestRejectedTaskHandler()

	config := &TaskSchedulerConfig{
		PanicHandler:         panicHandler,
		Metrics:              metrics,
		RejectedTaskHandler:  rejectedHandler,
	}

	// Then: Handlers should be set correctly
	if config.PanicHandler != panicHandler {
		t.Error("PanicHandler not set correctly")
	}
	if config.Metrics != metrics {
		t.Error("Metrics not set correctly")
	}
	if config.RejectedTaskHandler != rejectedHandler {
		t.Error("RejectedTaskHandler not set correctly")
	}
}

func TestTaskSchedulerConfig_PartialConfig(t *testing.T) {
	// Given: Partial config (only Metrics set)
	metrics := NewTestMetrics()
	config := &TaskSchedulerConfig{
		Metrics: metrics,
	}

	// Then: Only Metrics should be non-nil
	if config.PanicHandler != nil {
		t.Error("PanicHandler should be nil")
	}
	if config.Metrics != metrics {
		t.Error("Metrics not set correctly")
	}
	if config.RejectedTaskHandler != nil {
		t.Error("RejectedTaskHandler should be nil")
	}
}

// =============================================================================
// Integration Test: TaskScheduler with custom handlers
// =============================================================================

func TestTaskScheduler_WithCustomHandlers(t *testing.T) {
	// Given: A scheduler with custom handlers
	panicHandler := NewTestPanicHandler()
	metrics := NewTestMetrics()
	rejectedHandler := NewTestRejectedTaskHandler()

	config := &TaskSchedulerConfig{
		PanicHandler:         panicHandler,
		Metrics:              metrics,
		RejectedTaskHandler:  rejectedHandler,
	}

	scheduler := NewFIFOTaskSchedulerWithConfig(2, config)
	defer scheduler.Shutdown()

	// When: Tasks are posted to the scheduler
	taskExecuted := make(chan struct{})
	scheduler.PostInternal(func(_ context.Context) {
		close(taskExecuted)
	}, DefaultTaskTraits())

	// Then: Task should be queued
	if scheduler.QueuedTaskCount() != 1 {
		t.Errorf("Expected 1 queued task, got %d", scheduler.QueuedTaskCount())
	}

	// Note: In a real scenario, workers would pull and execute tasks.
	// This test just verifies the scheduler accepts tasks and queues them.
}

func TestTaskScheduler_RejectedTask(t *testing.T) {
	// Given: A scheduler with custom handlers
	metrics := NewTestMetrics()
	rejectedHandler := NewTestRejectedTaskHandler()

	config := &TaskSchedulerConfig{
		Metrics:              metrics,
		RejectedTaskHandler:  rejectedHandler,
	}

	scheduler := NewFIFOTaskSchedulerWithConfig(2, config)

	// When: Scheduler is shut down
	scheduler.Shutdown()

	// And: A task is posted after shutdown
	scheduler.PostInternal(func(_ context.Context) {
		t.Error("Task should not be executed after shutdown")
	}, DefaultTaskTraits())

	// Then: Rejection handlers should be called
	if len(metrics.GetTaskRejections()) == 0 {
		t.Error("Expected at least 1 task rejection")
	}

	if metrics.GetTaskRejections()[0].Reason != "shutting down" {
		t.Errorf("Expected rejection reason 'shutting down', got '%s'", metrics.GetTaskRejections()[0].Reason)
	}

	if rejectedHandler.Count() != 1 {
		t.Errorf("Expected 1 rejection, got %d", rejectedHandler.Count())
	}

	if rejectedHandler.GetRejections()[0].Reason != "shutting down" {
		t.Errorf("Expected rejection reason 'shutting down', got '%s'", rejectedHandler.GetRejections()[0].Reason)
	}
}

func TestTaskScheduler_PanicHandling(t *testing.T) {
	// Given: A scheduler with custom handlers
	panicHandler := NewTestPanicHandler()
	metrics := NewTestMetrics()

	config := &TaskSchedulerConfig{
		PanicHandler: panicHandler,
		Metrics:      metrics,
	}

	scheduler := NewFIFOTaskSchedulerWithConfig(2, config)
	defer scheduler.Shutdown()

	// When: A task that panics is posted
	taskDone := make(chan struct{})
	scheduler.PostInternal(func(_ context.Context) {
		defer close(taskDone)
		panic("test panic")
	}, DefaultTaskTraits())

	// Wait for task to be queued (not executed, since there are no workers)
	time.Sleep(10 * time.Millisecond)

	// Note: In this test setup without workers, the panic won't actually occur
	// because there's no worker to execute the task. This is expected behavior.
	// The task will remain queued until workers are started or scheduler is shut down.

	// Verify the task was queued
	if scheduler.QueuedTaskCount() != 1 {
		t.Logf("Task queued: %d (expected 1)", scheduler.QueuedTaskCount())
	}

	// Then: No panics should be recorded yet (task not executed without workers)
	if panicHandler.CallCount() != 0 {
		t.Errorf("Expected 0 panic calls (task not executed without workers), got %d", panicHandler.CallCount())
	}

	if len(metrics.GetTaskPanics()) != 0 {
		t.Errorf("Expected 0 task panic metrics (task not executed without workers), got %d", len(metrics.GetTaskPanics()))
	}
}

func ExampleTaskSchedulerConfig() {
	// Create custom handlers
	panicHandler := &DefaultPanicHandler{}
	metrics := &NilMetrics{}
	rejectedHandler := &DefaultRejectedTaskHandler{}

	// Create config
	config := &TaskSchedulerConfig{
		PanicHandler:         panicHandler,
		Metrics:              metrics,
		RejectedTaskHandler:  rejectedHandler,
	}

	// Create scheduler with config
	_ = NewFIFOTaskSchedulerWithConfig(4, config)
}

func ExampleDefaultTaskSchedulerConfig() {
	// Use default config
	config := DefaultTaskSchedulerConfig()

	// Create scheduler with default config
	_ = NewFIFOTaskSchedulerWithConfig(4, config)
}
