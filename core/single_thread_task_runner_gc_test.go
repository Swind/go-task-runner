package core_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Swind/go-task-runner/core"
)

// TestSingleThreadTaskRunner_GC_TaskWithStructMethod verifies that structs
// whose methods are posted as tasks can be garbage collected after execution.
func TestSingleThreadTaskRunner_GC_TaskWithStructMethod(t *testing.T) {
	runner := core.NewSingleThreadTaskRunner()
	defer runner.Stop()

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Create a scope to ensure the struct can be collected
	func() {
		// Create a struct that we want to GC
		obj := &TestObject{
			ID:   "single-thread-obj-1",
			Data: make([]byte, 1024*1024), // 1MB
		}

		// Set finalizer to detect when object is GC'd
		runtime.SetFinalizer(obj, func(o *TestObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		// Post the struct's method as a task
		runner.PostTask(obj.Process)

		// Wait for task to complete
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runner.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle failed: %v", err)
		}

		// obj goes out of scope here
	}()

	// Force GC multiple times
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for finalizer with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !finalizerCalled.Load() {
			t.Error("Finalizer was not called, object may not have been GC'd")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for object to be GC'd")
	}
}

// TestSingleThreadTaskRunner_GC_ClosureCapturedObjects verifies that objects
// captured by closures can be GC'd after task execution.
func TestSingleThreadTaskRunner_GC_ClosureCapturedObjects(t *testing.T) {
	runner := core.NewSingleThreadTaskRunner()
	defer runner.Stop()

	const numObjects = 100
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numObjects)

	// Create a scope for the objects
	func() {
		for i := 0; i < numObjects; i++ {
			obj := &TestObject{
				ID:   "single-thread-closure-obj",
				Data: make([]byte, 10*1024), // 10KB each
			}

			runtime.SetFinalizer(obj, func(o *TestObject) {
				finalizerCount.Add(1)
				wg.Done()
			})

			// Post a closure that captures the object
			runner.PostTask(func(ctx context.Context) {
				_ = obj.ID // Use the object
			})
		}

		// Wait for all tasks to complete
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runner.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle failed: %v", err)
		}

		// All objects go out of scope here
	}()

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for finalizers
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		collected := finalizerCount.Load()
		if collected != numObjects {
			t.Errorf("Expected %d objects to be GC'd, got %d", numObjects, collected)
		}
	case <-time.After(3 * time.Second):
		collected := finalizerCount.Load()
		t.Errorf("Timeout: only %d/%d objects were GC'd", collected, numObjects)
	}
}

// TestSingleThreadTaskRunner_GC_RepeatingTaskStopped verifies that objects
// referenced by repeating tasks can be GC'd after the task is stopped.
func TestSingleThreadTaskRunner_GC_RepeatingTaskStopped(t *testing.T) {
	runner := core.NewSingleThreadTaskRunner()
	defer runner.Stop()

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	var handle core.RepeatingTaskHandle

	// Create scope for the object
	func() {
		obj := &TestObject{
			ID:   "single-thread-repeating-obj",
			Data: make([]byte, 100*1024), // 100KB
		}

		runtime.SetFinalizer(obj, func(o *TestObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		// Start a repeating task that captures the object
		handle = runner.PostRepeatingTask(func(ctx context.Context) {
			_ = obj.ID
		}, 10*time.Millisecond)

		// Let it run a few times
		time.Sleep(50 * time.Millisecond)

		// Stop the repeating task
		handle.Stop()

		// Wait for any pending execution
		time.Sleep(50 * time.Millisecond)

		// obj goes out of scope here
	}()

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for finalizer
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !finalizerCalled.Load() {
			t.Error("Object captured by stopped repeating task was not GC'd")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout: object captured by stopped repeating task was not GC'd")
	}
}

// TestSingleThreadTaskRunner_GC_ShutdownClearsQueue verifies that pending
// tasks in the channel are released when runner is shutdown and stopped.
func TestSingleThreadTaskRunner_GC_ShutdownClearsQueue(t *testing.T) {
	runner := core.NewSingleThreadTaskRunner()

	const numPendingTasks = 50 // Smaller than channel buffer to avoid blocking
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numPendingTasks)

	// Create scope for objects
	func() {
		// Block the runner with a task that checks context
		runner.PostTask(func(ctx context.Context) {
			// Wait for context cancellation (which happens on Shutdown)
			<-ctx.Done()
		})

		// Give the blocking task time to start
		time.Sleep(10 * time.Millisecond)

		// Post many tasks that will be pending in the channel
		for i := 0; i < numPendingTasks; i++ {
			obj := &TestObject{
				ID:   "single-thread-pending-obj",
				Data: make([]byte, 1024),
			}

			runtime.SetFinalizer(obj, func(o *TestObject) {
				finalizerCount.Add(1)
				wg.Done()
			})

			// Capture object in closure
			runner.PostTask(func(ctx context.Context) {
				_ = obj.ID
			})
		}

		// Give tasks time to queue up in channel
		time.Sleep(10 * time.Millisecond)

		// Shutdown runner (cancels context and marks as closed)
		runner.Shutdown()

		// Wait a bit for the blocker to exit
		time.Sleep(50 * time.Millisecond)

		// Stop runner (terminates runLoop)
		// This should cause pending tasks in channel to be abandoned
		runner.Stop()

		// All objects go out of scope here
	}()

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for finalizers
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		collected := finalizerCount.Load()
		if collected != numPendingTasks {
			t.Errorf("Expected %d pending tasks to be GC'd after shutdown, got %d", numPendingTasks, collected)
		}
	case <-time.After(3 * time.Second):
		collected := finalizerCount.Load()
		t.Errorf("Timeout: only %d/%d pending tasks were GC'd after shutdown", collected, numPendingTasks)
	}
}

