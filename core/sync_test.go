package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// WaitIdle Tests
// =============================================================================

// TestSequencedTaskRunner_WaitIdle tests WaitIdle for SequencedTaskRunner
// Main test items:
// 1. WaitIdle blocks until all tasks complete
// 2. Returns nil when all tasks are done
// 3. All tasks execute successfully
func TestSequencedTaskRunner_WaitIdle(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter atomic.Int32

	// Post some tasks
	for i := 0; i < 5; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	// Wait for all tasks to complete
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitIdle(ctx)
	if err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	if counter.Load() != 5 {
		t.Errorf("Expected 5 tasks to complete, got %d", counter.Load())
	}
}

// TestSequencedTaskRunner_WaitIdle_Timeout tests WaitIdle timeout behavior
// Main test items:
// 1. WaitIdle returns context.DeadlineExceeded on timeout
// 2. Long-running task causes timeout
func TestSequencedTaskRunner_WaitIdle_Timeout(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Post a long-running task
	runner.PostTask(func(ctx context.Context) {
		time.Sleep(5 * time.Second)
	})

	// WaitIdle with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runner.WaitIdle(ctx)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
}

// TestSequencedTaskRunner_WaitIdle_AfterShutdown tests WaitIdle after shutdown
// Main test items:
// 1. WaitIdle returns error when runner is closed
// 2. Shutdown prevents waiting
func TestSequencedTaskRunner_WaitIdle_AfterShutdown(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)
	runner.Shutdown()

	err := runner.WaitIdle(context.Background())
	if err == nil {
		t.Error("Expected error for closed runner, got nil")
	}
}

// TestSingleThreadTaskRunner_WaitIdle tests WaitIdle for SingleThreadTaskRunner
// Main test items:
// 1. WaitIdle blocks until all tasks complete
// 2. Returns nil when all tasks are done
// 3. All tasks execute successfully on single thread
func TestSingleThreadTaskRunner_WaitIdle(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32

	// Post some tasks
	for i := 0; i < 5; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	// Wait for all tasks to complete
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitIdle(ctx)
	if err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	if counter.Load() != 5 {
		t.Errorf("Expected 5 tasks to complete, got %d", counter.Load())
	}
}

// =============================================================================
// FlushAsync Tests
// =============================================================================

// TestSequencedTaskRunner_FlushAsync tests FlushAsync for SequencedTaskRunner
// Main test items:
// 1. FlushAsync callback is called after all tasks complete
// 2. Callback executes on the runner's sequence
// 3. All tasks complete before callback
func TestSequencedTaskRunner_FlushAsync(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter atomic.Int32
	var flushCalled atomic.Bool

	// Post some tasks
	for i := 0; i < 5; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	// Flush callback
	runner.FlushAsync(func() {
		flushCalled.Store(true)
		if counter.Load() != 5 {
			t.Errorf("Flush called but not all tasks completed: %d/5", counter.Load())
		}
	})

	// Wait for flush
	time.Sleep(200 * time.Millisecond)

	if !flushCalled.Load() {
		t.Error("Flush callback was not called")
	}
}

// TestSingleThreadTaskRunner_FlushAsync tests FlushAsync for SingleThreadTaskRunner
// Main test items:
// 1. FlushAsync callback is called after all tasks complete
// 2. Callback executes on the dedicated thread
// 3. All tasks complete before callback
func TestSingleThreadTaskRunner_FlushAsync(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32
	var flushCalled atomic.Bool

	// Post some tasks
	for i := 0; i < 5; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	// Flush callback
	runner.FlushAsync(func() {
		flushCalled.Store(true)
		if counter.Load() != 5 {
			t.Errorf("Flush called but not all tasks completed: %d/5", counter.Load())
		}
	})

	// Wait for flush
	time.Sleep(200 * time.Millisecond)

	if !flushCalled.Load() {
		t.Error("Flush callback was not called")
	}
}

// =============================================================================
// WaitShutdown Tests
// =============================================================================

// TestSequencedTaskRunner_WaitShutdown_External tests external shutdown notification
// Main test items:
// 1. WaitShutdown unblocks when Shutdown is called
// 2. Returns nil when shutdown is triggered
// 3. External Shutdown() call wakes up waiters
func TestSequencedTaskRunner_WaitShutdown_External(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var shutdownReceived atomic.Bool

	// Goroutine waiting for shutdown
	go func() {
		err := runner.WaitShutdown(context.Background())
		if err != nil {
			t.Errorf("WaitShutdown failed: %v", err)
		}
		shutdownReceived.Store(true)
	}()

	// Shutdown after delay
	time.Sleep(100 * time.Millisecond)
	runner.Shutdown()

	// Wait for shutdown to be received
	time.Sleep(100 * time.Millisecond)

	if !shutdownReceived.Load() {
		t.Error("WaitShutdown did not receive shutdown signal")
	}
}

