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
// Given: a SequencedTaskRunner with 5 posted tasks
// When: WaitIdle is called with a timeout context
// Then: all tasks complete and WaitIdle returns nil with counter = 5
func TestSequencedTaskRunner_WaitIdle(t *testing.T) {
	// Arrange - Setup thread pool, runner, and counter
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)
	var counter atomic.Int32

	// Act - Post 5 tasks and wait for them to complete
	for i := 0; i < 5; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitIdle(ctx)

	// Assert - Verify all tasks completed and no error occurred
	if err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	got := counter.Load()
	want := int32(5)
	if got != want {
		t.Errorf("task count: got = %d, want %d", got, want)
	}
}

// TestSequencedTaskRunner_WaitIdle_Timeout tests WaitIdle timeout behavior
// Given: a SequencedTaskRunner with a long-running task (5 seconds)
// When: WaitIdle is called with a short timeout (100ms)
// Then: WaitIdle returns context.DeadlineExceeded error
func TestSequencedTaskRunner_WaitIdle_Timeout(t *testing.T) {
	// Arrange - Setup thread pool, runner, and post long-running task
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Act - Post long task and wait with short timeout
	runner.PostTask(func(ctx context.Context) {
		time.Sleep(5 * time.Second)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runner.WaitIdle(ctx)

	// Assert - Verify timeout error occurred
	if err == nil {
		t.Error("timeout error: got = nil, want = context.DeadlineExceeded")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("error type: got = %v, want = context.DeadlineExceeded", err)
	}
}

// TestSequencedTaskRunner_WaitIdle_AfterShutdown tests WaitIdle after shutdown
// Given: a SequencedTaskRunner that has been shutdown
// When: WaitIdle is called
// Then: WaitIdle returns an error indicating the runner is closed
func TestSequencedTaskRunner_WaitIdle_AfterShutdown(t *testing.T) {
	// Arrange - Setup thread pool, runner, and shutdown
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)
	runner.Shutdown()

	// Act - Call WaitIdle on shutdown runner
	err := runner.WaitIdle(context.Background())

	// Assert - Verify error is returned for closed runner
	if err == nil {
		t.Error("error for closed runner: got = nil, want = non-nil error")
	}
}

// TestSingleThreadTaskRunner_WaitIdle tests WaitIdle for SingleThreadTaskRunner
// Given: a SingleThreadTaskRunner with 5 posted tasks
// When: WaitIdle is called with a timeout context
// Then: all tasks complete and WaitIdle returns nil with counter = 5
func TestSingleThreadTaskRunner_WaitIdle(t *testing.T) {
	// Arrange - Setup runner and counter
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32

	// Act - Post 5 tasks and wait for completion
	for i := 0; i < 5; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitIdle(ctx)

	// Assert - Verify all tasks completed and no error occurred
	if err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	got := counter.Load()
	want := int32(5)
	if got != want {
		t.Errorf("task count: got = %d, want %d", got, want)
	}
}

// =============================================================================
// FlushAsync Tests
// =============================================================================

// TestSequencedTaskRunner_FlushAsync tests FlushAsync for SequencedTaskRunner
// Given: a SequencedTaskRunner with 5 posted tasks and a flush callback
// When: FlushAsync is called to register the callback
// Then: the callback is invoked after all tasks complete with counter = 5
func TestSequencedTaskRunner_FlushAsync(t *testing.T) {
	// Arrange - Setup thread pool, runner, counter, and callback flag
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)
	var counter atomic.Int32
	var flushCalled atomic.Bool

	// Act - Post 5 tasks and register flush callback
	for i := 0; i < 5; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	runner.FlushAsync(func() {
		flushCalled.Store(true)
		if counter.Load() != 5 {
			t.Errorf("Flush called but not all tasks completed: %d/5", counter.Load())
		}
	})

	// Wait for flush to complete
	time.Sleep(200 * time.Millisecond)

	// Assert - Verify flush callback was called
	got := flushCalled.Load()
	want := true
	if got != want {
		t.Errorf("flush callback called: got = %v, want %v", got, want)
	}
}

