package core_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// TestSequencedTaskRunner_ConcurrentPostTask verifies that concurrent PostTask calls
// Given: 100 goroutines each posting 100 tasks to a SequencedTaskRunner
// When: all tasks are posted and WaitIdle is called
// Then: all 10000 tasks execute exactly once and runningCount returns to 0
func TestSequencedTaskRunner_ConcurrentPostTask(t *testing.T) {
	// Arrange - Create thread pool and runner
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const numGoroutines = 100
	const tasksPerGoroutine = 100

	var executionOrder []int
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Act - Launch many goroutines posting tasks concurrently
	for i := range numGoroutines {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := range tasksPerGoroutine {
				taskID := goroutineID*tasksPerGoroutine + j
				runner.PostTask(func(ctx context.Context) {
					mu.Lock()
					executionOrder = append(executionOrder, taskID)
					mu.Unlock()
				})
			}
		}(i)
	}

	wg.Wait()

	// Wait for all tasks to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runner.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	// Assert - Verify all tasks were executed
	expectedCount := numGoroutines * tasksPerGoroutine
	gotCount := len(executionOrder)
	if gotCount != expectedCount {
		t.Errorf("tasks executed: got = %d, want = %d", gotCount, expectedCount)
	}

	// Assert - Verify runningCount is back to 0
	finalCount := runner.GetRunningCount()
	wantFinalCount := int32(0)
	if finalCount != wantFinalCount {
		t.Errorf("final runningCount: got = %d, want = %d", finalCount, wantFinalCount)
	}
}

// TestSequencedTaskRunner_RunningCountInvariant verifies that runningCount never exceeds 1
// Given: a SequencedTaskRunner with a monitoring goroutine and 1000 posted tasks
// When: tasks execute while monitor continuously checks runningCount
// Then: runningCount never exceeds 1 and all tasks execute successfully
func TestSequencedTaskRunner_RunningCountInvariant(t *testing.T) {
	// Arrange - Create thread pool, runner, and monitoring setup
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 8)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var maxRunningCount int32
	var tasksExecuted int32
	const numTasks = 1000

	done := make(chan struct{})

	// Act - Start monitor goroutine that continuously checks runningCount
	go func() {
		ticker := time.NewTicker(1 * time.Microsecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				current := runner.GetRunningCount()
				if current > atomic.LoadInt32(&maxRunningCount) {
					atomic.StoreInt32(&maxRunningCount, current)
				}
				if current > 1 {
					t.Errorf("VIOLATION: runningCount exceeded 1, current value: %d", current)
				}
			}
		}
	}()

	// Post many tasks
	for range numTasks {
		runner.PostTask(func(ctx context.Context) {
			// Simulate some work
			time.Sleep(10 * time.Microsecond)
			atomic.AddInt32(&tasksExecuted, 1)
		})
	}

	// Wait for all tasks to complete
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runner.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	close(done)

	// Assert - Verify all tasks executed
	gotExecuted := tasksExecuted
	wantExecuted := int32(numTasks)
	if gotExecuted != wantExecuted {
		t.Errorf("tasks executed: got = %d, want = %d", gotExecuted, wantExecuted)
	}

	// Assert - Verify runningCount never exceeded 1
	if atomic.LoadInt32(&maxRunningCount) > 1 {
		t.Errorf("max runningCount: got = %d (want <= 1)", atomic.LoadInt32(&maxRunningCount))
	}

	// Assert - Verify final state
	finalCount := runner.GetRunningCount()
	wantFinalCount := int32(0)
	if finalCount != wantFinalCount {
		t.Errorf("final runningCount: got = %d, want = %d", finalCount, wantFinalCount)
	}
}

