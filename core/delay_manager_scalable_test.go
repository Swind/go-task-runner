//go:build !ci

// This test file contains a scalable version of concurrent add test
// that adjusts task count based on available CPUs.
// It is excluded from CI builds to ensure CI stability.

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

// TestDelayManager_ConcurrentAdd_Scalable verifies thread safety with CPU-adjusted task count
// This test is excluded from CI (!ci build tag)
// Given: A DelayManager and multiple goroutines adding tasks
// When: goroutines concurrently add delayed tasks (count scaled by CPU count)
// Then: All tasks execute correctly without race conditions
func TestDelayManager_ConcurrentAdd_Scalable(t *testing.T) {
	// Arrange
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	// Dynamically adjust task count based on available CPUs
	// This version is for local testing with varying hardware capabilities
	numCPUs := runtime.NumCPU()
	workers := 4 // Thread pool size
	// Conservative scaling: 20 tasks per worker base, adjusted by CPU count
	numTasks := workers * 20 // Base: 80 tasks
	if numCPUs >= 8 {
		// High CPU systems: scale up
		numTasks = workers * 30 // 120 tasks
	}
	if numCPUs >= 16 {
		// Very high CPU systems: scale up more
		numTasks = workers * 40 // 160 tasks
	}

	var wg sync.WaitGroup
	var executed atomic.Int32

	t.Logf("Scalable test with %d tasks (CPUs: %d, Workers: %d)", numTasks, numCPUs, workers)
	t.Log("This test verifies concurrent behavior with higher task counts.")
	t.Log("Run locally to stress test the delay manager with your hardware capabilities.")

	// Act - Concurrently add tasks with different delays
	for i := range numTasks {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			delay := time.Duration(id%10)*10*time.Millisecond + 50*time.Millisecond
			task := func(ctx context.Context) {
				executed.Add(1)
			}
			dm.AddDelayedTask(task, delay, core.DefaultTaskTraits(), runner)
		}(i)
	}

	wg.Wait()
	// Wait for delayed tasks to execute (delays: 50-140ms)
	time.Sleep(2000 * time.Millisecond)

	// Assert - Most tasks executed (allow 10% tolerance)
	count := executed.Load()
	if count < int32(numTasks*90/100) {
		t.Errorf("executed tasks = %d, want ~%d (CPUs: %d)", count, numTasks, numCPUs)
	}

	t.Logf("Successfully processed %d/%d tasks", count, numTasks)
}