// TestSingleThreadTaskRunner_FlushAsync tests FlushAsync for SingleThreadTaskRunner
// Given: a SingleThreadTaskRunner with 5 posted tasks and a flush callback
// When: FlushAsync is called to register the callback
// Then: the callback is invoked on the dedicated thread after all tasks complete
func TestSingleThreadTaskRunner_FlushAsync(t *testing.T) {
	// Arrange - Setup runner, counter, and callback flag
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32
	var flushCalled atomic.Bool

	// Act - Post 5 tasks and register flush callback
	for i := 0; i < 5; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	runner.FlushAsync(func() {
		flushCalled.Store(true)
		if counter.Load() != 5 {
			t.Errorf("Flush called but not all tasks completed: %d/5", counter.Load())
		}
	})

	// Wait for flush to complete
	time.Sleep(200 * time.Millisecond)

	// Assert - Verify flush callback was called
	got := flushCalled.Load()
	want := true
	if got != want {
		t.Errorf("flush callback called: got = %v, want %v", got, want)
	}
}

// =============================================================================
// WaitShutdown Tests
// =============================================================================

// TestSequencedTaskRunner_WaitShutdown_External tests external shutdown notification
// Given: a goroutine waiting on WaitShutdown and a SequencedTaskRunner
// When: Shutdown is called externally
// Then: WaitShutdown unblocks and returns nil
func TestSequencedTaskRunner_WaitShutdown_External(t *testing.T) {
	// Arrange - Setup thread pool, runner, and start waiting goroutine
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)
	var shutdownReceived atomic.Bool

	// Act - Start goroutine waiting for shutdown, then trigger shutdown
	go func() {
		err := runner.WaitShutdown(context.Background())
		if err != nil {
			t.Errorf("WaitShutdown failed: %v", err)
		}
		shutdownReceived.Store(true)
	}()

	time.Sleep(100 * time.Millisecond)
	runner.Shutdown()

	// Wait for shutdown to be received
	time.Sleep(100 * time.Millisecond)

	// Assert - Verify shutdown was received
	got := shutdownReceived.Load()
	want := true
	if got != want {
		t.Errorf("shutdown signal received: got = %v, want %v", got, want)
	}
}