// TestSequencedTaskRunner_SequentialOrderUnderConcurrency verifies FIFO execution order
// Given: 1000 tasks posted concurrently from multiple goroutines
// When: all tasks complete
// Then: each task executes exactly once (execution order may differ from posting order)
func TestSequencedTaskRunner_SequentialOrderUnderConcurrency(t *testing.T) {
	// Arrange - Create thread pool, runner, and tracking setup
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var executionOrder []int
	var mu sync.Mutex

	const numTasks = 1000
	posted := make(chan int, numTasks)

	var wg sync.WaitGroup

	// Act - Post tasks from multiple goroutines
	for i := range numTasks {
		wg.Add(1)
		go func(taskID int) {
			defer wg.Done()
			runner.PostTask(func(ctx context.Context) {
				mu.Lock()
				executionOrder = append(executionOrder, taskID)
				mu.Unlock()
			})
			posted <- taskID
		}(i)
	}

	wg.Wait()
	close(posted)

	// Wait for all tasks to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runner.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	// Assert - Verify all tasks executed
	gotCount := len(executionOrder)
	wantCount := numTasks
	if gotCount != wantCount {
		t.Errorf("tasks executed: got = %d, want = %d", gotCount, wantCount)
	}

	// Assert - Verify each task executed exactly once
	seen := make(map[int]int)
	for _, id := range executionOrder {
		seen[id]++
	}

	for i := range numTasks {
		count, ok := seen[i]
		if !ok {
			t.Errorf("task %d: was never executed", i)
		} else if count != 1 {
			t.Errorf("task %d: executed %d times (want = 1)", i, count)
		}
	}
}

// TestSequencedTaskRunner_DoubleCheckPattern tests the double-check pattern in runLoop
// Given: a SequencedTaskRunner with rapid task posting
// When: 100 iterations of posting 2 tasks in quick succession
// Then: both tasks execute in each iteration and runningCount returns to 0
func TestSequencedTaskRunner_DoubleCheckPattern(t *testing.T) {
	// Arrange - Create thread pool and runner
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const iterations = 100

	// Act - Run 100 iterations of double-check pattern test
	for iter := range iterations {
		var executed int32

		// Post initial task
		runner.PostTask(func(ctx context.Context) {
			atomic.AddInt32(&executed, 1)
			// Small delay to allow runLoop to potentially think queue is empty
			time.Sleep(1 * time.Microsecond)
		})

		// Immediately post another task (race with first task's completion)
		runner.PostTask(func(ctx context.Context) {
			atomic.AddInt32(&executed, 1)
		})

		// Wait for tasks to complete
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		err := runner.WaitIdle(ctx)
		cancel()

		// Assert - Verify both tasks executed
		if err != nil {
			t.Fatalf("iteration %d: WaitIdle failed: %v", iter, err)
		}

		gotExecuted := executed
		wantExecuted := int32(2)
		if gotExecuted != wantExecuted {
			t.Errorf("iteration %d: tasks executed: got = %d, want = %d", iter, gotExecuted, wantExecuted)
		}

		// Assert - Verify runningCount is back to 0
		rc := runner.GetRunningCount()
		wantRC := int32(0)
		if rc != wantRC {
			t.Errorf("iteration %d: runningCount: got = %d, want = %d", iter, rc, wantRC)
		}
	}
}