// TestSingleThreadTaskRunner_GC_TaskAndReplyPattern verifies that objects
// captured in PostTaskAndReply can be GC'd after completion.
func TestSingleThreadTaskRunner_GC_TaskAndReplyPattern(t *testing.T) {
	bgRunner := core.NewSingleThreadTaskRunner()
	defer bgRunner.Stop()

	uiRunner := core.NewSingleThreadTaskRunner()
	defer uiRunner.Stop()

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Create scope for object
	func() {
		obj := &TestObject{
			ID:   "single-thread-task-reply-obj",
			Data: make([]byte, 50*1024), // 50KB
		}

		runtime.SetFinalizer(obj, func(o *TestObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		// Use object in task and reply
		done := make(chan struct{})
		uiRunner.PostTask(func(ctx context.Context) {
			bgRunner.PostTaskAndReply(
				func(ctx context.Context) {
					_ = obj.ID // Use in background task
				},
				func(ctx context.Context) {
					_ = obj.ID // Use in reply
					close(done)
				},
				uiRunner,
			)
		})

		// Wait for task and reply to complete
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for task and reply")
		}

		// obj goes out of scope here
	}()

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for finalizer
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !finalizerCalled.Load() {
			t.Error("Object in PostTaskAndReply was not GC'd")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout: object in PostTaskAndReply was not GC'd")
	}
}

// TestSingleThreadTaskRunner_GC_DelayedTask verifies that objects captured
// in delayed tasks can be GC'd after the task executes.
func TestSingleThreadTaskRunner_GC_DelayedTask(t *testing.T) {
	runner := core.NewSingleThreadTaskRunner()
	defer runner.Stop()

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Create scope for object
	func() {
		obj := &TestObject{
			ID:   "single-thread-delayed-obj",
			Data: make([]byte, 20*1024), // 20KB
		}

		runtime.SetFinalizer(obj, func(o *TestObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		// Post delayed task
		done := make(chan struct{})
		runner.PostDelayedTask(func(ctx context.Context) {
			_ = obj.ID
			close(done)
		}, 50*time.Millisecond)

		// Wait for task to execute
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for delayed task")
		}

		// obj goes out of scope here
	}()

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for finalizer
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !finalizerCalled.Load() {
			t.Error("Object in delayed task was not GC'd")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout: object in delayed task was not GC'd")
	}
}

// TestSingleThreadTaskRunner_GC_MemoryGrowth performs a stress test to verify
// that memory doesn't grow unbounded when posting many tasks.
func TestSingleThreadTaskRunner_GC_MemoryGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory growth test in short mode")
	}

	runner := core.NewSingleThreadTaskRunner()
	defer runner.Stop()

	// Get baseline memory
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Post many tasks that allocate memory
	const iterations = 1000
	for i := 0; i < iterations; i++ {
		// Create a large object in each task
		runner.PostTask(func(ctx context.Context) {
			data := make([]byte, 100*1024) // 100KB
			_ = data[0]                    // Use it
		})

		// Periodically wait for tasks to complete
		if i%100 == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			runner.WaitIdle(ctx)
			cancel()
		}
	}

	// Wait for all tasks to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runner.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	// Force GC
	runtime.GC()
	runtime.GC()

	// Check final memory
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Calculate memory growth (handle case where GC reduced memory)
	var allocated int64
	if m2.Alloc > m1.Alloc {
		allocated = int64(m2.Alloc - m1.Alloc)
	} else {
		allocated = -int64(m1.Alloc - m2.Alloc)
	}
	totalAllocated := m2.TotalAlloc - m1.TotalAlloc

	t.Logf("Memory stats:")
	t.Logf("  Initial Alloc: %d MB", m1.Alloc/1024/1024)
	t.Logf("  Final Alloc: %d MB", m2.Alloc/1024/1024)
	t.Logf("  Growth: %d MB", allocated/1024/1024)
	t.Logf("  Total allocated during test: %d MB", totalAllocated/1024/1024)
	t.Logf("  GC runs: %d", m2.NumGC-m1.NumGC)

	// We allocated 100MB total (1000 * 100KB), but after GC,
	// the retained memory should be much smaller
	maxAcceptableGrowth := int64(10 * 1024 * 1024) // 10MB
	if allocated > maxAcceptableGrowth {
		t.Errorf("Memory growth too large: %d MB (max acceptable: %d MB)",
			allocated/1024/1024, maxAcceptableGrowth/1024/1024)
		t.Error("Possible memory leak detected")
	}
}

