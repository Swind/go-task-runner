package core

import (
	"container/heap"
	"context"
	"sync"
	"time"
)

// DelayedTask represents a task scheduled for the future
type DelayedTask struct {
	RunAt  time.Time
	Task   Task
	Traits TaskTraits
	Target TaskRunner
	index  int // for heap interface
}

// DelayedTaskHeap implements heap.Interface
type DelayedTaskHeap []*DelayedTask

func (h DelayedTaskHeap) Len() int           { return len(h) }
func (h DelayedTaskHeap) Less(i, j int) bool { return h[i].RunAt.Before(h[j].RunAt) }
func (h DelayedTaskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *DelayedTaskHeap) Push(x any) {
	n := len(*h)
	item := x.(*DelayedTask)
	item.index = n
	*h = append(*h, item)
}

func (h *DelayedTaskHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	item.index = -1
	*h = old[0 : n-1]
	return item
}

func (h *DelayedTaskHeap) Peek() *DelayedTask {
	if len(*h) == 0 {
		return nil
	}
	return (*h)[0]
}

type DelayManager struct {
	pq     DelayedTaskHeap
	mu     sync.Mutex
	wakeup chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

type delayNextRunState int

const (
	delayStateEmpty delayNextRunState = iota
	delayStateExpiredNow
	delayStateFuture
)

func NewDelayManager() *DelayManager {
	ctx, cancel := context.WithCancel(context.Background())
	dm := &DelayManager{
		pq:     make(DelayedTaskHeap, 0),
		wakeup: make(chan struct{}, 1),
		ctx:    ctx,
		cancel: cancel,
	}
	heap.Init(&dm.pq)
	go dm.loop()
	return dm
}

func (dm *DelayManager) AddDelayedTask(task Task, delay time.Duration, traits TaskTraits, target TaskRunner) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	item := &DelayedTask{
		RunAt:  time.Now().Add(delay),
		Task:   task,
		Traits: traits,
		Target: target,
	}
	heap.Push(&dm.pq, item)

	if item.index == 0 {
		select {
		case dm.wakeup <- struct{}{}:
		default:
		}
	}
}

func (dm *DelayManager) loop() {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	for {
		// Distinguish empty queue from already-expired tasks.
		nextRun, state := dm.calculateNextRun()
		if state == delayStateExpiredNow {
			dm.processExpiredTasks()
			continue
		}
		if state == delayStateEmpty {
			nextRun = 1000 * time.Hour
		}

		stopAndDrainTimer(timer)
		timer.Reset(nextRun)

		select {
		case <-dm.ctx.Done():
			return
		case <-timer.C:
			// Timer fired, process all expired tasks in one go
			dm.processExpiredTasks()
		case <-dm.wakeup:
			// New task added, recalculate schedule in next loop iteration.
		}
	}
}

// calculateNextRun determines how long to wait until the next task
// and whether the queue is empty, expired-now, or future.
func (dm *DelayManager) calculateNextRun() (time.Duration, delayNextRunState) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	item := dm.pq.Peek()
	if item == nil {
		return 0, delayStateEmpty
	}

	now := time.Now()
	if !item.RunAt.After(now) {
		return 0, delayStateExpiredNow
	}
	return item.RunAt.Sub(now), delayStateFuture
}

// processExpiredTasks processes all tasks that have expired
// This is more efficient than processing one at a time with continue
func (dm *DelayManager) processExpiredTasks() {
	dm.mu.Lock()

	now := time.Now()
	// Collect all expired tasks to avoid holding lock while posting
	var expired []*DelayedTask

	for dm.pq.Len() > 0 {
		item := dm.pq.Peek()
		if item.RunAt.After(now) {
			break // No more expired tasks
		}
		// Task has expired
		heap.Pop(&dm.pq)
		expired = append(expired, item)
	}

	dm.mu.Unlock()

	// Post expired tasks outside the lock
	for _, item := range expired {
		item.Target.PostTaskWithTraits(item.Task, item.Traits)
	}
}

func (dm *DelayManager) Stop() {
	dm.cancel()

	// Clear pq to release all TaskRunner references
	dm.mu.Lock()
	dm.pq = make(DelayedTaskHeap, 0)
	heap.Init(&dm.pq)
	dm.mu.Unlock()
}

func (dm *DelayManager) TaskCount() int {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return len(dm.pq)
}

func stopAndDrainTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
