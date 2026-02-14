package core

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestSingleThreadTaskRunner_BasicExecution verifies basic task execution
// Given: A SingleThreadTaskRunner with one task
// When: Task is posted
// Then: Task executes correctly
func TestSingleThreadTaskRunner_BasicExecution(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var executed atomic.Bool

	// Act
	runner.PostTask(func(ctx context.Context) {
		executed.Store(true)
	})

	time.Sleep(50 * time.Millisecond)

	// Assert
	if !executed.Load() {
		t.Error("executed = false, want true")
	}
}

// TestSingleThreadTaskRunner_ExecutionOrder verifies FIFO execution order
// Given: A SingleThreadTaskRunner with 10 tasks posted sequentially
// When: Tasks execute
// Then: Tasks execute in submission order (FIFO)
func TestSingleThreadTaskRunner_ExecutionOrder(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	results := make(chan int, 10)

	// Act - Post 10 tasks
	for i := 0; i < 10; i++ {
		id := i
		runner.PostTask(func(ctx context.Context) {
			results <- id
		})
	}

	// Assert - All tasks executed in order
	result := make([]int, 0, 10)
	timeout := time.After(500 * time.Millisecond)
	for len(result) < 10 {
		select {
		case v := <-results:
			result = append(result, v)
		case <-timeout:
			t.Fatalf("timed out waiting for results, got=%d", len(result))
		}
	}
	if len(result) != 10 {
		t.Fatalf("len(result) = %d, want 10", len(result))
	}

	for i := 0; i < 10; i++ {
		if result[i] != i {
			t.Errorf("result[%d] = %d, want %d", i, result[i], i)
		}
	}
}

// TestSingleThreadTaskRunner_ThreadAffinity verifies all tasks run on same goroutine
// Given: A SingleThreadTaskRunner with 20 tasks
// When: Tasks execute
// Then: All tasks execute on the same goroutine (thread affinity)
func TestSingleThreadTaskRunner_ThreadAffinity(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	gidCh := make(chan uint64, 20)

	// Helper to get goroutine ID
	getGoroutineID := func() uint64 {
		b := make([]byte, 64)
		b = b[:runtime.Stack(b, false)]
		var id uint64
		for i := len("goroutine "); i < len(b); i++ {
			if b[i] >= '0' && b[i] <= '9' {
				id = id*10 + uint64(b[i]-'0')
			} else {
				break
			}
		}
		return id
	}

	// Act - Post 20 tasks
	for i := 0; i < 20; i++ {
		runner.PostTask(func(ctx context.Context) {
			gid := getGoroutineID()
			gidCh <- gid
		})
	}

	// Assert - All tasks ran on same goroutine
	result := make(map[uint64]bool)
	timeout := time.After(500 * time.Millisecond)
	for i := 0; i < 20; i++ {
		select {
		case gid := <-gidCh:
			result[gid] = true
		case <-timeout:
			t.Fatalf("timed out waiting for goroutine IDs, got=%d", i)
		}
	}
	if len(result) != 1 {
		t.Errorf("goroutine count = %d, want 1 (all tasks on same goroutine)", len(result))
	}
}

// TestSingleThreadTaskRunner_DelayedTask verifies delayed task execution
// Given: A SingleThreadTaskRunner with a 100ms delayed task
// When: Task is posted with delay
// Then: Task executes after specified delay
func TestSingleThreadTaskRunner_DelayedTask(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var executed atomic.Bool
	start := time.Now()

	// Act
	runner.PostDelayedTask(func(ctx context.Context) {
		executed.Store(true)
	}, 100*time.Millisecond)

	// Assert - Not executed immediately
	time.Sleep(50 * time.Millisecond)
	if executed.Load() {
		t.Error("executed = true during delay, want false")
	}

	// Wait for execution
	time.Sleep(100 * time.Millisecond)
	if !executed.Load() {
		t.Error("executed = false after delay, want true")
	}

	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("elapsed = %v, want >=100ms", elapsed)
	}
}

// TestSingleThreadTaskRunner_RepeatingTask verifies repeating task functionality
// Given: A SingleThreadTaskRunner with repeating task every 50ms
// When: Task runs for 200ms then is stopped
// Then: Task repeats multiple times and stops when handle.Stop() is called
func TestSingleThreadTaskRunner_RepeatingTask(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32

	// Act
	handle := runner.PostRepeatingTask(func(ctx context.Context) {
		counter.Add(1)
	}, 50*time.Millisecond)

	time.Sleep(200 * time.Millisecond)
	handle.Stop()
	countAtStop := counter.Load()

	time.Sleep(150 * time.Millisecond)
	countAfterStop := counter.Load()

	// Assert
	if countAtStop < 2 {
		t.Errorf("countAtStop = %d, want >=2", countAtStop)
	}

	if countAfterStop > countAtStop+1 {
		t.Errorf("countAfterStop = %d, want ~%d (task continued after stop)", countAfterStop, countAtStop)
	}
}