// TestSingleThreadTaskRunner_GC_RunnerItself verifies that a SingleThreadTaskRunner
// can be GC'd after it's stopped and no longer referenced.
func TestSingleThreadTaskRunner_GC_RunnerItself(t *testing.T) {
	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Create scope for runner
	func() {
		runner := core.NewSingleThreadTaskRunner()

		runtime.SetFinalizer(runner, func(r *core.SingleThreadTaskRunner) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		// Post and execute a task
		done := make(chan struct{})
		runner.PostTask(func(ctx context.Context) {
			close(done)
		})

		<-done

		// Stop runner
		runner.Stop()

		// runner goes out of scope here
	}()

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for finalizer
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !finalizerCalled.Load() {
			t.Error("SingleThreadTaskRunner itself was not GC'd")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout: SingleThreadTaskRunner itself was not GC'd")
	}
}

// TestSingleThreadTaskRunner_GC_DedicatedGoroutineCleanup verifies that
// the dedicated goroutine properly terminates and doesn't prevent GC.
func TestSingleThreadTaskRunner_GC_DedicatedGoroutineCleanup(t *testing.T) {
	// Track goroutine count
	initialGoroutines := runtime.NumGoroutine()

	const numRunners = 10
	runners := make([]*core.SingleThreadTaskRunner, numRunners)

	// Create multiple runners
	for i := 0; i < numRunners; i++ {
		runners[i] = core.NewSingleThreadTaskRunner()

		// Post a task to ensure runner is active
		done := make(chan struct{})
		runners[i].PostTask(func(ctx context.Context) {
			close(done)
		})
		<-done
	}

	// Should have created new goroutines
	afterCreateGoroutines := runtime.NumGoroutine()
	createdGoroutines := afterCreateGoroutines - initialGoroutines
	if createdGoroutines < numRunners {
		t.Logf("Warning: Expected at least %d new goroutines, got %d", numRunners, createdGoroutines)
	}

	// Stop all runners
	for _, runner := range runners {
		runner.Stop()
	}

	// Clear references
	runners = nil

	// Wait a bit and force GC
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Check that goroutines have been cleaned up
	finalGoroutines := runtime.NumGoroutine()

	t.Logf("Goroutine count:")
	t.Logf("  Initial: %d", initialGoroutines)
	t.Logf("  After creating %d runners: %d (+%d)", numRunners, afterCreateGoroutines, createdGoroutines)
	t.Logf("  After stopping and GC: %d", finalGoroutines)

	// Allow some tolerance for background goroutines
	tolerance := 5
	if finalGoroutines > initialGoroutines+tolerance {
		t.Errorf("Goroutines not properly cleaned up: started with %d, now have %d (expected ≤ %d)",
			initialGoroutines, finalGoroutines, initialGoroutines+tolerance)
		t.Error("Possible goroutine leak in SingleThreadTaskRunner")
	}
}
