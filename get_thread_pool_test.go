package taskrunner_test

import (
	"context"
	"testing"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// TestGetThreadPool demonstrates using GetThreadPool() to create multiple runners
// on the same thread pool.
func TestGetThreadPool(t *testing.T) {
	// Create initial thread pool and runner
	pool := taskrunner.NewGoroutineThreadPool("shared-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner1 := core.NewSequencedTaskRunner(pool)
	runner1.SetName("Runner1")

	// Verify we can get the thread pool back
	retrievedPool := runner1.GetThreadPool()
	if retrievedPool == nil {
		t.Fatal("GetThreadPool() returned nil for SequencedTaskRunner")
	}

	// Verify it's the same pool
	if retrievedPool != pool {
		t.Error("GetThreadPool() returned different pool instance")
	}

	// Use the retrieved pool to create another runner
	runner2 := core.NewSequencedTaskRunner(retrievedPool)
	runner2.SetName("Runner2")

	// Both runners should share the same pool
	if runner2.GetThreadPool() != pool {
		t.Error("Runner2 doesn't share the same pool")
	}

	// Test that both runners can execute tasks on the same pool
	done := make(chan struct{})
	var count int

	runner1.PostTask(func(ctx context.Context) {
		count++
	})

	runner2.PostTask(func(ctx context.Context) {
		count++
		if count == 2 {
			close(done)
		}
	})

	<-done

	if count != 2 {
		t.Errorf("Expected 2 tasks executed, got %d", count)
	}

	t.Logf("✓ Successfully created multiple runners on the same pool")
	t.Logf("  Pool: %s", pool.ID())
	t.Logf("  Runner1: %s", runner1.Name())
	t.Logf("  Runner2: %s", runner2.Name())
}

// TestGetThreadPool_SingleThread demonstrates that SingleThreadTaskRunner returns nil
func TestGetThreadPool_SingleThread(t *testing.T) {
	runner := core.NewSingleThreadTaskRunner()
	defer runner.Shutdown()

	pool := runner.GetThreadPool()
	if pool != nil {
		t.Error("SingleThreadTaskRunner should return nil for GetThreadPool()")
	}

	t.Logf("✓ SingleThreadTaskRunner correctly returns nil for GetThreadPool()")
}

// TestGetThreadPool_GlobalPool demonstrates using GetThreadPool with global pool
func TestGetThreadPool_GlobalPool(t *testing.T) {
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	// Create first runner using global pool
	runner1 := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	// Get the pool and create second runner
	pool := runner1.GetThreadPool()
	if pool == nil {
		t.Fatal("GetThreadPool() returned nil")
	}

	runner2 := core.NewSequencedTaskRunner(pool)

	// Both should share the global pool
	globalPool := taskrunner.GetGlobalThreadPool()
	if runner1.GetThreadPool() != globalPool {
		t.Error("Runner1 doesn't use global pool")
	}
	if runner2.GetThreadPool() != globalPool {
		t.Error("Runner2 doesn't use global pool")
	}

	t.Logf("✓ Multiple runners successfully sharing global pool: %s", globalPool.ID())
}
