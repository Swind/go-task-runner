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

// TestPostTaskAndReply_BasicExecution verifies basic task and reply execution
// Given: Two SequencedTaskRunners (target and reply)
// When: PostTaskAndReply is called with task and reply functions
// Then: Both task and reply execute correctly
func TestPostTaskAndReply_BasicExecution(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var taskExecuted, replyExecuted atomic.Bool

	// Act
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

	// Assert
	if !taskExecuted.Load() {
		t.Error("task executed = false, want true")
	}
	if !replyExecuted.Load() {
		t.Error("reply executed = false, want true")
	}
}

// TestPostTaskAndReply_ExecutionOrder verifies task executes before reply
// Given: A task that takes 50ms and a reply function
// When: PostTaskAndReply is called
// Then: Task executes first, then reply executes
func TestPostTaskAndReply_ExecutionOrder(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	orderCh := make(chan string, 2)

	// Act
	targetRunner.PostTaskAndReply(
		func(ctx context.Context) {
			time.Sleep(50 * time.Millisecond)
			orderCh <- "task"
		},
		func(ctx context.Context) {
			orderCh <- "reply"
		},
		replyRunner,
	)

	// Assert
	order := make([]string, 0, 2)
	timeout := time.After(500 * time.Millisecond)
	for len(order) < 2 {
		select {
		case v := <-orderCh:
			order = append(order, v)
		case <-timeout:
			t.Fatalf("timed out waiting for order results, got=%d", len(order))
		}
	}
	if len(order) != 2 {
		t.Fatalf("len(order) = %d, want 2", len(order))
	}
	if order[0] != "task" || order[1] != "reply" {
		t.Errorf("order = %v, want [task reply]", order)
	}
}

// TestPostTaskAndReply_TaskPanic verifies reply doesn't execute when task panics
// Given: A task function that panics
// When: PostTaskAndReply is called
// Then: Reply does not execute
func TestPostTaskAndReply_TaskPanic(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var replyExecuted atomic.Bool

	// Act
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

	// Assert - Reply should NOT execute after task panic
	if replyExecuted.Load() {
		t.Error("reply executed = true, want false (task panicked)")
	}
}

// TestPostTaskAndReply_NilReplyRunner verifies task executes when replyRunner is nil
// Given: A target runner and nil reply runner
// When: PostTaskAndReply is called with nil replyRunner
// Then: Task executes, reply does not
func TestPostTaskAndReply_NilReplyRunner(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	var taskExecuted atomic.Bool

	// Act
	targetRunner.PostTaskAndReply(
		func(ctx context.Context) {
			taskExecuted.Store(true)
		},
		func(ctx context.Context) {
			t.Error("reply should not execute with nil replyRunner")
		},
		nil,
	)

	time.Sleep(100 * time.Millisecond)

	// Assert
	if !taskExecuted.Load() {
		t.Error("task executed = false, want true")
	}
}

// =============================================================================
// PostTaskAndReplyWithTraits Tests
// =============================================================================

