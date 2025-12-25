package core

import (
	"context"
	"testing"
)

func TestPriorityTaskQueue_Stability(t *testing.T) {
	q := NewPriorityTaskQueue()

	// Helper to create a dummy task
	noop := func(ctx context.Context) {}

	// Push tasks with mixed priorities
	// Expectations:
	// Priority UserBlocking (2) -> First
	// Priority UserVisible (1) -> Second
	// Priority BestEffort (0) -> Third

	// Within same priority, order should be FIFO

	// 1. Push Low Priority 1
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	// 2. Push High Priority 1
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})
	// 3. Push Low Priority 2
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	// 4. Push High Priority 2
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})
	// 5. Push Medium Priority
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})

	// Expected Order by Priority:
	// UserBlocking (2), UserBlocking (2), UserVisible (1), BestEffort (0), BestEffort (0)
	expectedPriorities := []TaskPriority{
		TaskPriorityUserBlocking, // High-1
		TaskPriorityUserBlocking, // High-2
		TaskPriorityUserVisible,  // Med-1
		TaskPriorityBestEffort,   // Low-1
		TaskPriorityBestEffort,   // Low-2
	}

	for i, expectedPriority := range expectedPriorities {
		item, ok := q.Pop()
		if !ok {
			t.Fatalf("Step %d: Expected priority %d but queue is empty", i, expectedPriority)
		}
		if item.Traits.Priority != expectedPriority {
			t.Errorf("Step %d: Expected priority %d, got %d",
				i, expectedPriority, item.Traits.Priority)
		}
	}
}

func TestPriorityTaskQueue_PopUpTo(t *testing.T) {
	q := NewPriorityTaskQueue()
	noop := func(ctx context.Context) {}

	// Push 5 tasks with different priorities
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})

	// Pop up to 3 tasks (should get highest priority ones first)
	tasks := q.PopUpTo(3)

	if len(tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(tasks))
	}

	// First 2 should be UserBlocking, then UserVisible
	if tasks[0].Traits.Priority != TaskPriorityUserBlocking {
		t.Errorf("Expected first task to be UserBlocking, got %d", tasks[0].Traits.Priority)
	}
	if tasks[1].Traits.Priority != TaskPriorityUserBlocking {
		t.Errorf("Expected second task to be UserBlocking, got %d", tasks[1].Traits.Priority)
	}
	if tasks[2].Traits.Priority != TaskPriorityUserVisible {
		t.Errorf("Expected third task to be UserVisible, got %d", tasks[2].Traits.Priority)
	}

	// Queue should still have 2 remaining BestEffort tasks
	if q.Len() != 2 {
		t.Errorf("Expected 2 remaining tasks, got %d", q.Len())
	}
}

func TestPriorityTaskQueue_PeekTraits(t *testing.T) {
	q := NewPriorityTaskQueue()
	noop := func(ctx context.Context) {}

	// Empty queue - should return false
	_, ok := q.PeekTraits()
	if ok {
		t.Error("Expected false for empty queue")
	}

	// Push a task with specific traits
	traits := TaskTraits{Priority: TaskPriorityUserBlocking}
	q.Push(noop, traits)

	// Peek should return the traits
	peekedTraits, ok := q.PeekTraits()
	if !ok {
		t.Fatal("Expected true for non-empty queue")
	}
	if peekedTraits.Priority != TaskPriorityUserBlocking {
		t.Errorf("Expected priority UserBlocking, got %d", peekedTraits.Priority)
	}

	// Queue should still have the task (Peek doesn't remove)
	if q.Len() != 1 {
		t.Errorf("Expected 1 task after peek, got %d", q.Len())
	}
}

func TestPriorityTaskQueue_MaybeCompact(t *testing.T) {
	q := NewPriorityTaskQueue()
	noop := func(ctx context.Context) {}

	// Push 10 tasks
	for i := 0; i < 10; i++ {
		q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	}

	// Pop all tasks
	for i := 0; i < 10; i++ {
		q.Pop()
	}

	// Queue should have empty slice with capacity > 0
	// MaybeCompact should reduce the capacity
	q.MaybeCompact()

	// Push a new task - should still work
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})

	if q.Len() != 1 {
		t.Errorf("Expected 1 task after MaybeCompact, got %d", q.Len())
	}

	// Verify it can still pop
	item, ok := q.Pop()
	if !ok {
		t.Error("Failed to pop task after MaybeCompact")
	}
	if item.Traits.Priority != TaskPriorityUserVisible {
		t.Errorf("Expected priority UserVisible, got %d", item.Traits.Priority)
	}
}

