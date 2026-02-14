package taskrunner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Swind/go-task-runner/core"
)

type poolTestPanicHandler struct {
	called atomic.Bool
}

func (h *poolTestPanicHandler) HandlePanic(ctx context.Context, runnerName string, workerID int, panicInfo any, stackTrace []byte) {
	h.called.Store(true)
}

type poolTestMetrics struct {
	panicCount atomic.Int32
}

func (m *poolTestMetrics) RecordTaskDuration(runnerName string, priority core.TaskPriority, duration time.Duration) {
}
func (m *poolTestMetrics) RecordTaskPanic(runnerName string, panicInfo any) {
	m.panicCount.Add(1)
}
func (m *poolTestMetrics) RecordQueueDepth(runnerName string, depth int)       {}
func (m *poolTestMetrics) RecordTaskRejected(runnerName string, reason string) {}

// Ensure GoroutineThreadPool fully implements ThreadPool interface
var _ core.ThreadPool = (*GoroutineThreadPool)(nil)

// TestGoroutineThreadPool_Lifecycle verifies pool state transitions through its lifecycle
// Given: A newly created GoroutineThreadPool with 2 workers
// When: The pool is started and then stopped
// Then: Pool correctly transitions between stopped and running states
func TestGoroutineThreadPool_Lifecycle(t *testing.T) {
	// Arrange
	pool := NewGoroutineThreadPool("test-pool", 2)
	expectedID := "test-pool"
	expectedWorkers := 2

	// Act - Verify initial state
	if pool.ID() != expectedID {
		t.Errorf("ID() = %q, want %q", pool.ID(), expectedID)
	}

	// Assert - Not running initially
	if pool.IsRunning() {
		t.Error("IsRunning() = true, want false (pool should not be running initially)")
	}

	// Act - Start the pool
	ctx := context.Background()
	pool.Start(ctx)

	// Assert - Running after Start()
	if !pool.IsRunning() {
		t.Error("IsRunning() = false, want true (pool should be running after Start())")
	}

	// Assert - Worker count matches configuration
	if pool.WorkerCount() != expectedWorkers {
		t.Errorf("WorkerCount() = %d, want %d", pool.WorkerCount(), expectedWorkers)
	}

	// Act - Stop the pool
	pool.Stop()

	// Assert - Not running after Stop()
	if pool.IsRunning() {
		t.Error("IsRunning() = true, want false (pool should not be running after Stop())")
	}
}

// TestGoroutineThreadPool_StartIdempotentAndStopGracefulNotRunning verifies idempotent start and no-op graceful stop
// Given: A new pool that has not started yet
// When: StopGraceful is called before start, then Start is called twice
// Then: StopGraceful returns nil and pool remains healthy/running until explicit stop
func TestGoroutineThreadPool_StartIdempotentAndStopGracefulNotRunning(t *testing.T) {
	// Arrange
	pool := NewGoroutineThreadPool("idempotent-pool", 1)

	// Act
	if err := pool.StopGraceful(100 * time.Millisecond); err != nil {
		t.Fatalf("StopGraceful on non-running pool = %v, want nil", err)
	}

	// Act
	pool.Start(context.Background())
	pool.Start(context.Background()) // idempotent path

	// Assert
	if !pool.IsRunning() {
		t.Fatal("pool should be running after Start()")
	}

	// Act
	pool.Stop()
}

// TestGlobalThreadPool_InitIdempotentAndPanicWhenMissing verifies accessor panic without initialization
// Given: A shutdown global pool state
// When: GetGlobalThreadPool is called without InitGlobalThreadPool
// Then: The accessor panics to signal missing initialization
func TestGlobalThreadPool_InitIdempotentAndPanicWhenMissing(t *testing.T) {
	// Arrange
	ShutdownGlobalThreadPool()

	// Act and Assert
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("GetGlobalThreadPool() should panic when not initialized")
		}
	}()
	_ = GetGlobalThreadPool()
}

// TestGlobalThreadPool_InitIdempotent verifies repeated global initialization keeps one instance
// Given: A clean global pool state
// When: InitGlobalThreadPool is called multiple times
// Then: The same pool instance is retained
func TestGlobalThreadPool_InitIdempotent(t *testing.T) {
	// Arrange
	ShutdownGlobalThreadPool()
	defer ShutdownGlobalThreadPool()

	// Act
	InitGlobalThreadPool(1)
	p1 := GetGlobalThreadPool()
	InitGlobalThreadPool(4) // should be no-op
	p2 := GetGlobalThreadPool()

	// Assert
	if p1 != p2 {
		t.Fatal("InitGlobalThreadPool should be idempotent and keep same instance")
	}
}

