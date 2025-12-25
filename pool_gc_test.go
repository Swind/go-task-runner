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

// TestThreadPoolTaskRunner_GC_BasicCleanup tests ThreadPool and TaskRunner GC
// Given: a ThreadPool with a TaskRunner that has executed tasks
// When: both are shutdown and references are dropped
// Then: both ThreadPool and TaskRunner are garbage collected
func TestThreadPoolTaskRunner_GC_BasicCleanup(t *testing.T) {
	// Arrange - Create ThreadPool and TaskRunner with finalizers
	var poolFinalized atomic.Bool
	var runnerFinalized atomic.Bool

	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())

	runner := core.NewSequencedTaskRunner(pool)

	runtime.SetFinalizer(pool, func(p *taskrunner.GoroutineThreadPool) {
		poolFinalized.Store(true)
	})
	runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
		runnerFinalized.Store(true)
	})

	// Act - Execute tasks and shutdown
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

	<-tasksDone

	runner.Shutdown()
	pool.Stop()

	runner = nil
	pool = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Assert - Verify both objects were collected
	if !poolFinalized.Load() {
		t.Error("ThreadPool GC'd: got = false, want = true")
	}
	if !runnerFinalized.Load() {
		t.Error("TaskRunner GC'd: got = false, want = true")
	}

	t.Logf("Both ThreadPool and TaskRunner were successfully garbage collected")
}

// TestThreadPoolTaskRunner_GC_DelayedTaskReference tests delayed task doesn't prevent GC
// Given: a TaskRunner with a pending delayed task (1 hour delay)
// When: runner is shutdown and pool is stopped
// Then: TaskRunner is garbage collected despite pending delayed task
func TestThreadPoolTaskRunner_GC_DelayedTaskReference(t *testing.T) {
	// Arrange - Create pool, runner, and delayed task
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

	var delayedTaskExecuted atomic.Bool
	runner.PostDelayedTask(func(ctx context.Context) {
		delayedTaskExecuted.Store(true)
	}, 1*time.Hour)

	time.Sleep(50 * time.Millisecond)

	// Act - Shutdown runner and pool
	runner.Shutdown()
	pool.Stop()

	runner = nil
	pool = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Assert - Verify delayed task didn't execute
	if delayedTaskExecuted.Load() {
		t.Error("delayed task executed: got = true, want = false (cancelled)")
	}

	// Assert - Verify both objects collected
	if !runnerFinalized.Load() {
		t.Error("TaskRunner GC'd: got = false, want = true (possible leak in DelayManager.pq)")
	}
	if !poolFinalized.Load() {
		t.Error("ThreadPool GC'd: got = false, want = true")
	}

	t.Logf("TaskRunner with pending delayed task was successfully garbage collected")
}

// TestThreadPoolTaskRunner_GC_QueuedRunLoopTasks tests queued runLoop tasks don't prevent GC
// Given: a TaskRunner with 100 tasks queued in scheduler (pool not started)
// When: runner is shutdown and pool is stopped
// Then: TaskRunner is garbage collected despite queued runLoop tasks
func TestThreadPoolTaskRunner_GC_QueuedRunLoopTasks(t *testing.T) {
	// Arrange - Create pool (not started) and runner with queued tasks
	var runnerFinalized atomic.Bool

	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	// Do NOT call pool.Start() - tasks will queue up

	runner := core.NewSequencedTaskRunner(pool)

	runtime.SetFinalizer(runner, func(r *core.SequencedTaskRunner) {
		runnerFinalized.Store(true)
	})

	// Post many tasks - runLoop will queue in scheduler
	for i := 0; i < 100; i++ {
		runner.PostTask(func(ctx context.Context) {
			// This won't execute
		})
	}

	time.Sleep(50 * time.Millisecond)

	queuedCount := pool.QueuedTaskCount()
	t.Logf("Queued tasks in scheduler: %d", queuedCount)

	// Act - Shutdown runner and stop pool
	runner.Shutdown()
	pool.Stop()

	runner = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Assert - Verify TaskRunner was collected
	if !runnerFinalized.Load() {
		t.Error("TaskRunner GC'd: got = false, want = true (possible leak: runLoop tasks in scheduler.queue)")
	}

	pool = nil
	t.Logf("TaskRunner with queued runLoop tasks was successfully garbage collected")
}

