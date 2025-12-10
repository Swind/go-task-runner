package core

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestSingleThreadTaskRunner_BasicExecution(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var executed atomic.Bool

	runner.PostTask(func(ctx context.Context) {
		executed.Store(true)
	})

	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("Task was not executed")
	}
}

func TestSingleThreadTaskRunner_ExecutionOrder(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var order []int
	var mu atomic.Value
	mu.Store(&order)

	for i := 0; i < 10; i++ {
		id := i
		runner.PostTask(func(ctx context.Context) {
			ptr := mu.Load().(*[]int)
			*ptr = append(*ptr, id)
		})
	}

	time.Sleep(100 * time.Millisecond)

	result := *mu.Load().(*[]int)
	if len(result) != 10 {
		t.Fatalf("Expected 10 tasks executed, got %d", len(result))
	}

	for i := 0; i < 10; i++ {
		if result[i] != i {
			t.Errorf("Task order incorrect: expected %d at position %d, got %d", i, i, result[i])
		}
	}
}

func TestSingleThreadTaskRunner_ThreadAffinity(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	goroutineIDs := make(map[uint64]bool)
	var mu atomic.Value
	mu.Store(&goroutineIDs)

	// Get goroutine ID helper
	getGoroutineID := func() uint64 {
		b := make([]byte, 64)
		b = b[:runtime.Stack(b, false)]
		// Parse "goroutine 123 [running]:"
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

	// Post multiple tasks
	for i := 0; i < 20; i++ {
		runner.PostTask(func(ctx context.Context) {
			gid := getGoroutineID()
			ptr := mu.Load().(*map[uint64]bool)
			(*ptr)[gid] = true
		})
	}

	time.Sleep(100 * time.Millisecond)

	result := *mu.Load().(*map[uint64]bool)
	if len(result) != 1 {
		t.Errorf("Expected all tasks to run on same goroutine, but found %d different goroutines", len(result))
	}
}

func TestSingleThreadTaskRunner_DelayedTask(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var executed atomic.Bool
	start := time.Now()

	runner.PostDelayedTask(func(ctx context.Context) {
		executed.Store(true)
	}, 100*time.Millisecond)

	// Should not execute immediately
	time.Sleep(50 * time.Millisecond)
	if executed.Load() {
		t.Error("Delayed task executed too early")
	}

	// Wait for execution
	time.Sleep(100 * time.Millisecond)
	if !executed.Load() {
		t.Error("Delayed task was not executed")
	}

	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("Delayed task executed too early: %v", elapsed)
	}
}

func TestSingleThreadTaskRunner_RepeatingTask(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32

	handle := runner.PostRepeatingTask(func(ctx context.Context) {
		counter.Add(1)
	}, 50*time.Millisecond)

	// Let it run a few times
	time.Sleep(200 * time.Millisecond)

	// Stop the repeating task
	handle.Stop()
	countAtStop := counter.Load()

	// Wait and verify it stopped
	time.Sleep(150 * time.Millisecond)
	countAfterStop := counter.Load()

	if countAtStop < 2 {
		t.Errorf("Repeating task should have run at least 2 times, got %d", countAtStop)
	}

	if countAfterStop > countAtStop+1 {
		t.Errorf("Repeating task continued after stop: before=%d, after=%d", countAtStop, countAfterStop)
	}
}

func TestSingleThreadTaskRunner_RepeatingTaskWithInitialDelay(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32
	start := time.Now()

	handle := runner.PostRepeatingTaskWithInitialDelay(
		func(ctx context.Context) {
			counter.Add(1)
		},
		100*time.Millisecond, // Initial delay
		50*time.Millisecond,  // Interval
		DefaultTaskTraits(),
	)
	defer handle.Stop()

	// Should not execute immediately
	time.Sleep(50 * time.Millisecond)
	if counter.Load() > 0 {
		t.Error("Repeating task with initial delay executed too early")
	}

	// Wait for initial delay + some intervals
	time.Sleep(200 * time.Millisecond)

	elapsed := time.Since(start)
	count := counter.Load()

	if count < 1 {
		t.Error("Repeating task did not execute after initial delay")
	}

	if elapsed < 100*time.Millisecond {
		t.Errorf("First execution happened before initial delay: %v", elapsed)
	}
}