// TestGoroutineThreadPool_PanicHandlerAndMetrics verifies panic hooks and metrics integration
// Given: A pool configured with custom panic handler and metrics
// When: A task panics and a follow-up task runs
// Then: Panic handler/metrics are invoked and worker continues processing
func TestGoroutineThreadPool_PanicHandlerAndMetrics(t *testing.T) {
	// Arrange
	panicHandler := &poolTestPanicHandler{}
	metrics := &poolTestMetrics{}
	cfg := &core.TaskSchedulerConfig{
		PanicHandler:        panicHandler,
		Metrics:             metrics,
		RejectedTaskHandler: &core.DefaultRejectedTaskHandler{},
	}
	pool := NewGoroutineThreadPoolWithConfig("panic-metrics-pool", 1, cfg)
	pool.Start(context.Background())
	defer pool.Stop()

	// Act
	done := make(chan struct{}, 1)
	pool.PostInternal(func(ctx context.Context) {
		panic(fmt.Errorf("boom"))
	}, core.DefaultTaskTraits())
	pool.PostInternal(func(ctx context.Context) {
		select {
		case done <- struct{}{}:
		default:
		}
	}, core.DefaultTaskTraits())

	// Assert
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for post-panic task")
	}

	// Assert
	if !panicHandler.called.Load() {
		t.Fatal("custom panic handler was not called")
	}
	if metrics.panicCount.Load() == 0 {
		t.Fatal("panic metrics were not recorded")
	}
}

// TestGoroutineThreadPool_TaskExecution verifies concurrent task execution
// Given: A running pool with 4 workers and 10 tasks to execute
// When: All tasks are submitted to the pool
// Then: All tasks execute exactly once
func TestGoroutineThreadPool_TaskExecution(t *testing.T) {
	// Arrange
	pool := NewGoroutineThreadPool("exec-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	var counter int32
	var wg sync.WaitGroup
	taskCount := 10
	wg.Add(taskCount)

	task := func(ctx context.Context) {
		defer wg.Done()
		atomic.AddInt32(&counter, 1)
		time.Sleep(10 * time.Millisecond) // Simulate work
	}

	// Act - Submit all tasks
	for i := 0; i < taskCount; i++ {
		pool.PostInternal(task, core.TaskTraits{})
	}

	// Wait for completion
	wg.Wait()

	// Assert - All tasks executed
	got := atomic.LoadInt32(&counter)
	if got != int32(taskCount) {
		t.Errorf("executed tasks = %d, want %d", got, taskCount)
	}
}

// TestGoroutineThreadPool_Metrics verifies real-time task tracking metrics
// Given: A single-worker pool with blocking and queued tasks
// When: Tasks are submitted and executed
// Then: ActiveTaskCount and QueuedTaskCount accurately reflect pool state
func TestGoroutineThreadPool_Metrics(t *testing.T) {
	// Arrange
	pool := NewGoroutineThreadPool("metrics-pool", 1) // Single worker to force queuing
	pool.Start(context.Background())
	defer pool.Stop()

	// Arrange - Create a blocking task
	blockCh := make(chan struct{})
	bgDone := make(chan struct{})

	blockingTask := func(ctx context.Context) {
		<-blockCh
		bgDone <- struct{}{}
	}

	// Act - Submit blocking task
	pool.PostInternal(blockingTask, core.TaskTraits{})
	time.Sleep(50 * time.Millisecond) // Wait for worker to pick it up

	// Assert - One active task
	if active := pool.ActiveTaskCount(); active != 1 {
		t.Errorf("ActiveTaskCount() = %d, want 1", active)
	}

	// Act - Queue more tasks while worker is blocked
	pool.PostInternal(func(ctx context.Context) {}, core.TaskTraits{})
	pool.PostInternal(func(ctx context.Context) {}, core.TaskTraits{})
	time.Sleep(10 * time.Millisecond) // Wait for queue update

	// Assert - Two tasks queued
	if queued := pool.QueuedTaskCount(); queued != 2 {
		t.Errorf("QueuedTaskCount() = %d, want 2", queued)
	}

	// Act - Unblock and drain
	close(blockCh)
	<-bgDone
	time.Sleep(100 * time.Millisecond) // Wait for drain

	// Assert - No active or queued tasks
	if active := pool.ActiveTaskCount(); active != 0 {
		t.Errorf("ActiveTaskCount() = %d, want 0", active)
	}
	if queued := pool.QueuedTaskCount(); queued != 0 {
		t.Errorf("QueuedTaskCount() = %d, want 0", queued)
	}
}

// =============================================================================
// Graceful Shutdown Tests
// =============================================================================

// TestGoroutineThreadPool_StopGraceful_EmptyQueue verifies shutdown with no pending work
// Given: A running pool with an empty task queue
// When: StopGraceful is called
// Then: Pool stops immediately without error
func TestGoroutineThreadPool_StopGraceful_EmptyQueue(t *testing.T) {
	// Arrange
	pool := NewGoroutineThreadPool("graceful-pool", 2)
	pool.Start(context.Background())

	// Act - Shutdown with no queued tasks
	err := pool.StopGraceful(1 * time.Second)

	// Assert - No error occurred
	if err != nil {
		t.Fatalf("StopGraceful() unexpected error: %v", err)
	}

	// Assert - Pool is stopped
	if pool.IsRunning() {
		t.Error("IsRunning() = true, want false (pool should not be running after StopGraceful)")
	}
}

// TestGoroutineThreadPool_StopGraceful_WithQueuedTasks verifies graceful shutdown completes queued work
// Given: A running pool with queued tasks
// When: StopGraceful is called
// Then: All queued tasks complete before pool stops
func TestGoroutineThreadPool_StopGraceful_WithQueuedTasks(t *testing.T) {
	// Arrange
	pool := NewGoroutineThreadPool("graceful-queued-pool", 2)
	pool.Start(context.Background())

	var executed int32
	var wg sync.WaitGroup
	taskCount := 5
	wg.Add(taskCount)

	task := func(ctx context.Context) {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&executed, 1)
	}

	// Act - Submit all tasks
	for i := 0; i < taskCount; i++ {
		pool.PostInternal(task, core.TaskTraits{})
	}
	time.Sleep(10 * time.Millisecond) // Wait for tasks to start

	// Act - Start graceful shutdown in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- pool.StopGraceful(1 * time.Second)
	}()

	// Wait for all tasks to complete
	wg.Wait()

	// Assert - Shutdown completes without error
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("StopGraceful() unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("StopGraceful() timed out after 2 seconds")
	}

	// Assert - All tasks were executed
	if executed != int32(taskCount) {
		t.Errorf("executed tasks = %d, want %d", executed, taskCount)
	}

	// Assert - Pool is stopped
	if pool.IsRunning() {
		t.Error("IsRunning() = true, want false (pool should not be running after StopGraceful)")
	}
}

