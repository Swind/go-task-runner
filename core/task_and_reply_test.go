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
