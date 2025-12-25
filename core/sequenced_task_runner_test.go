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

func TestSequencedTaskRunner_SequentialExecution(t *testing.T) {
	mockPool := &MockThreadPool{}
	runner := core.NewSequencedTaskRunner(mockPool)

	var executionOrder []int

	// Helper to create valid task
	createTask := func(id int) core.Task {
		return func(ctx context.Context) {
			executionOrder = append(executionOrder, id)
		}
	}

	// 1. Post Task 1
	runner.PostTask(createTask(1))

	// Should execute immediately via PostInternal(runLoop)
	if len(mockPool.postedTasks) != 1 {
		t.Fatalf("expected 1 posted task (runLoop), got %d", len(mockPool.postedTasks))
	}

	// Simulate Worker executing the runLoop
	runLoopTask := mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil // Reset
	runLoopTask(context.Background())

	if len(executionOrder) != 1 || executionOrder[0] != 1 {
		t.Errorf("Task 1 should have executed")
	}

	// 2. Post Task 2 & 3
	runner.PostTask(createTask(2))
	runner.PostTask(createTask(3))

	// Should trigger runLoop again?
	// If the previous runLoop finished and queue was empty, isRunning became false.
	// But if we post 2, it sets isRunning=true and posts runLoop.

	if len(mockPool.postedTasks) == 0 {
		t.Fatal("expected runLoop to be posted for Task 2")
	}

	// Run loop for Task 2
	runLoopTask = mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil
	runLoopTask(context.Background())

	// After Task 2, does it execute Task 3 immediately in same loop (old behavior) or repost (new behavior)?
	// Old behavior (MaxTasksPerSlice=4): Task 3 executed in same loop.
	// New behavior (MaxTasksPerSlice=1): Task 3 requires repost.

	// Let's verify what happened.
	// If Task 3 is NOT in executionOrder, it means we need to process next repost.

	if len(executionOrder) == 3 {
		// Old behavior: processed batch
		t.Log("Executed in batch")
	} else if len(executionOrder) == 2 {
		// New behavior: executed one, posted next
		if len(mockPool.postedTasks) != 1 {
			t.Errorf("Expected repost for Task 3")
		}
		// Execute Task 3
		mockPool.postedTasks[0].Task(context.Background())
	}

	if len(executionOrder) != 3 {
		t.Errorf("All tasks should operate")
	}
}

func TestSequencedTaskRunner_Shutdown_PreventsNewTasks(t *testing.T) {
	mockPool := &MockThreadPool{}
	runner := core.NewSequencedTaskRunner(mockPool)

	var executed atomic.Int32
	// Task that increments counter
	task1 := func(ctx context.Context) { executed.Add(1) }
	runner.PostTask(task1)

	// Simulate runLoop execution for the posted task
	if len(mockPool.postedTasks) != 1 {
		t.Fatalf("expected 1 posted runLoop task, got %d", len(mockPool.postedTasks))
	}
	runLoop := mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil
	runLoop(context.Background())

	// Verify task1 ran
	if executed.Load() != 1 {
		t.Fatalf("task1 should have executed before shutdown")
	}

	// Shutdown the runner
	runner.Shutdown()

	// Attempt to post another task after shutdown
	task2 := func(ctx context.Context) { executed.Add(1) }
	runner.PostTask(task2)

	// No new runLoop should be posted
	if len(mockPool.postedTasks) != 0 {
		t.Fatalf("no runLoop should be posted after shutdown")
	}

	// Ensure counter unchanged
	if executed.Load() != 1 {
		t.Fatalf("task2 should not have executed after shutdown")
	}
}

func TestSequencedTaskRunner_Shutdown_ClearsPendingQueue(t *testing.T) {
	mockPool := &MockThreadPool{}
	runner := core.NewSequencedTaskRunner(mockPool)

	var executed atomic.Int32
	task1 := func(ctx context.Context) { executed.Add(1) }
	task2 := func(ctx context.Context) { executed.Add(1) }

	// Post two tasks
	runner.PostTask(task1)
	runner.PostTask(task2)

	// Shutdown before any runLoop execution
	runner.Shutdown()

	// Run the posted runLoop (only one should be posted for first task)
	if len(mockPool.postedTasks) != 1 {
		t.Fatalf("expected 1 runLoop task posted, got %d", len(mockPool.postedTasks))
	}
	runLoop := mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil
	runLoop(context.Background())

	// Only first task should have executed; second cleared
	if executed.Load() != 0 {
		t.Fatalf("no tasks should execute after shutdown, got %d", executed.Load())
	}
}

