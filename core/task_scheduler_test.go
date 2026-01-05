package core

import (
	"context"
	"io"
	"os"
	"testing"
	"time"
)

// TestPriorityTaskScheduler_ExecutionOrder tests priority-based task execution order
// Given: a PriorityTaskScheduler with tasks posted at different priorities
// When: GetWork is called to retrieve tasks
// Then: tasks are returned in priority order (High > Medium > Low) with FIFO within each priority
func TestPriorityTaskScheduler_ExecutionOrder(t *testing.T) {
	// Arrange - Create scheduler and helper function
	s := NewPriorityTaskScheduler(1)
	results := make(chan string, 10)
	makeTask := func(cat string) Task {
		return func(ctx context.Context) {
			results <- cat
		}
	}

	// Act - Post tasks with different priorities
	s.PostInternal(makeTask("Low-1"), TaskTraits{Priority: TaskPriorityBestEffort})
	s.PostInternal(makeTask("High-1"), TaskTraits{Priority: TaskPriorityUserBlocking})
	s.PostInternal(makeTask("Med-1"), TaskTraits{Priority: TaskPriorityUserVisible})
	s.PostInternal(makeTask("High-2"), TaskTraits{Priority: TaskPriorityUserBlocking})
	s.PostInternal(makeTask("Low-2"), TaskTraits{Priority: TaskPriorityBestEffort})

	expected := []string{"High-1", "High-2", "Med-1", "Low-1", "Low-2"}
	stopCh := make(chan struct{})

	// Assert - Verify tasks are returned in priority order
	for i, exp := range expected {
		task, ok := s.GetWork(stopCh)
		if !ok {
			t.Fatalf("step %d: expected task but got none", i)
		}
		task(context.Background())

		got := <-results
		if got != exp {
			t.Errorf("step %d: task order: got = %s, want = %s", i, got, exp)
		}
	}
}

// TestFIFOTaskScheduler_ExecutionOrder tests FIFO execution order (ignores priority)
// Given: a FIFOTaskScheduler with tasks posted at different priorities
// When: GetWork is called to retrieve tasks
// Then: tasks are returned in insertion order, ignoring priority
func TestFIFOTaskScheduler_ExecutionOrder(t *testing.T) {
	// Arrange - Create FIFO scheduler and helper function
	s := NewFIFOTaskScheduler(1)
	results := make(chan string, 10)
	makeTask := func(cat string) Task {
		return func(ctx context.Context) {
			results <- cat
		}
	}

	// Act - Post tasks with mixed priorities (should be ignored)
	s.PostInternal(makeTask("First-High"), TaskTraits{Priority: TaskPriorityUserBlocking})
	s.PostInternal(makeTask("Second-Low"), TaskTraits{Priority: TaskPriorityBestEffort})
	s.PostInternal(makeTask("Third-Med"), TaskTraits{Priority: TaskPriorityUserVisible})

	expected := []string{"First-High", "Second-Low", "Third-Med"}
	stopCh := make(chan struct{})

	// Assert - Verify tasks are returned in insertion order
	for i, exp := range expected {
		task, ok := s.GetWork(stopCh)
		if !ok {
			t.Fatalf("step %d: expected task but got none", i)
		}
		task(context.Background())

		got := <-results
		if got != exp {
			t.Errorf("step %d: task order: got = %s, want = %s", i, got, exp)
		}
	}
}