// TestPostTaskAndReplyWithTraits_DifferentPriorities verifies tasks with different priorities
// Given: A low priority task and high priority reply
// When: PostTaskAndReplyWithTraits is called
// Then: Both task and reply execute regardless of priority
func TestPostTaskAndReplyWithTraits_DifferentPriorities(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var taskExecuted, replyExecuted atomic.Bool

	// Act
	targetRunner.PostTaskAndReplyWithTraits(
		func(ctx context.Context) {
			taskExecuted.Store(true)
		},
		TraitsBestEffort(),
		func(ctx context.Context) {
			replyExecuted.Store(true)
		},
		TraitsUserBlocking(),
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	// Assert
	if !taskExecuted.Load() {
		t.Error("task executed = false, want true")
	}
	if !replyExecuted.Load() {
		t.Error("reply executed = false, want true")
	}
}

// =============================================================================
// Generic PostTaskAndReplyWithResult Tests
// =============================================================================

// TestPostTaskAndReplyWithResult_IntResult verifies generic result passing with int
// Given: A task that returns int 42
// When: PostTaskAndReplyWithResult is called
// Then: Reply receives the int result correctly
func TestPostTaskAndReplyWithResult_IntResult(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var receivedResult atomic.Int32
	var receivedError atomic.Value

	// Act
	PostTaskAndReplyWithResult(
		targetRunner,
		func(ctx context.Context) (int, error) {
			return 42, nil
		},
		func(ctx context.Context, result int, err error) {
			receivedResult.Store(int32(result))
			if err != nil {
				receivedError.Store(err)
			}
		},
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	// Assert
	if receivedResult.Load() != 42 {
		t.Errorf("result = %d, want 42", receivedResult.Load())
	}
	if receivedError.Load() != nil {
		t.Errorf("error = %v, want nil", receivedError.Load())
	}
}

// TestPostTaskAndReplyWithResult_StringResult verifies generic result passing with string
// Given: A task that returns "Hello World"
// When: PostTaskAndReplyWithResult is called
// Then: Reply receives the string result correctly
func TestPostTaskAndReplyWithResult_StringResult(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var receivedResult atomic.Value

	// Act
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

	// Assert
	result := receivedResult.Load().(string)
	if result != "Hello World" {
		t.Errorf("result = %q, want \"Hello World\"", result)
	}
}

// TestPostTaskAndReplyWithResult_StructResult verifies generic result passing with struct
// Given: A task that returns a UserData struct
// When: PostTaskAndReplyWithResult is called
// Then: Reply receives the struct with correct field values
func TestPostTaskAndReplyWithResult_StructResult(t *testing.T) {
	type UserData struct {
		Name string
		Age  int
	}

	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var receivedResult atomic.Value

	// Act
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

	// Assert
	result := receivedResult.Load().(*UserData)
	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("result = %+v, want {Name: Alice, Age: 30}", result)
	}
}

// TestPostTaskAndReplyWithResult_WithError verifies error passing to reply
// Given: A task that returns an error
// When: PostTaskAndReplyWithResult is called
// Then: Reply receives the error correctly
func TestPostTaskAndReplyWithResult_WithError(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var receivedError atomic.Value

	// Act
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

	// Assert
	err := receivedError.Load()
	if err != context.DeadlineExceeded {
		t.Errorf("error = %v, want DeadlineExceeded", err)
	}
}

// =============================================================================
// PostDelayedTaskAndReplyWithResult Tests
// =============================================================================

// TestPostDelayedTaskAndReplyWithResult_Timing verifies delayed task timing
// Given: A task with 100ms delay
// When: PostDelayedTaskAndReplyWithResult is called
// Then: Task executes after ~100ms, reply executes immediately after
func TestPostDelayedTaskAndReplyWithResult_Timing(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	start := time.Now()
	taskTimeCh := make(chan time.Time, 1)
	replyTimeCh := make(chan time.Time, 1)

	// Act
	PostDelayedTaskAndReplyWithResult(
		targetRunner,
		func(ctx context.Context) (int, error) {
			taskTimeCh <- time.Now()
			return 42, nil
		},
		100*time.Millisecond,
		func(ctx context.Context, result int, err error) {
			replyTimeCh <- time.Now()
		},
		replyRunner,
	)

	var taskTime time.Time
	var replyTime time.Time
	select {
	case taskTime = <-taskTimeCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for delayed task execution")
	}
	select {
	case replyTime = <-replyTimeCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for reply execution")
	}

	// Assert - Task started after ~100ms
	taskDelay := taskTime.Sub(start)
	if taskDelay < 100*time.Millisecond {
		t.Errorf("task delay = %v, want >=100ms", taskDelay)
	}

	// Assert - Reply executed immediately after task
	replyDelay := replyTime.Sub(taskTime)
	if replyDelay > 50*time.Millisecond {
		t.Errorf("reply delay = %v, want <=50ms", replyDelay)
	}
}

// =============================================================================
// SingleThreadTaskRunner Tests
// =============================================================================

// TestSingleThreadTaskRunner_PostTaskAndReply verifies task-and-reply with SingleThreadTaskRunner
// Given: Two SingleThreadTaskRunners
// When: PostTaskAndReply is called
// Then: Both task and reply execute correctly
func TestSingleThreadTaskRunner_PostTaskAndReply(t *testing.T) {
	// Arrange
	targetRunner := NewSingleThreadTaskRunner()
	defer targetRunner.Stop()

	replyRunner := NewSingleThreadTaskRunner()
	defer replyRunner.Stop()

	var taskExecuted, replyExecuted atomic.Bool

	// Act
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

	// Assert
	if !taskExecuted.Load() {
		t.Error("task executed = false, want true")
	}
	if !replyExecuted.Load() {
		t.Error("reply executed = false, want true")
	}
}

// TestSingleThreadTaskRunner_PostTaskAndReplyWithResult verifies generic version with SingleThreadTaskRunner
// Given: Two SingleThreadTaskRunners
// When: PostTaskAndReplyWithResult is called returning int
// Then: Reply receives the result correctly
func TestSingleThreadTaskRunner_PostTaskAndReplyWithResult(t *testing.T) {
	// Arrange
	targetRunner := NewSingleThreadTaskRunner()
	defer targetRunner.Stop()

	replyRunner := NewSingleThreadTaskRunner()
	defer replyRunner.Stop()

	var receivedResult atomic.Int32

	// Act
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

	// Assert
	if receivedResult.Load() != 99 {
		t.Errorf("result = %d, want 99", receivedResult.Load())
	}
}

// =============================================================================
// Cross-Runner Tests
// =============================================================================

// TestPostTaskAndReply_CrossRunner verifies task-and-reply across different runner types
// Given: A SequencedTaskRunner and SingleThreadTaskRunner
// When: PostTaskAndReply is called from sequenced to single-thread
// Then: Both task and reply execute correctly
func TestPostTaskAndReply_CrossRunner(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	sequencedRunner := NewSequencedTaskRunner(pool)
	singleThreadRunner := NewSingleThreadTaskRunner()
	defer singleThreadRunner.Stop()

	var taskExecuted, replyExecuted atomic.Bool

	// Act - Post from sequenced to single-thread
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

	// Assert
	if !taskExecuted.Load() {
		t.Error("task executed = false, want true")
	}
	if !replyExecuted.Load() {
		t.Error("reply executed = false, want true")
	}
}

// TestPostTaskAndReplyWithResult_SameRunner verifies task and reply on same runner
// Given: A single SequencedTaskRunner
// When: PostTaskAndReplyWithResult is called with same runner for task and reply
// Then: Task executes first, then reply executes on same runner
func TestPostTaskAndReplyWithResult_SameRunner(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	runner := NewSequencedTaskRunner(pool)

	orderCh := make(chan string, 2)

	// Act
	PostTaskAndReplyWithResult(
		runner,
		func(ctx context.Context) (string, error) {
			orderCh <- "task"
			return "result", nil
		},
		func(ctx context.Context, result string, err error) {
			orderCh <- "reply"
		},
		runner,
	)

	// Assert
	order := make([]string, 0, 2)
	timeout := time.After(500 * time.Millisecond)
	for len(order) < 2 {
		select {
		case v := <-orderCh:
			order = append(order, v)
		case <-timeout:
			t.Fatalf("timed out waiting for same-runner order results, got=%d", len(order))
		}
	}
	if len(order) != 2 || order[0] != "task" || order[1] != "reply" {
		t.Errorf("order = %v, want [task reply]", order)
	}
}

// =============================================================================
// Traits Tests
// =============================================================================

// TestPostTaskAndReplyWithResultAndTraits verifies generic version with traits
// Given: Task with BestEffort priority and reply with UserBlocking priority
// When: PostTaskAndReplyWithResultAndTraits is called
// Then: Both task and reply execute correctly
func TestPostTaskAndReplyWithResultAndTraits(t *testing.T) {
	// Arrange
	pool := newTestThreadPool()
	pool.start()
	defer pool.stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)

	var taskExecuted, replyExecuted atomic.Bool

	// Act
	PostTaskAndReplyWithResultAndTraits(
		targetRunner,
		func(ctx context.Context) (int, error) {
			taskExecuted.Store(true)
			return 42, nil
		},
		TraitsBestEffort(),
		func(ctx context.Context, result int, err error) {
			replyExecuted.Store(true)
		},
		TraitsUserBlocking(),
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	// Assert
	if !taskExecuted.Load() {
		t.Error("task executed = false, want true")
	}
	if !replyExecuted.Load() {
		t.Error("reply executed = false, want true")
	}
}

// TestTraitsUserVisible verifies TraitsUserVisible helper function
// Given: The TraitsUserVisible() function
// When: Called
// Then: Returns traits with UserVisible priority
func TestTraitsUserVisible(t *testing.T) {
	// Arrange & Act
	traits := TraitsUserVisible()

	// Assert
	if traits.Priority != TaskPriorityUserVisible {
		t.Errorf("priority = %d, want %d (UserVisible)", traits.Priority, TaskPriorityUserVisible)
	}
}

// TestSingleThreadTaskRunner_GetThreadPool verifies SingleThreadTaskRunner doesn't use thread pool
// Given: A SingleThreadTaskRunner
// When: GetThreadPool is called
// Then: Returns nil (SingleThreadTaskRunner is independent)
func TestSingleThreadTaskRunner_GetThreadPool(t *testing.T) {
	// Arrange
	runner := NewSingleThreadTaskRunner()

	// Act
	pool := runner.GetThreadPool()

	// Assert
	if pool != nil {
		t.Error("GetThreadPool() = non-nil, want nil")
	}

	runner.Shutdown()
}

// TestSingleThreadTaskRunner_PostTaskAndReplyWithTraits verifies traits support on SingleThreadTaskRunner
// Given: Two SingleThreadTaskRunners with different priority traits
// When: PostTaskAndReplyWithTraits is called
// Then: Both task and reply execute correctly
func TestSingleThreadTaskRunner_PostTaskAndReplyWithTraits(t *testing.T) {
	// Arrange
	targetRunner := NewSingleThreadTaskRunner()
	defer targetRunner.Shutdown()

	replyRunner := NewSingleThreadTaskRunner()
	defer replyRunner.Shutdown()

	var taskExecuted, replyExecuted atomic.Bool

	// Act
	targetRunner.PostTaskAndReplyWithTraits(
		func(ctx context.Context) {
			taskExecuted.Store(true)
		},
		TraitsBestEffort(),
		func(ctx context.Context) {
			replyExecuted.Store(true)
		},
		TraitsUserBlocking(),
		replyRunner,
	)

	time.Sleep(100 * time.Millisecond)

	// Assert
	if !taskExecuted.Load() {
		t.Error("task executed = false, want true")
	}
	if !replyExecuted.Load() {
		t.Error("reply executed = false, want true")
	}
}
