package taskrunner

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Swind/go-task-runner/core"
)

// Ensure GoroutineThreadPool fully implements ThreadPool interface
var _ core.ThreadPool = (*GoroutineThreadPool)(nil)

func TestGoroutineThreadPool_Lifecycle(t *testing.T) {
	pool := NewGoroutineThreadPool("test-pool", 2)

	if pool.ID() != "test-pool" {
		t.Errorf("expected ID 'test-pool', got %s", pool.ID())
	}

	if pool.IsRunning() {
		t.Error("pool should not be running initially")
	}

	ctx := context.Background()
	pool.Start(ctx)

	if !pool.IsRunning() {
		t.Error("pool should be running after Start()")
	}

	if pool.WorkerCount() != 2 {
		t.Errorf("expected 2 workers, got %d", pool.WorkerCount())
	}

	pool.Stop()

	if pool.IsRunning() {
		t.Error("pool should not be running after Stop()")
	}
}

func TestGoroutineThreadPool_TaskExecution(t *testing.T) {
	pool := NewGoroutineThreadPool("exec-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	var counter int32
	var wg sync.WaitGroup
	taskCount := 10

	wg.Add(taskCount)

	task := func(ctx context.Context) {
		defer wg.Done()
		atomic.AddInt32(&counter, 1)
		time.Sleep(10 * time.Millisecond) // Simulate work
	}

	for i := 0; i < taskCount; i++ {
		// We use PostInternal to simulate task submission via the interface
		pool.PostInternal(task, core.TaskTraits{})
	}

	wg.Wait()

	if val := atomic.LoadInt32(&counter); val != int32(taskCount) {
		t.Errorf("expected %d executed tasks, got %d", taskCount, val)
	}
}

func TestGoroutineThreadPool_Metrics(t *testing.T) {
	pool := NewGoroutineThreadPool("metrics-pool", 1) // Single worker to force queuing
	pool.Start(context.Background())
	defer pool.Stop()

	// 1. Block the worker
	blockCh := make(chan struct{})
	bgDone := make(chan struct{})

	blockingTask := func(ctx context.Context) {
		<-blockCh
		bgDone <- struct{}{}
	}

	pool.PostInternal(blockingTask, core.TaskTraits{})

	// Wait a bit for worker to pick it up
	time.Sleep(50 * time.Millisecond)

	if active := pool.ActiveTaskCount(); active != 1 {
		t.Errorf("expected 1 active task, got %d", active)
	}

	// 2. Queue more tasks
	pool.PostInternal(func(ctx context.Context) {}, core.TaskTraits{})
	pool.PostInternal(func(ctx context.Context) {}, core.TaskTraits{})

	// Wait for queue update
	time.Sleep(10 * time.Millisecond)

	if queued := pool.QueuedTaskCount(); queued != 2 {
		t.Errorf("expected 2 queued tasks, got %d", queued)
	}

	// 3. Unblock
	close(blockCh)
	<-bgDone

	// Wait for drain
	time.Sleep(100 * time.Millisecond)

	if active := pool.ActiveTaskCount(); active != 0 {
		t.Errorf("expected 0 active tasks, got %d", active)
	}
	if queued := pool.QueuedTaskCount(); queued != 0 {
		t.Errorf("expected 0 queued tasks, got %d", queued)
	}
}

// =============================================================================
// Graceful Shutdown Tests
// =============================================================================

func TestGoroutineThreadPool_StopGraceful_EmptyQueue(t *testing.T) {
	pool := NewGoroutineThreadPool("graceful-pool", 2)
	pool.Start(context.Background())

	// No tasks queued, should stop immediately
	err := pool.StopGraceful(1 * time.Second)
	if err != nil {
		t.Fatalf("StopGraceful failed: %v", err)
	}

	if pool.IsRunning() {
		t.Error("pool should not be running after StopGraceful")
	}
}

func TestGoroutineThreadPool_StopGraceful_WithQueuedTasks(t *testing.T) {
	pool := NewGoroutineThreadPool("graceful-queued-pool", 2)
	pool.Start(context.Background())

	var executed int32
	var wg sync.WaitGroup
	taskCount := 5

	wg.Add(taskCount)

	// Create tasks that complete quickly
	task := func(ctx context.Context) {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&executed, 1)
	}

	// Submit all tasks
	for i := 0; i < taskCount; i++ {
		pool.PostInternal(task, core.TaskTraits{})
	}

	// Wait a bit for tasks to start
	time.Sleep(10 * time.Millisecond)

	// Start graceful shutdown in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- pool.StopGraceful(1 * time.Second)
	}()

	// Wait for all tasks to complete
	wg.Wait()

	// Wait for shutdown to complete
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("StopGraceful failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("StopGraceful timed out")
	}

	// Verify all tasks were executed
	if executed != int32(taskCount) {
		t.Errorf("expected %d executed tasks, got %d", taskCount, executed)
	}

	if pool.IsRunning() {
		t.Error("pool should not be running after StopGraceful")
	}
}

func TestGoroutineThreadPool_StopGraceful_Timeout(t *testing.T) {
	pool := NewGoroutineThreadPool("timeout-pool", 1)
	pool.Start(context.Background())

	// Create a task that blocks longer than the shutdown timeout
	// The task checks context and should exit when context is cancelled
	longRunningTask := func(ctx context.Context) {
		// Block for 500ms, but check context
		select {
		case <-time.After(500 * time.Millisecond):
			// Task completes normally
		case <-ctx.Done():
			// Context cancelled, exit early
			return
		}
	}

	pool.PostInternal(longRunningTask, core.TaskTraits{})

	// Wait for task to start
	time.Sleep(20 * time.Millisecond)

	// Shutdown with 50ms timeout - task takes 500ms so this should timeout
	start := time.Now()
	err := pool.StopGraceful(50 * time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected timeout error, got nil")
	}

	// StopGraceful should return in roughly 50-100ms (timeout + one ticker interval)
	// NOT 500ms (the task duration) because context cancellation should interrupt the task
	if elapsed > 200*time.Millisecond {
		t.Errorf("StopGraceful took too long: %v (expected ~50-100ms)", elapsed)
	}

	if pool.IsRunning() {
		t.Error("pool should not be running after timeout StopGraceful")
	}
}

func TestGoroutineThreadPool_StopImmediateVsGraceful(t *testing.T) {
	// Test immediate stop
	p1 := NewGoroutineThreadPool("immediate-pool", 1)
	p1.Start(context.Background())

	// Add some tasks
	for i := 0; i < 5; i++ {
		p1.PostInternal(func(ctx context.Context) {
			time.Sleep(100 * time.Millisecond)
		}, core.TaskTraits{})
	}

	// Immediate stop - should clear queue immediately
	p1.Stop()

	if p1.QueuedTaskCount() != 0 {
		// Queue should be cleared (but may not be 0 if tasks were picked up)
		// This depends on timing, so we just check that Stop() doesn't hang
	}

	// Test graceful stop
	p2 := NewGoroutineThreadPool("graceful-pool", 2)
	p2.Start(context.Background())

	var executed int32
	for i := 0; i < 3; i++ {
		p2.PostInternal(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&executed, 1)
		}, core.TaskTraits{})
	}

	// Wait for tasks to complete, then shutdown
	time.Sleep(100 * time.Millisecond)
	err := p2.StopGraceful(500 * time.Millisecond)
	if err != nil {
		t.Errorf("StopGraceful failed: %v", err)
	}

	// All tasks should have been executed
	if executed != 3 {
		t.Errorf("expected 3 executed tasks, got %d", executed)
	}
}