func TestSingleThreadTaskRunner_Shutdown(t *testing.T) {
	runner := NewSingleThreadTaskRunner()

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

func TestSingleThreadTaskRunner_Shutdown_StopsRepeatingTasks(t *testing.T) {
	runner := NewSingleThreadTaskRunner()

	var counter atomic.Int32

	runner.PostRepeatingTask(func(ctx context.Context) {
		counter.Add(1)
	}, 50*time.Millisecond)

	// Let it run a few times
	time.Sleep(150 * time.Millisecond)

	// Shutdown
	runner.Shutdown()
	countAtShutdown := counter.Load()

	// Wait and verify no more executions
	time.Sleep(150 * time.Millisecond)
	countAfterShutdown := counter.Load()

	if countAfterShutdown > countAtShutdown {
		t.Errorf("Repeating task continued after shutdown: before=%d, after=%d",
			countAtShutdown, countAfterShutdown)
	}
}

func TestSingleThreadTaskRunner_PostTaskAfterShutdown(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	runner.Shutdown()

	var executed atomic.Bool

	// Post task after shutdown
	runner.PostTask(func(ctx context.Context) {
		executed.Store(true)
	})

	time.Sleep(100 * time.Millisecond)

	// Task should not execute
	if executed.Load() {
		t.Error("Task should not execute after shutdown")
	}
}

func TestSingleThreadTaskRunner_Stop(t *testing.T) {
	runner := NewSingleThreadTaskRunner()

	// Add a task that executes immediately
	var executed atomic.Bool
	runner.PostTask(func(ctx context.Context) {
		executed.Store(true)
	})

	// Let it start executing
	time.Sleep(50 * time.Millisecond)

	// Stop the runner
	runner.Stop()

	if !runner.IsClosed() {
		t.Error("Runner should be closed after Stop()")
	}

	// The task that executed before stop should have completed
	if !executed.Load() {
		t.Error("Task should have completed before stop")
	}

	// New tasks posted after stop should not execute
	var executed2 atomic.Bool
	runner.PostTask(func(ctx context.Context) {
		executed2.Store(true)
	})

	time.Sleep(100 * time.Millisecond)
	if executed2.Load() {
		t.Error("Task posted after stop should not execute")
	}
}

func TestSingleThreadTaskRunner_MultipleRepeatingTasks(t *testing.T) {
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

	// Let them run
	time.Sleep(200 * time.Millisecond)

	// All should have executed multiple times
	if counter1.Load() < 3 {
		t.Errorf("Task 1 should have run at least 3 times, got %d", counter1.Load())
	}
	if counter2.Load() < 2 {
		t.Errorf("Task 2 should have run at least 2 times, got %d", counter2.Load())
	}
	if counter3.Load() < 2 {
		t.Errorf("Task 3 should have run at least 2 times, got %d", counter3.Load())
	}

	// Stop all
	handle1.Stop()
	handle2.Stop()
	handle3.Stop()

	c1 := counter1.Load()
	c2 := counter2.Load()
	c3 := counter3.Load()

	// Wait and verify all stopped
	time.Sleep(150 * time.Millisecond)

	if counter1.Load() > c1+1 {
		t.Error("Task 1 continued after stop")
	}
	if counter2.Load() > c2+1 {
		t.Error("Task 2 continued after stop")
	}
	if counter3.Load() > c3+1 {
		t.Error("Task 3 continued after stop")
	}
}

func TestSingleThreadTaskRunner_PanicRecovery(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var executed atomic.Bool

	// Post task that panics
	runner.PostTask(func(ctx context.Context) {
		panic("test panic")
	})

	// Post task after panic
	runner.PostTask(func(ctx context.Context) {
		executed.Store(true)
	})

	time.Sleep(100 * time.Millisecond)

	// Second task should still execute despite panic in first task
	if !executed.Load() {
		t.Error("Task after panic was not executed")
	}

	// Runner should still be operational
	if runner.IsClosed() {
		t.Error("Runner should not be closed after panic")
	}
}

func TestSingleThreadTaskRunner_IdempotentShutdown(t *testing.T) {
	runner := NewSingleThreadTaskRunner()

	// Multiple shutdowns should be safe
	runner.Shutdown()
	runner.Shutdown()
	runner.Stop()
	runner.Stop()

	if !runner.IsClosed() {
		t.Error("Runner should be closed")
	}
}

func TestSingleThreadTaskRunner_ConcurrentPostTask(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	defer runner.Stop()

	var counter atomic.Int32
	done := make(chan struct{})

	// Post tasks from multiple goroutines
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

	// Wait for all goroutines to finish posting
	for i := 0; i < 10; i++ {
		<-done
	}

	// Wait for all tasks to execute
	time.Sleep(200 * time.Millisecond)

	// All 100 tasks should have executed
	if counter.Load() != 100 {
		t.Errorf("Expected 100 tasks executed, got %d", counter.Load())
	}
}