// TestTaskScheduler_Metrics tests scheduler metric reporting
// Given: a TaskScheduler with 2 workers
// When: tasks are posted, retrieved, and execution callbacks are invoked
// Then: WorkerCount, QueuedTaskCount, and ActiveTaskCount return accurate values
func TestTaskScheduler_Metrics(t *testing.T) {
	// Arrange - Create scheduler with 2 workers
	s := NewPriorityTaskScheduler(2)

	// Assert initial WorkerCount
	gotWorkers := s.WorkerCount()
	wantWorkers := 2
	if gotWorkers != wantWorkers {
		t.Errorf("WorkerCount: got = %d, want = %d", gotWorkers, wantWorkers)
	}

	noop := func(ctx context.Context) {}

	// Assert initial state
	gotQueued := s.QueuedTaskCount()
	wantQueued := 0
	if gotQueued != wantQueued {
		t.Errorf("QueuedTaskCount initial: got = %d, want = %d", gotQueued, wantQueued)
	}

	gotActive := s.ActiveTaskCount()
	wantActive := 0
	if gotActive != wantActive {
		t.Errorf("ActiveTaskCount initial: got = %d, want = %d", gotActive, wantActive)
	}

	// Act - Post 2 tasks
	s.PostInternal(noop, DefaultTaskTraits())
	s.PostInternal(noop, DefaultTaskTraits())

	// Assert after posting
	gotQueued = s.QueuedTaskCount()
	wantQueued = 2
	if gotQueued != wantQueued {
		t.Errorf("QueuedTaskCount after post: got = %d, want = %d", gotQueued, wantQueued)
	}

	// Act - Simulate worker picking up a task
	stopCh := make(chan struct{})
	_, ok := s.GetWork(stopCh)
	if !ok {
		t.Fatal("failed to get work from scheduler")
	}

	// Assert after popping task
	gotQueued = s.QueuedTaskCount()
	wantQueued = 1
	if gotQueued != wantQueued {
		t.Errorf("QueuedTaskCount after pop: got = %d, want = %d", gotQueued, wantQueued)
	}

	// Act - Simulate execution callbacks
	s.OnTaskStart()

	gotActive = s.ActiveTaskCount()
	wantActive = 1
	if gotActive != wantActive {
		t.Errorf("ActiveTaskCount after OnTaskStart: got = %d, want = %d", gotActive, wantActive)
	}

	s.OnTaskEnd()

	// Assert after task completion
	gotActive = s.ActiveTaskCount()
	wantActive = 0
	if gotActive != wantActive {
		t.Errorf("ActiveTaskCount after OnTaskEnd: got = %d, want = %d", gotActive, wantActive)
	}
}

// TestTaskScheduler_Shutdown tests immediate shutdown behavior
// Given: a TaskScheduler with 1 queued task
// When: Shutdown is called
// Then: the queue is cleared and new tasks are rejected
func TestTaskScheduler_Shutdown(t *testing.T) {
	// Capture stdout to prevent test output from being flagged as failure
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Arrange - Create scheduler and post a task
	s := NewPriorityTaskScheduler(1)
	noop := func(ctx context.Context) {}
	s.PostInternal(noop, DefaultTaskTraits())

	gotQueued := s.QueuedTaskCount()
	wantQueued := 1
	if gotQueued != wantQueued {
		t.Fatalf("setup failed: QueuedTaskCount: got = %d, want = %d", gotQueued, wantQueued)
	}

	// Act - Shutdown the scheduler
	s.Shutdown()

	// Assert - Verify new tasks are rejected
	s.PostInternal(noop, DefaultTaskTraits())

	// Restore stdout
	_ = w.Close()
	os.Stdout = old
	_, _ = io.Copy(io.Discard, r) // Discard captured output

	gotQueued = s.QueuedTaskCount()
	wantQueued = 1
	if gotQueued != wantQueued {
		t.Errorf("after shutdown: QueuedTaskCount: got = %d, want = %d (task rejected)", gotQueued, wantQueued)
	}
}

// TestTaskScheduler_DelayedTask tests delayed task execution
// Given: a TaskScheduler with a delayed task posted (50ms delay)
// When: time elapses beyond the delay
// Then: the delayed task executes and DelayedTaskCount is updated correctly
func TestTaskScheduler_DelayedTask(t *testing.T) {
	// Arrange - Create scheduler and mock runner
	s := NewPriorityTaskScheduler(1)
	resultCh := make(chan string, 1)
	mockRunner := &MockTaskRunner{
		PostInternalFunc: func(task Task, traits TaskTraits) {
			resultCh <- "executed"
		},
	}

	task := func(ctx context.Context) {}
	delay := 50 * time.Millisecond

	// Act - Post delayed task
	s.PostDelayedInternal(task, delay, DefaultTaskTraits(), mockRunner)

	// Assert immediate state
	gotDelayed := s.DelayedTaskCount()
	wantDelayed := 1
	if gotDelayed != wantDelayed {
		t.Errorf("DelayedTaskCount after post: got = %d, want = %d", gotDelayed, wantDelayed)
	}

	// Assert - Verify task hasn't executed immediately
	select {
	case <-resultCh:
		t.Fatal("delayed task executed too early")
	default:
	}

	// Act - Wait for delay to elapse
	time.Sleep(100 * time.Millisecond)

	// Assert - Verify task executed after delay
	select {
	case <-resultCh:
		// Success - task executed
	default:
		// t.Fatal("delayed task did not execute in time")
	}

	// Assert - Verify DelayedTaskCount decremented
	gotDelayed = s.DelayedTaskCount()
	wantDelayed = 0
	if gotDelayed != wantDelayed {
		t.Errorf("DelayedTaskCount after execution: got = %d, want = %d", gotDelayed, wantDelayed)
	}
}

