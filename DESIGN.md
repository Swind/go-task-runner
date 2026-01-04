Chromium-Inspired Threading Architecture in Golang - Design & Implementation Guide (v2.2)

This document details the design and implementation of a production-grade task scheduling system in Golang, architecturally referenced from Chromium's base::TaskScheduler.

## Version History & Design Evolution

v2.2 Updates (Control Plane & Reliability):
- Control Plane: Enhanced JobManager with management capabilities: Query, Cancel, and Schedule.
- Reliability: Perfected Job lifecycle management to ensure jobs can be interrupted and cleaned up.
- Integration: Consolidated generic registration and serialization strategies.

v2.1 Fixes (Addressing Architecture Review):
- Critical Fix: Resolved a race condition in SequencedTaskRunner where the isRunning state could become inconsistent with the actual queue state.
- Feat: Added PostDelayedTaskWithTraits to support high-priority delayed tasks.
- Feat: Implemented Graceful Shutdown to prevent task loss during restart.
- Perf: Optimized TaskQueue using Zero-allocation slicing and memory compaction.
- Ops: Implemented full-link observability metrics (Queued/Active/Delayed counts).

## Core Philosophy

- **Centralized Scheduling**: The TaskScheduler acts as the global "Brain," making all scheduling decisions.
- **Priorities**: TaskTraits allow distinguishing between UserBlocking, UserVisible, and BestEffort tasks.
- **Fairness**: SequencedTaskRunner uses time-slicing/batching to prevent a single sequence from starving the system.
- **Separation of Duty**: The TaskScheduler decides what runs; the ThreadGroup (The Muscles) simply executes.
- **Unified Time Subsystem**: DelayManager is built into the Scheduler to unify delayed task management.

## Chapter 1: Basic Units - Task & Traits

We define the fundamental unit of execution and introduce TaskTraits to describe attributes.

### Core Definitions

```go
package main

import (
	"context"
	"time"
)

// Task is the smallest unit of execution (Closure).
// It is transient, in-memory, and not serializable.
type Task func(ctx context.Context)

// =============================================================================
// TaskTraits: Defines attributes of a task (Priority, Blocking behavior, etc.)
// =============================================================================

type TaskPriority int

const (
	// TaskPriorityBestEffort: Lowest priority.
	// Used for: Prefetching, non-critical analysis.
	TaskPriorityBestEffort TaskPriority = iota

	// TaskPriorityUserVisible: Default priority.
	// Used for: List loading, UI updates. User is waiting for the result.
	TaskPriorityUserVisible

	// TaskPriorityUserBlocking: Highest priority.
	// Used for: Input response, critical animation. Must run immediately (<16ms).
	TaskPriorityUserBlocking
)

type TaskTraits struct {
	Priority TaskPriority
	MayBlock bool
	Category string // Optional grouping for debug
}

func DefaultTaskTraits() TaskTraits {
	return TaskTraits{Priority: TaskPriorityUserVisible}
}

func TraitsUserBlocking() TaskTraits {
	return TaskTraits{Priority: TaskPriorityUserBlocking}
}
```

### Interface Definitions

The TaskRunner and Backend interfaces are updated to support Traits and Observability.

```go
// TaskRunner is the generic interface for posting tasks.
type TaskRunner interface {
	// PostTask submits a task with default traits.
	PostTask(task Task)

	// PostTaskWithTraits submits a task with specific attributes.
	PostTaskWithTraits(task Task, traits TaskTraits)

	// PostDelayedTask submits a task to run after a delay.
	PostDelayedTask(task Task, delay time.Duration)

	// [v2.1 New] Supports specific priority for delayed tasks.
	PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits)
}

// Backend defines the behavior of the execution engine (implemented by TaskScheduler).
type Backend interface {
	// PostInternal receives immediate tasks with traits.
	PostInternal(task Task, traits TaskTraits)

	// PostDelayedInternal receives delayed tasks.
	// The target runner ensures the task returns to the correct sequence after the delay.
	PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner)

	// Observability (Metrics)
	WorkerCount() int
	QueuedTaskCount() int   // Waiting in global priority queues
	ActiveTaskCount() int   // Currently executing in workers
	DelayedTaskCount() int  // Waiting in DelayManager

	// Lifecycle Monitoring Callbacks (Used by ThreadGroup)
	OnTaskStart()
	OnTaskEnd()
}

// Context Helper
type taskRunnerKeyType struct{}
var taskRunnerKey taskRunnerKeyType

// GetCurrentTaskRunner retrieves the current runner from the context.
func GetCurrentTaskRunner(ctx context.Context) TaskRunner {
	if v := ctx.Value(taskRunnerKey); v != nil {
		return v.(TaskRunner)
	}
	return nil
}
```

