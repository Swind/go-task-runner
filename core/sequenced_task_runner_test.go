package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// MockThreadPool implements ThreadPool for testing
type MockThreadPool struct {
	postedTasks []struct {
		Task   Task
		Traits TaskTraits
	}
}

func (m *MockThreadPool) PostInternal(task Task, traits TaskTraits) {
	m.postedTasks = append(m.postedTasks, struct {
		Task   Task
		Traits TaskTraits
	}{task, traits})
}

func (m *MockThreadPool) PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner) {
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
	runner := NewSequencedTaskRunner(mockPool)

	var executionOrder []int

	// Helper to create valid task
	createTask := func(id int) Task {
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
	runner := NewSequencedTaskRunner(mockPool)

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
	runner := NewSequencedTaskRunner(mockPool)

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
	runner := NewSequencedTaskRunner(mockPool)

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