// TestSequencedTaskRunner_WaitShutdown_Internal tests internal shutdown notification
// Main test items:
// 1. WaitShutdown unblocks when task calls Shutdown
// 2. Shutdown can be triggered from within a task
// 3. Task can self-initiate shutdown
func TestSequencedTaskRunner_WaitShutdown_Internal(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var taskExecuted atomic.Bool
	var heartbeatCount atomic.Int32

	// Post multiple heartbeat tasks
	for i := 0; i < 15; i++ {
		runner.PostTask(func(ctx context.Context) {
			count := heartbeatCount.Add(1)
			taskExecuted.Store(true)

			// Shutdown at 10th heartbeat
			if count >= 10 {
				me := GetCurrentTaskRunner(ctx)
				me.Shutdown()
			}
		})
	}

	// Wait for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitShutdown(ctx)
	if err != nil {
		t.Fatalf("WaitShutdown failed: %v", err)
	}

	// Verify shutdown happened
	if !runner.IsClosed() {
		t.Error("Runner should be closed")
	}

	// Wait a bit to see final count
	time.Sleep(100 * time.Millisecond)

	// Should have executed 10 tasks, 11th task triggered shutdown
	count := heartbeatCount.Load()
	if count < 10 {
		t.Errorf("Expected at least 10 tasks executed, got %d", count)
	}
	if count > 11 {
		t.Errorf("Expected at most 11 tasks executed (including shutdown task), got %d", count)
	}
}

// TestSingleThreadTaskRunner_WaitShutdown_Internal tests internal shutdown for SingleThreadTaskRunner
// Main test items:
// 1. WaitShutdown unblocks when task calls Shutdown
// 2. Shutdown can be triggered from within a task
// 3. Task can self-initiate shutdown on dedicated thread
func TestSingleThreadTaskRunner_WaitShutdown_Internal(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var heartbeatCount atomic.Int32

	// Post multiple heartbeat tasks
	for i := 0; i < 15; i++ {
		runner.PostTask(func(ctx context.Context) {
			count := heartbeatCount.Add(1)

			// Shutdown at 10th heartbeat
			if count >= 10 {
				me := GetCurrentTaskRunner(ctx)
				me.Shutdown()
			}
		})
	}

	// Wait for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitShutdown(ctx)
	if err != nil {
		t.Fatalf("WaitShutdown failed: %v", err)
	}

	// Verify shutdown happened
	if !runner.IsClosed() {
		t.Error("Runner should be closed")
	}

	// Stop the runner (Shutdown doesn't stop it immediately)
	runner.Stop()
}

// TestSequencedTaskRunner_WaitShutdown_Timeout tests WaitShutdown timeout behavior
// Main test items:
// 1. WaitShutdown returns context.DeadlineExceeded on timeout
// 2. Timeout occurs when no shutdown is triggered
func TestSequencedTaskRunner_WaitShutdown_Timeout(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Wait with timeout (no shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runner.WaitShutdown(ctx)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}

	runner.Shutdown() // Cleanup
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestSequencedTaskRunner_WaitIdle_ThenShutdown tests WaitIdle followed by Shutdown
// Main test items:
// 1. WaitIdle completes when all tasks are done
// 2. Shutdown can be called after WaitIdle
// 3. WaitShutdown completes after Shutdown
func TestSequencedTaskRunner_WaitIdle_ThenShutdown(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter atomic.Int32

	// Post some tasks
	for i := 0; i < 10; i++ {
		runner.PostTask(func(ctx context.Context) {
			counter.Add(1)
		})
	}

	// Wait for all tasks to complete
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitIdle(ctx)
	if err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	if counter.Load() != 10 {
		t.Errorf("Expected 10 tasks, got %d", counter.Load())
	}

	// Now shutdown
	runner.Shutdown()

	// Verify shutdown
	err = runner.WaitShutdown(context.Background())
	if err != nil {
		t.Errorf("WaitShutdown failed: %v", err)
	}
}

// TestSequencedTaskRunner_MultipleWaitShutdown tests multiple WaitShutdown calls
// Main test items:
// 1. Multiple goroutines can wait for shutdown
// 2. All waiters are unblocked on Shutdown
// 3. All WaitShutdown calls return nil
func TestSequencedTaskRunner_MultipleWaitShutdown(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var waiter1Done, waiter2Done atomic.Bool

	// Multiple goroutines waiting
	go func() {
		runner.WaitShutdown(context.Background())
		waiter1Done.Store(true)
	}()

	go func() {
		runner.WaitShutdown(context.Background())
		waiter2Done.Store(true)
	}()

	time.Sleep(50 * time.Millisecond)

	// Shutdown
	runner.Shutdown()

	// Wait for both to receive
	time.Sleep(100 * time.Millisecond)

	if !waiter1Done.Load() {
		t.Error("Waiter 1 did not receive shutdown")
	}
	if !waiter2Done.Load() {
		t.Error("Waiter 2 did not receive shutdown")
	}
}

// TestSequencedTaskRunner_MultipleShutdownCalls tests multiple Shutdown calls
// Main test items:
// 1. Multiple Shutdown() calls are safe (idempotent)
// 2. IsClosed() returns true after first call
func TestSequencedTaskRunner_MultipleShutdownCalls(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Multiple shutdowns should be safe (idempotent)
	runner.Shutdown()
	runner.Shutdown()
	runner.Shutdown()

	if !runner.IsClosed() {
		t.Error("Runner should be closed")
	}
}

// TestSingleThreadTaskRunner_MultipleShutdownCalls tests multiple Shutdown calls for SingleThreadTaskRunner
// Main test items:
// 1. Multiple Shutdown() calls are safe (idempotent)
// 2. IsClosed() returns true after first call
func TestSingleThreadTaskRunner_MultipleShutdownCalls(t *testing.T) {
	runner := NewSingleThreadTaskRunner()

	// Multiple shutdowns should be safe (idempotent)
	runner.Shutdown()
	runner.Shutdown()
	runner.Shutdown()

	if !runner.IsClosed() {
		t.Error("Runner should be closed")
	}
}
