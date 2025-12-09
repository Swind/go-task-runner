package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRepeatingTask_BasicExecution(t *testing.T) {
	// Create a simple in-memory thread pool for testing
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	// Counter to track executions
	var counter atomic.Int32

	// Post a repeating task
	handle := runner.PostRepeatingTask(func(ctx context.Context) {
		counter.Add(1)
	}, 50*time.Millisecond)

	// Let it run for ~250ms (should execute ~5 times)
	time.Sleep(250 * time.Millisecond)

	// Stop the task
	handle.Stop()

	// Wait a bit to ensure no more executions
	finalCount := counter.Load()
	time.Sleep(100 * time.Millisecond)
	afterStopCount := counter.Load()

	// Verify it executed multiple times
	if finalCount < 3 {
		t.Errorf("Expected at least 3 executions, got %d", finalCount)
	}

	// Verify no executions after stop
	if afterStopCount != finalCount {
		t.Errorf("Task continued executing after Stop(): before=%d, after=%d", finalCount, afterStopCount)
	}

	// Verify IsStopped
	if !handle.IsStopped() {
		t.Error("Expected IsStopped() to return true after Stop()")
	}
}

func TestRepeatingTask_WithInitialDelay(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter atomic.Int32
	startTime := time.Now()
	var firstExecutionTime atomic.Value

	handle := runner.PostRepeatingTaskWithInitialDelay(
		func(ctx context.Context) {
			if counter.Add(1) == 1 {
				firstExecutionTime.Store(time.Now())
			}
		},
		100*time.Millisecond, // initialDelay
		50*time.Millisecond,  // interval
		DefaultTaskTraits(),
	)
	defer handle.Stop()

	// Wait for first execution
	time.Sleep(150 * time.Millisecond)

	firstExec := firstExecutionTime.Load().(time.Time)
	elapsed := firstExec.Sub(startTime)

	// Verify initial delay was respected (with some tolerance)
	if elapsed < 90*time.Millisecond || elapsed > 150*time.Millisecond {
		t.Errorf("Initial delay not respected: elapsed=%v", elapsed)
	}

	// Wait for more executions
	time.Sleep(200 * time.Millisecond)

	count := counter.Load()
	if count < 3 {
		t.Errorf("Expected at least 3 executions after initial delay, got %d", count)
	}
}

func TestRepeatingTask_WithTraits(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter atomic.Int32

	// Post with high priority traits
	handle := runner.PostRepeatingTaskWithTraits(
		func(ctx context.Context) {
			counter.Add(1)
		},
		50*time.Millisecond,
		TraitsUserBlocking(),
	)

	time.Sleep(200 * time.Millisecond)
	handle.Stop()

	count := counter.Load()
	if count < 2 {
		t.Errorf("Expected at least 2 executions, got %d", count)
	}
}

func TestRepeatingTask_StopBeforeFirstExecution(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var executed atomic.Bool

	handle := runner.PostRepeatingTaskWithInitialDelay(
		func(ctx context.Context) {
			executed.Store(true)
		},
		200*time.Millisecond, // long initial delay
		50*time.Millisecond,
		DefaultTaskTraits(),
	)

	// Stop immediately
	handle.Stop()

	// Wait to see if it executes (it shouldn't)
	time.Sleep(250 * time.Millisecond)

	if executed.Load() {
		t.Error("Task executed despite being stopped before first execution")
	}
}

func TestRepeatingTask_ConcurrentStop(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	handle := runner.PostRepeatingTask(func(ctx context.Context) {
		time.Sleep(10 * time.Millisecond)
	}, 20*time.Millisecond)

	// Call Stop() from multiple goroutines concurrently
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			handle.Stop()
			done <- struct{}{}
		}()
	}

	// Wait for all stops to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should be stopped
	if !handle.IsStopped() {
		t.Error("Expected IsStopped() to be true after concurrent stops")
	}
}

func TestRepeatingTask_ContextPropagation(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var gotRunner atomic.Bool
	var handle RepeatingTaskHandle

	handle = runner.PostRepeatingTask(func(ctx context.Context) {
		// Verify we can get the runner from context
		r := GetCurrentTaskRunner(ctx)
		if r != nil {
			gotRunner.Store(true)
		}
		handle.Stop() // Stop after first execution
	}, 50*time.Millisecond)

	time.Sleep(100 * time.Millisecond)

	if !gotRunner.Load() {
		t.Error("Failed to get TaskRunner from context")
	}
}

// =============================================================================
// Test helper: simple thread pool for testing
// =============================================================================

type testThreadPool struct {
	scheduler *TaskScheduler
	ctx       context.Context
	cancel    context.CancelFunc
}

func newTestThreadPool() *testThreadPool {
	return &testThreadPool{
		scheduler: NewFIFOTaskScheduler(2),
	}
}

func (tp *testThreadPool) start() {
	tp.ctx, tp.cancel = context.WithCancel(context.Background())
	// Start 2 workers
	for i := 0; i < 2; i++ {
		go tp.worker()
	}
}

func (tp *testThreadPool) worker() {
	for {
		task, ok := tp.scheduler.GetWork(tp.ctx.Done())
		if !ok {
			return
		}
		tp.scheduler.OnTaskStart()
		func() {
			defer tp.scheduler.OnTaskEnd()
			task(tp.ctx)
		}()
	}
}

func (tp *testThreadPool) stop() {
	tp.scheduler.Shutdown()
	if tp.cancel != nil {
		tp.cancel()
	}
}

func (tp *testThreadPool) PostInternal(task Task, traits TaskTraits) {
	tp.scheduler.PostInternal(task, traits)
}

func (tp *testThreadPool) PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner) {
	tp.scheduler.PostDelayedInternal(task, delay, traits, target)
}

func (tp *testThreadPool) Start(ctx context.Context)  {}
func (tp *testThreadPool) Stop()                      {}
func (tp *testThreadPool) ID() string                 { return "test-pool" }
func (tp *testThreadPool) IsRunning() bool            { return true }
func (tp *testThreadPool) WorkerCount() int           { return 2 }
func (tp *testThreadPool) QueuedTaskCount() int       { return tp.scheduler.QueuedTaskCount() }
func (tp *testThreadPool) ActiveTaskCount() int       { return tp.scheduler.ActiveTaskCount() }
func (tp *testThreadPool) DelayedTaskCount() int      { return tp.scheduler.DelayedTaskCount() }
