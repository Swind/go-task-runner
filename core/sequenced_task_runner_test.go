package core_test

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	core "github.com/Swind/go-task-runner/core"
)

// MockThreadPool implements ThreadPool for testing
type MockThreadPool struct {
	postedTasks []struct {
		Task   core.Task
		Traits core.TaskTraits
	}
}

func (m *MockThreadPool) PostInternal(task core.Task, traits core.TaskTraits) {
	m.postedTasks = append(m.postedTasks, struct {
		Task   core.Task
		Traits core.TaskTraits
	}{task, traits})
}

func (m *MockThreadPool) PostDelayedInternal(task core.Task, delay time.Duration, traits core.TaskTraits, target core.TaskRunner) {
	// Not needed for this test yet
}

func (m *MockThreadPool) Start(ctx context.Context) {}
func (m *MockThreadPool) Stop()                     {}
func (m *MockThreadPool) Join()                     {}
func (m *MockThreadPool) ID() string                { return "mock" }
func (m *MockThreadPool) IsRunning() bool           { return true }
func (m *MockThreadPool) WorkerCount() int          { return 1 }
func (m *MockThreadPool) QueuedTaskCount() int      { return 0 }
func (m *MockThreadPool) ActiveTaskCount() int      { return 0 }
func (m *MockThreadPool) DelayedTaskCount() int     { return 0 }

// TestSequencedTaskRunner_SequentialExecution verifies FIFO task execution
// Given: A SequencedTaskRunner with mock thread pool
// When: Multiple tasks are posted
// Then: Tasks execute in FIFO order, one at a time
func TestSequencedTaskRunner_SequentialExecution(t *testing.T) {
	// Arrange
	mockPool := &MockThreadPool{}
	runner := core.NewSequencedTaskRunner(mockPool)

	var executionOrder []int
	createTask := func(id int) core.Task {
		return func(ctx context.Context) {
			executionOrder = append(executionOrder, id)
		}
	}

	// Act - Post Task 1
	runner.PostTask(createTask(1))

	// Assert - runLoop was posted
	if len(mockPool.postedTasks) != 1 {
		t.Fatalf("len(postedTasks) = %d, want 1 (runLoop)", len(mockPool.postedTasks))
	}

	// Act - Execute runLoop (simulates worker execution)
	runLoopTask := mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil
	runLoopTask(context.Background())

	// Assert - Task 1 executed
	if len(executionOrder) != 1 || executionOrder[0] != 1 {
		t.Error("Task 1 did not execute")
	}

	// Act - Post Tasks 2 & 3
	runner.PostTask(createTask(2))
	runner.PostTask(createTask(3))

	// Assert - runLoop reposted for Task 2
	if len(mockPool.postedTasks) == 0 {
		t.Fatal("runLoop not posted for Task 2")
	}

	// Act - Execute runLoop for Task 2
	runLoopTask = mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil
	runLoopTask(context.Background())

	// Act - Execute Task 3 if reposted
	if len(executionOrder) == 2 && len(mockPool.postedTasks) == 1 {
		mockPool.postedTasks[0].Task(context.Background())
	}

	// Assert - All tasks executed
	if len(executionOrder) != 3 {
		t.Errorf("execution order length = %d, want 3", len(executionOrder))
	}
}

// TestSequencedTaskRunner_Shutdown_PreventsNewTasks verifies shutdown prevents new tasks
// Given: A SequencedTaskRunner with one executed task
// When: Shutdown is called and a new task is posted
// Then: New task is rejected and does not execute
func TestSequencedTaskRunner_Shutdown_PreventsNewTasks(t *testing.T) {
	// Arrange
	mockPool := &MockThreadPool{}
	runner := core.NewSequencedTaskRunner(mockPool)

	var executed atomic.Int32
	task1 := func(ctx context.Context) { executed.Add(1) }
	runner.PostTask(task1)

	// Execute runLoop for task1
	if len(mockPool.postedTasks) != 1 {
		t.Fatalf("len(postedTasks) = %d, want 1", len(mockPool.postedTasks))
	}
	runLoop := mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil
	runLoop(context.Background())

	// Assert - task1 executed
	if executed.Load() != 1 {
		t.Fatalf("executed = %d, want 1 (task1 before shutdown)", executed.Load())
	}

	// Act - Shutdown the runner
	runner.Shutdown()

	// Act - Try to post task2 after shutdown
	task2 := func(ctx context.Context) { executed.Add(1) }
	runner.PostTask(task2)

	// Assert - No new runLoop posted after shutdown
	if len(mockPool.postedTasks) != 0 {
		t.Errorf("postedTasks after shutdown = %d, want 0", len(mockPool.postedTasks))
	}

	// Assert - task2 did not execute
	if executed.Load() != 1 {
		t.Errorf("executed = %d, want 1 (task2 should not run)", executed.Load())
	}
}