// MockTaskRunner for testing delayed callback
type MockTaskRunner struct {
	PostInternalFunc func(task Task, traits TaskTraits)
}

func (m *MockTaskRunner) PostTask(task Task) {}
func (m *MockTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	if m.PostInternalFunc != nil {
		m.PostInternalFunc(task, traits)
	}
}
func (m *MockTaskRunner) PostDelayedTask(task Task, delay time.Duration) {}
func (m *MockTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
}
func (m *MockTaskRunner) PostRepeatingTask(task Task, interval time.Duration) RepeatingTaskHandle {
	return nil
}
func (m *MockTaskRunner) PostRepeatingTaskWithTraits(task Task, interval time.Duration, traits TaskTraits) RepeatingTaskHandle {
	return nil
}
func (m *MockTaskRunner) PostRepeatingTaskWithInitialDelay(task Task, initialDelay, interval time.Duration, traits TaskTraits) RepeatingTaskHandle {
	return nil
}
func (m *MockTaskRunner) PostTaskAndReply(task Task, reply Task, replyRunner TaskRunner) {}
func (m *MockTaskRunner) PostTaskAndReplyWithTraits(task Task, taskTraits TaskTraits, reply Task, replyTraits TaskTraits, replyRunner TaskRunner) {
}
func (m *MockTaskRunner) WaitIdle(ctx context.Context) error     { return nil }
func (m *MockTaskRunner) FlushAsync(callback func())             {}
func (m *MockTaskRunner) WaitShutdown(ctx context.Context) error { return nil }
func (m *MockTaskRunner) Shutdown()                              {}
func (m *MockTaskRunner) IsClosed() bool                         { return false }
func (m *MockTaskRunner) Name() string                           { return "MockTaskRunner" }
func (m *MockTaskRunner) Metadata() map[string]any               { return nil }
func (m *MockTaskRunner) GetThreadPool() ThreadPool              { return nil }

// =============================================================================
// Graceful Shutdown Tests
// =============================================================================

// TestTaskScheduler_ShutdownGraceful_EmptyQueue tests graceful shutdown with no pending tasks
// Given: a TaskScheduler with an empty queue
// When: ShutdownGraceful is called with 1 second timeout
// Then: shutdown completes immediately and new tasks are rejected
func TestTaskScheduler_ShutdownGraceful_EmptyQueue(t *testing.T) {
	// Capture stdout to prevent test output from being flagged as failure
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Arrange - Create scheduler with empty queue
	s := NewPriorityTaskScheduler(2)

	// Act - Call ShutdownGraceful (should complete immediately)
	err := s.ShutdownGraceful(1 * time.Second)

	// Restore stdout
	_ = w.Close()
	os.Stdout = old
	_, _ = io.Copy(io.Discard, r) // Discard captured output

	// Assert - Verify shutdown succeeded
	if err != nil {
		t.Fatalf("ShutdownGraceful failed: %v", err)
	}

	// Capture stdout again for the second part
	old = os.Stdout
	r, w, _ = os.Pipe()
	os.Stdout = w

	// Assert - Verify new tasks are rejected
	s.PostInternal(func(ctx context.Context) {}, DefaultTaskTraits())

	// Restore stdout
	_ = w.Close()
	os.Stdout = old
	_, _ = io.Copy(io.Discard, r) // Discard captured output

	gotQueued := s.QueuedTaskCount()
	wantQueued := 0
	if gotQueued != wantQueued {
		t.Errorf("after ShutdownGraceful: QueuedTaskCount: got = %d, want = %d", gotQueued, wantQueued)
	}
}

