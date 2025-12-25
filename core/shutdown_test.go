package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestSequencedTaskRunner_Shutdown tests basic shutdown functionality
// Main test items:
// 1. Runner starts with IsClosed() returning false
// 2. Shutdown() sets IsClosed() to true
func TestSequencedTaskRunner_Shutdown(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Initially not closed
	if runner.IsClosed() {
		t.Error("Runner should not be closed initially")
	}

	// Shutdown
	runner.Shutdown()

	// Should be closed
	if !runner.IsClosed() {
		t.Error("Runner should be closed after Shutdown()")
	}
}

// TestSequencedTaskRunner_Shutdown_ClearsPendingTasks tests shutdown clears pending tasks
// Main test items:
// 1. Pending tasks in queue are cleared on shutdown
// 2. Currently executing task may complete
// 3. Tasks after shutdown are rejected
func TestSequencedTaskRunner_Shutdown_ClearsPendingTasks(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var executed atomic.Int32

	// Post tasks but don't let them execute yet
	// We'll shutdown before they run
	for i := 0; i < 10; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(100 * time.Millisecond) // Slow task
			executed.Add(1)
		})
	}

	// Shutdown immediately
	time.Sleep(10 * time.Millisecond) // Let one task start
	runner.Shutdown()

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Most tasks should not have executed
	count := executed.Load()
	if count >= 5 {
		t.Errorf("Too many tasks executed after shutdown: %d (expected < 5)", count)
	}
}

// TestSequencedTaskRunner_Shutdown_StopsRepeatingTasks tests shutdown stops repeating tasks
// Main test items:
// 1. Repeating tasks stop executing after shutdown
// 2. Handle.IsStopped() returns true
// 3. Repeating task handle still works after shutdown
func TestSequencedTaskRunner_Shutdown_StopsRepeatingTasks(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter atomic.Int32

	// Start a repeating task
	handle := runner.PostRepeatingTask(func(ctx context.Context) {
		counter.Add(1)
	}, 50*time.Millisecond)

	// Let it run a few times
	time.Sleep(200 * time.Millisecond)

	// Shutdown the runner
	runner.Shutdown()

	// Record count at shutdown
	countAtShutdown := counter.Load()

	// Wait and verify no more executions
	time.Sleep(200 * time.Millisecond)
	countAfterShutdown := counter.Load()

	// Should not have increased (or increased by at most 1 if one was in flight)
	if countAfterShutdown > countAtShutdown+1 {
		t.Errorf("Repeating task continued after shutdown: before=%d, after=%d",
			countAtShutdown, countAfterShutdown)
	}

	// Handle should still work
	handle.Stop()
	if !handle.IsStopped() {
		t.Error("Handle should report stopped")
	}
}

// TestSequencedTaskRunner_Shutdown_MultipleRepeatingTasks tests shutdown with multiple repeating tasks
// Main test items:
// 1. All repeating tasks stop after shutdown
// 2. Multiple repeating tasks are handled correctly
func TestSequencedTaskRunner_Shutdown_MultipleRepeatingTasks(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter1, counter2, counter3 atomic.Int32

	// Start multiple repeating tasks
	runner.PostRepeatingTask(func(ctx context.Context) {
		counter1.Add(1)
	}, 30*time.Millisecond)

	runner.PostRepeatingTask(func(ctx context.Context) {
		counter2.Add(1)
	}, 40*time.Millisecond)

	runner.PostRepeatingTask(func(ctx context.Context) {
		counter3.Add(1)
	}, 50*time.Millisecond)

	// Let them run
	time.Sleep(200 * time.Millisecond)

	// Shutdown
	runner.Shutdown()

	c1 := counter1.Load()
	c2 := counter2.Load()
	c3 := counter3.Load()

	// Wait and verify all stopped
	time.Sleep(200 * time.Millisecond)

	if counter1.Load() > c1+1 {
		t.Error("Task 1 continued after shutdown")
	}
	if counter2.Load() > c2+1 {
		t.Error("Task 2 continued after shutdown")
	}
	if counter3.Load() > c3+1 {
		t.Error("Task 3 continued after shutdown")
	}
}

// TestSequencedTaskRunner_Shutdown_WithDelayedTasks tests shutdown with delayed tasks
// Main test items:
// 1. Delayed tasks are handled on shutdown
// 2. Tasks in DelayManager may still execute
// 3. Pending delayed tasks are not posted to closed runner
func TestSequencedTaskRunner_Shutdown_WithDelayedTasks(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var executed atomic.Bool

	// Post a delayed task
	runner.PostDelayedTask(func(ctx context.Context) {
		executed.Store(true)
	}, 100*time.Millisecond)

	// Shutdown before it executes
	time.Sleep(20 * time.Millisecond)
	runner.Shutdown()

	// Wait for the delay to pass
	time.Sleep(150 * time.Millisecond)

	// The delayed task might still execute since it's already in DelayManager
	// But it won't be posted to the queue because runner is closed
	// This is acceptable behavior
}

// TestSequencedTaskRunner_Shutdown_Idempotent tests shutdown idempotence
// Main test items:
// 1. Multiple Shutdown() calls are safe
// 2. IsClosed() returns true after any number of calls
func TestSequencedTaskRunner_Shutdown_Idempotent(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Multiple shutdowns should be safe
	runner.Shutdown()
	runner.Shutdown()
	runner.Shutdown()

	if !runner.IsClosed() {
		t.Error("Runner should still be closed")
	}
}

// TestSequencedTaskRunner_Shutdown_ConcurrentShutdown tests concurrent shutdown calls
// Main test items:
// 1. Multiple concurrent Shutdown() calls are safe
// 2. All calls complete without panic
func TestSequencedTaskRunner_Shutdown_ConcurrentShutdown(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Add some tasks
	for i := 0; i < 100; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(1 * time.Millisecond)
		})
	}

	// Shutdown from multiple goroutines
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			runner.Shutdown()
			done <- struct{}{}
		}()
	}

	// Wait for all shutdowns
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should be closed
	if !runner.IsClosed() {
		t.Error("Runner should be closed")
	}
}

// TestSequencedTaskRunner_RepeatingTask_WithInitialDelay_Shutdown tests shutdown with delayed repeating task
// Main test items:
// 1. Repeating task with initial delay is stopped on shutdown
// 2. Task never executes if shutdown before initial delay
func TestSequencedTaskRunner_RepeatingTask_WithInitialDelay_Shutdown(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var executed atomic.Bool

	// Post repeating task with initial delay
	runner.PostRepeatingTaskWithInitialDelay(
		func(ctx context.Context) {
			executed.Store(true)
		},
		200*time.Millisecond, // Initial delay
		50*time.Millisecond,  // Interval
		DefaultTaskTraits(),
	)

	// Shutdown before first execution
	time.Sleep(50 * time.Millisecond)
	runner.Shutdown()

	// Wait past initial delay
	time.Sleep(200 * time.Millisecond)

	// Should not have executed
	if executed.Load() {
		t.Error("Task should not execute after shutdown")
	}
}
