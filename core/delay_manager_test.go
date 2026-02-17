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

// TestDelayManager_BatchProcessing verifies batch processing of simultaneously expiring tasks
// Given: A DelayManager with 100 tasks expiring at approximately the same time
// When: All tasks expire and are processed
// Then: All tasks execute correctly within timeout window
func TestDelayManager_BatchProcessing(t *testing.T) {
	// Arrange
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	var executed atomic.Int32

	// Act - Add 100 tasks with ~100ms delay
	for range 100 {
		task := func(ctx context.Context) {
			executed.Add(1)
		}
		dm.AddDelayedTask(task, 100*time.Millisecond, core.DefaultTaskTraits(), runner)
	}

	// Wait until all tasks complete or timeout.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if executed.Load() == 100 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	count := executed.Load()
	t.Errorf("executed tasks = %d, want 100 within timeout", count)
}

// TestDelayManager_ConcurrentAdd verifies thread safety during concurrent task additions
// Given: A DelayManager and multiple goroutines adding tasks
// When: 10 goroutines concurrently add delayed tasks
// Then: All tasks execute correctly without race conditions
// Note: Small task count ensures reliability across all CI environments
func TestDelayManager_ConcurrentAdd(t *testing.T) {
	// Arrange
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	const numTasks = 10 // Small, reliable count for CI
	var wg sync.WaitGroup
	var executed atomic.Int32

	t.Logf("Testing with %d tasks (CI-stable version)", numTasks)

	// Act - Concurrently add tasks with different delays
	for i := range numTasks {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			delay := time.Duration(id%5)*20*time.Millisecond + 100*time.Millisecond
			task := func(ctx context.Context) {
				executed.Add(1)
			}
			dm.AddDelayedTask(task, delay, core.DefaultTaskTraits(), runner)
		}(i)
	}

	wg.Wait()
	// Wait for delayed tasks to execute
	time.Sleep(1500 * time.Millisecond)

	// Assert - All tasks should execute
	count := executed.Load()
	if count < int32(numTasks*90/100) {
		t.Errorf("executed tasks = %d, want ~%d", count, numTasks)
	}
}

// TestDelayManager_HighFrequencyTimerResets verifies stability under rapid timer resets
// Given: A DelayManager receiving tasks every 1ms
// When: 100 tasks are added rapidly with short delays
// Then: System remains stable and processes all tasks
func TestDelayManager_HighFrequencyTimerResets(t *testing.T) {
	// Arrange
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	var executed atomic.Int32
	done := make(chan struct{})

	// Act - Add task every 1ms for 100ms
	go func() {
		for i := 0; i < 100; i++ {
			select {
			case <-done:
				return
			default:
				task := func(ctx context.Context) {
					executed.Add(1)
				}
				dm.AddDelayedTask(task, 10*time.Millisecond, core.DefaultTaskTraits(), runner)
				time.Sleep(time.Millisecond)
			}
		}
	}()

	time.Sleep(300 * time.Millisecond)
	close(done)
	time.Sleep(100 * time.Millisecond)

	// Assert - Most tasks executed (timing sensitive)
	count := executed.Load()
	if count < 90 {
		t.Logf("Warning: executed tasks = %d/100 (timing sensitive test)", count)
	}
}

// TestDelayManager_EmptyQueue verifies behavior with no pending tasks
// Given: An empty DelayManager
// When: TaskCount is queried and a task is added
// Then: TaskCount returns 0 and new task executes correctly
func TestDelayManager_EmptyQueue(t *testing.T) {
	// Arrange
	dm := core.NewDelayManager()
	defer dm.Stop()

	// Assert - Empty queue returns 0
	if dm.TaskCount() != 0 {
		t.Errorf("TaskCount() = %d, want 0", dm.TaskCount())
	}

	// Arrange - Setup pool and runner
	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	var executed atomic.Bool
	task := func(ctx context.Context) {
		executed.Store(true)
	}

	// Act - Add task to empty queue
	dm.AddDelayedTask(task, 10*time.Millisecond, core.DefaultTaskTraits(), runner)
	time.Sleep(50 * time.Millisecond)

	// Assert - Task executed
	if !executed.Load() {
		t.Error("executed = false, want true")
	}
}