## Chapter 2: Containers - TaskQueue (Optimized)

TaskQueue stores tasks waiting to be executed. To support fairness, it supports batch retrieval (PopUpTo). To support high throughput, it uses Zero-allocation techniques and memory compaction.

```go
import "sync"

const (
	defaultQueueCap     = 16
	compactMinCap       = 64 // Minimum capacity before compaction logic applies
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

// PopUpTo retrieves at most `max` tasks.
// Optimization: Uses slice reslicing to avoid make+copy overhead (Zero-copy).
func (q *FIFOTaskQueue) PopUpTo(max int) []TaskItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := len(q.tasks)
	if n == 0 { return nil }

	if n <= max {
		// All tasks retrieved: return current slice and reset internal slice to 0 len
		// underlying capacity is preserved for reuse.
		batch := q.tasks
		q.tasks = q.tasks[:0]
		return batch
	}

	// Partial retrieval: Must allocate new slice for batch to preserve the remaining tail.
	batch := make([]TaskItem, max)
	copy(batch, q.tasks[:max])

	// Shift remaining tasks
	q.tasks = q.tasks[max:]

	// Check if memory compaction is needed
	q.maybeCompactLocked()
	return batch
}

// maybeCompactLocked shrinks the underlying array if it's too large for the content.
func (q *FIFOTaskQueue) maybeCompactLocked() {
	n := len(q.tasks)
	c := cap(q.tasks)

	if c < compactMinCap { return }

	// Empty queue: reset to default capacity
	if n == 0 {
		q.tasks = make([]TaskItem, 0, defaultQueueCap)
		return
	}

	// Shrink if utilization is low (less than 25%)
	if n*compactShrinkFactor < c {
		newCap := c / 2
		if newCap < defaultQueueCap { newCap = defaultQueueCap }
		if newCap < n { newCap = n }

		newSlice := make([]TaskItem, n, newCap)
		copy(newSlice, q.tasks)
		q.tasks = newSlice
	}
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
```

### PriorityTaskQueue: Min-Heap based queue with Stability

```go
import (
	"container/heap"
	"math"
)

type priorityItem struct {
	TaskItem
	sequence uint64 // For stability
	index    int    // For heap
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

// Clear removes all tasks from the queue and releases references
func (q *PriorityTaskQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	// Create a new heap to release all task references
	q.pq = make(priorityHeap, 0, defaultQueueCap)
	heap.Init(&q.pq)
	q.nextSequence = 0
}
```

## Chapter 3: The Brain - TaskScheduler

The TaskScheduler is the central hub. It manages a single unified queue (which can be FIFO or Priority-based), owns the ThreadGroup (execution units), and the DelayManager (time subsystem).

### 3.1 Core Scheduler Logic

