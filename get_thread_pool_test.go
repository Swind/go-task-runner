package taskrunner_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// TestGetThreadPool demonstrates using GetThreadPool() to create multiple runners
// Given: a ThreadPool with one SequencedTaskRunner
// When: GetThreadPool() is called to retrieve the pool and create another runner
// Then: both runners share the same pool and can execute tasks
func TestGetThreadPool(t *testing.T) {
	// Arrange - Create thread pool and first runner
	pool := taskrunner.NewGoroutineThreadPool("shared-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner1 := core.NewSequencedTaskRunner(pool)
	runner1.SetName("Runner1")

	// Act - Get the thread pool back from runner
	retrievedPool := runner1.GetThreadPool()

	// Assert - Verify pool is not nil
	if retrievedPool == nil {
		t.Fatal("GetThreadPool() returned nil for SequencedTaskRunner")
	}

	// Assert - Verify it's the same pool
	if retrievedPool != pool {
		t.Error("GetThreadPool() returned different pool instance")
	}

	// Act - Use retrieved pool to create another runner
	runner2 := core.NewSequencedTaskRunner(retrievedPool)
	runner2.SetName("Runner2")

	// Assert - Verify both runners share the same pool
	if runner2.GetThreadPool() != pool {
		t.Error("Runner2 doesn't share the same pool")
	}

	// Act - Execute tasks on both runners
	done := make(chan struct{}, 1)
	var count atomic.Int32
	trySignalDone := func() {
		if count.Load() == 2 {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}

	runner1.PostTask(func(ctx context.Context) {
		count.Add(1)
		trySignalDone()
	})

	runner2.PostTask(func(ctx context.Context) {
		count.Add(1)
		trySignalDone()
	})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for both tasks")
	}

	// Assert - Verify both tasks executed
	gotCount := count.Load()
	wantCount := int32(2)
	if gotCount != wantCount {
		t.Errorf("tasks executed: got = %d, want = %d", gotCount, wantCount)
	}

	t.Logf("Successfully created multiple runners on the same pool")
	t.Logf("  Pool: %s", pool.ID())
	t.Logf("  Runner1: %s", runner1.Name())
	t.Logf("  Runner2: %s", runner2.Name())
}

// TestGetThreadPool_SingleThread demonstrates SingleThreadTaskRunner returns nil
// Given: a SingleThreadTaskRunner
// When: GetThreadPool() is called
// Then: nil is returned (SingleThreadTaskRunner doesn't use a ThreadPool)
func TestGetThreadPool_SingleThread(t *testing.T) {
	// Arrange - Create SingleThreadTaskRunner
	runner := core.NewSingleThreadTaskRunner()
	defer runner.Shutdown()

	// Act - Get thread pool
	pool := runner.GetThreadPool()

	// Assert - Verify nil is returned
	if pool != nil {
		t.Error("SingleThreadTaskRunner.GetThreadPool(): got = non-nil, want = nil")
	}

	t.Logf("SingleThreadTaskRunner correctly returns nil for GetThreadPool()")
}

// TestGetThreadPool_GlobalPool demonstrates GetThreadPool with global pool
// Given: the global ThreadPool with one runner created via CreateTaskRunner
// When: GetThreadPool() is used to create another runner
// Then: both runners share the global pool
func TestGetThreadPool_GlobalPool(t *testing.T) {
	// Arrange - Initialize global thread pool
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	// Create first runner using global pool
	runner1 := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	// Act - Get the pool and create second runner
	pool := runner1.GetThreadPool()
	if pool == nil {
		t.Fatal("GetThreadPool() returned nil")
	}

	runner2 := core.NewSequencedTaskRunner(pool)

	// Assert - Verify both share the global pool
	globalPool := taskrunner.GetGlobalThreadPool()
	if runner1.GetThreadPool() != globalPool {
		t.Error("Runner1 doesn't use global pool")
	}
	if runner2.GetThreadPool() != globalPool {
		t.Error("Runner2 doesn't use global pool")
	}

	t.Logf("Multiple runners successfully sharing global pool: %s", globalPool.ID())
}

// TestGetThreadPool_CreateTaskRunner verifies CreateTaskRunner uses global pool
// Given: the global ThreadPool is initialized
// When: CreateTaskRunner is called
// Then: the returned runner's GetThreadPool() matches the global pool
func TestGetThreadPool_CreateTaskRunner(t *testing.T) {
	// Arrange - Initialize global thread pool
	taskrunner.InitGlobalThreadPool(3)
	defer taskrunner.ShutdownGlobalThreadPool()

	// Act - Create runner via CreateTaskRunner
	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	// Assert - Verify GetThreadPool matches global pool
	gotPool := runner.GetThreadPool()
	wantPool := taskrunner.GetGlobalThreadPool()
	if gotPool != wantPool {
		t.Error("CreateTaskRunner GetThreadPool: got = different pool, want = global pool")
	}
}