// TestSequencedTaskRunner_Shutdown_ClearsPendingQueue verifies shutdown clears pending tasks
// Given: A SequencedTaskRunner with 2 queued tasks
// When: Shutdown is called before any execution
// Then: Pending queue is cleared and no tasks execute
func TestSequencedTaskRunner_Shutdown_ClearsPendingQueue(t *testing.T) {
	// Arrange
	mockPool := &MockThreadPool{}
	runner := core.NewSequencedTaskRunner(mockPool)

	var executed atomic.Int32
	task1 := func(ctx context.Context) { executed.Add(1) }
	task2 := func(ctx context.Context) { executed.Add(1) }

	// Act - Post two tasks
	runner.PostTask(task1)
	runner.PostTask(task2)

	// Act - Shutdown before execution
	runner.Shutdown()

	// Act - Execute posted runLoop
	if len(mockPool.postedTasks) != 1 {
		t.Fatalf("len(postedTasks) = %d, want 1", len(mockPool.postedTasks))
	}
	runLoop := mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil
	runLoop(context.Background())

	// Assert - No tasks executed after shutdown
	if executed.Load() != 0 {
		t.Errorf("executed = %d, want 0 (queue cleared)", executed.Load())
	}
}

// TestSequencedTaskRunner_Shutdown_FromTaskPreventsFurtherPosts verifies shutdown from task prevents further posts
// Given: A task that calls Shutdown and posts another task
// When: The task executes
// Then: Second post is rejected and only first task executes
func TestSequencedTaskRunner_Shutdown_FromTaskPreventsFurtherPosts(t *testing.T) {
	// Arrange
	mockPool := &MockThreadPool{}
	runner := core.NewSequencedTaskRunner(mockPool)

	var executed atomic.Int32
	task1 := func(ctx context.Context) {
		executed.Add(1)
		runner.Shutdown()
		runner.PostTask(func(ctx context.Context) { executed.Add(1) })
	}

	// Act
	runner.PostTask(task1)

	// Execute runLoop
	if len(mockPool.postedTasks) != 1 {
		t.Fatalf("len(postedTasks) = %d, want 1", len(mockPool.postedTasks))
	}
	runLoop := mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil
	runLoop(context.Background())

	// Assert - No additional runLoop after shutdown from task
	if len(mockPool.postedTasks) != 0 {
		t.Errorf("postedTasks after shutdown = %d, want 0", len(mockPool.postedTasks))
	}

	// Assert - Only first task executed
	if executed.Load() != 1 {
		t.Errorf("executed = %d, want 1", executed.Load())
	}
}

// TestSequencedTaskRunner_Integration_WithRealThreadPool verifies integration with real thread pool
// Given: A SequencedTaskRunner with real GoroutineThreadPool
// When: Tasks are posted and then runner is shut down
// Then: Tasks execute correctly and new tasks are rejected after shutdown
func TestSequencedTaskRunner_Integration_WithRealThreadPool(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var executed atomic.Int32
	task := func(ctx context.Context) { executed.Add(1) }

	// Act - Post several tasks
	runner.PostTask(task)
	runner.PostTask(task)
	runner.PostTask(task)
	time.Sleep(100 * time.Millisecond)

	// Assert - All tasks executed
	if executed.Load() != 3 {
		t.Errorf("executed = %d, want 3", executed.Load())
	}

	// Act - Shutdown
	runner.Shutdown()

	// Act - Try to post after shutdown
	runner.PostTask(task)
	time.Sleep(50 * time.Millisecond)

	// Assert - No new tasks executed
	if executed.Load() != 3 {
		t.Errorf("executed = %d, want 3 (no new tasks after shutdown)", executed.Load())
	}

	// Assert - Runner reports closed
	if !runner.IsClosed() {
		t.Error("IsClosed() = false, want true")
	}
}

// TestSequencedTaskRunner_Integration_WithGlobalThreadPool verifies integration with global thread pool
// Given: A SequencedTaskRunner using global thread pool
// When: Tasks are posted and runner is shut down
// Then: Tasks execute correctly and runner reports closed
func TestSequencedTaskRunner_Integration_WithGlobalThreadPool(t *testing.T) {
	// Arrange
	taskrunner.InitGlobalThreadPool(2)
	defer taskrunner.ShutdownGlobalThreadPool()

	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	var executed atomic.Int32
	task := func(ctx context.Context) { executed.Add(1) }

	// Act - Post tasks
	runner.PostTask(task)
	runner.PostTask(task)
	runner.PostTask(task)
	time.Sleep(100 * time.Millisecond)

	// Assert - All tasks executed
	if executed.Load() != 3 {
		t.Errorf("executed = %d, want 3", executed.Load())
	}

	// Act - Shutdown
	runner.Shutdown()

	// Act - Try to post after shutdown
	runner.PostTask(task)
	time.Sleep(50 * time.Millisecond)

	// Assert - No new tasks executed
	if executed.Load() != 3 {
		t.Errorf("executed = %d, want 3 (no new tasks after shutdown)", executed.Load())
	}

	// Assert - Runner reports closed
	if !runner.IsClosed() {
		t.Error("IsClosed() = false, want true")
	}
}