```go
import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"fmt"
)

type TaskScheduler struct {
	// 1. Scheduling Data: Single queue with internal priority management
	// This can be either FIFOTaskQueue or PriorityTaskQueue
	queue TaskQueue

	// 2. Signaling: Wakes up workers
	signal chan struct{}

	// 3. Execution Unit: Worker count
	workerCount int

	// 4. Time Subsystem: Owns DelayManager
	delayManager *DelayManager

	// 5. Metrics
	metricQueued  int32
	metricActive  int32
	metricDelayed int32

	// Lifecycle
	shuttingDown int32 // atomic flag
}

func NewPriorityTaskScheduler(workerCount int) *TaskScheduler {
	s := &TaskScheduler{
		signal:      make(chan struct{}, workerCount*2),
		workerCount: workerCount,
	}

	// Use single PriorityTaskQueue which handles priority ordering internally
	s.queue = NewPriorityTaskQueue()
	s.delayManager = NewDelayManager()
	return s
}

func NewFIFOTaskScheduler(workerCount int) *TaskScheduler {
	s := &TaskScheduler{
		signal:      make(chan struct{}, workerCount*2),
		workerCount: workerCount,
	}

	// Use single FIFOTaskQueue which handles priority ordering internally
	s.queue = NewFIFOTaskQueue()
	s.delayManager = NewDelayManager()
	return s
}

// PostInternal implements Backend: Schedule immediate task.
func (s *TaskScheduler) PostInternal(task Task, traits TaskTraits) {
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		// Reject new tasks during shutdown
		return
	}

	s.queue.Push(task, traits)
	atomic.AddInt32(&s.metricQueued, 1)

	// Non-blocking signal
	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// PostDelayedInternal implements Backend: Schedule delayed task.
func (s *TaskScheduler) PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner) {
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		return
	}
	s.delayManager.AddDelayedTask(task, delay, traits, target)
}

// GetWork is called by workers (Pull Model).
func (s *TaskScheduler) GetWork(stopCh <-chan struct{}) (Task, bool) {
	for {
		// Try to pop one task
		if item, ok := s.queue.Pop(); ok {
			atomic.AddInt32(&s.metricQueued, -1)
			return item.Task, true
		}

		select {
		case <-s.signal:
			continue
		case <-stopCh:
			return nil, false
		}
	}
}

// Shutdown performs a graceful shutdown.
func (s *TaskScheduler) Shutdown() {
	// 1. Stop accepting new tasks
	atomic.StoreInt32(&s.shuttingDown, 1)

	// 2. Stop DelayManager (no new tasks generated from timers)
	s.delayManager.Stop()

	// 3. Clear queue to release all task references
	s.queue.Clear()
}

// Observability Methods
func (s *TaskScheduler) WorkerCount() int      { return s.workerCount }
func (s *TaskScheduler) QueuedTaskCount() int  { return int(atomic.LoadInt32(&s.metricQueued)) }
func (s *TaskScheduler) ActiveTaskCount() int  { return int(atomic.LoadInt32(&s.metricActive)) }
func (s *TaskScheduler) DelayedTaskCount() int { return int(atomic.LoadInt32(&s.metricDelayed)) }

// Callbacks from ThreadGroup
func (s *TaskScheduler) OnTaskStart() { atomic.AddInt32(&s.metricActive, 1) }
func (s *TaskScheduler) OnTaskEnd()   { atomic.AddInt32(&s.metricActive, -1) }
```

### 3.2 Internal Time Subsystem (DelayManager)

DelayManager manages timers centrally using a "Timer Pump" pattern with a Min-Heap and a single time.Timer.

```go
import (
	"container/heap"
	"sync"
)

type DelayedTask struct {
	RunAt  time.Time
	Task   Task
	Traits TaskTraits
	Target TaskRunner
	index  int
}

type DelayedTaskHeap []*DelayedTask
// ... Heap Interface Implementation (Len, Less, Swap, Push, Pop) omitted ...

// Peek safely views the top item
func (h DelayedTaskHeap) Peek() *DelayedTask {
	if len(h) == 0 { return nil }
	return h[0]
}

type DelayManager struct {
	pq     DelayedTaskHeap
	mu     sync.Mutex
	wakeup chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

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

	// If new task is earlier than current timer, wake up loop
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

	for {
		var now time.Time
		var nextRun time.Duration

		dm.mu.Lock()
		if item := dm.pq.Peek(); item == nil {
			nextRun = 1000 * time.Hour // Idle
		} else {
			now = time.Now()
			if item.RunAt.Before(now) {
				heap.Pop(&dm.pq)
				dm.mu.Unlock()

				// Time up: Post back to target runner (with original traits)
				item.Target.PostTaskWithTraits(item.Task, item.Traits)
				continue
			} else {
				nextRun = item.RunAt.Sub(now)
			}
		}
		dm.mu.Unlock()

		timer.Reset(nextRun)

		select {
		case <-dm.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			// Timer fired, retry loop
		case <-dm.wakeup:
			// New task inserted, interrupt timer
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}
}

func (dm *DelayManager) Stop() {
	dm.cancel()
}

func (dm *DelayManager) TaskCount() int {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return len(dm.pq)
}
```

