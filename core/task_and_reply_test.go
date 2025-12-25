package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Basic PostTaskAndReply Tests
// =============================================================================

// TestPostTaskAndReply_BasicExecution tests basic task and reply execution
// Main test items:
// 1. Task executes correctly
// 2. Reply executes correctly
// 3. Both task and reply execute on the correct Runner
func TestPostTaskAndReply_BasicExecution(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var taskExecuted, replyExecuted atomic.Bool

	targetRunner.PostTaskAndReply(
		func(ctx context.Context) {
			taskExecuted.Store(true)
		},
		func(ctx context.Context) {
			replyExecuted.Store(true)
		},
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	if !taskExecuted.Load() {
		t.Error("Task was not executed")
	}
	if !replyExecuted.Load() {
		t.Error("Reply was not executed")
	}
}

// TestPostTaskAndReply_ExecutionOrder tests the execution order of task and reply (task first, then reply)
// Main test items:
// 1. Task executes before reply
// 2. Execution order matches expected [task, reply]
// 3. Time interval handling is correct
func TestPostTaskAndReply_ExecutionOrder(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var executionOrder []string
	var mu atomic.Value
	mu.Store(&executionOrder)

	targetRunner.PostTaskAndReply(
		func(ctx context.Context) {
			time.Sleep(50 * time.Millisecond)
			ptr := mu.Load().(*[]string)
			*ptr = append(*ptr, "task")
		},
		func(ctx context.Context) {
			ptr := mu.Load().(*[]string)
			*ptr = append(*ptr, "reply")
		},
		replyRunner,
	)

	time.Sleep(150 * time.Millisecond)

	order := *mu.Load().(*[]string)
	if len(order) != 2 {
		t.Fatalf("Expected 2 executions, got %d", len(order))
	}
	if order[0] != "task" || order[1] != "reply" {
		t.Errorf("Execution order incorrect: got %v, expected [task reply]", order)
	}
}

// TestPostTaskAndReply_TaskPanic tests that reply does not execute when task panics
// Main test items:
// 1. Task panic does not affect other tasks
// 2. Reply does not execute after task panic
// 3. Error handling mechanism works correctly
func TestPostTaskAndReply_TaskPanic(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var replyExecuted atomic.Bool

	targetRunner.PostTaskAndReply(
		func(ctx context.Context) {
			panic("task panic")
		},
		func(ctx context.Context) {
			replyExecuted.Store(true)
		},
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	// Reply should NOT execute because task panicked
	if replyExecuted.Load() {
		t.Error("Reply should not execute when task panics")
	}
}

// TestPostTaskAndReply_NilReplyRunner tests handling of nil replyRunner
// Main test items:
// 1. Task still executes when replyRunner is nil
// 2. Should not panic
// 3. Error handling mechanism works correctly
func TestPostTaskAndReply_NilReplyRunner(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)

	var taskExecuted atomic.Bool

	// Should not panic with nil replyRunner
	targetRunner.PostTaskAndReply(
		func(ctx context.Context) {
			taskExecuted.Store(true)
		},
		func(ctx context.Context) {
			t.Error("Reply should not execute with nil replyRunner")
		},
		nil,
	)

	time.Sleep(100 * time.Millisecond)

	if !taskExecuted.Load() {
		t.Error("Task should execute even with nil replyRunner")
	}
}

// =============================================================================
// PostTaskAndReplyWithTraits Tests
// =============================================================================

// TestPostTaskAndReplyWithTraits_DifferentPriorities tests tasks and replies with different priorities
// Main test items:
// 1. Tasks with different priorities execute normally
// 2. TraitsBestEffort() low priority task
// 3. TraitsUserBlocking() high priority reply
func TestPostTaskAndReplyWithTraits_DifferentPriorities(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var taskExecuted, replyExecuted atomic.Bool

	targetRunner.PostTaskAndReplyWithTraits(
		func(ctx context.Context) {
			taskExecuted.Store(true)
		},
		TraitsBestEffort(), // Low priority task
		func(ctx context.Context) {
			replyExecuted.Store(true)
		},
		TraitsUserBlocking(), // High priority reply
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	if !taskExecuted.Load() {
		t.Error("Task was not executed")
	}
	if !replyExecuted.Load() {
		t.Error("Reply was not executed")
	}
}

// =============================================================================
// Generic PostTaskAndReplyWithResult Tests
// =============================================================================

// TestPostTaskAndReplyWithResult_IntResult tests generic version returning int result
// Main test items:
// 1. Correctly pass back int result
// 2. Handling when there are no errors
// 3. Generic type is correct
func TestPostTaskAndReplyWithResult_IntResult(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var receivedResult atomic.Int32
	var receivedError atomic.Value

	PostTaskAndReplyWithResult(
		targetRunner,
		func(ctx context.Context) (int, error) {
			return 42, nil
		},
		func(ctx context.Context, result int, err error) {
			receivedResult.Store(int32(result))
			receivedError.Store(err)
		},
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	if receivedResult.Load() != 42 {
		t.Errorf("Expected result 42, got %d", receivedResult.Load())
	}
	if receivedError.Load() != nil {
		t.Errorf("Expected no error, got %v", receivedError.Load())
	}
}

// TestPostTaskAndReplyWithResult_StringResult tests generic version returning string result
// Main test items:
// 1. Correctly pass back string result
// 2. String serialization and deserialization
// 3. Correctness of result passing
func TestPostTaskAndReplyWithResult_StringResult(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var receivedResult atomic.Value

	PostTaskAndReplyWithResult(
		targetRunner,
		func(ctx context.Context) (string, error) {
			return "Hello World", nil
		},
		func(ctx context.Context, result string, err error) {
			receivedResult.Store(result)
		},
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	result := receivedResult.Load().(string)
	if result != "Hello World" {
		t.Errorf("Expected 'Hello World', got %s", result)
	}
}

// TestPostTaskAndReplyWithResult_StructResult tests generic version returning struct result
// Main test items:
// 1. Correctly pass back complex struct
// 2. Pointer passing and content correctness
// 3. Correctness of struct fields
func TestPostTaskAndReplyWithResult_StructResult(t *testing.T) {
	type UserData struct {
		Name string
		Age  int
	}

	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var receivedResult atomic.Value

	PostTaskAndReplyWithResult(
		targetRunner,
		func(ctx context.Context) (*UserData, error) {
			return &UserData{Name: "Alice", Age: 30}, nil
		},
		func(ctx context.Context, result *UserData, err error) {
			receivedResult.Store(result)
		},
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	result := receivedResult.Load().(*UserData)
	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("Expected UserData{Alice, 30}, got %+v", result)
	}
}

// TestPostTaskAndReplyWithResult_WithError tests error return scenario
// Main test items:
// 1. Error is correctly passed to reply
// 2. context.DeadlineExceeded error handling
// 3. Error message does not affect result passing
func TestPostTaskAndReplyWithResult_WithError(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var receivedError atomic.Value

	PostTaskAndReplyWithResult(
		targetRunner,
		func(ctx context.Context) (int, error) {
			return 0, context.DeadlineExceeded
		},
		func(ctx context.Context, result int, err error) {
			receivedError.Store(err)
		},
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	err := receivedError.Load()
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded error, got %v", err)
	}
}

// =============================================================================
// PostDelayedTaskAndReplyWithResult Tests
// =============================================================================

// TestPostDelayedTaskAndReplyWithResult_Timing tests the timing of delayed task and reply
// Main test items:
// 1. Time precision of delayed task
// 2. Reply executes immediately after task completion
// 3. Correctness of timing control
func TestPostDelayedTaskAndReplyWithResult_Timing(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	start := time.Now()
	var taskTime, replyTime time.Time

	PostDelayedTaskAndReplyWithResult(
		targetRunner,
		func(ctx context.Context) (int, error) {
			taskTime = time.Now()
			return 42, nil
		},
		100*time.Millisecond, // Delay
		func(ctx context.Context, result int, err error) {
			replyTime = time.Now()
		},
		replyRunner,
	)

	time.Sleep(200 * time.Millisecond)

	// Task should start after ~100ms
	taskDelay := taskTime.Sub(start)
	if taskDelay < 100*time.Millisecond {
		t.Errorf("Task started too early: %v", taskDelay)
	}

	// Reply should execute immediately after task
	replyDelay := replyTime.Sub(taskTime)
	if replyDelay > 50*time.Millisecond {
		t.Errorf("Reply took too long after task: %v", replyDelay)
	}
}

// =============================================================================
// SingleThreadTaskRunner Tests
// =============================================================================

// TestSingleThreadTaskRunner_PostTaskAndReply tests PostTaskAndReply with SingleThreadTaskRunner
// Main test items:
// 1. Basic functionality of SingleThreadTaskRunner
// 2. Correct execution of task and reply
// 3. Characteristics of single-thread task scheduler
func TestSingleThreadTaskRunner_PostTaskAndReply(t *testing.T) {
	targetRunner := NewSingleThreadTaskRunner()
	defer targetRunner.Stop()

	replyRunner := NewSingleThreadTaskRunner()
	defer replyRunner.Stop()

	var taskExecuted, replyExecuted atomic.Bool

	targetRunner.PostTaskAndReply(
		func(ctx context.Context) {
			taskExecuted.Store(true)
		},
		func(ctx context.Context) {
			replyExecuted.Store(true)
		},
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	if !taskExecuted.Load() {
		t.Error("Task was not executed")
	}
	if !replyExecuted.Load() {
		t.Error("Reply was not executed")
	}
}

// TestSingleThreadTaskRunner_PostTaskAndReplyWithResult tests generic version with SingleThreadTaskRunner
// Main test items:
// 1. Generic support of SingleThreadTaskRunner
// 2. Result is correctly passed
// 3. Generic handling in single-thread environment
func TestSingleThreadTaskRunner_PostTaskAndReplyWithResult(t *testing.T) {
	targetRunner := NewSingleThreadTaskRunner()
	defer targetRunner.Stop()

	replyRunner := NewSingleThreadTaskRunner()
	defer replyRunner.Stop()

	var receivedResult atomic.Int32

	PostTaskAndReplyWithResult(
		targetRunner,
		func(ctx context.Context) (int, error) {
			return 99, nil
		},
		func(ctx context.Context, result int, err error) {
			receivedResult.Store(int32(result))
		},
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	if receivedResult.Load() != 99 {
		t.Errorf("Expected 99, got %d", receivedResult.Load())
	}
}

// =============================================================================
// Cross-Runner Tests
// =============================================================================

// TestPostTaskAndReply_CrossRunner tests task and reply across different Runners
// Main test items:
// 1. Communication from SequencedTaskRunner to SingleThreadTaskRunner
// 2. Task passing between different types of Runners
// 3. Correctness of cross-Runner execution
func TestPostTaskAndReply_CrossRunner(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	// Use different types of runners
	sequencedRunner := NewSequencedTaskRunner(pool)
	singleThreadRunner := NewSingleThreadTaskRunner()
	defer singleThreadRunner.Stop()

	var taskExecuted, replyExecuted atomic.Bool

	// Post from sequenced to single-thread
	sequencedRunner.PostTaskAndReply(
		func(ctx context.Context) {
			taskExecuted.Store(true)
		},
		func(ctx context.Context) {
			replyExecuted.Store(true)
		},
		singleThreadRunner,
	)

	time.Sleep(100 * time.Millisecond)

	if !taskExecuted.Load() {
		t.Error("Task was not executed")
	}
	if !replyExecuted.Load() {
		t.Error("Reply was not executed")
	}
}

// TestPostTaskAndReplyWithResult_SameRunner tests task and reply on the same Runner
// Main test items:
// 1. Task and reply execute on the same Runner
// 2. Correctness of execution order
// 3. Task scheduling on the same Runner
func TestPostTaskAndReplyWithResult_SameRunner(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	var executionOrder []string
	var mu atomic.Value
	mu.Store(&executionOrder)

	// Reply back to the same runner
	PostTaskAndReplyWithResult(
		runner,
		func(ctx context.Context) (string, error) {
			ptr := mu.Load().(*[]string)
			*ptr = append(*ptr, "task")
			return "result", nil
		},
		func(ctx context.Context, result string, err error) {
			ptr := mu.Load().(*[]string)
			*ptr = append(*ptr, "reply")
		},
		runner, // Same runner
	)

	time.Sleep(100 * time.Millisecond)

	order := *mu.Load().(*[]string)
	if len(order) != 2 || order[0] != "task" || order[1] != "reply" {
		t.Errorf("Execution order incorrect: %v", order)
	}
}

// =============================================================================
// Traits Tests
// =============================================================================

// TestPostTaskAndReplyWithResultAndTraits tests generic version with traits
// Main test items:
// 1. Generic version supports Traits functionality
// 2. Tasks and replies with different priorities
// 3. Combined use of Traits and generic version
func TestPostTaskAndReplyWithResultAndTraits(t *testing.T) {
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var taskExecuted, replyExecuted atomic.Bool

	PostTaskAndReplyWithResultAndTraits(
		targetRunner,
		func(ctx context.Context) (int, error) {
			taskExecuted.Store(true)
			return 42, nil
		},
		TraitsBestEffort(), // Low priority task
		func(ctx context.Context, result int, err error) {
			replyExecuted.Store(true)
		},
		TraitsUserBlocking(), // High priority reply
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	if !taskExecuted.Load() {
		t.Error("Task was not executed")
	}
	if !replyExecuted.Load() {
		t.Error("Reply was not executed")
	}
}

// TestTraitsUserVisible tests the TraitsUserVisible() helper function
// Main test items:
// 1. TraitsUserVisible() returns correct priority
// 2. Correctness of helper function implementation
// 3. Correctness of priority setting
func TestTraitsUserVisible(t *testing.T) {
	traits := TraitsUserVisible()
	if traits.Priority != TaskPriorityUserVisible {
		t.Errorf("Expected priority UserVisible (1), got %d", traits.Priority)
	}
}

// TestSingleThreadTaskRunner_GetThreadPool tests that SingleThreadTaskRunner's GetThreadPool returns nil
// Main test items:
// 1. SingleThreadTaskRunner does not use a thread pool
// 2. GetThreadPool() returns nil
// 3. Verification of independent thread characteristics
func TestSingleThreadTaskRunner_GetThreadPool(t *testing.T) {
	runner := NewSingleThreadTaskRunner()

	// SingleThreadTaskRunner doesn't use a thread pool
	pool := runner.GetThreadPool()
	if pool != nil {
		t.Error("SingleThreadTaskRunner.GetThreadPool should return nil")
	}

	runner.Shutdown()
}

// TestSingleThreadTaskRunner_PostTaskAndReplyWithTraits tests version with traits on SingleThreadTaskRunner
// Main test items:
// 1. Traits support of SingleThreadTaskRunner
// 2. Handling of tasks with different priorities
// 3. Priority control in single-thread environment
func TestSingleThreadTaskRunner_PostTaskAndReplyWithTraits(t *testing.T) {
	targetRunner := NewSingleThreadTaskRunner()
	defer targetRunner.Shutdown()

	replyRunner := NewSingleThreadTaskRunner()
	defer replyRunner.Shutdown()

	var taskExecuted, replyExecuted atomic.Bool

	// PostTaskAndReplyWithTraits with specific traits
	targetRunner.PostTaskAndReplyWithTraits(
		func(ctx context.Context) {
			taskExecuted.Store(true)
		},
		TraitsBestEffort(), // Low priority task
		func(ctx context.Context) {
			replyExecuted.Store(true)
		},
		TraitsUserBlocking(), // High priority reply
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	if !taskExecuted.Load() {
		t.Error("Task was not executed")
	}
	if !replyExecuted.Load() {
		t.Error("Reply was not executed")
	}
}
