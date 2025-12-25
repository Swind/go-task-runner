package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestRepeatingTask_BasicExecution verifies basic repeating task functionality
// Given: A repeating task every 50ms
// When: Task runs for 250ms then is stopped
// Then: Task executes multiple times and stops when handle.Stop() is called
func TestRepeatingTask_BasicExecution(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter atomic.Int32

	// Act
	handle := runner.PostRepeatingTask(func(ctx context.Context) {
		counter.Add(1)
	}, 50*time.Millisecond)

	time.Sleep(250 * time.Millisecond)
	handle.Stop()

	finalCount := counter.Load()
	time.Sleep(100 * time.Millisecond)
	afterStopCount := counter.Load()

	// Assert
	if finalCount < 3 {
		t.Errorf("finalCount = %d, want >=3", finalCount)
	}

	if afterStopCount != finalCount {
		t.Errorf("afterStopCount = %d, want %d (no more executions)", afterStopCount, finalCount)
	}

	if !handle.IsStopped() {
		t.Error("IsStopped() = false, want true")
	}
}

// TestRepeatingTask_WithInitialDelay verifies repeating task with initial delay
// Given: A repeating task with 100ms initial delay and 50ms interval
// When: Task is posted
// Then: First execution respects initial delay, then periodic execution continues
func TestRepeatingTask_WithInitialDelay(t *testing.T) {
	// Arrange
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
		100*time.Millisecond,
		50*time.Millisecond,
		DefaultTaskTraits(),
	)
	defer handle.Stop()

	// Act - Wait for first execution
	time.Sleep(150 * time.Millisecond)

	firstExec := firstExecutionTime.Load().(time.Time)
	elapsed := firstExec.Sub(startTime)

	// Assert - Initial delay respected
	if elapsed < 90*time.Millisecond || elapsed > 150*time.Millisecond {
		t.Errorf("elapsed = %v, want 90-150ms", elapsed)
	}

	// Wait for more executions
	time.Sleep(200 * time.Millisecond)

	count := counter.Load()
	if count < 3 {
		t.Errorf("count = %d, want >=3", count)
	}
}

// TestRepeatingTask_WithTraits verifies repeating task with custom traits
// Given: A repeating task with UserBlocking priority
// When: Task runs for 200ms
// Then: Task executes multiple times with correct priority
func TestRepeatingTask_WithTraits(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var counter atomic.Int32

	// Act
	handle := runner.PostRepeatingTaskWithTraits(
		func(ctx context.Context) {
			counter.Add(1)
		},
		50*time.Millisecond,
		TraitsUserBlocking(),
	)

	time.Sleep(200 * time.Millisecond)
	handle.Stop()

	// Assert
	count := counter.Load()
	if count < 2 {
		t.Errorf("count = %d, want >=2", count)
	}
}

// TestRepeatingTask_StopBeforeFirstExecution verifies stopping before first execution
// Given: A repeating task with 200ms initial delay
// When: Stop is called immediately
// Then: Task never executes
func TestRepeatingTask_StopBeforeFirstExecution(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var executed atomic.Bool

	handle := runner.PostRepeatingTaskWithInitialDelay(
		func(ctx context.Context) {
			executed.Store(true)
		},
		200*time.Millisecond,
		50*time.Millisecond,
		DefaultTaskTraits(),
	)

	// Act - Stop immediately
	handle.Stop()

	time.Sleep(250 * time.Millisecond)

	// Assert
	if executed.Load() {
		t.Error("executed = true, want false (stopped before first execution)")
	}
}

// TestRepeatingTask_ConcurrentStop verifies concurrent Stop() calls are safe
// Given: A repeating task
// When: 10 goroutines call Stop() concurrently
// Then: All calls complete, IsStopped returns true
func TestRepeatingTask_ConcurrentStop(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	handle := runner.PostRepeatingTask(func(ctx context.Context) {
		time.Sleep(10 * time.Millisecond)
	}, 20*time.Millisecond)

	// Act - Concurrent stops
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			handle.Stop()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Assert
	if !handle.IsStopped() {
		t.Error("IsStopped() = false, want true")
	}
}

// TestRepeatingTask_ContextPropagation verifies context propagation to repeating tasks
// Given: A repeating task
// When: Task executes
// Then: TaskRunner is available from context
func TestRepeatingTask_ContextPropagation(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var gotRunner atomic.Bool
	var handle RepeatingTaskHandle

	handle = runner.PostRepeatingTask(func(ctx context.Context) {
		r := GetCurrentTaskRunner(ctx)
		if r != nil {
			gotRunner.Store(true)
		}
		handle.Stop()
	}, 50*time.Millisecond)

	time.Sleep(100 * time.Millisecond)

	// Assert
	if !gotRunner.Load() {
		t.Error("gotRunner = false, want true (TaskRunner available from context)")
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