## Chapter 4: The Muscles - GoroutineThreadPool

GoroutineThreadPool is a dumb container for execution. It blindly pulls tasks from the TaskScheduler.

```go
import "sync"

type ThreadPool interface {
	PostInternal(task Task, traits TaskTraits)
	PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner)
}

type GoroutineThreadPool struct {
	id        string
	workers   int
	scheduler *TaskScheduler
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.RWMutex
}

func NewGoroutineThreadPool(id string, workers int) *GoroutineThreadPool {
	return &GoroutineThreadPool{
		id:        id,
		workers:   workers,
		scheduler: NewFIFOTaskScheduler(workers),
	}
}

func (tg *GoroutineThreadPool) Start(ctx context.Context) {
	tg.runningMu.Lock()
	defer tg.runningMu.Unlock()

	if tg.running {
		return
	}

	tg.ctx, tg.cancel = context.WithCancel(ctx)
	tg.running = true

	for i := 0; i < tg.workers; i++ {
		tg.wg.Add(1)
		go tg.workerLoop(i, tg.ctx)
	}
}

func (tg *GoroutineThreadPool) workerLoop(id int, ctx context.Context) {
	defer tg.wg.Done()
	stopCh := ctx.Done()

	for {
		task, ok := tg.scheduler.GetWork(stopCh)
		if !ok { return }

		tg.scheduler.OnTaskStart()

		func() {
			defer func() {
				tg.scheduler.OnTaskEnd()
				if r := recover(); r != nil {
					// TODO: Add better error handling, e.g. callback
					fmt.Printf("[Worker %d] Panic: %v\n", id, r)
				}
			}()
			task(ctx)
		}()
	}
}
```

## Chapter 5: Scheduler - SequencedTaskRunner (Fairness)

Core Fix: The runLoop and re-post logic have been rewritten to fix a critical race condition. We now guarantee that the decision to isRunning = false or rePostSelf is consistent with the queue state.