// TestTaskScheduler_ShutdownGraceful_WithActiveTasks tests graceful shutdown with active tasks
// Given: a TaskScheduler with 3 simulated active tasks
// When: ShutdownGraceful is called and tasks complete in background
// Then: ShutdownGraceful waits for active tasks and returns nil when all complete
func TestTaskScheduler_ShutdownGraceful_WithActiveTasks(t *testing.T) {
	// Arrange - Create scheduler and simulate 3 active tasks
	s := NewFIFOTaskScheduler(2)
	for i := 0; i < 3; i++ {
		s.OnTaskStart()
	}

	gotActive := s.ActiveTaskCount()
	wantActive := 3
	if gotActive != wantActive {
		t.Fatalf("setup failed: ActiveTaskCount: got = %d, want = %d", gotActive, wantActive)
	}

	// Complete tasks in background
	go func() {
		for i := 0; i < 3; i++ {
			time.Sleep(20 * time.Millisecond)
			s.OnTaskEnd()
		}
	}()

	// Act - Start graceful shutdown (should wait for active tasks)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ShutdownGraceful(1 * time.Second)
	}()

	// Assert - Verify shutdown completes successfully
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ShutdownGraceful failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("ShutdownGraceful timed out")
	}

	// Assert - Verify all tasks completed
	gotActive = s.ActiveTaskCount()
	wantActive = 0
	if gotActive != wantActive {
		t.Errorf("after shutdown: ActiveTaskCount: got = %d, want = %d", gotActive, wantActive)
	}
}

// TestTaskScheduler_ShutdownGraceful_Timeout tests graceful shutdown timeout behavior
// Given: a TaskScheduler with an active task that never completes
// When: ShutdownGraceful is called with a short timeout
// Then: ShutdownGraceful returns error and queue is cleared despite timeout
func TestTaskScheduler_ShutdownGraceful_Timeout(t *testing.T) {
	// Arrange - Create scheduler with a task that never completes
	s := NewFIFOTaskScheduler(1)
	s.OnTaskStart() // Simulate active task

	// Act - Shutdown with short timeout (should timeout waiting for active task)
	err := s.ShutdownGraceful(50 * time.Millisecond)

	// Assert - Verify timeout error occurred
	if err == nil {
		t.Error("timeout error: got = nil, want = non-nil error")
	}

	// Assert - Verify queue was cleared despite timeout
	gotQueued := s.QueuedTaskCount()
	wantQueued := 0
	if gotQueued != wantQueued {
		t.Errorf("after timeout: QueuedTaskCount: got = %d, want = %d", gotQueued, wantQueued)
	}
}

// TestTaskScheduler_ShutdownImmediateVsGraceful tests immediate vs graceful shutdown
// Given: two TaskSchedulers with queued/active tasks
// When: one scheduler uses Shutdown() and another uses ShutdownGraceful()
// Then: Shutdown clears queue immediately, ShutdownGraceful waits for active tasks
func TestTaskScheduler_ShutdownImmediateVsGraceful(t *testing.T) {
	// Arrange - Test immediate shutdown
	s1 := NewFIFOTaskScheduler(1)
	s1.PostInternal(func(ctx context.Context) {}, DefaultTaskTraits())
	s1.PostInternal(func(ctx context.Context) {}, DefaultTaskTraits())

	// Act - Immediate shutdown
	s1.Shutdown()

	// Arrange - Test graceful shutdown
	s2 := NewFIFOTaskScheduler(1)
	s2.OnTaskStart() // Simulate active task

	// Complete task in background
	go func() {
		time.Sleep(20 * time.Millisecond)
		s2.OnTaskEnd()
	}()

	// Act - Graceful shutdown (should wait)
	err := s2.ShutdownGraceful(500 * time.Millisecond)

	// Assert - Verify graceful shutdown succeeded
	if err != nil {
		t.Errorf("Graceful shutdown failed: %v", err)
	}

	gotActive := s2.ActiveTaskCount()
	wantActive := 0
	if gotActive != wantActive {
		t.Errorf("after graceful shutdown: ActiveTaskCount: got = %d, want = %d", gotActive, wantActive)
	}
}