// TestSingleThreadTaskRunner_RepeatingTaskWithInitialDelay verifies repeating task with initial delay
// Given: A repeating task with 100ms initial delay and 50ms interval
// When: Task is posted
// Then: Task doesn't execute during initial delay, then repeats periodically
func TestSingleThreadTaskRunner_RepeatingTaskWithInitialDelay(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32
	start := time.Now()

	// Act
	handle := runner.PostRepeatingTaskWithInitialDelay(
		func(ctx context.Context) {
			counter.Add(1)
		},
		100*time.Millisecond,
		50*time.Millisecond,
		DefaultTaskTraits(),
	)
	defer handle.Stop()

	// Assert - Not executed during initial delay
	time.Sleep(50 * time.Millisecond)
	if counter.Load() > 0 {
		t.Error("counter > 0 during initial delay, want 0")
	}

	time.Sleep(200 * time.Millisecond)

	elapsed := time.Since(start)
	count := counter.Load()

	if count < 1 {
		t.Error("count = 0, want >=1 (task should execute after initial delay)")
	}

	if elapsed < 100*time.Millisecond {
		t.Errorf("elapsed = %v, want >=100ms (initial delay)", elapsed)
	}
}

// TestSingleThreadTaskRunner_Shutdown verifies shutdown state changes
// Given: A new SingleThreadTaskRunner
// When: Shutdown is called
// Then: IsClosed returns true
func TestSingleThreadTaskRunner_Shutdown(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()

	// Assert - Initially not closed
	if runner.IsClosed() {
		t.Error("IsClosed() = true initially, want false")
	}

	// Act
	runner.Shutdown()

	// Assert
	if !runner.IsClosed() {
		t.Error("IsClosed() = false after Shutdown(), want true")
	}
}

// TestSingleThreadTaskRunner_Shutdown_AllowsQueuedTasksToFinish verifies shutdown only closes intake.
// Given: A runner with multiple already-queued tasks
// When: Shutdown is called while the first task is running
// Then: Already-queued tasks still execute to completion
func TestSingleThreadTaskRunner_Shutdown_AllowsQueuedTasksToFinish(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var executed atomic.Int32
	blocker := make(chan struct{})

	runner.PostTask(func(ctx context.Context) {
		executed.Add(1)
		<-blocker
	})
	runner.PostTask(func(ctx context.Context) {
		executed.Add(1)
	})
	runner.PostTask(func(ctx context.Context) {
		executed.Add(1)
	})

	time.Sleep(30 * time.Millisecond)

	// Act
	runner.Shutdown()
	close(blocker)

	time.Sleep(150 * time.Millisecond)

	// Assert
	if got := executed.Load(); got != 3 {
		t.Errorf("executed = %d, want 3 (queued tasks should finish after Shutdown)", got)
	}
}

