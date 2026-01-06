package core_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	core "github.com/Swind/go-task-runner/core"
)

// testThreadPool is a minimal mock for initial testing
type testThreadPool struct {
	started bool
}

func (p *testThreadPool) PostInternal(task core.Task, traits core.TaskTraits) {
	// Execute immediately for constructor test
	task(context.Background())
}

func (p *testThreadPool) PostDelayedInternal(task core.Task, delay time.Duration, traits core.TaskTraits, target core.TaskRunner) {
}
func (p *testThreadPool) Start(ctx context.Context) { p.started = true }
func (p *testThreadPool) Stop()                     {}
func (p *testThreadPool) ID() string                { return "test" }
func (p *testThreadPool) IsRunning() bool           { return p.started }
func (p *testThreadPool) WorkerCount() int          { return 1 }
func (p *testThreadPool) QueuedTaskCount() int      { return 0 }
func (p *testThreadPool) ActiveTaskCount() int      { return 0 }
func (p *testThreadPool) DelayedTaskCount() int     { return 0 }

// TestParallelTaskRunner_Constructor verifies runner initialization
// Given: A thread pool and maxConcurrency of 4
// When: NewParallelTaskRunner is called
// Then: Runner is created with correct maxConcurrency
func TestParallelTaskRunner_Constructor(t *testing.T) {
	// Arrange
	pool := &testThreadPool{}
	maxConcurrency := 4

	// Act
	runner := core.NewParallelTaskRunner(pool, maxConcurrency)

	// Assert
	if runner == nil {
		t.Fatal("NewParallelTaskRunner returned nil")
	}
	if runner.MaxConcurrency() != maxConcurrency {
		t.Errorf("MaxConcurrency() = %d, want %d", runner.MaxConcurrency(), maxConcurrency)
	}
	if runner.IsClosed() {
		t.Error("New runner should not be closed")
	}
}

// concurrentTestThreadPool tracks posted tasks without executing them
type concurrentTestThreadPool struct {
	mu          sync.Mutex
	postedTasks []core.Task
}

func (p *concurrentTestThreadPool) PostInternal(task core.Task, traits core.TaskTraits) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.postedTasks = append(p.postedTasks, task)
}

// postedTaskCount returns the current count of posted tasks (thread-safe)
func (p *concurrentTestThreadPool) postedTaskCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.postedTasks)
}

func (p *concurrentTestThreadPool) PostDelayedInternal(task core.Task, delay time.Duration, traits core.TaskTraits, target core.TaskRunner) {
}
func (p *concurrentTestThreadPool) Start(ctx context.Context) {}
func (p *concurrentTestThreadPool) Stop()                     {}
func (p *concurrentTestThreadPool) ID() string                { return "test" }
func (p *concurrentTestThreadPool) IsRunning() bool           { return true }
func (p *concurrentTestThreadPool) WorkerCount() int          { return 1 }
func (p *concurrentTestThreadPool) QueuedTaskCount() int      { return 0 }
func (p *concurrentTestThreadPool) ActiveTaskCount() int      { return 0 }
func (p *concurrentTestThreadPool) DelayedTaskCount() int     { return 0 }

// TestParallelTaskRunner_PostTask_QueuesTasks verifies tasks are queued
// Given: A ParallelTaskRunner with maxConcurrency=1
// When: Two tasks are posted
// Then: First task is posted to pool, second is queued
func TestParallelTaskRunner_PostTask_QueuesTasks(t *testing.T) {
	// Arrange
	pool := &concurrentTestThreadPool{}
	runner := core.NewParallelTaskRunner(pool, 1)

	executed := make([]int, 0)
	task1 := func(ctx context.Context) {
		executed = append(executed, 1)
	}
	task2 := func(ctx context.Context) {
		executed = append(executed, 2)
	}

	// Act
	runner.PostTask(task1)
	runner.PostTask(task2)

	// Give scheduler time to process tasks (SingleThreadTaskRunner is async)
	time.Sleep(10 * time.Millisecond)

	// Assert - First task submitted to pool (use thread-safe method)
	if pool.postedTaskCount() != 1 {
		t.Fatalf("postedTasks = %d, want 1", pool.postedTaskCount())
	}
}

