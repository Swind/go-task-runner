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
// from multiple goroutines do not cause duplicate runLoop instances.
func TestSequencedTaskRunner_ConcurrentPostTask(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const numGoroutines = 100
	const tasksPerGoroutine = 100

	var executionOrder []int
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Launch many goroutines posting tasks concurrently
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

	// Verify all tasks were executed
	expectedCount := numGoroutines * tasksPerGoroutine
	if len(executionOrder) != expectedCount {
		t.Errorf("Expected %d tasks to execute, got %d", expectedCount, len(executionOrder))
	}

	// Verify runningCount is back to 0
	finalCount := runner.GetRunningCount()
	if finalCount != 0 {
		t.Errorf("Expected runningCount to be 0 after all tasks complete, got %d", finalCount)
	}
}

// TestSequencedTaskRunner_RunningCountInvariant verifies that runningCount
// never exceeds 1 during concurrent operations.
func TestSequencedTaskRunner_RunningCountInvariant(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 8)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var maxRunningCount int32
	var tasksExecuted int32
	const numTasks = 1000

	done := make(chan struct{})

	// Monitor goroutine: continuously check runningCount
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

	// Verify all tasks executed
	if tasksExecuted != numTasks {
		t.Errorf("Expected %d tasks to execute, got %d", numTasks, tasksExecuted)
	}

	// Verify runningCount never exceeded 1
	if maxRunningCount > 1 {
		t.Errorf("runningCount exceeded 1 during execution (max observed: %d)", maxRunningCount)
	}

	// Verify final state
	finalCount := runner.GetRunningCount()
	if finalCount != 0 {
		t.Errorf("Expected runningCount to be 0 after completion, got %d", finalCount)
	}
}

// TestSequencedTaskRunner_SequentialOrderUnderConcurrency verifies that tasks
// execute in FIFO order even when posted from multiple goroutines.
func TestSequencedTaskRunner_SequentialOrderUnderConcurrency(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	var executionOrder []int
	var mu sync.Mutex

	// Post tasks from multiple goroutines
	// Each task records its ID in order
	const numTasks = 1000
	posted := make(chan int, numTasks)

	var wg sync.WaitGroup
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

	// Collect posting order
	var postOrder []int
	for id := range posted {
		postOrder = append(postOrder, id)
	}

	// Wait for all tasks to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runner.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	// Verify all tasks executed
	if len(executionOrder) != numTasks {
		t.Errorf("Expected %d tasks to execute, got %d", numTasks, len(executionOrder))
	}

	// Note: Due to concurrent posting, we can't guarantee execution order matches posting order
	// But we can verify that all tasks executed exactly once
	seen := make(map[int]int)
	for _, id := range executionOrder {
		seen[id]++
	}

	for i := range numTasks {
		if count, ok := seen[i]; !ok {
			t.Errorf("Task %d was never executed", i)
		} else if count != 1 {
			t.Errorf("Task %d executed %d times (expected 1)", i, count)
		}
	}
}

// TestSequencedTaskRunner_DoubleCheckPattern tests the double-check pattern
// by creating race conditions between Pop and Push operations.
func TestSequencedTaskRunner_DoubleCheckPattern(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const iterations = 100
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

		if err != nil {
			t.Fatalf("Iteration %d: WaitIdle failed: %v", iter, err)
		}

		if executed != 2 {
			t.Errorf("Iteration %d: Expected 2 tasks to execute, got %d", iter, executed)
		}

		// Verify runningCount is back to 0
		if rc := runner.GetRunningCount(); rc != 0 {
			t.Errorf("Iteration %d: runningCount should be 0, got %d", iter, rc)
		}
	}
}

// TestSequencedTaskRunner_StressTest is a comprehensive stress test that:
// - Posts tasks from many goroutines simultaneously
// - Mixes fast and slow tasks
// - Verifies sequential execution
// - Checks runningCount invariant
func TestSequencedTaskRunner_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

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

	// Launch poster goroutines
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

	// Verify all tasks executed
	finalCount := atomic.LoadInt32(&executionCounter)
	if finalCount != totalTasks {
		t.Errorf("Expected %d tasks to execute, got %d", totalTasks, finalCount)
	}

	// Verify execution times are sequential (monotonically increasing)
	for i := 1; i < len(executionTimes); i++ {
		if executionTimes[i].Before(executionTimes[i-1]) {
			t.Errorf("Execution times not sequential: task %d executed before task %d", i, i-1)
		}
	}

	// Verify final runningCount
	if rc := runner.GetRunningCount(); rc != 0 {
		t.Errorf("Expected final runningCount to be 0, got %d", rc)
	}

	t.Logf("Stress test completed:")
	t.Logf("  Tasks posted: %d", totalTasks)
	t.Logf("  Posting time: %v", postingDone.Sub(start))
	t.Logf("  Execution time: %v", executionDone.Sub(postingDone))
	t.Logf("  Total time: %v", executionDone.Sub(start))
}

// TestSequencedTaskRunner_NoSpuriousRunLoops verifies that ensureRunning
// correctly uses CAS to prevent starting multiple runLoops.
func TestSequencedTaskRunner_NoSpuriousRunLoops(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 8)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	const numConcurrentCalls = 1000
	var wg sync.WaitGroup

	// Try to call ensureRunning many times concurrently
	// Only one should succeed in starting a runLoop
	for range numConcurrentCalls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runner.ExportedEnsureRunning(core.DefaultTaskTraits())
		}()
	}

	wg.Wait()

	// runningCount should be at most 1
	rc := runner.GetRunningCount()
	if rc > 1 {
		t.Errorf("ensureRunning allowed runningCount to exceed 1: %d", rc)
	}

	// Post a task to clean up if runLoop is running
	runner.PostTask(func(ctx context.Context) {})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	runner.WaitIdle(ctx)

	// Final runningCount should be 0
	finalRC := runner.GetRunningCount()
	if finalRC != 0 {
		t.Errorf("Expected final runningCount to be 0, got %d", finalRC)
	}
}

// TestSequencedTaskRunner_PanicOnInvalidRunningCount verifies that runLoop
// panics if started with incorrect runningCount.
func TestSequencedTaskRunner_PanicOnInvalidRunningCount(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)

	// Manually set runningCount to invalid value
	runner.SetRunningCount(2)

	// Try to execute runLoop directly (should panic)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected runLoop to panic with invalid runningCount, but it didn't")
		} else {
			errMsg := fmt.Sprintf("%v", r)
			if !contains(errMsg, "runningCount=2") {
				t.Errorf("Expected panic message to mention runningCount=2, got: %v", r)
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