func TestSequencedTaskRunner_Shutdown_FromTaskPreventsFurtherPosts(t *testing.T) {
	mockPool := &MockThreadPool{}
	runner := core.NewSequencedTaskRunner(mockPool)

	var executed atomic.Int32
	// Task that shuts down runner and then tries to post another task
	task1 := func(ctx context.Context) {
		executed.Add(1)
		runner.Shutdown()
		// Attempt to post a second task
		runner.PostTask(func(ctx context.Context) { executed.Add(1) })
	}

	runner.PostTask(task1)

	// Run the runLoop
	if len(mockPool.postedTasks) != 1 {
		t.Fatalf("expected runLoop task posted, got %d", len(mockPool.postedTasks))
	}
	runLoop := mockPool.postedTasks[0].Task
	mockPool.postedTasks = nil
	runLoop(context.Background())

	// No additional runLoop should be posted after shutdown
	if len(mockPool.postedTasks) != 0 {
		t.Fatalf("no additional runLoop should be posted after shutdown inside task")
	}

	if executed.Load() != 1 {
		t.Fatalf("only the first task should have executed, got %d", executed.Load())
	}
}

func TestSequencedTaskRunner_Integration_WithRealThreadPool(t *testing.T) {
	// Create and start a real GoroutineThreadPool with 2 workers
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	// Create a SequencedTaskRunner using the real pool
	runner := core.NewSequencedTaskRunner(pool)

	var executed atomic.Int32
	task := func(ctx context.Context) { executed.Add(1) }

	// Post several tasks
	runner.PostTask(task)
	runner.PostTask(task)
	runner.PostTask(task)

	// Allow some time for tasks to be processed
	time.Sleep(100 * time.Millisecond)

	if executed.Load() != 3 {
		t.Fatalf("expected 3 tasks to have executed, got %d", executed.Load())
	}

	// Shutdown the runner
	runner.Shutdown()

	// Attempt to post another task after shutdown
	runner.PostTask(task)

	// Wait to ensure no new tasks are processed
	time.Sleep(50 * time.Millisecond)

	if executed.Load() != 3 {
		t.Fatalf("no tasks should run after shutdown, expected 3, got %d", executed.Load())
	}

	if !runner.IsClosed() {
		t.Fatalf("runner should report closed after shutdown")
	}
}

func TestSequencedTaskRunner_Integration_WithGlobalThreadPool(t *testing.T) {
	// Initialize global thread pool with 2 workers
	taskrunner.InitGlobalThreadPool(2)
	defer taskrunner.ShutdownGlobalThreadPool()

	// Create a SequencedTaskRunner using the global pool
	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	var executed atomic.Int32
	// Simple task increments counter
	task := func(ctx context.Context) { executed.Add(1) }

	// Post a few tasks
	runner.PostTask(task)
	runner.PostTask(task)
	runner.PostTask(task)

	// Allow some time for tasks to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify that tasks have run (up to 3)
	if executed.Load() != 3 {
		t.Fatalf("expected 3 tasks to have executed, got %d", executed.Load())
	}

	// Shutdown the runner
	runner.Shutdown()

	// Attempt to post another task after shutdown
	runner.PostTask(task)

	// Wait a bit to ensure no new tasks are processed
	time.Sleep(50 * time.Millisecond)

	// Counter should remain at 3
	if executed.Load() != 3 {
		t.Fatalf("no tasks should run after shutdown, expected 3, got %d", executed.Load())
	}

	if !runner.IsClosed() {
		t.Fatalf("runner should report closed after shutdown")
	}
}