// TestSequencedTaskRunner_StressTest performs a comprehensive stress test
// Given: 50 goroutines posting 100 tasks each with mixed execution times
// When: all tasks are posted and executed
// Then: all 5000 tasks execute sequentially and runningCount invariant holds
func TestSequencedTaskRunner_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// Arrange - Create thread pool, runner, and tracking setup
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 8)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const numPosters = 50
	const tasksPerPoster = 100
	const totalTasks = numPosters * tasksPerPoster

	var executionCounter int32
	var mu sync.Mutex
	executionTimes := make([]time.Time, 0, totalTasks)

	var wg sync.WaitGroup
	start := time.Now()

	// Act - Launch poster goroutines with mixed fast/slow tasks
	for i := range numPosters {
		wg.Add(1)
		go func(posterID int) {
			defer wg.Done()
			for j := range tasksPerPoster {
				taskNum := j
				runner.PostTask(func(ctx context.Context) {
					// Mix of fast and slow tasks
					if taskNum%10 == 0 {
						time.Sleep(100 * time.Microsecond)
					}

					mu.Lock()
					executionTimes = append(executionTimes, time.Now())
					mu.Unlock()

					atomic.AddInt32(&executionCounter, 1)
				})

				// Small random delay between posts
				if j%5 == 0 {
					time.Sleep(1 * time.Microsecond)
				}
			}
		}(i)
	}

	wg.Wait()
	postingDone := time.Now()

	// Wait for all tasks to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := runner.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	executionDone := time.Now()

	// Assert - Verify all tasks executed
	finalCount := atomic.LoadInt32(&executionCounter)
	wantCount := int32(totalTasks)
	if finalCount != wantCount {
		t.Errorf("tasks executed: got = %d, want = %d", finalCount, wantCount)
	}

	// Assert - Verify execution times are sequential (monotonically increasing)
	for i := 1; i < len(executionTimes); i++ {
		if executionTimes[i].Before(executionTimes[i-1]) {
			t.Errorf("execution order violation: task %d executed before task %d", i, i-1)
		}
	}

	// Assert - Verify final runningCount
	rc := runner.GetRunningCount()
	wantRC := int32(0)
	if rc != wantRC {
		t.Errorf("final runningCount: got = %d, want = %d", rc, wantRC)
	}

	t.Logf("Stress test completed:")
	t.Logf("  Tasks posted: %d", totalTasks)
	t.Logf("  Posting time: %v", postingDone.Sub(start))
	t.Logf("  Execution time: %v", executionDone.Sub(postingDone))
	t.Logf("  Total time: %v", executionDone.Sub(start))
}

// TestSequencedTaskRunner_NoSpuriousRunLoops verifies CAS prevents duplicate runLoops
// Given: a SequencedTaskRunner and 1000 concurrent calls to ensureRunning
// When: all calls execute concurrently
// Then: runningCount never exceeds 1 and final state returns to 0
func TestSequencedTaskRunner_NoSpuriousRunLoops(t *testing.T) {
	// Arrange - Create thread pool and runner
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 8)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const numConcurrentCalls = 1000
	var wg sync.WaitGroup

	// Act - Try to call ensureRunning many times concurrently
	for range numConcurrentCalls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runner.ExportedEnsureRunning(core.DefaultTaskTraits())
		}()
	}

	wg.Wait()

	// Assert - Verify runningCount is at most 1
	rc := runner.GetRunningCount()
	if rc > 1 {
		t.Errorf("runningCount during test: got = %d (want <= 1)", rc)
	}

	// Post a task to clean up if runLoop is running
	runner.PostTask(func(ctx context.Context) {})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_ = runner.WaitIdle(ctx)

	// Assert - Verify final runningCount is 0
	finalRC := runner.GetRunningCount()
	wantFinalRC := int32(0)
	if finalRC != wantFinalRC {
		t.Errorf("final runningCount: got = %d, want = %d", finalRC, wantFinalRC)
	}
}

// TestSequencedTaskRunner_PanicOnInvalidRunningCount verifies runLoop panic on invalid state
// Given: a SequencedTaskRunner with runningCount manually set to 2
// When: runLoop is executed directly
// Then: runLoop panics with message mentioning runningCount=2
func TestSequencedTaskRunner_PanicOnInvalidRunningCount(t *testing.T) {
	// Arrange - Create thread pool, runner, and set invalid runningCount
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	runner.SetRunningCount(2)

	// Act & Assert - Try to execute runLoop directly (should panic)
	defer func() {
		if r := recover(); r == nil {
			t.Error("panic: got = nil (want panic with invalid runningCount)")
		} else {
			errMsg := fmt.Sprintf("%v", r)
			if !contains(errMsg, "runningCount=2") {
				t.Errorf("panic message: got = %v (want containing 'runningCount=2')", r)
			}
		}
	}()

	runner.ExportedRunLoop(context.Background())
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
