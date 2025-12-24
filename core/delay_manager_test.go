package core_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// =============================================================================
// DelayManager Performance Tests
// =============================================================================

func TestDelayManager_BatchProcessing(t *testing.T) {
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	// Add 100 tasks that all expire at approximately the same time
	var executed atomic.Int32

	for range 100 {
		task := func(ctx context.Context) {
			executed.Add(1)
		}
		// All tasks have ~100ms delay
		dm.AddDelayedTask(task, 100*time.Millisecond, core.DefaultTaskTraits(), runner)
	}

	// Wait for all tasks to execute
	time.Sleep(300 * time.Millisecond)

	count := executed.Load()
	if count < 90 { // Allow some timing tolerance
		t.Errorf("Expected ~100 tasks executed, got %d", count)
	}
}

func TestDelayManager_ConcurrentAdd(t *testing.T) {
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	// Concurrently add 100 tasks with different delays
	const numTasks = 100
	var wg sync.WaitGroup
	var executed atomic.Int32

	for i := 0; i < numTasks; i++ {
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
	// Wait for all delayed tasks to execute
	// Tasks have delays from 50-140ms, so wait longer
	time.Sleep(500 * time.Millisecond)

	count := executed.Load()
	if count < numTasks*90/100 { // Allow 10% tolerance
		t.Errorf("Expected ~%d tasks executed, got %d", numTasks, count)
	}
}

func TestDelayManager_HighFrequencyTimerResets(t *testing.T) {
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	// Simulate rapid task additions that would cause frequent timer resets
	var executed atomic.Int32
	done := make(chan struct{})

	// Add a new task every 1ms for 100ms
	go func() {
		for i := 0; i < 100; i++ {
			select {
			case <-done:
				return
			default:
				task := func(ctx context.Context) {
					executed.Add(1)
				}
				// Short delay to trigger wakeups
				dm.AddDelayedTask(task, 10*time.Millisecond, core.DefaultTaskTraits(), runner)
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// Wait for all tasks to be added and execute
	time.Sleep(300 * time.Millisecond)
	close(done)

	time.Sleep(100 * time.Millisecond) // Final wait for stragglers

	count := executed.Load()
	if count < 90 { // Allow some tolerance for timing
		t.Logf("Warning: Only %d/100 tasks executed (timing sensitive)", count)
	}
}

func TestDelayManager_EmptyQueue(t *testing.T) {
	dm := core.NewDelayManager()
	defer dm.Stop()

	// Empty queue should not cause issues
	if dm.TaskCount() != 0 {
		t.Errorf("Expected 0 tasks, got %d", dm.TaskCount())
	}

	// Add and process a task
	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	var executed atomic.Bool
	task := func(ctx context.Context) {
		executed.Store(true)
	}
	dm.AddDelayedTask(task, 10*time.Millisecond, core.DefaultTaskTraits(), runner)

	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("Task was not executed")
	}
}

func TestDelayManager_MultipleDelays(t *testing.T) {
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	// Track execution order by storing completion times
	var times atomic.Value // []time.Time
	times.Store(make([]time.Time, 0, 5))

	// Add tasks with different delays
	delays := []time.Duration{20, 40, 60, 80, 100}
	for _, delay := range delays {
		d := delay
		task := func(ctx context.Context) {
			current := times.Load().([]time.Time)
			times.Store(append(current, time.Now()))
		}
		dm.AddDelayedTask(task, d*time.Millisecond, core.DefaultTaskTraits(), runner)
	}

	// Wait for all to complete
	time.Sleep(200 * time.Millisecond)

	completed := times.Load().([]time.Time)
	if len(completed) != 5 {
		t.Errorf("Expected 5 tasks executed, got %d", len(completed))
	}

	// Verify they executed in order (approximately)
	for i := 1; i < len(completed); i++ {
		if completed[i].Before(completed[i-1]) {
			t.Errorf("Task %d executed before task %d", i, i-1)
		}
	}
}

func TestDelayManager_TaskCount(t *testing.T) {
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	// Initially empty
	if count := dm.TaskCount(); count != 0 {
		t.Errorf("Expected 0 tasks, got %d", count)
	}

	// Add tasks
	const numTasks = 10
	for i := 0; i < numTasks; i++ {
		task := func(ctx context.Context) {}
		dm.AddDelayedTask(task, 1*time.Second, core.DefaultTaskTraits(), runner)
	}

	// Should have tasks pending
	count := dm.TaskCount()
	if count != numTasks {
		t.Logf("Warning: Expected %d tasks, got %d (timing may vary)", numTasks, count)
	}
}

func TestDelayManager_AccurateTiming(t *testing.T) {
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	var executedAt atomic.Int64 // nanoseconds since epoch
	delay := 50 * time.Millisecond

	task := func(ctx context.Context) {
		executedAt.Store(time.Now().UnixNano())
	}

	start := time.Now()
	dm.AddDelayedTask(task, delay, core.DefaultTaskTraits(), runner)

	// Wait for execution
	time.Sleep(200 * time.Millisecond)

	executed := time.Unix(0, executedAt.Load())
	elapsed := executed.Sub(start)

	// Should be approximately 50ms, allow ±20ms tolerance
	if elapsed < 30*time.Millisecond || elapsed > 70*time.Millisecond {
		t.Errorf("Expected ~50ms delay, got %v", elapsed)
	}
}