// TestParallelTaskRunner_ConcurrentPostTask verifies concurrent PostTask calls are safe
// Given: A ParallelTaskRunner with maxConcurrency=2
// When: Multiple goroutines post tasks concurrently
// Then: No race conditions occur (test passes with -race flag)
func TestParallelTaskRunner_ConcurrentPostTask(t *testing.T) {
	// Arrange
	pool := &concurrentTestThreadPool{}
	runner := core.NewParallelTaskRunner(pool, 2)

	const numTasks = 100
	var wg sync.WaitGroup

	// Act - Post tasks concurrently from multiple goroutines
	// The key is that this test runs with -race to detect data races
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(taskNum int) {
			defer wg.Done()
			runner.PostTask(func(ctx context.Context) {})
		}(i)
	}
	wg.Wait()

	// Assert - Test passes if no data race is detected by -race flag
	// The actual number of posted tasks may vary due to timing
	pool.mu.Lock()
	postedCount := len(pool.postedTasks)
	pool.mu.Unlock()

	// We should have at least 1 task posted
	if postedCount < 1 {
		t.Errorf("postedTasks = %d, want at least 1", postedCount)
	}

	t.Logf("Successfully posted %d tasks concurrently without race conditions", numTasks)
}

// executingTestThreadPool actually executes posted tasks
type executingTestThreadPool struct {
	maxWorkers int
	tasks      chan core.Task
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func (p *executingTestThreadPool) Start(ctx context.Context) {
	p.mu.Lock()
	p.ctx, p.cancel = context.WithCancel(ctx)
	// Initialize tasks channel if not already done
	if p.tasks == nil {
		p.tasks = make(chan core.Task, 100)
	}
	// Capture p.ctx for use in worker closures
	runCtx := p.ctx
	p.mu.Unlock()

	for i := 0; i < p.maxWorkers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case task := <-p.tasks:
					task(runCtx)
				case <-runCtx.Done():
					return
				}
			}
		}()
	}
}

func (p *executingTestThreadPool) PostInternal(task core.Task, traits core.TaskTraits) {
	p.mu.Lock()
	// Initialize tasks channel on first use if needed
	// This allows PostInternal to be called before Start
	if p.tasks == nil {
		p.tasks = make(chan core.Task, 100)
	}
	p.mu.Unlock()
	p.tasks <- task
}

func (p *executingTestThreadPool) PostDelayedInternal(task core.Task, delay time.Duration, traits core.TaskTraits, target core.TaskRunner) {
	time.AfterFunc(delay, func() {
		target.PostTask(task)
	})
}

func (p *executingTestThreadPool) Stop() {
	p.cancel()
	p.wg.Wait()
}

func (p *executingTestThreadPool) ID() string            { return "executing-test" }
func (p *executingTestThreadPool) IsRunning() bool       { return p.ctx != nil }
func (p *executingTestThreadPool) WorkerCount() int      { return p.maxWorkers }
func (p *executingTestThreadPool) QueuedTaskCount() int  { return len(p.tasks) }
func (p *executingTestThreadPool) ActiveTaskCount() int  { return 0 }
func (p *executingTestThreadPool) DelayedTaskCount() int { return 0 }

// TestParallelTaskRunner_WaitIdle_WaitsForAllTasks verifies WaitIdle blocks until completion
// Given: A ParallelTaskRunner with pending tasks
// When: WaitIdle is called
// Then: It returns only after all tasks complete
func TestParallelTaskRunner_WaitIdle_WaitsForAllTasks(t *testing.T) {
	// Arrange
	pool := &executingTestThreadPool{maxWorkers: 2}
	runner := core.NewParallelTaskRunner(pool, 2)

	taskCount := 3
	executed := make([]int, 0, taskCount)
	var executedMu sync.Mutex

	// Post tasks FIRST - before pool starts, ensuring they queue
	for i := range taskCount {
		idx := i
		task := func(ctx context.Context) {
			executedMu.Lock()
			executed = append(executed, idx)
			executedMu.Unlock()
		}
		runner.PostTask(task)
	}

	// Act - Start WaitIdle BEFORE pool starts
	done := make(chan struct{})
	go func() {
		err := runner.WaitIdle(context.Background())
		if err != nil {
			t.Errorf("WaitIdle returned error: %v", err)
		}
		close(done)
	}()

	// Start pool LAST - tasks are already queued, WaitIdle is already waiting
	ctx := context.Background()
	pool.Start(ctx)

	// Assert - WaitIdle completes after all tasks
	select {
	case <-done:
		executedMu.Lock()
		executedCount := len(executed)
		executedMu.Unlock()
		if executedCount != taskCount {
			t.Errorf("executed = %d, want %d", executedCount, taskCount)
		}
	case <-time.After(2 * time.Second):
		executedMu.Lock()
		executedCount := len(executed)
		executedMu.Unlock()
		t.Fatalf("WaitIdle timed out, executed = %d, want %d", executedCount, taskCount)
	}

	pool.Stop()
}