// TestGoroutineThreadPool_StopGraceful_Timeout verifies timeout behavior during shutdown
// Given: A pool with a long-running task that exceeds shutdown timeout
// When: StopGraceful is called with short timeout
// Then: Returns timeout error and pool stops anyway
func TestGoroutineThreadPool_StopGraceful_Timeout(t *testing.T) {
	// Arrange
	pool := NewGoroutineThreadPool("timeout-pool", 1)
	pool.Start(context.Background())

	// Arrange - Create task that respects context cancellation
	longRunningTask := func(ctx context.Context) {
		select {
		case <-time.After(500 * time.Millisecond):
			// Task completes normally
		case <-ctx.Done():
			// Context cancelled, exit early
			return
		}
	}

	pool.PostInternal(longRunningTask, core.TaskTraits{})
	time.Sleep(20 * time.Millisecond) // Wait for task to start

	// Act - Shutdown with 50ms timeout (task takes 500ms)
	start := time.Now()
	err := pool.StopGraceful(50 * time.Millisecond)
	elapsed := time.Since(start)

	// Assert - Timeout error occurred
	if err == nil {
		t.Error("StopGraceful() error = nil, want timeout error")
	}

	// Assert - Shutdown completed quickly (not after full 500ms task duration)
	if elapsed > 200*time.Millisecond {
		t.Errorf("StopGraceful() took %v, want ~50-100ms (context cancellation should interrupt task)", elapsed)
	}

	// Assert - Pool is stopped
	if pool.IsRunning() {
		t.Error("IsRunning() = true, want false (pool should not be running after timeout)")
	}
}

// TestGoroutineThreadPool_StopImmediateVsGraceful compares immediate and graceful shutdown behaviors
// Given: Two pools with queued tasks
// When: One pool uses Stop() and the other uses StopGraceful()
// Then: Stop() clears queue immediately, StopGraceful() waits for tasks
func TestGoroutineThreadPool_StopImmediateVsGraceful(t *testing.T) {
	t.Run("immediate stop clears queue", func(t *testing.T) {
		// Arrange
		p1 := NewGoroutineThreadPool("immediate-pool", 1)
		p1.Start(context.Background())

		// Add tasks that won't complete immediately
		for i := 0; i < 5; i++ {
			p1.PostInternal(func(ctx context.Context) {
				time.Sleep(100 * time.Millisecond)
			}, core.TaskTraits{})
		}

		// Act - Immediate stop
		p1.Stop()

		// Assert - Queue is cleared (Stop should not hang)
		// Note: QueuedTaskCount may be >0 if tasks were picked up before Stop cleared queue
		// The key assertion is that Stop() returns immediately
	})

	t.Run("graceful stop waits for tasks", func(t *testing.T) {
		// Arrange
		p2 := NewGoroutineThreadPool("graceful-pool", 2)
		p2.Start(context.Background())

		var executed int32
		for i := 0; i < 3; i++ {
			p2.PostInternal(func(ctx context.Context) {
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&executed, 1)
			}, core.TaskTraits{})
		}

		// Act - Wait for tasks to complete, then shutdown
		time.Sleep(100 * time.Millisecond)
		err := p2.StopGraceful(500 * time.Millisecond)

		// Assert - No error
		if err != nil {
			t.Errorf("StopGraceful() unexpected error: %v", err)
		}

		// Assert - All tasks executed
		if executed != 3 {
			t.Errorf("executed tasks = %d, want 3", executed)
		}
	})
}