// TestThreadPoolTaskRunner_GC_MultipleRunners tests selective runner GC
// Given: 3 TaskRunners sharing the same ThreadPool
// When: 2 runners are shutdown but 1 remains active
// Then: the 2 shutdown runners are GC'd while the active runner remains
func TestThreadPoolTaskRunner_GC_MultipleRunners(t *testing.T) {
	// Arrange - Create pool and 3 runners with finalizers
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

	// Act - Execute tasks on all runners
	for _, runner := range []*core.SequencedTaskRunner{runnerA, runnerB, runnerC} {
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(1 * time.Millisecond)
		})
	}

	time.Sleep(50 * time.Millisecond)

	// Shutdown A and B
	runnerA.Shutdown()
	runnerB.Shutdown()

	runnerA = nil
	runnerB = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Assert - Verify A and B were collected
	if !runnerA_Finalized.Load() {
		t.Error("RunnerA GC'd: got = false, want = true")
	}
	if !runnerB_Finalized.Load() {
		t.Error("RunnerB GC'd: got = false, want = true")
	}

	// Assert - Verify C was NOT collected (still in use)
	if runnerC_Finalized.Load() {
		t.Error("RunnerC GC'd: got = true, want = false (still in use)")
	}

	// Act - Shutdown C and pool
	runnerC.Shutdown()
	pool.Stop()
	runnerC = nil
	pool = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Assert - Verify C was collected
	if !runnerC_Finalized.Load() {
		t.Error("RunnerC after shutdown GC'd: got = false, want = true")
	}

	t.Logf("Multiple runners sharing ThreadPool: partial shutdown allows GC")
}

// TestThreadPoolTaskRunner_GC_GlobalThreadPool tests global pool GC
// Given: the global ThreadPool with 2 TaskRunners
// When: runners and global pool are shutdown
// Then: all objects are garbage collected
func TestThreadPoolTaskRunner_GC_GlobalThreadPool(t *testing.T) {
	// Arrange - Initialize global pool and create runners
	var runner1_Finalized atomic.Bool
	var runner2_Finalized atomic.Bool
	var poolFinalized atomic.Bool

	taskrunner.InitGlobalThreadPool(4)

	pool := taskrunner.GetGlobalThreadPool()
	runtime.SetFinalizer(pool, func(p *taskrunner.GoroutineThreadPool) {
		poolFinalized.Store(true)
	})

	runner1 := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
	runner2 := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	runtime.SetFinalizer(runner1, func(r *taskrunner.SequencedTaskRunner) {
		runner1_Finalized.Store(true)
	})
	runtime.SetFinalizer(runner2, func(r *taskrunner.SequencedTaskRunner) {
		runner2_Finalized.Store(true)
	})

	// Act - Execute tasks
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

	// Shutdown all
	runner1.Shutdown()
	runner2.Shutdown()
	taskrunner.ShutdownGlobalThreadPool()

	runner1 = nil
	runner2 = nil
	pool = nil

	// Force GC
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Assert - Verify all objects collected
	if !runner1_Finalized.Load() {
		t.Error("Runner1 GC'd: got = false, want = true")
	}
	if !runner2_Finalized.Load() {
		t.Error("Runner2 GC'd: got = false, want = true")
	}
	if !poolFinalized.Load() {
		t.Error("Global ThreadPool GC'd: got = false, want = true")
	}

	t.Logf("Global ThreadPool and all TaskRunners were successfully garbage collected")
}