func TestSequencedTaskRunner_WaitIdle(t *testing.T) {
	// 1. Setup real thread pool
	pool := taskrunner.NewGoroutineThreadPool("wait-idle-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var counter atomic.Int32
	taskCount := 10

	// 2. Post tasks
	for i := 0; i < taskCount; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	// 3. WaitIdle
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitIdle(ctx)
	if err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	// 4. Verify all tasks executed
	if count := counter.Load(); int(count) != taskCount {
		t.Errorf("Expected %d tasks to complete, got %d", taskCount, count)
	}
}

func TestSequencedTaskRunner_FlushAsync(t *testing.T) {
	// 1. Setup real thread pool
	pool := taskrunner.NewGoroutineThreadPool("flush-async-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var counter atomic.Int32
	var flushCalled atomic.Bool
	taskCount := 10

	// 2. Post tasks
	for i := 0; i < taskCount; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	// 3. FlushAsync
	runner.FlushAsync(func() {
		flushCalled.Store(true)
		if count := counter.Load(); int(count) != taskCount {
			t.Errorf("Flush called but not all tasks completed: %d/%d", count, taskCount)
		}
	})

	// 4. Wait for flush - FlushAsync is non-blocking, so we sleep a bit or use a channel to wait for callback
	// Ideally we pass a channel to the flush callback to signal done.
	done := make(chan struct{})
	runner.FlushAsync(func() {
		close(done)
	})

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Flush callback timed out")
	}

	if !flushCalled.Load() {
		t.Error("First Flush callback was not called")
	}
}

func TestSequencedTaskRunner_WaitShutdown(t *testing.T) {
	// 1. Setup real thread pool
	pool := taskrunner.NewGoroutineThreadPool("wait-shutdown-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var shutdownReceived atomic.Bool
	var executed atomic.Int32

	// 2. Goroutine waiting for shutdown
	go func() {
		err := runner.WaitShutdown(context.Background())
		if err != nil {
			t.Errorf("WaitShutdown failed: %v", err)
		}
		shutdownReceived.Store(true)
	}()

	// 3. Post some work
	runner.PostTask(func(ctx context.Context) {
		time.Sleep(50 * time.Millisecond)
		executed.Add(1)
	})

	// 4. Shutdown after a delay
	time.Sleep(100 * time.Millisecond)
	runner.Shutdown()

	// 5. Verify shutdown signal received
	// Give it a moment to propagate
	time.Sleep(100 * time.Millisecond)

	if !shutdownReceived.Load() {
		t.Error("WaitShutdown did not receive shutdown signal")
	}

	if !runner.IsClosed() {
		t.Error("Runner should be closed")
	}

	if executed.Load() != 1 {
		t.Errorf("Task should have executed, executed count: %d", executed.Load())
	}
}

func TestSequencedTaskRunner_GarbageCollection(t *testing.T) {
	// 1. Setup
	// Create a channel to signal finalizer execution
	finalizerCalled := make(chan struct{})

	// Create a scope to ensure runner reference is dropped
	func() {
		// Use a real pool or mock pool (interface satisfaction is key)
		pool := &MockThreadPool{}
		runner := core.NewSequencedTaskRunner(pool)

		// Set finalizer
		runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
			close(finalizerCalled)
		})

		// Shutdown the runner (optional, but good practice to release resources)
		runner.Shutdown()

		// runner goes out of scope here
	}()

	// 2. Trigger GC
	// We might need multiple GC cycles to ensure collection
	for i := 0; i < 5; i++ {
		runtime.GC()
		select {
		case <-finalizerCalled:
			return // Success
		case <-time.After(100 * time.Millisecond):
			// Continue trying
		}
	}

	t.Fatal("SequencedTaskRunner was not garbage collected (finalizer not called)")
}

func TestSequencedTaskRunner_GarbageCollection_WithRealThreadPool(t *testing.T) {
	// 1. Setup
	finalizerCalled := make(chan struct{})

	// Create a scope to ensure runner reference is dropped
	func() {
		// Use a real GoroutineThreadPool
		pool := taskrunner.NewGoroutineThreadPool("gc-test-pool", 2)
		pool.Start(context.Background())
		// Ensure pool is stopped to clean up its own resources, though for this test
		// we mainly care about the runner being collected.
		// Note: stopping the pool might be important if the pool holds references.
		// However, SequencedTaskRunner holds reference TO the pool, not vice versa (usually).
		// But tasks in the pool hold reference to the runner (in the closure).
		// So we must ensure no tasks are pending that reference the runner.
		defer pool.Stop()

		runner := core.NewSequencedTaskRunner(pool)

		// Post a task to ensure it's working and tied to the pool
		done := make(chan struct{})
		runner.PostTask(func(ctx context.Context) {
			close(done)
		})
		<-done

		// Set finalizer
		runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
			close(finalizerCalled)
		})

		// Shutdown the runner
		// This is CRITICAL. It clears the queue and sets isRunning=false.
		// If we don't shutdown, and if there were pending tasks (there aren't here),
		// or if the runner was somehow registered in the pool (it isn't generally, mostly tasks),
		// it might stay alive.
		// More importantly, the user code is expected to Shutdown() before losing reference if they want clean cleanup,
		// though GC should technically work if no goroutines reference it.
		// But SequencedTaskRunner has a 'runLoop' executed by the pool.
		// If runLoop is active, it references 'r'.
		// runLoop terminates when queue is empty.
		// So if queue is empty, runLoop should exit, and pool should drop reference to runLoop closure.
		runner.Shutdown()

		// runner goes out of scope here
	}()

	// 2. Trigger GC
	for i := 0; i < 10; i++ {
		runtime.GC()
		select {
		case <-finalizerCalled:
			return // Success
		case <-time.After(100 * time.Millisecond):
			// Continue trying
		}
	}

	t.Fatal("SequencedTaskRunner with Real ThreadPool was not garbage collected")
}

func TestSequencedTaskRunner_GetThreadPool(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	// GetThreadPool should return the pool that was passed to NewSequencedTaskRunner
	retrievedPool := runner.GetThreadPool()

	if retrievedPool != pool {
		t.Error("GetThreadPool should return the same pool that was passed to NewSequencedTaskRunner")
	}
}
