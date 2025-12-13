package taskrunner_test

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// TestThreadPoolTaskRunner_GC_BasicCleanup tests that both ThreadPool and TaskRunner
// can be garbage collected after proper shutdown.
func TestThreadPoolTaskRunner_GC_BasicCleanup(t *testing.T) {
	var poolFinalized atomic.Bool
	var runnerFinalized atomic.Bool

	// Create ThreadPool and TaskRunner
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())

	runner := core.NewSequencedTaskRunner(pool)

	// Set finalizers to detect GC
	runtime.SetFinalizer(pool, func(p *taskrunner.GoroutineThreadPool) {
		poolFinalized.Store(true)
	})
	runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
		runnerFinalized.Store(true)
	})

	// Execute some tasks
	tasksDone := make(chan struct{})
	var executedCount int32
	for i := 0; i < 10; i++ {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(1 * time.Millisecond)
			if atomic.AddInt32(&executedCount, 1) == 10 {
				close(tasksDone)
			}
		})
	}

	// Wait for all tasks to complete
	<-tasksDone

	// Shutdown in correct order
	runner.Shutdown()
	pool.Stop()

	// Clear references
	runner = nil
	pool = nil

	// Force GC multiple times
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Verify both objects were collected
	if !poolFinalized.Load() {
		t.Error("ThreadPool was not garbage collected")
	}
	if !runnerFinalized.Load() {
		t.Error("TaskRunner was not garbage collected")
	}

	t.Logf("✓ Both ThreadPool and TaskRunner were successfully garbage collected")
}

// TestThreadPoolTaskRunner_GC_DelayedTaskReference tests that DelayedTask holding
// TaskRunner reference doesn't prevent GC after shutdown.
func TestThreadPoolTaskRunner_GC_DelayedTaskReference(t *testing.T) {
	var runnerFinalized atomic.Bool
	var poolFinalized atomic.Bool

	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())

	runner := core.NewSequencedTaskRunner(pool)

	runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
		runnerFinalized.Store(true)
	})
	runtime.SetFinalizer(pool, func(p *taskrunner.GoroutineThreadPool) {
		poolFinalized.Store(true)
	})

	// Post a delayed task with very long delay (1 hour)
	// This task will be stored in DelayManager.pq with Target = runner
	var delayedTaskExecuted atomic.Bool
	runner.PostDelayedTask(func(ctx context.Context) {
		delayedTaskExecuted.Store(true)
	}, 1*time.Hour)

	// Give DelayManager time to register the task
	time.Sleep(50 * time.Millisecond)

	// Shutdown runner (should clear its queue)
	// Then stop pool (should stop DelayManager and clear pq)
	runner.Shutdown()
	pool.Stop()

	// Clear references
	runner = nil
	pool = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Verify delayed task was NOT executed (it was cancelled)
	if delayedTaskExecuted.Load() {
		t.Error("Delayed task should not have executed after shutdown")
	}

	// Verify both objects were collected
	if !runnerFinalized.Load() {
		t.Error("TaskRunner was not garbage collected (possible leak in DelayManager.pq)")
	}
	if !poolFinalized.Load() {
		t.Error("ThreadPool was not garbage collected")
	}

	t.Logf("✓ TaskRunner with pending delayed task was successfully garbage collected")
}

// TestThreadPoolTaskRunner_GC_QueuedRunLoopTasks tests that runLoop tasks queued
// in TaskScheduler don't prevent TaskRunner GC after shutdown.
func TestThreadPoolTaskRunner_GC_QueuedRunLoopTasks(t *testing.T) {
	var runnerFinalized atomic.Bool

	// Create pool but don't start it (workers won't pull tasks)
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	// Do NOT call pool.Start() - tasks will queue up

	runner := core.NewSequencedTaskRunner(pool)

	runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
		runnerFinalized.Store(true)
	})

	// Post many tasks - since pool isn't started, runLoop will queue up in scheduler
	for i := 0; i < 100; i++ {
		runner.PostTask(func(ctx context.Context) {
			// This won't execute
		})
	}

	// Give some time for tasks to be queued
	time.Sleep(50 * time.Millisecond)

	// Verify tasks are queued
	queuedCount := pool.QueuedTaskCount()
	t.Logf("Queued tasks in scheduler: %d", queuedCount)

	// Shutdown runner (should clear its internal queue)
	// But what about the runLoop tasks already posted to scheduler.queue?
	runner.Shutdown()

	// Stop pool (should stop scheduler)
	pool.Stop()

	// Clear reference
	runner = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Verify TaskRunner was collected
	if !runnerFinalized.Load() {
		t.Error("TaskRunner was not garbage collected (possible leak: runLoop tasks in scheduler.queue)")
	}

	pool = nil
	t.Logf("✓ TaskRunner with queued runLoop tasks was successfully garbage collected")
}

