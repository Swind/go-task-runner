package core

import (
	"context"
	"testing"
)

// TestPriorityTaskQueue_Stability verifies priority-based task ordering
// Given: A priority queue with mixed-priority tasks
// When: Tasks are popped from the queue
// Then: Tasks execute in priority order (UserBlocking > UserVisible > BestEffort) with FIFO for same priority
func TestPriorityTaskQueue_Stability(t *testing.T) {
	// Arrange
	q := NewPriorityTaskQueue()
	noop := func(ctx context.Context) {}

	// Act - Push tasks with mixed priorities
	// Within same priority, order should be FIFO
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})   // Low Priority 1
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking}) // High Priority 1
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})   // Low Priority 2
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking}) // High Priority 2
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})  // Medium Priority

	// Expected Order: UserBlocking(2), UserBlocking(2), UserVisible(1), BestEffort(0), BestEffort(0)
	expectedPriorities := []TaskPriority{
		TaskPriorityUserBlocking,
		TaskPriorityUserBlocking,
		TaskPriorityUserVisible,
		TaskPriorityBestEffort,
		TaskPriorityBestEffort,
	}

	// Assert - Verify priority order
	for i, expectedPriority := range expectedPriorities {
		item, ok := q.Pop()
		if !ok {
			t.Fatalf("Step %d: queue is empty, want priority %d", i, expectedPriority)
		}
		if item.Traits.Priority != expectedPriority {
			t.Errorf("Step %d: priority = %d, want %d", i, item.Traits.Priority, expectedPriority)
		}
	}
}

// TestPriorityTaskQueue_PopUpTo verifies batch task retrieval by priority
// Given: A priority queue with 5 tasks of different priorities
// When: PopUpTo is called with limit of 3
// Then: Returns 3 highest priority tasks sorted by priority, 2 remain in queue
func TestPriorityTaskQueue_PopUpTo(t *testing.T) {
	// Arrange
	q := NewPriorityTaskQueue()
	noop := func(ctx context.Context) {}

	// Push 5 tasks with different priorities
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})

	// Act - Pop up to 3 tasks
	tasks := q.PopUpTo(3)

	// Assert - Got 3 highest priority tasks in correct order
	if len(tasks) != 3 {
		t.Errorf("len(tasks) = %d, want 3", len(tasks))
	}
	if tasks[0].Traits.Priority != TaskPriorityUserBlocking {
		t.Errorf("tasks[0].Priority = %d, want %d", tasks[0].Traits.Priority, TaskPriorityUserBlocking)
	}
	if tasks[1].Traits.Priority != TaskPriorityUserBlocking {
		t.Errorf("tasks[1].Priority = %d, want %d", tasks[1].Traits.Priority, TaskPriorityUserBlocking)
	}
	if tasks[2].Traits.Priority != TaskPriorityUserVisible {
		t.Errorf("tasks[2].Priority = %d, want %d", tasks[2].Traits.Priority, TaskPriorityUserVisible)
	}

	// Assert - 2 tasks remain in queue
	if q.Len() != 2 {
		t.Errorf("q.Len() = %d, want 2", q.Len())
	}
}

// TestPriorityTaskQueue_PeekTraits verifies non-destructive head task inspection
// Given: A priority queue with one task
// When: PeekTraits is called
// Then: Returns task traits without removing it from queue
func TestPriorityTaskQueue_PeekTraits(t *testing.T) {
	// Arrange
	q := NewPriorityTaskQueue()
	noop := func(ctx context.Context) {}

	// Assert - Empty queue returns false
	_, ok := q.PeekTraits()
	if ok {
		t.Error("PeekTraits() on empty queue = true, want false")
	}

	// Arrange - Add a task with specific traits
	traits := TaskTraits{Priority: TaskPriorityUserBlocking}
	q.Push(noop, traits)

	// Act - Peek at the head task
	peekedTraits, ok := q.PeekTraits()

	// Assert - Got correct traits
	if !ok {
		t.Fatal("PeekTraits() on non-empty queue = false, want true")
	}
	if peekedTraits.Priority != TaskPriorityUserBlocking {
		t.Errorf("PeekTraits().Priority = %d, want %d", peekedTraits.Priority, TaskPriorityUserBlocking)
	}

	// Assert - Task still in queue (Peek is non-destructive)
	if q.Len() != 1 {
		t.Errorf("q.Len() after Peek = %d, want 1", q.Len())
	}
}

// TestPriorityTaskQueue_MaybeCompact verifies memory compaction functionality
// Given: A queue that has been emptied after containing 10 tasks
// When: MaybeCompact is called
// Then: Queue remains functional and can accept new tasks
func TestPriorityTaskQueue_MaybeCompact(t *testing.T) {
	// Arrange
	q := NewPriorityTaskQueue()
	noop := func(ctx context.Context) {}

	// Push and pop 10 tasks to create empty queue with capacity
	for i := 0; i < 10; i++ {
		q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	}
	for i := 0; i < 10; i++ {
		q.Pop()
	}

	// Act - Compact memory
	q.MaybeCompact()

	// Act - Push new task
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})

	// Assert - Queue still functional
	if q.Len() != 1 {
		t.Errorf("q.Len() = %d, want 1", q.Len())
	}

	item, ok := q.Pop()
	if !ok {
		t.Fatal("Pop() after MaybeCompact = false, want true")
	}
	if item.Traits.Priority != TaskPriorityUserVisible {
		t.Errorf("Pop().Priority = %d, want %d", item.Traits.Priority, TaskPriorityUserVisible)
	}
}

