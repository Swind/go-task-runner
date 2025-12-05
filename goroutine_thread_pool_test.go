package main

import (
	"context"
	"swind/go-task-runner/domain"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Ensure GoroutineThreadPool fully implements ThreadPool interface
var _ domain.ThreadPool = (*GoroutineThreadPool)(nil)

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
		pool.PostInternal(task, domain.TaskTraits{})
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

	pool.PostInternal(blockingTask, domain.TaskTraits{})

	// Wait a bit for worker to pick it up
	time.Sleep(50 * time.Millisecond)

	if active := pool.ActiveTaskCount(); active != 1 {
		t.Errorf("expected 1 active task, got %d", active)
	}

	// 2. Queue more tasks
	pool.PostInternal(func(ctx context.Context) {}, domain.TaskTraits{})
	pool.PostInternal(func(ctx context.Context) {}, domain.TaskTraits{})

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