// TestParallelTaskRunner_BarrierTaskRunning_NoRace verifies concurrent FlushAsync calls don't race
// Given: A ParallelTaskRunner with multiple concurrent FlushAsync calls
// When: Multiple goroutines call FlushAsync simultaneously
// Then: No data races occur on barrierTaskRunning flag (verified with -race flag)
func TestParallelTaskRunner_BarrierTaskRunning_NoRace(t *testing.T) {
	// Arrange
	pool := &executingTestThreadPool{maxWorkers: 4}
	runner := core.NewParallelTaskRunner(pool, 4)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	const numFlushes = 10
	const numTasks = 5
	var wg sync.WaitGroup
	var callbackCount atomic.Int32

	// Post some tasks first to make barriers more interesting
	for i := 0; i < numTasks; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
		})
	}

	// Act - Multiple concurrent FlushAsync calls
	for i := 0; i < numFlushes; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			runner.FlushAsync(func() {
				callbackCount.Add(1)
			})
		}(i)
	}

	// Wait for all FlushAsync calls to complete
	wg.Wait()

	// Give callbacks time to execute
	time.Sleep(500 * time.Millisecond)

	// Assert - All callbacks executed (no race, no deadlock)
	count := callbackCount.Load()
	if count != numFlushes {
		t.Errorf("callback count = %d, want %d", count, numFlushes)
	}
}

// TestParallelTaskRunner_WaitIdle_MultipleCallsHandled verifies WaitIdle works after barrier completes
// Given: A ParallelTaskRunner that has completed a WaitIdle
// When: WaitIdle is called again
// Then: The second WaitIdle also completes successfully (barrierTaskRunning was properly reset)
func TestParallelTaskRunner_WaitIdle_MultipleCallsHandled(t *testing.T) {
	// Arrange
	pool := &executingTestThreadPool{maxWorkers: 2}
	runner := core.NewParallelTaskRunner(pool, 2)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	// Post a task before first WaitIdle
	runner.PostTask(func(ctx context.Context) {
		time.Sleep(10 * time.Millisecond)
	})

	// Act - First WaitIdle
	err := runner.WaitIdle(context.Background())
	if err != nil {
		t.Fatalf("First WaitIdle failed: %v", err)
	}

	// Post another task
	runner.PostTask(func(ctx context.Context) {
		time.Sleep(10 * time.Millisecond)
	})

	// Act - Second WaitIdle
	err = runner.WaitIdle(context.Background())
	if err != nil {
		t.Fatalf("Second WaitIdle failed: %v", err)
	}

	// Assert - Both WaitIdle calls completed successfully
	// If barrierTaskRunning wasn't reset, second WaitIdle would hang
}

// TestParallelTaskRunner_FlushAsync_PanicInCallback verifies barrier cleanup on panic
// Given: A ParallelTaskRunner with FlushAsync callback that panics
// When: The callback panics
// Then: barrierTaskRunning is still reset, subsequent FlushAsync works
func TestParallelTaskRunner_FlushAsync_PanicInCallback(t *testing.T) {
	// Arrange
	pool := &executingTestThreadPool{maxWorkers: 2}
	runner := core.NewParallelTaskRunner(pool, 2)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	var secondCallbackExecuted atomic.Bool

	// Act - First FlushAsync with panic
	runner.FlushAsync(func() {
		panic("intentional test panic")
	})

	// Give first callback time to panic and cleanup
	time.Sleep(100 * time.Millisecond)

	// Act - Second FlushAsync should work
	runner.FlushAsync(func() {
		secondCallbackExecuted.Store(true)
	})

	// Wait for second callback
	time.Sleep(100 * time.Millisecond)

	// Assert - Second callback executed (barrier was properly reset despite panic)
	if !secondCallbackExecuted.Load() {
		t.Error("Second FlushAsync callback did not execute - barrier may not have been reset")
	}
}