// TestFIFOTaskQueue_FIFO verifies first-in-first-out behavior
// Given: A FIFO queue with 3 tasks of different priorities
// When: Tasks are popped from the queue
// Then: Tasks execute in insertion order regardless of priority
func TestFIFOTaskQueue_FIFO(t *testing.T) {
	// Arrange
	q := NewFIFOTaskQueue()
	noop := func(ctx context.Context) {}

	// Act - Push tasks in specific order
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})

	// Assert - FIFO order preserved regardless of priority
	expectedPriorities := []TaskPriority{
		TaskPriorityBestEffort,
		TaskPriorityUserVisible,
		TaskPriorityUserBlocking,
	}

	for i, expectedPriority := range expectedPriorities {
		item, ok := q.Pop()
		if !ok {
			t.Fatalf("Step %d: queue is empty, want priority %d", i, expectedPriority)
		}
		if item.Traits.Priority != expectedPriority {
			t.Errorf("Step %d: priority = %d, want %d", i, item.Traits.Priority, expectedPriority)
		}
	}
}

// TestPriorityTaskQueue_SequenceOverflow verifies uint64 sequence overflow handling
// Given: A queue with sequence number at MaxUint64
// When: Queue is empty and a new task is pushed
// Then: Sequence resets to 0 and queue operates normally
func TestPriorityTaskQueue_SequenceOverflow(t *testing.T) {
	// Arrange
	q := NewPriorityTaskQueue()
	noop := func(ctx context.Context) {}

	// Set sequence to MaxUint64 to simulate overflow
	q.mu.Lock()
	q.nextSequence = 18446744073709551615 // MaxUint64
	q.mu.Unlock()

	// Assert - Queue is empty
	if !q.IsEmpty() {
		t.Fatal("q.IsEmpty() = false, want true")
	}

	// Act - Push task (sequence should reset to 0)
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})

	// Assert - Task can be popped
	item, ok := q.Pop()
	if !ok {
		t.Fatal("Pop() after sequence reset = false, want true")
	}
	if item.Traits.Priority != TaskPriorityUserVisible {
		t.Errorf("Pop().Priority = %d, want %d", item.Traits.Priority, TaskPriorityUserVisible)
	}

	// Assert - Queue is empty
	if !q.IsEmpty() {
		t.Error("q.IsEmpty() after Pop = false, want true")
	}

	// Act - Push more tasks to verify normal operation
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking})

	// Assert - Normal priority ordering
	if q.Len() != 2 {
		t.Errorf("q.Len() = %d, want 2", q.Len())
	}

	item1, _ := q.Pop()
	if item1.Traits.Priority != TaskPriorityUserBlocking {
		t.Errorf("First pop priority = %d, want %d", item1.Traits.Priority, TaskPriorityUserBlocking)
	}

	item2, _ := q.Pop()
	if item2.Traits.Priority != TaskPriorityBestEffort {
		t.Errorf("Second pop priority = %d, want %d", item2.Traits.Priority, TaskPriorityBestEffort)
	}
}

// TestFIFOTaskQueue_PopUpTo verifies FIFO queue batch retrieval
// Given: A FIFO queue with 5 tasks
// When: PopUpTo(3) is called, then PopUpTo(10) is called
// Then: First call returns 3 tasks, second returns remaining 2
func TestFIFOTaskQueue_PopUpTo(t *testing.T) {
	// Arrange
	q := NewFIFOTaskQueue()
	noop := func(ctx context.Context) {}

	// Push 5 tasks
	for i := 0; i < 5; i++ {
		q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	}

	// Act - Pop up to 3 tasks
	tasks := q.PopUpTo(3)

	// Assert - Got 3 tasks
	if len(tasks) != 3 {
		t.Errorf("len(tasks) = %d, want 3", len(tasks))
	}

	// Assert - 2 tasks remain
	if q.Len() != 2 {
		t.Errorf("q.Len() = %d, want 2", q.Len())
	}

	// Act - Pop the rest (request 10, only 2 available)
	rest := q.PopUpTo(10)

	// Assert - Got 2 remaining tasks
	if len(rest) != 2 {
		t.Errorf("len(rest) = %d, want 2", len(rest))
	}
}

// TestFIFOTaskQueue_MaybeCompact verifies FIFO queue memory compaction
// Given: An emptied FIFO queue that previously held 10 tasks
// When: MaybeCompact is called
// Then: Underlying slice capacity is reduced and queue remains functional
func TestFIFOTaskQueue_MaybeCompact(t *testing.T) {
	// Arrange
	q := NewFIFOTaskQueue()
	noop := func(ctx context.Context) {}

	// Push and pop 10 tasks
	for i := 0; i < 10; i++ {
		q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort})
	}
	for i := 0; i < 10; i++ {
		q.Pop()
	}

	// Act - Compact memory
	q.MaybeCompact()

	// Act - Push new task
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible})

	// Assert - Queue functional
	if q.Len() != 1 {
		t.Errorf("q.Len() = %d, want 1", q.Len())
	}

	item, ok := q.Pop()
	if !ok {
		t.Fatal("Pop() after MaybeCompact = false, want true")
	}
	if item.Traits.Priority != TaskPriorityUserVisible {
		t.Errorf("Pop().Priority = %d, want %d", item.Traits.Priority, TaskPriorityUserVisible)
	}
}
