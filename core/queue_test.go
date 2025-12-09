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
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort, Category: "Low-1"})
	// 2. Push High Priority 1
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking, Category: "High-1"})
	// 3. Push Low Priority 2
	q.Push(noop, TaskTraits{Priority: TaskPriorityBestEffort, Category: "Low-2"})
	// 4. Push High Priority 2
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserBlocking, Category: "High-2"})
	// 5. Push Medium Priority
	q.Push(noop, TaskTraits{Priority: TaskPriorityUserVisible, Category: "Med-1"})

	// Expected Order:
	// High-1, High-2, Med-1, Low-1, Low-2

	expectedOrder := []string{"High-1", "High-2", "Med-1", "Low-1", "Low-2"}

	for i, expected := range expectedOrder {
		item, ok := q.Pop()
		if !ok {
			t.Fatalf("Step %d: Expected %s but queue is empty", i, expected)
		}
		if item.Traits.Category != expected {
			t.Errorf("Step %d: Expected category %s, got %s (Priority %d)",
				i, expected, item.Traits.Category, item.Traits.Priority)
		}
	}
}

func TestFIFOTaskQueue_FIFO(t *testing.T) {
	q := NewFIFOTaskQueue()
	noop := func(ctx context.Context) {}

	q.Push(noop, TaskTraits{Category: "1"})
	q.Push(noop, TaskTraits{Category: "2"})
	q.Push(noop, TaskTraits{Category: "3"})

	expectedOrder := []string{"1", "2", "3"}

	for _, expected := range expectedOrder {
		item, ok := q.Pop()
		if !ok {
			t.Fatalf("Expected %s but queue is empty", expected)
		}
		if item.Traits.Category != expected {
			t.Errorf("Expected category %s, got %s", expected, item.Traits.Category)
		}
	}
}