// TestSequencedTaskRunner_WaitShutdown_Internal tests internal shutdown notification
// Given: a SequencedTaskRunner with multiple heartbeat tasks
// When: a task calls Shutdown internally when heartbeat count reaches 10
// Then: WaitShutdown unblocks and runner is closed
func TestSequencedTaskRunner_WaitShutdown_Internal(t *testing.T) {
	// Arrange - Setup thread pool, runner, and heartbeat counter
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)
	var taskExecuted atomic.Bool
	var heartbeatCount atomic.Int32

	// Act - Post tasks that trigger shutdown at 10th heartbeat
	for i := 0; i < 15; i++ {
		runner.PostTask(func(ctx context.Context) {
			count := heartbeatCount.Add(1)
			taskExecuted.Store(true)

			if count >= 10 {
				me := GetCurrentTaskRunner(ctx)
				me.Shutdown()
			}
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitShutdown(ctx)

	// Assert - Verify shutdown completed and runner is closed
	if err != nil {
		t.Fatalf("WaitShutdown failed: %v", err)
	}

	if !runner.IsClosed() {
		t.Error("runner closed: got = false, want = true")
	}

	// Verify task execution count
	time.Sleep(100 * time.Millisecond)
	count := heartbeatCount.Load()
	if count < 10 {
		t.Errorf("heartbeat count: got = %d (want >= 10)", count)
	}
	if count > 11 {
		t.Errorf("heartbeat count: got = %d (want <= 11)", count)
	}
}

// TestSingleThreadTaskRunner_WaitShutdown_Internal tests internal shutdown for SingleThreadTaskRunner
// Given: a SingleThreadTaskRunner with multiple heartbeat tasks
// When: a task calls Shutdown internally when heartbeat count reaches 10
// Then: WaitShutdown unblocks and runner is closed
func TestSingleThreadTaskRunner_WaitShutdown_Internal(t *testing.T) {
	// Arrange - Setup runner and heartbeat counter
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var heartbeatCount atomic.Int32

	// Act - Post tasks that trigger shutdown at 10th heartbeat
	for i := 0; i < 15; i++ {
		runner.PostTask(func(ctx context.Context) {
			count := heartbeatCount.Add(1)

			if count >= 10 {
				me := GetCurrentTaskRunner(ctx)
				me.Shutdown()
			}
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitShutdown(ctx)

	// Assert - Verify shutdown completed and runner is closed
	if err != nil {
		t.Fatalf("WaitShutdown failed: %v", err)
	}

	if !runner.IsClosed() {
		t.Error("runner closed: got = false, want = true")
	}

	runner.Stop()
}

// TestSequencedTaskRunner_WaitShutdown_Timeout tests WaitShutdown timeout behavior
// Given: a SequencedTaskRunner with no shutdown triggered
// When: WaitShutdown is called with a timeout
// Then: WaitShutdown returns context.DeadlineExceeded error
func TestSequencedTaskRunner_WaitShutdown_Timeout(t *testing.T) {
	// Arrange - Setup thread pool and runner
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Act - Call WaitShutdown with timeout (no shutdown triggered)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runner.WaitShutdown(ctx)

	// Assert - Verify timeout error occurred
	if err == nil {
		t.Error("timeout error: got = nil, want = context.DeadlineExceeded")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("error type: got = %v, want = context.DeadlineExceeded", err)
	}

	runner.Shutdown() // Cleanup
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestSequencedTaskRunner_WaitIdle_ThenShutdown tests WaitIdle followed by Shutdown
// Given: a SequencedTaskRunner with 10 posted tasks
// When: WaitIdle is called first, then Shutdown and WaitShutdown
// Then: both operations complete successfully with all tasks executed
func TestSequencedTaskRunner_WaitIdle_ThenShutdown(t *testing.T) {
	// Arrange - Setup thread pool, runner, and counter
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)
	var counter atomic.Int32

	// Act - Post 10 tasks, wait for idle, then shutdown
	for i := 0; i < 10; i++ {
		runner.PostTask(func(ctx context.Context) {
			counter.Add(1)
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := runner.WaitIdle(ctx)
	if err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	runner.Shutdown()

	err = runner.WaitShutdown(context.Background())

	// Assert - Verify all tasks completed and shutdown succeeded
	got := counter.Load()
	want := int32(10)
	if got != want {
		t.Errorf("task count: got = %d, want %d", got, want)
	}

	if err != nil {
		t.Errorf("WaitShutdown failed: %v", err)
	}
}

// TestSequencedTaskRunner_MultipleWaitShutdown tests multiple WaitShutdown calls
// Given: multiple goroutines waiting on WaitShutdown for the same runner
// When: Shutdown is called
// Then: all waiters are unblocked and all WaitShutdown calls return nil
func TestSequencedTaskRunner_MultipleWaitShutdown(t *testing.T) {
	// Arrange - Setup thread pool, runner, and two waiting goroutines
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)
	var waiter1Done, waiter2Done atomic.Bool

	// Act - Start two goroutines waiting for shutdown, then trigger shutdown
	go func() {
		runner.WaitShutdown(context.Background())
		waiter1Done.Store(true)
	}()

	go func() {
		runner.WaitShutdown(context.Background())
		waiter2Done.Store(true)
	}()

	time.Sleep(50 * time.Millisecond)
	runner.Shutdown()

	time.Sleep(100 * time.Millisecond)

	// Assert - Verify both waiters received shutdown signal
	if !waiter1Done.Load() {
		t.Error("waiter 1 done: got = false, want = true")
	}
	if !waiter2Done.Load() {
		t.Error("waiter 2 done: got = false, want = true")
	}
}

// TestSequencedTaskRunner_MultipleShutdownCalls tests multiple Shutdown calls
// Given: a SequencedTaskRunner
// When: Shutdown is called multiple times
// Then: all calls succeed (idempotent) and IsClosed returns true
func TestSequencedTaskRunner_MultipleShutdownCalls(t *testing.T) {
	// Arrange - Setup thread pool and runner
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Act - Call Shutdown multiple times
	runner.Shutdown()
	runner.Shutdown()
	runner.Shutdown()

	// Assert - Verify runner is closed
	got := runner.IsClosed()
	want := true
	if got != want {
		t.Errorf("runner closed: got = %v, want %v", got, want)
	}
}

// TestSingleThreadTaskRunner_MultipleShutdownCalls tests multiple Shutdown calls for SingleThreadTaskRunner
// Given: a SingleThreadTaskRunner
// When: Shutdown is called multiple times
// Then: all calls succeed (idempotent) and IsClosed returns true
func TestSingleThreadTaskRunner_MultipleShutdownCalls(t *testing.T) {
	// Arrange - Setup runner
	runner := NewSingleThreadTaskRunner()

	// Act - Call Shutdown multiple times
	runner.Shutdown()
	runner.Shutdown()
	runner.Shutdown()

	// Assert - Verify runner is closed
	got := runner.IsClosed()
	want := true
	if got != want {
		t.Errorf("runner closed: got = %v, want %v", got, want)
	}
}
