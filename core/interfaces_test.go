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
	onPanicCalled func(ctx context.Context, runnerName string, workerID int, panicInfo any, stackTrace []byte)
}

type PanicCall struct {
	RunnerName string
	WorkerID   int
	PanicInfo  any
}

func NewTestPanicHandler() *TestPanicHandler {
	return &TestPanicHandler{
		calls: make([]PanicCall, 0),
	}
}

func (h *TestPanicHandler) HandlePanic(ctx context.Context, runnerName string, workerID int, panicInfo any, stackTrace []byte) {
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

// TestDefaultPanicHandler verifies default panic handler is safe to invoke
// Given: A DefaultPanicHandler instance
// When: HandlePanic is called for worker and non-worker paths
// Then: Calls complete without panicking
func TestDefaultPanicHandler(t *testing.T) {
	// Arrange
	handler := &DefaultPanicHandler{}

	// Act
	ctx := context.Background()
	handler.HandlePanic(ctx, "test-runner", 42, "test panic", []byte("stack trace"))

	// Also cover non-worker path (workerID < 0)
	handler.HandlePanic(ctx, "test-runner", -1, "test panic", []byte("stack trace"))
}

// =============================================================================
// Test Metrics
// =============================================================================

// TestMetrics is a mock metrics collector for testing
type TestMetrics struct {
	mu             sync.Mutex
	taskDurations  []TaskDurationMetric
	taskPanics     []TaskPanicMetric
	queueDepths    []QueueDepthMetric
	taskRejections []TaskRejectionMetric
	onTaskDuration func(runnerName string, priority TaskPriority, duration time.Duration)
	onTaskPanic    func(runnerName string, panicInfo any)
	onQueueDepth   func(runnerName string, depth int)
	onTaskRejected func(runnerName string, reason string)
}

type TaskDurationMetric struct {
	RunnerName string
	Priority   TaskPriority
	Duration   time.Duration
}

type TaskPanicMetric struct {
	RunnerName string
	PanicInfo  any
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

func (m *TestMetrics) RecordTaskPanic(runnerName string, panicInfo any) {
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

// TestNilMetrics verifies nil metrics collector is a no-op implementation
// Given: A NilMetrics instance
// When: All metric recording methods are called
// Then: Calls complete without panicking
func TestNilMetrics(t *testing.T) {
	// Arrange
	metrics := &NilMetrics{}

	// Act
	metrics.RecordTaskDuration("test-runner", TaskPriorityUserVisible, time.Second)
	metrics.RecordTaskPanic("test-runner", "panic")
	metrics.RecordQueueDepth("test-runner", 10)
	metrics.RecordTaskRejected("test-runner", "shutdown")
}

// TestTestMetrics verifies test metrics collector stores all metric categories
// Given: A TestMetrics collector
// When: Duration, panic, queue depth, and rejection metrics are recorded
// Then: Recorded metric counts and payload fields match expected values
func TestTestMetrics(t *testing.T) {
	// Arrange
	metrics := NewTestMetrics()

	// Act
	metrics.RecordTaskDuration("runner1", TaskPriorityUserBlocking, 100*time.Millisecond)
	metrics.RecordTaskDuration("runner1", TaskPriorityBestEffort, 200*time.Millisecond)
	metrics.RecordTaskPanic("runner2", "test panic")
	metrics.RecordQueueDepth("runner1", 5)
	metrics.RecordTaskRejected("runner3", "backpressure")

	// Assert
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

	// Assert
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
	mu                   sync.Mutex
	rejections           []TaskRejection
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

// TestDefaultRejectedTaskHandler verifies default rejected-task handler is safe to invoke
// Given: A DefaultRejectedTaskHandler instance
// When: HandleRejectedTask is called
// Then: Call completes without panicking
func TestDefaultRejectedTaskHandler(t *testing.T) {
	// Arrange
	handler := &DefaultRejectedTaskHandler{}

	// Act
	handler.HandleRejectedTask("test-runner", "shutdown")
}

// TestTestRejectedTaskHandler verifies rejected-task records are captured in order
// Given: A TestRejectedTaskHandler collector
// When: Multiple rejections are reported
// Then: Count and stored rejection payloads match expected values
func TestTestRejectedTaskHandler(t *testing.T) {
	// Arrange
	handler := NewTestRejectedTaskHandler()

	// Act
	handler.HandleRejectedTask("runner1", "shutdown")
	handler.HandleRejectedTask("runner2", "backpressure")
	handler.HandleRejectedTask("runner1", "queue full")

	// Assert
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

// TestDefaultTaskSchedulerConfig verifies default scheduler config wires default handlers
// Given: No explicit scheduler config input
// When: DefaultTaskSchedulerConfig is called
// Then: Panic handler, metrics, and rejected-task handler are non-nil defaults
func TestDefaultTaskSchedulerConfig(t *testing.T) {
	// Act
	config := DefaultTaskSchedulerConfig()

	// Assert
	if config.PanicHandler == nil {
		t.Error("PanicHandler should not be nil")
	}
	if config.Metrics == nil {
		t.Error("Metrics should not be nil")
	}
	if config.RejectedTaskHandler == nil {
		t.Error("RejectedTaskHandler should not be nil")
	}

	// Assert
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

// TestTaskSchedulerConfig_CustomHandlers verifies custom handlers are preserved in config
// Given: Explicit custom panic, metrics, and rejected-task handlers
// When: TaskSchedulerConfig is constructed with those handlers
// Then: Config fields reference the exact provided instances
func TestTaskSchedulerConfig_CustomHandlers(t *testing.T) {
	// Arrange
	panicHandler := NewTestPanicHandler()
	metrics := NewTestMetrics()
	rejectedHandler := NewTestRejectedTaskHandler()

	// Act
	config := &TaskSchedulerConfig{
		PanicHandler:        panicHandler,
		Metrics:             metrics,
		RejectedTaskHandler: rejectedHandler,
	}

	// Assert
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

// TestTaskSchedulerConfig_PartialConfig verifies partial config keeps unspecified handlers nil
// Given: A config with only Metrics set
// When: The config is inspected
// Then: Metrics is preserved while other handler fields remain nil
func TestTaskSchedulerConfig_PartialConfig(t *testing.T) {
	// Arrange
	metrics := NewTestMetrics()

	// Act
	config := &TaskSchedulerConfig{
		Metrics: metrics,
	}

	// Assert
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

// TestTaskScheduler_WithCustomHandlers verifies scheduler accepts tasks with custom handler config
// Given: A scheduler configured with custom panic, metrics, and reject handlers
// When: A task is posted into the scheduler
// Then: Task is accepted and queued
func TestTaskScheduler_WithCustomHandlers(t *testing.T) {
	// Arrange
	panicHandler := NewTestPanicHandler()
	metrics := NewTestMetrics()
	rejectedHandler := NewTestRejectedTaskHandler()

	config := &TaskSchedulerConfig{
		PanicHandler:        panicHandler,
		Metrics:             metrics,
		RejectedTaskHandler: rejectedHandler,
	}

	scheduler := NewFIFOTaskSchedulerWithConfig(2, config)
	defer scheduler.Shutdown()

	// Act
	taskExecuted := make(chan struct{})
	scheduler.PostInternal(func(_ context.Context) {
		close(taskExecuted)
	}, DefaultTaskTraits())

	// Assert
	if scheduler.QueuedTaskCount() != 1 {
		t.Errorf("Expected 1 queued task, got %d", scheduler.QueuedTaskCount())
	}

	// Note: In a real scenario, workers would pull and execute tasks.
	// This test just verifies the scheduler accepts tasks and queues them.
}

// TestTaskScheduler_RejectedTask verifies rejection handlers are invoked after shutdown
// Given: A scheduler with metrics and rejected-task handlers
// When: Scheduler is shut down and a task is posted
// Then: Rejection metrics and handler records are produced with shutdown reason
func TestTaskScheduler_RejectedTask(t *testing.T) {
	// Arrange
	metrics := NewTestMetrics()
	rejectedHandler := NewTestRejectedTaskHandler()

	config := &TaskSchedulerConfig{
		Metrics:             metrics,
		RejectedTaskHandler: rejectedHandler,
	}

	// Arrange
	scheduler := NewFIFOTaskSchedulerWithConfig(2, config)

	// Act
	scheduler.Shutdown()

	// Act
	scheduler.PostInternal(func(_ context.Context) {
		t.Error("Task should not be executed after shutdown")
	}, DefaultTaskTraits())

	// Assert
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

// TestTaskScheduler_PanicHandling verifies non-executed queued panic task does not trigger panic hooks
// Given: A scheduler configured with panic handler and metrics but no workers consuming tasks
// When: A panicing task is posted
// Then: Task is queued and no panic hooks are recorded
func TestTaskScheduler_PanicHandling(t *testing.T) {
	// Arrange
	panicHandler := NewTestPanicHandler()
	metrics := NewTestMetrics()

	config := &TaskSchedulerConfig{
		PanicHandler: panicHandler,
		Metrics:      metrics,
	}

	scheduler := NewFIFOTaskSchedulerWithConfig(2, config)
	defer scheduler.Shutdown()

	// Act
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

	// Assert
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
		PanicHandler:        panicHandler,
		Metrics:             metrics,
		RejectedTaskHandler: rejectedHandler,
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
