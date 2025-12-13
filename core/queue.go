package core

import (
	"container/heap"
	"sync"
)

const (
	defaultQueueCap     = 16
	compactMinCap       = 64 // Don't compact if capacity is less than this
	compactShrinkFactor = 4  // Trigger compaction when len < cap/4
)

type TaskItem struct {
	Task   Task
	Traits TaskTraits
}

// TaskQueue defines the interface for different queue implementations
type TaskQueue interface {
	Push(t Task, traits TaskTraits)
	Pop() (TaskItem, bool)
	PopUpTo(max int) []TaskItem
	PeekTraits() (TaskTraits, bool)
	Len() int
	IsEmpty() bool
	MaybeCompact()
	Clear() // Clear all tasks from the queue
}

// =============================================================================
// FIFOTaskQueue: The original efficient FIFO queue
// =============================================================================

type FIFOTaskQueue struct {
	mu    sync.Mutex
	tasks []TaskItem
}

func NewFIFOTaskQueue() *FIFOTaskQueue {
	return &FIFOTaskQueue{
		tasks: make([]TaskItem, 0, defaultQueueCap),
	}
}

func (q *FIFOTaskQueue) Push(t Task, traits TaskTraits) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, TaskItem{Task: t, Traits: traits})
}

func (q *FIFOTaskQueue) Pop() (TaskItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.tasks) == 0 {
		return TaskItem{}, false
	}

	item := q.tasks[0]
	// Zero out the element in the underlying array to prevent memory leak
	q.tasks[0] = TaskItem{}
	// Optimization: slice slicing
	q.tasks = q.tasks[1:]
	q.maybeCompactLocked()

	return item, true
}

func (q *FIFOTaskQueue) PopUpTo(max int) []TaskItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := len(q.tasks)
	if n == 0 {
		return nil
	}

	if n <= max {
		batch := q.tasks
		q.tasks = q.tasks[:0]
		return batch
	}

	batch := make([]TaskItem, max)
	copy(batch, q.tasks[:max])

	// Zero out the elements in the underlying array to prevent memory leak
	for i := range max {
		q.tasks[i] = TaskItem{}
	}

	q.tasks = q.tasks[max:]
	q.maybeCompactLocked()

	return batch
}

func (q *FIFOTaskQueue) MaybeCompact() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.maybeCompactLocked()
}

func (q *FIFOTaskQueue) maybeCompactLocked() {
	n := len(q.tasks)
	c := cap(q.tasks)

	if c < compactMinCap {
		return
	}
	if n == 0 {
		q.tasks = make([]TaskItem, 0, defaultQueueCap)
		return
	}
	if n*compactShrinkFactor >= c {
		return
	}

	newCap := max(max(c/2, defaultQueueCap), n)

	newSlice := make([]TaskItem, n, newCap)
	copy(newSlice, q.tasks)
	q.tasks = newSlice
}

func (q *FIFOTaskQueue) PeekTraits() (TaskTraits, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.tasks) == 0 {
		return TaskTraits{}, false
	}
	return q.tasks[0].Traits, true
}

func (q *FIFOTaskQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

func (q *FIFOTaskQueue) IsEmpty() bool {
	return q.Len() == 0
}

// Clear removes all tasks from the queue and releases references
func (q *FIFOTaskQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	// Create a new slice to release all task references
	q.tasks = make([]TaskItem, 0, defaultQueueCap)
}

// =============================================================================
// PriorityTaskQueue: Min-Heap based queue with Stability (FIFO for same priority)
// =============================================================================

type priorityItem struct {
	TaskItem
	sequence uint64 // For stability
	index    int    // For heap
}

// priorityHeap implements heap.Interface
type priorityHeap []*priorityItem

func (h priorityHeap) Len() int { return len(h) }

// Less implements priority logic: High priority first, then Small sequence first (FIFO)
func (h priorityHeap) Less(i, j int) bool {
	// Highest Priority first (e.g., UserBlocking > BestEffort)
	if h[i].Traits.Priority > h[j].Traits.Priority {
		return true
	}
	if h[i].Traits.Priority < h[j].Traits.Priority {
		return false
	}
	// Same priority: earlier sequence first (FIFO)
	return h[i].sequence < h[j].sequence
}

func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *priorityHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*priorityItem)
	item.index = n
	*h = append(*h, item)
}

func (h *priorityHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // Avoid memory leak
	item.index = -1
	*h = old[0 : n-1]
	return item
}

type PriorityTaskQueue struct {
	mu           sync.Mutex
	pq           priorityHeap
	nextSequence uint64
}

func NewPriorityTaskQueue() *PriorityTaskQueue {
	return &PriorityTaskQueue{
		pq: make(priorityHeap, 0, defaultQueueCap),
	}
}

func (q *PriorityTaskQueue) Push(t Task, traits TaskTraits) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item := &priorityItem{
		TaskItem: TaskItem{Task: t, Traits: traits},
		sequence: q.nextSequence,
	}
	q.nextSequence++

	heap.Push(&q.pq, item)
}

func (q *PriorityTaskQueue) Pop() (TaskItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pq) == 0 {
		return TaskItem{}, false
	}

	item := heap.Pop(&q.pq).(*priorityItem)
	return item.TaskItem, true
}

func (q *PriorityTaskQueue) PopUpTo(max int) []TaskItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := len(q.pq)
	if count == 0 {
		return nil
	}

	if count > max {
		count = max
	}

	batch := make([]TaskItem, count)
	for i := 0; i < count; i++ {
		item := heap.Pop(&q.pq).(*priorityItem)
		batch[i] = item.TaskItem
	}

	return batch
}

func (q *PriorityTaskQueue) PeekTraits() (TaskTraits, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pq) == 0 {
		return TaskTraits{}, false
	}
	// 0 is the highest priority item because we defined Less to put highest priority at top
	return q.pq[0].Traits, true
}

func (q *PriorityTaskQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pq)
}

func (q *PriorityTaskQueue) IsEmpty() bool {
	return q.Len() == 0
}

func (q *PriorityTaskQueue) MaybeCompact() {
	// Heap automanages capacity usually, or we can implement shrink if needed.
	// For standard slice based heap, standard append/slice mechanics apply.
	// But resetting capacity for heap is tricky without rebuilding.
	// Ignoring compaction for heap in this version as it's less critical given container/heap usage patterns.
}

// Clear removes all tasks from the queue and releases references
func (q *PriorityTaskQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	// Create a new heap to release all task references
	q.pq = make(priorityHeap, 0, defaultQueueCap)
	heap.Init(&q.pq)
	q.nextSequence = 0
}