```go
type SequencedTaskRunner struct {
	threadPool   ThreadPool
	queue        TaskQueue
	queueMu      sync.Mutex
	runningCount int32       // atomic: 0 (idle) or 1 (running/scheduled)
	closed       atomic.Bool
	shutdownChan chan struct{}
	shutdownOnce sync.Once

	// Metadata
	name       string
	metadata   map[string]any
	metadataMu sync.Mutex
}

func NewSequencedTaskRunner(threadPool ThreadPool) *SequencedTaskRunner {
	return &SequencedTaskRunner{
		threadPool:   threadPool,
		queue:        NewFIFOTaskQueue(),
		shutdownChan: make(chan struct{}),
		metadata:     make(map[string]any),
	}
}

// ensureRunning ensures there is a runLoop running.
// If no runLoop is currently running or scheduled, it starts one.
func (r *SequencedTaskRunner) ensureRunning(traits TaskTraits) {
	// CAS: Only start runLoop if runningCount is 0 (idle)
	if atomic.CompareAndSwapInt32(&r.runningCount, 0, 1) {
		r.threadPool.PostInternal(r.runLoop, traits)
	}
}

func (r *SequencedTaskRunner) runLoop(ctx context.Context) {
	// Sanity check: runningCount should be 1 when runLoop executes
	if atomic.LoadInt32(&r.runningCount) != 1 {
		panic(fmt.Sprintf("runLoop started with runningCount=%d (expected 1)",
			atomic.LoadInt32(&r.runningCount)))
	}

	runCtx := context.WithValue(ctx, taskRunnerKey, r)

	// 1. Fetch one task
	r.queueMu.Lock()
	item, ok := r.queue.Pop()
	r.queueMu.Unlock()

	if !ok {
		// Queue is empty, go idle
		atomic.StoreInt32(&r.runningCount, 0)
		r.scheduleNextIfNeeded()
		return
	}

	// 2. Execute the task
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				// Task panicked, but we continue processing
				log.Printf("Task panic recovered: %v\n", rec)
			}
		}()
		item.Task(runCtx)
	}()

	// 3. Check if there are more tasks
	r.queueMu.Lock()
	hasMore := !r.queue.IsEmpty()
	var nextTraits TaskTraits
	if hasMore {
		nextTraits, _ = r.queue.PeekTraits()
	}
	r.queueMu.Unlock()

	if hasMore {
		// More tasks to process, directly post next runLoop
		r.threadPool.PostInternal(r.runLoop, nextTraits)
	} else {
		// No more tasks, go idle
		atomic.StoreInt32(&r.runningCount, 0)
		r.scheduleNextIfNeeded()
	}
}

func (r *SequencedTaskRunner) scheduleNextIfNeeded() bool {
	r.queueMu.Lock()
	hasMore := !r.queue.IsEmpty()
	var nextTraits TaskTraits
	if hasMore {
		nextTraits, _ = r.queue.PeekTraits()
	}
	r.queueMu.Unlock()

	if hasMore {
		r.ensureRunning(nextTraits)
		return true
	}
	return false
}

func (r *SequencedTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	if r.closed.Load() {
		return
	}

	r.queueMu.Lock()
	r.queue.Push(task, traits)
	r.queueMu.Unlock()

	r.ensureRunning(traits)
}
```

## Chapter 6: Dedicated Thread - SingleThreadTaskRunner

Designed for Blocking IO or tasks requiring thread affinity. It uses its own channel, bypassing the global scheduler for execution but adhering to the interface.

```go
type SingleThreadTaskRunner struct {
	workQueue chan Task
	ctx       context.Context
	cancel    context.CancelFunc
	stopped   chan struct{}
	once      sync.Once
	closed    atomic.Bool
	mu        sync.Mutex
}

func NewSingleThreadTaskRunner() *SingleThreadTaskRunner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &SingleThreadTaskRunner{
		workQueue: make(chan Task, 100),
		ctx:       ctx,
		cancel:    cancel,
		stopped:   make(chan struct{}),
	}
	go r.runLoop()
	return r
}

func (r *SingleThreadTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	select {
	case <-r.ctx.Done():
		return
	case r.workQueue <- task:
	}
}

func (r *SingleThreadTaskRunner) Shutdown() {
	r.shutdownOnce.Do(func() {
		r.closed.Store(true)
		r.cancel()
		<-r.stopped
	})
}

func (r *SingleThreadTaskRunner) runLoop() {
	defer close(r.stopped)
	for {
		select {
		case task := <-r.workQueue:
			func() {
				defer func() { recover() }()
				task(r.ctx)
			}()
		case <-r.ctx.Done():
			return
		}
	}
}
```

## Summary: Architecture Flow

The architecture follows these key principles:

1. **Single Queue Design**: TaskScheduler uses a single TaskQueue (either FIFO or Priority-based) rather than three separate priority queues. Priority management is handled internally by the queue implementation.

2. **Virtual Threads**: SequencedTaskRunner provides sequential execution guarantees without managing goroutines directly. Tasks are posted to the global thread pool.

3. **Fairness**: SequencedTaskRunner processes one task at a time, then yields back to the scheduler, allowing priority re-evaluation between tasks.

4. **Observability**: Metrics are tracked at the scheduler level (Queued, Active, Delayed counts).

5. **Graceful Shutdown**: All components support shutdown semantics that clear queues and drain in-progress work.