// TestParallelTaskRunner_Shutdown_CompletesCleanup verifies Shutdown completes cleanup
// Given: A ParallelTaskRunner with pending tasks
// When: Shutdown is called
// Then: Queue is cleared and shutdown completes without hanging
func TestParallelTaskRunner_Shutdown_CompletesCleanup(t *testing.T) {
	// Arrange
	pool := &executingTestThreadPool{maxWorkers: 2}
	runner := core.NewParallelTaskRunner(pool, 2)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	// Post some tasks that won't execute (queue fills up)
	for i := 0; i < 10; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(100 * time.Millisecond)
		})
	}

	// Act - Shutdown should complete quickly
	done := make(chan struct{})
	go func() {
		runner.Shutdown()
		close(done)
	}()

	// Assert - Shutdown completes within reasonable time
	select {
	case <-done:
		// Success - shutdown completed
	case <-time.After(1 * time.Second):
		t.Fatal("Shutdown did not complete within 1 second")
	}

	// Verify runner is closed
	if !runner.IsClosed() {
		t.Error("Runner should be closed after Shutdown")
	}
}

// TestParallelTaskRunner_WaitIdle_ShutdownDuring verifies WaitIdle detects shutdown
// Given: A ParallelTaskRunner with WaitIdle in progress
// When: Shutdown is called during WaitIdle
// Then: WaitIdle returns with shutdown error instead of hanging
func TestParallelTaskRunner_WaitIdle_ShutdownDuring(t *testing.T) {
	// Arrange
	pool := &executingTestThreadPool{maxWorkers: 2}
	runner := core.NewParallelTaskRunner(pool, 2)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	// Post long-running tasks to keep runner busy
	for i := 0; i < 5; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(200 * time.Millisecond)
		})
	}

	// Start WaitIdle in background
	waitErr := make(chan error, 1)
	go func() {
		err := runner.WaitIdle(context.Background())
		waitErr <- err
	}()

	// Give WaitIdle time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Act - Shutdown during WaitIdle
	runner.Shutdown()

	// Assert - WaitIdle returns with shutdown error
	select {
	case err := <-waitErr:
		if err == nil {
			t.Error("Expected error from WaitIdle during shutdown, got nil")
		} else if err.Error() != "runner shutdown during WaitIdle" {
			t.Errorf("Expected shutdown error, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("WaitIdle did not return within timeout")
	}
}

// TestParallelTaskRunner_Constructor_NilThreadPool verifies panic on nil threadPool
// Given: NewParallelTaskRunner called with nil threadPool
// When: Constructor is called
// Then: Panic with appropriate error message
func TestParallelTaskRunner_Constructor_NilThreadPool(t *testing.T) {
	// Arrange & Act & Assert
	defer func() {
		if r := recover(); r != nil {
			msg := r.(string)
			if msg != "ParallelTaskRunner: threadPool must not be nil" {
				t.Errorf("Expected panic message about nil threadPool, got: %v", msg)
			}
		} else {
			t.Error("Expected panic for nil threadPool, but no panic occurred")
		}
	}()

	// This should panic
	core.NewParallelTaskRunner(nil, 4)
}

// TestParallelTaskRunner_Constructor_ExcessiveConcurrency verifies panic on excessive maxConcurrency
// Given: NewParallelTaskRunner called with maxConcurrency > 10000
// When: Constructor is called
// Then: Panic with appropriate error message
func TestParallelTaskRunner_Constructor_ExcessiveConcurrency(t *testing.T) {
	// Arrange
	pool := &testThreadPool{}

	// Act & Assert
	defer func() {
		if r := recover(); r != nil {
			msg := r.(string)
			expected := "ParallelTaskRunner: maxConcurrency must not exceed 10000"
			if msg != expected {
				t.Errorf("Expected panic message '%s', got: %v", expected, msg)
			}
		} else {
			t.Error("Expected panic for excessive maxConcurrency, but no panic occurred")
		}
	}()

	// This should panic
	core.NewParallelTaskRunner(pool, 20000)
}

// TestParallelTaskRunner_Constructor_InvalidConcurrency verifies panic on invalid maxConcurrency
// Given: NewParallelTaskRunner called with maxConcurrency < 1
// When: Constructor is called
// Then: Panic with appropriate error message (tests existing behavior still works)
func TestParallelTaskRunner_Constructor_InvalidConcurrency(t *testing.T) {
	// Arrange
	pool := &testThreadPool{}

	// Act & Assert
	defer func() {
		if r := recover(); r != nil {
			msg := r.(string)
			expected := "ParallelTaskRunner: maxConcurrency must be at least 1"
			if msg != expected {
				t.Errorf("Expected panic message '%s', got: %v", expected, msg)
			}
		} else {
			t.Error("Expected panic for invalid maxConcurrency, but no panic occurred")
		}
	}()

	// This should panic
	core.NewParallelTaskRunner(pool, 0)
}
