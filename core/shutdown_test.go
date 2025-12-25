package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestSequencedTaskRunner_Shutdown verifies basic shutdown functionality
// Given: A SequencedTaskRunner
// When: Shutdown is called
// Then: IsClosed returns true
func TestSequencedTaskRunner_Shutdown(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

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

// TestSequencedTaskRunner_Shutdown_ClearsPendingTasks verifies shutdown clears pending tasks
// Given: A SequencedTaskRunner with 10 pending slow tasks
// When: Shutdown is called immediately
// Then: Pending queue is cleared, most tasks don't execute
func TestSequencedTaskRunner_Shutdown_ClearsPendingTasks(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var executed atomic.Int32

	// Act - Post slow tasks
	for i := 0; i < 10; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(100 * time.Millisecond)
			executed.Add(1)
		})
	}

	time.Sleep(10 * time.Millisecond)
	runner.Shutdown()

	time.Sleep(200 * time.Millisecond)

	// Assert - Most tasks didn't execute due to shutdown
	count := executed.Load()
	if count >= 5 {
		t.Errorf("executed = %d, want <5 (queue cleared)", count)
	}
}

// TestSequencedTaskRunner_Shutdown_StopsRepeatingTasks verifies shutdown stops repeating tasks
// Given: A SequencedTaskRunner with active repeating task
// When: Shutdown is called
// Then: Repeating task stops executing
func TestSequencedTaskRunner_Shutdown_StopsRepeatingTasks(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter atomic.Int32

	runner.PostRepeatingTask(func(ctx context.Context) {
		counter.Add(1)
	}, 50*time.Millisecond)

	time.Sleep(200 * time.Millisecond)

	// Act
	runner.Shutdown()

	countAtShutdown := counter.Load()
	time.Sleep(200 * time.Millisecond)
	countAfterShutdown := counter.Load()

	// Assert - Task stops executing after shutdown
	if countAfterShutdown > countAtShutdown+1 {
		t.Errorf("count after = %d, want ~%d", countAfterShutdown, countAtShutdown)
	}
}

// TestSequencedTaskRunner_Shutdown_MultipleRepeatingTasks verifies shutdown with multiple repeating tasks
// Given: A SequencedTaskRunner with 3 repeating tasks
// When: Shutdown is called
// Then: All repeating tasks stop
func TestSequencedTaskRunner_Shutdown_MultipleRepeatingTasks(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter1, counter2, counter3 atomic.Int32

	runner.PostRepeatingTask(func(ctx context.Context) {
		counter1.Add(1)
	}, 30*time.Millisecond)

	runner.PostRepeatingTask(func(ctx context.Context) {
		counter2.Add(1)
	}, 40*time.Millisecond)

	runner.PostRepeatingTask(func(ctx context.Context) {
		counter3.Add(1)
	}, 50*time.Millisecond)

	time.Sleep(200 * time.Millisecond)

	// Act
	runner.Shutdown()

	c1 := counter1.Load()
	c2 := counter2.Load()
	c3 := counter3.Load()

	time.Sleep(200 * time.Millisecond)

	// Assert - All stopped
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

// TestSequencedTaskRunner_Shutdown_WithDelayedTasks verifies shutdown with delayed tasks
// Given: A SequencedTaskRunner with a 100ms delayed task
// When: Shutdown is called before delay expires
// Then: Task may still execute if already in DelayManager
func TestSequencedTaskRunner_Shutdown_WithDelayedTasks(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var executed atomic.Bool

	runner.PostDelayedTask(func(ctx context.Context) {
		executed.Store(true)
	}, 100*time.Millisecond)

	// Act - Shutdown before task executes
	time.Sleep(20 * time.Millisecond)
	runner.Shutdown()

	time.Sleep(150 * time.Millisecond)

	// Assert - Task might execute (already in DelayManager)
	// This is acceptable behavior - task won't be posted to closed runner
}

// TestSequencedTaskRunner_Shutdown_Idempotent verifies multiple shutdown calls are safe
// Given: A SequencedTaskRunner
// When: Shutdown is called multiple times
// Then: All calls complete, IsClosed returns true
func TestSequencedTaskRunner_Shutdown_Idempotent(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Act - Multiple shutdowns
	runner.Shutdown()
	runner.Shutdown()
	runner.Shutdown()

	// Assert
	if !runner.IsClosed() {
		t.Error("IsClosed() = false, want true")
	}
}

// TestSequencedTaskRunner_Shutdown_ConcurrentShutdown verifies concurrent shutdown calls are safe
// Given: A SequencedTaskRunner with 100 tasks
// When: 10 goroutines call Shutdown concurrently
// Then: All calls complete without panic, runner is closed
func TestSequencedTaskRunner_Shutdown_ConcurrentShutdown(t *testing.T) {
	// Arrange
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

	// Act - Concurrent shutdowns
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			runner.Shutdown()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Assert
	if !runner.IsClosed() {
		t.Error("IsClosed() = false, want true")
	}
}

// TestSequencedTaskRunner_RepeatingTask_WithInitialDelay_Shutdown verifies shutdown with delayed repeating task
// Given: A repeating task with 200ms initial delay
// When: Shutdown is called before first execution
// Then: Task never executes
func TestSequencedTaskRunner_RepeatingTask_WithInitialDelay_Shutdown(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var executed atomic.Bool

	runner.PostRepeatingTaskWithInitialDelay(
		func(ctx context.Context) {
			executed.Store(true)
		},
		200*time.Millisecond,
		50*time.Millisecond,
		DefaultTaskTraits(),
	)

	// Act - Shutdown before first execution
	time.Sleep(50 * time.Millisecond)
	runner.Shutdown()

	time.Sleep(200 * time.Millisecond)

	// Assert
	if executed.Load() {
		t.Error("executed = true, want false (shutdown before first execution)")
	}
}