func TestFIFOTaskQueue_FIFO(t *testing.T) {
	q := NewFIFOTaskQueue()
	noop := func(ctx context.Context) {}

	// Push tasks - they should come out in the same order
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})

	// FIFO order should be preserved regardless of priority
	expectedPriorities := []TaskPriority{
		TaskPriorityBestEffort,
		TaskPriorityUserVisible,
		TaskPriorityUserBlocking,
	}

	for i, expectedPriority := range expectedPriorities {
		item, ok := q.Pop()
		if !ok {
			t.Fatalf("Step %d: Expected priority %d but queue is empty", i, expectedPriority)
		}
		if item.Traits.Priority != expectedPriority {
			t.Errorf("Step %d: Expected priority %d, got %d",
				i, expectedPriority, item.Traits.Priority)
		}
	}
}

// TestPriorityTaskQueue_SequenceOverflow tests the defensive overflow handling
// While uint64 overflow is practically impossible, this test verifies the logic
func TestPriorityTaskQueue_SequenceOverflow(t *testing.T) {
	q := NewPriorityTaskQueue()
	noop := func(ctx context.Context) {}

	// Manually set sequence to MaxUint64 to simulate overflow scenario
	q.mu.Lock()
	q.nextSequence = 18446744073709551615 // MaxUint64
	q.mu.Unlock()

	// Queue should be empty at this point
	if !q.IsEmpty() {
		t.Fatal("Queue should be empty initially")
	}

	// Push a task - sequence should reset to 0 since queue is empty
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})

	// Verify the task can be popped successfully
	item, ok := q.Pop()
	if !ok {
		t.Fatal("Failed to pop task after sequence reset")
	}

	if item.Traits.Priority != TaskPriorityUserVisible {
		t.Errorf("Expected priority UserVisible, got %d", item.Traits.Priority)
	}

	// Verify queue is now empty
	if !q.IsEmpty() {
		t.Error("Queue should be empty after popping")
	}

	// Push more tasks to verify normal operation continues
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})

	// Should have 2 tasks
	if q.Len() != 2 {
		t.Errorf("Expected 2 tasks, got %d", q.Len())
	}

	// Pop should work correctly
	item1, _ := q.Pop()
	if item1.Traits.Priority != TaskPriorityUserBlocking {
		t.Errorf("Expected UserBlocking first, got %d", item1.Traits.Priority)
	}

	item2, _ := q.Pop()
	if item2.Traits.Priority != TaskPriorityBestEffort {
		t.Errorf("Expected BestEffort second, got %d", item2.Traits.Priority)
	}
}

func TestFIFOTaskQueue_PopUpTo(t *testing.T) {
	q := NewFIFOTaskQueue()
	noop := func(ctx context.Context) {}

	// Push 5 tasks
	for i := 0; i < 5; i++ {
		q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	}

	// Pop up to 3 tasks (should get first 3 in FIFO order)
	tasks := q.PopUpTo(3)

	if len(tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(tasks))
	}

	// Queue should still have 2 remaining tasks
	if q.Len() != 2 {
		t.Errorf("Expected 2 remaining tasks, got %d", q.Len())
	}

	// Pop the rest - should get 2 more
	rest := q.PopUpTo(10)
	if len(rest) != 2 {
		t.Errorf("Expected 2 remaining tasks, got %d", len(rest))
	}
}

func TestFIFOTaskQueue_MaybeCompact(t *testing.T) {
	q := NewFIFOTaskQueue()
	noop := func(ctx context.Context) {}

	// Push 10 tasks
	for i := 0; i < 10; i++ {
		q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	}

	// Pop all tasks
	for i := 0; i < 10; i++ {
		q.Pop()
	}

	// MaybeCompact should reduce the underlying slice capacity
	q.MaybeCompact()

	// Push a new task - should still work
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})

	if q.Len() != 1 {
		t.Errorf("Expected 1 task after MaybeCompact, got %d", q.Len())
	}

	// Verify it can still pop
	item, ok := q.Pop()
	if !ok {
		t.Error("Failed to pop task after MaybeCompact")
	}
	if item.Traits.Priority != TaskPriorityUserVisible {
		t.Errorf("Expected priority UserVisible, got %d", item.Traits.Priority)
	}
}
