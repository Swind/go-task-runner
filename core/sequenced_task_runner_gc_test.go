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

// TestSequencedTaskRunner_GC_TaskWithStructMethod verifies struct method GC
// Given: a struct with finalizer posted as a task method
// When: the task completes and object goes out of scope
// Then: the struct is garbage collected and finalizer is called
func TestSequencedTaskRunner_GC_TaskWithStructMethod(t *testing.T) {
	// Arrange - Create pool, runner, and object with finalizer
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Act - Create scope for object
	func() {
		obj := &TestObject{
			ID:   "test-obj-1",
			Data: make([]byte, 1024*1024), // 1MB
		}

		runtime.SetFinalizer(obj, func(o *TestObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		runner.PostTask(obj.Process)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runner.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle failed: %v", err)
		}
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
		// Assert - Verify finalizer was called
		if !finalizerCalled.Load() {
			t.Error("finalizer called: got = false, want = true")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for object to be GC'd")
	}
}

// TestSequencedTaskRunner_GC_ClosureCapturedObjects verifies closure-captured object GC
// Given: 100 objects captured by task closures
// When: tasks complete and objects go out of scope
// Then: all 100 objects are garbage collected and finalizers called
func TestSequencedTaskRunner_GC_ClosureCapturedObjects(t *testing.T) {
	// Arrange - Create pool, runner, and objects with finalizers
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const numObjects = 100
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numObjects)

	// Act - Create scope for objects
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

			runner.PostTask(func(ctx context.Context) {
				_ = obj.ID
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runner.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle failed: %v", err)
		}
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
		// Assert - Verify all objects collected
		collected := finalizerCount.Load()
		if collected != numObjects {
			t.Errorf("objects GC'd: got = %d, want = %d", collected, numObjects)
		}
	case <-time.After(3 * time.Second):
		collected := finalizerCount.Load()
		t.Errorf("timeout: only %d/%d objects were GC'd", collected, numObjects)
	}
}

// TestSequencedTaskRunner_GC_RepeatingTaskStopped verifies repeating task object GC
// Given: an object captured by a repeating task
// When: the repeating task is stopped
// Then: the object is garbage collected and finalizer is called
func TestSequencedTaskRunner_GC_RepeatingTaskStopped(t *testing.T) {
	// Arrange - Create pool, runner, and repeating task with captured object
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	var handle core.RepeatingTaskHandle

	// Act - Create scope for object
	func() {
		obj := &TestObject{
			ID:   "repeating-obj",
			Data: make([]byte, 100*1024), // 100KB
		}

		runtime.SetFinalizer(obj, func(o *TestObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		handle = runner.PostRepeatingTask(func(ctx context.Context) {
			_ = obj.ID
		}, 10*time.Millisecond)

		time.Sleep(50 * time.Millisecond)
		handle.Stop()
		time.Sleep(50 * time.Millisecond)
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
		// Assert - Verify finalizer called
		if !finalizerCalled.Load() {
			t.Error("finalizer called: got = false, want = true")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: object was not GC'd")
	}
}

// TestSequencedTaskRunner_GC_ShutdownClearsQueue verifies pending task GC on shutdown
// Given: 100 pending tasks in queue with captured objects
// When: runner is shutdown
// Then: all pending task objects are garbage collected
func TestSequencedTaskRunner_GC_ShutdownClearsQueue(t *testing.T) {
	// Arrange - Create pool, runner, and block with long task
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 1)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const numPendingTasks = 100
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numPendingTasks)

	// Act - Create scope for objects
	func() {
		blocker := make(chan struct{})
		runner.PostTask(func(ctx context.Context) {
			<-blocker
		})

		for i := 0; i < numPendingTasks; i++ {
			obj := &TestObject{
				ID:   "pending-obj",
				Data: make([]byte, 1024),
			}

			runtime.SetFinalizer(obj, func(o *TestObject) {
				finalizerCount.Add(1)
				wg.Done()
			})

			runner.PostTask(func(ctx context.Context) {
				_ = obj.ID
			})
		}

		time.Sleep(10 * time.Millisecond)
		runner.Shutdown()
		close(blocker)
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
		// Assert - Verify all pending tasks collected
		collected := finalizerCount.Load()
		if collected != numPendingTasks {
			t.Errorf("pending tasks GC'd: got = %d, want = %d", collected, numPendingTasks)
		}
	case <-time.After(3 * time.Second):
		collected := finalizerCount.Load()
		t.Errorf("timeout: only %d/%d pending tasks were GC'd", collected, numPendingTasks)
	}
}

// TestSequencedTaskRunner_GC_TaskAndReplyPattern verifies PostTaskAndReply object GC
// Given: an object captured in both task and reply closures
// When: task and reply complete
// Then: the object is garbage collected and finalizer is called
func TestSequencedTaskRunner_GC_TaskAndReplyPattern(t *testing.T) {
	// Arrange - Create pool, runners, and object with finalizer
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	bgRunner := core.NewSequencedTaskRunner(pool)
	uiRunner := core.NewSequencedTaskRunner(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Act - Create scope for object
	func() {
		obj := &TestObject{
			ID:   "task-reply-obj",
			Data: make([]byte, 50*1024), // 50KB
		}

		runtime.SetFinalizer(obj, func(o *TestObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		done := make(chan struct{})
		uiRunner.PostTask(func(ctx context.Context) {
			bgRunner.PostTaskAndReply(
				func(ctx context.Context) {
					_ = obj.ID
				},
				func(ctx context.Context) {
					_ = obj.ID
					close(done)
				},
				uiRunner,
			)
		})

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for task and reply")
		}
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
		// Assert - Verify finalizer called
		if !finalizerCalled.Load() {
			t.Error("finalizer called: got = false, want = true")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: object was not GC'd")
	}
}

// TestSequencedTaskRunner_GC_DelayedTask verifies delayed task object GC
// Given: an object captured by a delayed task
// When: the delayed task executes
// Then: the object is garbage collected and finalizer is called
func TestSequencedTaskRunner_GC_DelayedTask(t *testing.T) {
	// Arrange - Create pool, runner, and delayed task with captured object
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Act - Create scope for object
	func() {
		obj := &TestObject{
			ID:   "delayed-obj",
			Data: make([]byte, 20*1024), // 20KB
		}

		runtime.SetFinalizer(obj, func(o *TestObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		done := make(chan struct{})
		runner.PostDelayedTask(func(ctx context.Context) {
			_ = obj.ID
			close(done)
		}, 50*time.Millisecond)

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for delayed task")
		}
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
		// Assert - Verify finalizer called
		if !finalizerCalled.Load() {
			t.Error("finalizer called: got = false, want = true")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: object was not GC'd")
	}
}

// TestSequencedTaskRunner_GC_MemoryGrowth verifies no unbounded memory growth
// Given: a SequencedTaskRunner executing 1000 tasks with 100KB allocations each
// When: all tasks complete and GC runs
// Then: memory growth is less than 10MB (no memory leak)
func TestSequencedTaskRunner_GC_MemoryGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory growth test in short mode")
	}

	// Arrange - Create pool and runner
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	// Get baseline memory
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Act - Post many tasks that allocate memory
	const iterations = 1000
	for i := 0; i < iterations; i++ {
		runner.PostTask(func(ctx context.Context) {
			data := make([]byte, 100*1024) // 100KB
			_ = data[0]
		})

		if i%100 == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			_ = runner.WaitIdle(ctx)
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

	var allocated int64
	if m2.Alloc > m1.Alloc {
		allocated = int64(m2.Alloc - m1.Alloc)
	} else {
		allocated = -int64(m1.Alloc - m2.Alloc)
	}

	// Assert - Verify memory growth is acceptable
	maxAcceptableGrowth := int64(10 * 1024 * 1024) // 10MB
	if allocated > maxAcceptableGrowth {
		t.Errorf("memory growth: got = %d MB (max acceptable: %d MB)",
			allocated/1024/1024, maxAcceptableGrowth/1024/1024)
		t.Error("Possible memory leak detected")
	}

	t.Logf("Memory stats:")
	t.Logf("  Initial Alloc: %d MB", m1.Alloc/1024/1024)
	t.Logf("  Final Alloc: %d MB", m2.Alloc/1024/1024)
	t.Logf("  Growth: %d MB", allocated/1024/1024)
}

// TestSequencedTaskRunner_GC_RunnerItself verifies runner can be GC'd
// Given: a SequencedTaskRunner that has executed tasks and been shutdown
// When: all references are dropped
// Then: the runner is garbage collected and finalizer is called
func TestSequencedTaskRunner_GC_RunnerItself(t *testing.T) {
	// Arrange - Create pool
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Act - Create scope for runner
	func() {
		runner := core.NewSequencedTaskRunner(pool)

		runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		done := make(chan struct{})
		runner.PostTask(func(ctx context.Context) {
			close(done)
		})

		<-done
		runner.Shutdown()
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
		// Assert - Verify runner was GC'd
		if !finalizerCalled.Load() {
			t.Error("runner GC'd: got = false, want = true")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: runner was not GC'd")
	}
}

// TestObject is a struct used to test GC behavior
type TestObject struct {
	ID   string
	Data []byte
}

// Process is a method that can be posted as a task
func (o *TestObject) Process(ctx context.Context) {
	_ = o.ID
	_ = len(o.Data)
}

// TestSequencedTaskRunner_GC_WithGlobalThreadPool verifies global pool runner GC
// Given: a SequencedTaskRunner created from the global thread pool
// When: the runner is shutdown while global pool remains active
// Then: the runner is garbage collected and finalizer is called
func TestSequencedTaskRunner_GC_WithGlobalThreadPool(t *testing.T) {
	// Arrange - Initialize global thread pool
	taskrunner.InitGlobalThreadPool(2)
	defer taskrunner.ShutdownGlobalThreadPool()

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Act - Create scope for runner
	func() {
		runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

		runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		done := make(chan struct{})
		runner.PostTask(func(ctx context.Context) {
			close(done)
		})

		<-done
		runner.Shutdown()
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
		// Assert - Verify runner was GC'd
		if !finalizerCalled.Load() {
			t.Error("runner from global pool GC'd: got = false, want = true")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: runner from global pool was not GC'd")
	}
}

// TestSequencedTaskRunner_GC_TaskAndReplyWithResult verifies result-passing pattern GC
// Given: an object captured in PostTaskAndReplyWithResult
// When: task and reply complete
// Then: the object is garbage collected and finalizer is called
func TestSequencedTaskRunner_GC_TaskAndReplyWithResult(t *testing.T) {
	// Arrange - Create pool, runners, and object with finalizer
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	bgRunner := core.NewSequencedTaskRunner(pool)
	uiRunner := core.NewSequencedTaskRunner(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Act - Create scope for object
	func() {
		obj := &TestObject{
			ID:   "task-reply-result-obj",
			Data: make([]byte, 50*1024), // 50KB
		}

		runtime.SetFinalizer(obj, func(o *TestObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		done := make(chan struct{})

		core.PostTaskAndReplyWithResult(
			bgRunner,
			func(ctx context.Context) (string, error) {
				return obj.ID, nil
			},
			func(ctx context.Context, result string, err error) {
				// Ensure we used the result
				_ = result
				close(done)
			},
			uiRunner,
		)

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for task and reply")
		}
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
		// Assert - Verify finalizer called
		if !finalizerCalled.Load() {
			t.Error("finalizer called: got = false, want = true")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: object was not GC'd")
	}
}