// TestSequencedTaskRunner_WaitIdle verifies waiting for idle state
// Given: A SequencedTaskRunner with 10 delayed tasks
// When: WaitIdle is called
// Then: Returns after all tasks complete
func TestSequencedTaskRunner_WaitIdle(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("wait-idle-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var counter atomic.Int32
	taskCount := 10

	// Act - Post tasks
	for i := 0; i < taskCount; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	// Act - Wait for idle
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitIdle(ctx)

	// Assert
	if err != nil {
		t.Fatalf("WaitIdle() error = %v, want nil", err)
	}

	if count := counter.Load(); int(count) != taskCount {
		t.Errorf("executed tasks = %d, want %d", count, taskCount)
	}
}

// TestSequencedTaskRunner_FlushAsync verifies async flush callback functionality
// Given: A SequencedTaskRunner with tasks
// When: FlushAsync is called with callback
// Then: Callback executes after all tasks complete
func TestSequencedTaskRunner_FlushAsync(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("flush-async-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var counter atomic.Int32
	var flushCalled atomic.Bool
	taskCount := 10

	// Act - Post tasks
	for i := 0; i < taskCount; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	// Act - FlushAsync with callback
	done := make(chan struct{})
	runner.FlushAsync(func() {
		flushCalled.Store(true)
		close(done)
	})

	// Assert - Wait for callback
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Flush callback timed out")
	}

	if !flushCalled.Load() {
		t.Error("flush callback not called")
	}
}

// TestSequencedTaskRunner_WaitShutdown verifies shutdown signal waiting
// Given: A goroutine waiting for shutdown signal
// When: Runner is shut down
// Then: WaitShutdown returns and signal is received
func TestSequencedTaskRunner_WaitShutdown(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("wait-shutdown-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var shutdownReceived atomic.Bool
	var executed atomic.Int32

	// Act - Start goroutine waiting for shutdown
	go func() {
		err := runner.WaitShutdown(context.Background())
		if err != nil {
			t.Errorf("WaitShutdown() error = %v", err)
		}
		shutdownReceived.Store(true)
	}()

	// Act - Post task
	runner.PostTask(func(ctx context.Context) {
		time.Sleep(50 * time.Millisecond)
		executed.Add(1)
	})

	time.Sleep(100 * time.Millisecond)

	// Act - Shutdown
	runner.Shutdown()

	// Wait for signal propagation
	time.Sleep(100 * time.Millisecond)

	// Assert
	if !shutdownReceived.Load() {
		t.Error("shutdown signal not received")
	}

	if !runner.IsClosed() {
		t.Error("IsClosed() = false, want true")
	}

	if executed.Load() != 1 {
		t.Errorf("executed = %d, want 1", executed.Load())
	}
}

// TestSequencedTaskRunner_GarbageCollection verifies runner can be garbage collected
// Given: A SequencedTaskRunner with finalizer set
// When: Runner goes out of scope and GC is triggered
// Then: Finalizer is called
func TestSequencedTaskRunner_GarbageCollection(t *testing.T) {
	// Arrange
	finalizerCalled := make(chan struct{})

	// Create scope so runner goes out of scope
	func() {
		pool := &MockThreadPool{}
		runner := core.NewSequencedTaskRunner(pool)

		runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
			close(finalizerCalled)
		})

		runner.Shutdown()
		// runner goes out of scope
	}()

	// Act - Trigger GC multiple times
	for i := 0; i < 5; i++ {
		runtime.GC()
		select {
		case <-finalizerCalled:
			return // Success
		case <-time.After(100 * time.Millisecond):
			// Continue trying
		}
	}

	t.Fatal("SequencedTaskRunner not garbage collected (finalizer not called)")
}

// TestSequencedTaskRunner_GarbageCollection_WithRealThreadPool verifies GC with real pool
// Given: A SequencedTaskRunner with real thread pool
// When: Runner is shut down and goes out of scope
// Then: Runner can be garbage collected
func TestSequencedTaskRunner_GarbageCollection_WithRealThreadPool(t *testing.T) {
	// Arrange
	finalizerCalled := make(chan struct{})

	func() {
		pool := taskrunner.NewGoroutineThreadPool("gc-test-pool", 2)
		pool.Start(context.Background())
		defer pool.Stop()

		runner := core.NewSequencedTaskRunner(pool)

		// Post task to verify it's working
		done := make(chan struct{})
		runner.PostTask(func(ctx context.Context) {
			close(done)
		})
		<-done

		runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
			close(finalizerCalled)
		})

		runner.Shutdown()
		// runner goes out of scope
	}()

	// Act - Trigger GC
	for i := 0; i < 10; i++ {
		runtime.GC()
		select {
		case <-finalizerCalled:
			return // Success
		case <-time.After(100 * time.Millisecond):
			// Continue trying
		}
	}

	t.Fatal("SequencedTaskRunner with Real ThreadPool not garbage collected")
}

// TestSequencedTaskRunner_GetThreadPool verifies GetThreadPool returns the pool
// Given: A SequencedTaskRunner created with specific pool
// When: GetThreadPool is called
// Then: Returns the same pool that was passed to constructor
func TestSequencedTaskRunner_GetThreadPool(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	// Act
	retrievedPool := runner.GetThreadPool()

	// Assert
	if retrievedPool != pool {
		t.Error("GetThreadPool() returned different pool")
	}
}