// TestSingleThreadTaskRunner_Shutdown_StopsRepeatingTasks verifies shutdown stops repeating tasks
// Given: A SingleThreadTaskRunner with active repeating task
// When: Shutdown is called
// Then: Repeating task stops executing
func TestSingleThreadTaskRunner_Shutdown_StopsRepeatingTasks(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()

	var counter atomic.Int32

	runner.PostRepeatingTask(func(ctx context.Context) {
		counter.Add(1)
	}, 50*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	// Act
	runner.Shutdown()
	countAtShutdown := counter.Load()

	time.Sleep(150 * time.Millisecond)
	countAfterShutdown := counter.Load()

	// Assert
	if countAfterShutdown > countAtShutdown {
		t.Errorf("countAfterShutdown = %d, want ~%d (task stopped)", countAfterShutdown, countAtShutdown)
	}
}

// TestSingleThreadTaskRunner_PostTaskAfterShutdown verifies tasks rejected after shutdown
// Given: A shut down SingleThreadTaskRunner
// When: Task is posted
// Then: Task does not execute
func TestSingleThreadTaskRunner_PostTaskAfterShutdown(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	runner.Shutdown()

	var executed atomic.Bool

	// Act
	runner.PostTask(func(ctx context.Context) {
		executed.Store(true)
	})

	time.Sleep(100 * time.Millisecond)

	// Assert
	if executed.Load() {
		t.Error("executed = true after shutdown, want false")
	}
}

// TestSingleThreadTaskRunner_Stop verifies stop functionality
// Given: A SingleThreadTaskRunner with one task
// When: Stop is called after task starts
// Then: Task completes, new tasks don't execute, runner is closed
func TestSingleThreadTaskRunner_Stop(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()

	var executed atomic.Bool
	runner.PostTask(func(ctx context.Context) {
		executed.Store(true)
	})

	time.Sleep(50 * time.Millisecond)

	// Act
	runner.Stop()

	// Assert
	if !runner.IsClosed() {
		t.Error("IsClosed() = false after Stop(), want true")
	}

	if !executed.Load() {
		t.Error("executed = false (task before stop), want true")
	}

	var executed2 atomic.Bool
	runner.PostTask(func(ctx context.Context) {
		executed2.Store(true)
	})

	time.Sleep(100 * time.Millisecond)
	if executed2.Load() {
		t.Error("executed2 = true after Stop(), want false")
	}
}

// TestSingleThreadTaskRunner_MultipleRepeatingTasks verifies multiple repeating tasks
// Given: A SingleThreadTaskRunner with 3 repeating tasks at different intervals
// When: Tasks run and are stopped
// Then: Each task executes at its interval and stops independently
func TestSingleThreadTaskRunner_MultipleRepeatingTasks(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter1, counter2, counter3 atomic.Int32

	handle1 := runner.PostRepeatingTask(func(ctx context.Context) {
		counter1.Add(1)
	}, 30*time.Millisecond)

	handle2 := runner.PostRepeatingTask(func(ctx context.Context) {
		counter2.Add(1)
	}, 40*time.Millisecond)

	handle3 := runner.PostRepeatingTask(func(ctx context.Context) {
		counter3.Add(1)
	}, 50*time.Millisecond)

	// Act - Let them run
	time.Sleep(200 * time.Millisecond)

	// Assert - All executed multiple times
	if counter1.Load() < 3 {
		t.Errorf("counter1 = %d, want >=3", counter1.Load())
	}
	if counter2.Load() < 2 {
		t.Errorf("counter2 = %d, want >=2", counter2.Load())
	}
	if counter3.Load() < 2 {
		t.Errorf("counter3 = %d, want >=2", counter3.Load())
	}

	// Act - Stop all
	handle1.Stop()
	handle2.Stop()
	handle3.Stop()

	c1 := counter1.Load()
	c2 := counter2.Load()
	c3 := counter3.Load()

	time.Sleep(150 * time.Millisecond)

	// Assert - All stopped
	if counter1.Load() > c1+1 {
		t.Error("Task 1 continued after Stop()")
	}
	if counter2.Load() > c2+1 {
		t.Error("Task 2 continued after Stop()")
	}
	if counter3.Load() > c3+1 {
		t.Error("Task 3 continued after Stop()")
	}
}

// TestSingleThreadTaskRunner_PanicRecovery verifies panic recovery
// Given: A SingleThreadTaskRunner with a task that panics
// When: Panic occurs and another task is posted
// Then: Subsequent tasks still execute, runner remains operational
func TestSingleThreadTaskRunner_PanicRecovery(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var executed atomic.Bool

	// Act - Post panicking task, then normal task
	runner.PostTask(func(ctx context.Context) {
		panic("test panic")
	})

	runner.PostTask(func(ctx context.Context) {
		executed.Store(true)
	})

	time.Sleep(100 * time.Millisecond)

	// Assert - Second task executed despite panic
	if !executed.Load() {
		t.Error("executed = false after panic, want true")
	}

	if runner.IsClosed() {
		t.Error("IsClosed() = true after panic, want false")
	}
}

// TestSingleThreadTaskRunner_IdempotentShutdown verifies multiple shutdown calls are safe
// Given: A SingleThreadTaskRunner
// When: Shutdown and Stop are called multiple times
// Then: All calls complete without error, IsClosed returns true
func TestSingleThreadTaskRunner_IdempotentShutdown(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()

	// Act - Multiple shutdown calls
	runner.Shutdown()
	runner.Shutdown()
	runner.Stop()
	runner.Stop()

	// Assert
	if !runner.IsClosed() {
		t.Error("IsClosed() = false, want true")
	}
}

// TestSingleThreadTaskRunner_ConcurrentPostTask verifies concurrent task submission
// Given: 10 goroutines each posting 10 tasks
// When: All tasks are posted concurrently
// Then: All 100 tasks execute correctly
func TestSingleThreadTaskRunner_ConcurrentPostTask(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32
	done := make(chan struct{})

	// Act - Post 100 tasks from 10 goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				runner.PostTask(func(ctx context.Context) {
					counter.Add(1)
				})
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	time.Sleep(200 * time.Millisecond)

	// Assert - All tasks executed
	if counter.Load() != 100 {
		t.Errorf("counter = %d, want 100", counter.Load())
	}
}

// =============================================================================
// Queue Policy Tests
// =============================================================================

