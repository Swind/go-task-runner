package core_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// TestSequencedTaskRunner_GC_TaskWithStructMethod verifies that structs
// whose methods are posted as tasks can be garbage collected after execution.
func TestSequencedTaskRunner_GC_TaskWithStructMethod(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Create a scope to ensure the struct can be collected
	func() {
		// Create a struct that we want to GC
		obj := &TestObject{
			ID:   "test-obj-1",
			Data: make([]byte, 1024*1024), // 1MB to make it noticeable
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

// TestSequencedTaskRunner_GC_ClosureCapturedObjects verifies that objects
// captured by closures can be GC'd after task execution.
func TestSequencedTaskRunner_GC_ClosureCapturedObjects(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const numObjects = 100
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numObjects)

	// Create a scope for the objects
	func() {
		for i := 0; i < numObjects; i++ {
			obj := &TestObject{
				ID:   "closure-obj",
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

// TestSequencedTaskRunner_GC_RepeatingTaskStopped verifies that objects
// referenced by repeating tasks can be GC'd after the task is stopped.
func TestSequencedTaskRunner_GC_RepeatingTaskStopped(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	var handle core.RepeatingTaskHandle

	// Create scope for the object
	func() {
		obj := &TestObject{
			ID:   "repeating-obj",
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

// TestSequencedTaskRunner_GC_ShutdownClearsQueue verifies that pending
// tasks in the queue are released when runner is shutdown.
func TestSequencedTaskRunner_GC_ShutdownClearsQueue(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 1)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const numPendingTasks = 100
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numPendingTasks)

	// Create scope for objects
	func() {
		// Block the runner with a long task
		blocker := make(chan struct{})
		runner.PostTask(func(ctx context.Context) {
			<-blocker
		})

		// Post many tasks that will be pending
		for i := 0; i < numPendingTasks; i++ {
			obj := &TestObject{
				ID:   "pending-obj",
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

		// Give tasks time to queue up
		time.Sleep(10 * time.Millisecond)

		// Shutdown runner (should clear the queue)
		runner.Shutdown()

		// Unblock the blocker task
		close(blocker)

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

// TestSequencedTaskRunner_GC_TaskAndReplyPattern verifies that objects
// captured in PostTaskAndReply can be GC'd after completion.
func TestSequencedTaskRunner_GC_TaskAndReplyPattern(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	bgRunner := core.NewSequencedTaskRunner(pool)
	uiRunner := core.NewSequencedTaskRunner(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Create scope for object
	func() {
		obj := &TestObject{
			ID:   "task-reply-obj",
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

// TestSequencedTaskRunner_GC_DelayedTask verifies that objects captured
// in delayed tasks can be GC'd after the task executes.
func TestSequencedTaskRunner_GC_DelayedTask(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Create scope for object
	func() {
		obj := &TestObject{
			ID:   "delayed-obj",
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

// TestSequencedTaskRunner_GC_MemoryGrowth performs a stress test to verify
// that memory doesn't grow unbounded when posting many tasks.
func TestSequencedTaskRunner_GC_MemoryGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory growth test in short mode")
	}

	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

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

// TestSequencedTaskRunner_GC_RunnerItself verifies that a SequencedTaskRunner
// can be GC'd after it's no longer referenced.
func TestSequencedTaskRunner_GC_RunnerItself(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Create scope for runner
	func() {
		runner := core.NewSequencedTaskRunner(pool)

		runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		// Post and execute a task
		done := make(chan struct{})
		runner.PostTask(func(ctx context.Context) {
			close(done)
		})

		<-done

		// Shutdown runner
		runner.Shutdown()

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
			t.Error("SequencedTaskRunner itself was not GC'd")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout: SequencedTaskRunner itself was not GC'd")
	}
}

// =============================================================================
// Test Helper Types
// =============================================================================

// TestObject is a struct used to test GC behavior
type TestObject struct {
	ID   string
	Data []byte
}

// Process is a method that can be posted as a task
func (o *TestObject) Process(ctx context.Context) {
	// Simulate some work
	_ = o.ID
	_ = len(o.Data)
}

// TestSequencedTaskRunner_GC_WithGlobalThreadPool verifies that a SequencedTaskRunner
// created from the global thread pool can be GC'd after it is shutdown, even if
// the global thread pool remains active.
func TestSequencedTaskRunner_GC_WithGlobalThreadPool(t *testing.T) {
	// Initialize global thread pool
	taskrunner.InitGlobalThreadPool(2)
	// Ensure we shut it down at the end, but the test focuses on the runner GC
	// while the pool is still alive (or at least before this defer runs).
	defer taskrunner.ShutdownGlobalThreadPool()

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Create scope for runner
	func() {
		// Create a runner using the global pool
		runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

		runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		// Post and execute a task to make sure it's active
		done := make(chan struct{})
		runner.PostTask(func(ctx context.Context) {
			close(done)
		})

		<-done

		// Shutdown the runner. This is crucial.
		// If we don't shutdown, the runner might still be referenced by the pool
		// or have pending operations (though here it's idle).
		// Explicit shutdown should detach it from the pool's scheduling if implemented correctly.
		runner.Shutdown()

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
			t.Error("SequencedTaskRunner from Global Pool was not GC'd")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout: SequencedTaskRunner from Global Pool was not GC'd")
	}
}