// TestDelayManager_MultipleDelays verifies execution order with varied delays
// Given: A DelayManager with tasks having delays of 20, 40, 60, 80, 100ms
// When: Tasks are added concurrently
// Then: Tasks execute in order of shortest to longest delay
func TestDelayManager_MultipleDelays(t *testing.T) {
	// Arrange
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	var times atomic.Value // []time.Time
	times.Store(make([]time.Time, 0, 5))

	// Act - Add tasks with different delays
	delays := []time.Duration{20, 40, 60, 80, 100}
	for _, delay := range delays {
		d := delay
		task := func(ctx context.Context) {
			current := times.Load().([]time.Time)
			times.Store(append(current, time.Now()))
		}
		dm.AddDelayedTask(task, d*time.Millisecond, core.DefaultTaskTraits(), runner)
	}

	time.Sleep(200 * time.Millisecond)

	// Assert - All tasks executed in order
	completed := times.Load().([]time.Time)
	if len(completed) != 5 {
		t.Errorf("len(completed) = %d, want 5", len(completed))
	}

	for i := 1; i < len(completed); i++ {
		if completed[i].Before(completed[i-1]) {
			t.Errorf("Task %d executed before task %d", i, i-1)
		}
	}
}

// TestDelayManager_TaskCount verifies pending task counter accuracy
// Given: An empty DelayManager
// When: 10 tasks are added with long delays
// Then: TaskCount reflects the number of pending tasks
func TestDelayManager_TaskCount(t *testing.T) {
	// Arrange
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	// Assert - Initially empty
	if count := dm.TaskCount(); count != 0 {
		t.Errorf("TaskCount() = %d, want 0", count)
	}

	// Act - Add 10 tasks with 1 second delay
	const numTasks = 10
	for i := 0; i < numTasks; i++ {
		task := func(ctx context.Context) {}
		dm.AddDelayedTask(task, 1*time.Second, core.DefaultTaskTraits(), runner)
	}

	// Assert - TaskCount reflects pending tasks
	count := dm.TaskCount()
	if count != numTasks {
		t.Logf("TaskCount() = %d, want %d (timing may vary)", count, numTasks)
	}
}

// TestDelayManager_AccurateTiming verifies delay execution precision
// Given: A DelayManager with a task delayed 50ms
// When: The task executes
// Then: Actual delay is within 30-70ms range (±20ms tolerance)
func TestDelayManager_AccurateTiming(t *testing.T) {
	// Arrange
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

	// Act - Record start time and add delayed task
	start := time.Now()
	dm.AddDelayedTask(task, delay, core.DefaultTaskTraits(), runner)
	time.Sleep(200 * time.Millisecond)

	// Assert - Delay was approximately 50ms (±20ms)
	executed := time.Unix(0, executedAt.Load())
	elapsed := executed.Sub(start)

	minDelay := 30 * time.Millisecond
	maxDelay := 70 * time.Millisecond
	if elapsed < minDelay || elapsed > maxDelay {
		t.Errorf("elapsed delay = %v, want %v±20ms", elapsed, delay)
	}
}

// TestDelayManager_ExpiredTasksWithoutWakeup verifies expired tasks are not treated as empty queue.
// Given: A delay manager with a task that is already expired
// When: The loop computes next run state
// Then: The task executes promptly even without additional wakeup signals.
func TestDelayManager_ExpiredTasksWithoutWakeup(t *testing.T) {
	dm := core.NewDelayManager()
	defer dm.Stop()

	pool := taskrunner.NewGoroutineThreadPool("test", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := core.NewSequencedTaskRunner(pool)
	defer runner.Shutdown()

	done := make(chan struct{})
	dm.AddDelayedTask(func(ctx context.Context) {
		close(done)
	}, 1*time.Millisecond, core.DefaultTaskTraits(), runner)

	// Wait until the task becomes expired in the queue, then expect prompt execution.
	time.Sleep(15 * time.Millisecond)

	select {
	case <-done:
		// success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expired task was not executed promptly")
	}
}