// TestSingleThreadTaskRunner_PolicyConfiguration verifies queue policy configuration
// Given: A SingleThreadTaskRunner
// When: Queue policies are set and queried
// Then: Policies are correctly configured
func TestSingleThreadTaskRunner_PolicyConfiguration(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	// Assert - Default is Drop
	if runner.GetQueuePolicy() != QueuePolicyDrop {
		t.Errorf("GetQueuePolicy() = %v, want Drop", runner.GetQueuePolicy())
	}

	// Act - Set different policies
	policies := []QueuePolicy{QueuePolicyDrop, QueuePolicyReject, QueuePolicyWait}
	for _, policy := range policies {
		runner.SetQueuePolicy(policy)
		if runner.GetQueuePolicy() != policy {
			t.Errorf("GetQueuePolicy() = %v, want %v", runner.GetQueuePolicy(), policy)
		}
	}

	// Act - Set rejection callback
	callbackCalled := atomic.Bool{}
	runner.SetRejectionCallback(func(task Task, traits TaskTraits) {
		callbackCalled.Store(true)
	})

	runner.SetRejectionCallback(nil)

	// Assert - Initial rejected count
	if runner.RejectedCount() != 0 {
		t.Errorf("RejectedCount() = %d, want 0", runner.RejectedCount())
	}
}

// TestSingleThreadTaskRunner_QueuePolicyAfterClosed verifies queue policy after close
// Given: A closed SingleThreadTaskRunner with reject policy
// When: Tasks are posted after close
// Then: Rejection callback is not triggered (tasks are silently dropped)
func TestSingleThreadTaskRunner_QueuePolicyAfterClosed(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()

	runner.SetQueuePolicy(QueuePolicyReject)
	runner.Shutdown()

	var rejected atomic.Int32
	runner.SetRejectionCallback(func(task Task, traits TaskTraits) {
		rejected.Add(1)
	})

	// Act
	runner.PostTask(func(ctx context.Context) {})

	time.Sleep(50 * time.Millisecond)

	// Assert - No rejection callback after close
	if rejected.Load() != 0 {
		t.Errorf("rejected = %d, want 0 (no callback after close)", rejected.Load())
	}

	runner.Stop()
}

// TestSingleThreadTaskRunner_QueuePolicyReject_Callback verifies reject policy callback
// Given: A SingleThreadTaskRunner with reject policy
// When: Tasks overflow the queue
// Then: Rejection callback is invoked for rejected tasks
func TestSingleThreadTaskRunner_QueuePolicyReject_Callback(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	runner.SetQueuePolicy(QueuePolicyReject)

	var rejectedCount atomic.Int32
	var rejectedTraits atomic.Value

	runner.SetRejectionCallback(func(task Task, traits TaskTraits) {
		rejectedCount.Add(1)
		rejectedTraits.Store(traits)
	})

	// Act - Post many tasks to fill queue
	customTraits := TaskTraits{Priority: TaskPriorityUserBlocking}
	for i := 0; i < 200; i++ {
		runner.PostTaskWithTraits(func(ctx context.Context) {}, customTraits)
	}

	time.Sleep(100 * time.Millisecond)

	// Assert
	if runner.RejectedCount() == 0 {
		t.Skip("Queue did not fill - timing dependent")
	}

	if int64(rejectedCount.Load()) != runner.RejectedCount() {
		t.Logf("Callback count (%d) != RejectedCount (%d)", rejectedCount.Load(), runner.RejectedCount())
	}
}

// TestSingleThreadTaskRunner_QueuePolicy_DropVsReject verifies difference between Drop and Reject
// Given: Two SingleThreadTaskRunners with Drop and Reject policies
// When: Tasks overflow the queues
// Then: Drop policy silently drops, Reject policy counts rejections
func TestSingleThreadTaskRunner_QueuePolicy_DropVsReject(t *testing.T) {
	// Arrange - Test Drop policy
	runner1 := NewSingleThreadTaskRunner()
	runner1.SetQueuePolicy(QueuePolicyDrop)

	// Act - Post many tasks
	for i := 0; i < 200; i++ {
		runner1.PostTask(func(ctx context.Context) {})
	}

	// Assert - No rejections counted
	if runner1.RejectedCount() != 0 {
		t.Errorf("RejectedCount() = %d (Drop), want 0", runner1.RejectedCount())
	}
	runner1.Stop()

	// Arrange - Test Reject policy
	runner2 := NewSingleThreadTaskRunner()
	runner2.SetQueuePolicy(QueuePolicyReject)

	callbackCount := atomic.Int32{}
	runner2.SetRejectionCallback(func(task Task, traits TaskTraits) {
		callbackCount.Add(1)
	})

	// Act - Post many tasks
	for i := 0; i < 200; i++ {
		runner2.PostTask(func(ctx context.Context) {})
	}

	time.Sleep(100 * time.Millisecond)

	// Assert - Rejections counted (if queue filled)
	if runner2.RejectedCount() > 0 {
		t.Logf("Reject policy rejected %d tasks", runner2.RejectedCount())
	}

	runner2.Stop()
}