// TestThreadPoolTaskRunner_GC_MultipleRunners tests that when multiple TaskRunners
// share the same ThreadPool, shutting down some runners allows them to be GC'd
// while others remain active.
func TestThreadPoolTaskRunner_GC_MultipleRunners(t *testing.T) {
	var runnerA_Finalized atomic.Bool
	var runnerB_Finalized atomic.Bool
	var runnerC_Finalized atomic.Bool

	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())

	runnerA := core.NewSequencedTaskRunner(pool)
	runnerB := core.NewSequencedTaskRunner(pool)
	runnerC := core.NewSequencedTaskRunner(pool)

	runtime.SetFinalizer(runnerA, func(r *core.SequencedTaskRunner) {
		runnerA_Finalized.Store(true)
	})
	runtime.SetFinalizer(runnerB, func(r *core.SequencedTaskRunner) {
		runnerB_Finalized.Store(true)
	})
	runtime.SetFinalizer(runnerC, func(r *core.SequencedTaskRunner) {
		runnerC_Finalized.Store(true)
	})

	// All runners execute some tasks
	for _, runner := range []*core.SequencedTaskRunner{runnerA, runnerB, runnerC} {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(1 * time.Millisecond)
		})
	}

	// Wait for tasks to complete
	time.Sleep(50 * time.Millisecond)

	// Shutdown A and B
	runnerA.Shutdown()
	runnerB.Shutdown()

	// Clear references to A and B
	runnerA = nil
	runnerB = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Verify A and B were collected
	if !runnerA_Finalized.Load() {
		t.Error("RunnerA was not garbage collected")
	}
	if !runnerB_Finalized.Load() {
		t.Error("RunnerB was not garbage collected")
	}

	// Verify C was NOT collected (still in use)
	if runnerC_Finalized.Load() {
		t.Error("RunnerC should not be garbage collected yet (still in use)")
	}

	// Now shutdown C and the pool
	runnerC.Shutdown()
	pool.Stop()
	runnerC = nil
	pool = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Verify C was collected
	if !runnerC_Finalized.Load() {
		t.Error("RunnerC was not garbage collected after shutdown")
	}

	t.Logf("✓ Multiple runners sharing ThreadPool: partial shutdown allows GC")
}

// TestThreadPoolTaskRunner_GC_GlobalThreadPool tests that the global singleton
// ThreadPool and its TaskRunners can be properly garbage collected.
func TestThreadPoolTaskRunner_GC_GlobalThreadPool(t *testing.T) {
	var runner1_Finalized atomic.Bool
	var runner2_Finalized atomic.Bool
	var poolFinalized atomic.Bool

	// Initialize global thread pool
	taskrunner.InitGlobalThreadPool(4)

	// Get reference to set finalizer (before it's cleared by ShutdownGlobalThreadPool)
	pool := taskrunner.GetGlobalThreadPool()
	runtime.SetFinalizer(pool, func(p *taskrunner.GoroutineThreadPool) {
		poolFinalized.Store(true)
	})

	// Create runners using global pool
	runner1 := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
	runner2 := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	runtime.SetFinalizer(runner1, func(r *taskrunner.SequencedTaskRunner) {
		runner1_Finalized.Store(true)
	})
	runtime.SetFinalizer(runner2, func(r *taskrunner.SequencedTaskRunner) {
		runner2_Finalized.Store(true)
	})

	// Execute some tasks
	var executed int32
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		runner := runner1
		if i%2 == 0 {
			runner = runner2
		}
		runner.PostTask(func(ctx context.Context) {
			if atomic.AddInt32(&executed, 1) == 10 {
				close(done)
			}
		})
	}

	<-done

	// Shutdown all runners
	runner1.Shutdown()
	runner2.Shutdown()

	// Shutdown global thread pool (sets globalThreadPool = nil)
	taskrunner.ShutdownGlobalThreadPool()

	// Clear local references
	runner1 = nil
	runner2 = nil
	pool = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Verify all objects were collected
	if !runner1_Finalized.Load() {
		t.Error("Runner1 was not garbage collected")
	}
	if !runner2_Finalized.Load() {
		t.Error("Runner2 was not garbage collected")
	}
	if !poolFinalized.Load() {
		t.Error("Global ThreadPool was not garbage collected")
	}

	t.Logf("✓ Global ThreadPool and all TaskRunners were successfully garbage collected")
}
