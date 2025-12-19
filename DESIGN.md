Chromium-Inspired Threading Architecture in Golang - Design & Implementation Guide (v2.2)This document details the design and implementation of a production-grade task scheduling system in Golang, architecturally referenced from Chromium's base::TaskScheduler.Version History & Design Evolutionv2.2 Updates (Control Plane & Reliability):Control Plane: Enhanced JobManager with management capabilities: Query, Cancel, and Schedule.Reliability: Perfected Job lifecycle management to ensure jobs can be interrupted and cleaned up.Integration: Consolidated generic registration and serialization strategies.v2.1 Fixes (Addressing Architecture Review):Critical Fix: Resolved a race condition in SequencedTaskRunner where the isRunning state could become inconsistent with the actual queue state.Feat: Added PostDelayedTaskWithTraits to support high-priority delayed tasks.Feat: Implemented Graceful Shutdown to prevent task loss during restart.Perf: Optimized TaskQueue using Zero-allocation slicing and memory compaction.Ops: Implemented full-link observability metrics (Queued/Active/Delayed counts).Core PhilosophyCentralized Scheduling: The TaskScheduler acts as the global "Brain," making all scheduling decisions.Priorities: TaskTraits allow distinguishing between UserBlocking, UserVisible, and BestEffort tasks.Fairness: SequencedTaskRunner uses time-slicing/batching to prevent a single sequence from starving the system.Separation of Duty: The TaskScheduler decides what runs; the ThreadGroup (The Muscles) simply executes.Unified Time Subsystem: DelayManager is built into the Scheduler to unify delayed task management.Chapter 1: Basic Units - Task & TraitsWe define the fundamental unit of execution and introduce TaskTraits to describe attributes.Core Definitionspackage main

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
Interface DefinitionsThe TaskRunner and Backend interfaces are updated to support Traits and Observability.// TaskRunner is the generic interface for posting tasks.
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
Chapter 2: Containers - TaskQueue (Optimized)TaskQueue stores tasks waiting to be executed. To support fairness, it supports batch retrieval (PopUpTo). To support high throughput, it uses Zero-allocation techniques and memory compaction.import "sync"

const (
	defaultQueueCap     = 16
	compactMinCap       = 64 // Minimum capacity before compaction logic applies
	compactShrinkFactor = 4  // Trigger compaction when len < cap/4
)

type TaskItem struct {
	Task   Task
	Traits TaskTraits
}

type TaskQueue struct {
	mu    sync.Mutex
	tasks []TaskItem
}

func NewTaskQueue() *TaskQueue {
	return &TaskQueue{
		tasks: make([]TaskItem, 0, defaultQueueCap),
	}
}

func (q *TaskQueue) Push(t Task, traits TaskTraits) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, TaskItem{Task: t, Traits: traits})
}

// PopUpTo retrieves at most `max` tasks.
// Optimization: Uses slice reslicing to avoid make+copy overhead (Zero-copy).
func (q *TaskQueue) PopUpTo(max int) []TaskItem {
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
func (q *TaskQueue) maybeCompactLocked() {
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

func (q *TaskQueue) PeekTraits() (TaskTraits, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.tasks) == 0 {
		return TaskTraits{}, false
	}
	return q.tasks[0].Traits, true
}

func (q *TaskQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

func (q *TaskQueue) IsEmpty() bool {
	return q.Len() == 0
}
Chapter 3: The Brain - TaskSchedulerThe TaskScheduler is the central hub. It manages priority queues, owns the ThreadGroup (execution units), and the DelayManager (time subsystem).3.1 Core Scheduler Logicimport (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"fmt"
)

type TaskScheduler struct {
	// 1. Scheduling Data: Priority Queues
	queues [3]*TaskQueue
	
	// 2. Signaling: Wakes up workers
	signal chan struct{}

	// 3. Execution Unit: Owns ThreadGroup
	threadGroup *ThreadGroup

	// 4. Time Subsystem: Owns DelayManager
	delayManager *DelayManager

	// 5. Metrics
	metricQueued  int32
	metricActive  int32
	metricDelayed int32

	// Lifecycle
	ctx          context.Context
	cancel       context.CancelFunc
	shuttingDown int32 // atomic flag
}

func NewTaskScheduler(workerCount int) *TaskScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &TaskScheduler{
		signal: make(chan struct{}, workerCount*2),
		ctx:    ctx,
		cancel: cancel,
	}
	
	for i := 0; i < 3; i++ {
		s.queues[i] = NewTaskQueue()
	}

	s.delayManager = NewDelayManager(s)
	s.threadGroup = NewThreadGroup(workerCount, s)
	s.threadGroup.Start(ctx)

	return s
}

// PostInternal implements Backend: Schedule immediate task.
func (s *TaskScheduler) PostInternal(task Task, traits TaskTraits) {
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		// Reject new tasks during shutdown
		return
	}

	idx := 1
	switch traits.Priority {
	case TaskPriorityUserBlocking: idx = 0
	case TaskPriorityUserVisible:  idx = 1
	case TaskPriorityBestEffort:   idx = 2
	}

	s.queues[idx].Push(task, traits)
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

// GetWork is called by ThreadGroup (Pull Model).
// Implements Strict Priority: Always checks High queue first.
func (s *TaskScheduler) GetWork(stopCh <-chan struct{}) (Task, bool) {
	for {
		for i := 0; i < 3; i++ {
			// Pop 1 task at a time to allow frequent priority re-checks
			batch := s.queues[i].PopUpTo(1)
			if len(batch) > 0 {
				atomic.AddInt32(&s.metricQueued, -1)
				return batch[0].Task, true
			}
		}

		select {
		case <-s.signal:
			continue // Woken up, rescan queues
		case <-stopCh:
			return nil, false
		}
	}
}

// Shutdown performs a Graceful Shutdown.
func (s *TaskScheduler) Shutdown() {
	// 1. Stop accepting new tasks
	atomic.StoreInt32(&s.shuttingDown, 1)

	// 2. Stop DelayManager (no new tasks generated from timers)
	s.delayManager.Stop()
	
	// 3. Wait for queues to drain (Simplified)
	// Real implementation might use a WaitGroup or Condition Variable
	for {
		if s.QueuedTaskCount() == 0 && s.ActiveTaskCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 4. Force stop workers
	s.cancel()
	s.threadGroup.Join()
}

// Observability Methods
func (s *TaskScheduler) WorkerCount() int      { return s.threadGroup.workers }
func (s *TaskScheduler) QueuedTaskCount() int  { return int(atomic.LoadInt32(&s.metricQueued)) }
func (s *TaskScheduler) ActiveTaskCount() int  { return int(atomic.LoadInt32(&s.metricActive)) }
func (s *TaskScheduler) DelayedTaskCount() int { return int(atomic.LoadInt32(&s.metricDelayed)) }

// Callbacks from ThreadGroup
func (s *TaskScheduler) OnTaskStart() { atomic.AddInt32(&s.metricActive, 1) }
func (s *TaskScheduler) OnTaskEnd()   { atomic.AddInt32(&s.metricActive, -1) }
3.2 Internal Time Subsystem (DelayManager)DelayManager manages timers centrally using a "Timer Pump" pattern with a Min-Heap and a single time.Timer.DelayedTaskHeap:import (
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
DelayManager Implementation:type DelayManager struct {
	scheduler *TaskScheduler // For metric updates
	pq        DelayedTaskHeap
	mu        sync.Mutex
	wakeup    chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewDelayManager(s *TaskScheduler) *DelayManager {
	ctx, cancel := context.WithCancel(context.Background())
	dm := &DelayManager{
		scheduler: s,
		pq:        make(DelayedTaskHeap, 0),
		wakeup:    make(chan struct{}, 1),
		ctx:       ctx,
		cancel:    cancel,
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
	
	// Update Metric
	atomic.AddInt32(&dm.scheduler.metricDelayed, 1)

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
				atomic.AddInt32(&dm.scheduler.metricDelayed, -1)
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
Chapter 4: The Muscles - ThreadGroupThreadGroup is a dumb container for execution. It blindly pulls tasks from the WorkSource.import "sync"
import "fmt"

type WorkSource interface {
	GetWork(stopCh <-chan struct{}) (Task, bool)
	OnTaskStart()
	OnTaskEnd()
}

type ThreadGroup struct {
	workers int
	source  WorkSource
	wg      sync.WaitGroup
}

func NewThreadGroup(workers int, source WorkSource) *ThreadGroup {
	return &ThreadGroup{
		workers: workers,
		source:  source,
	}
}

func (tg *ThreadGroup) Start(ctx context.Context) {
	for i := 0; i < tg.workers; i++ {
		tg.wg.Add(1)
		go tg.workerLoop(i, ctx)
	}
}

func (tg *ThreadGroup) workerLoop(id int, ctx context.Context) {
	defer tg.wg.Done()
	stopCh := ctx.Done()

	for {
		task, ok := tg.source.GetWork(stopCh)
		if !ok { return }

		tg.source.OnTaskStart()
		func() {
			defer func() {
				tg.source.OnTaskEnd()
				if r := recover(); r != nil {
					fmt.Printf("[Worker %d] Panic: %v\n", id, r)
				}
			}()
			task(ctx)
		}()
	}
}

func (tg *ThreadGroup) Join() { tg.wg.Wait() }
Chapter 5: Scheduler - SequencedTaskRunner (Fairness)Core Fix: The runLoop and re-post logic have been rewritten to fix a critical race condition. We now guarantee that the decision to isRunning = false or rePostSelf is consistent with the queue state.Implementationconst MaxTasksPerSlice = 4 // Fairness: Max tasks per batch

type SequencedTaskRunner struct {
	backend Backend
	queue   *TaskQueue
	mu      sync.Mutex
	isRunning bool
}

func NewSequencedTaskRunner(backend Backend) *SequencedTaskRunner {
	return &SequencedTaskRunner{
		backend: backend,
		queue:   NewTaskQueue(),
	}
}

func (r *SequencedTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

func (r *SequencedTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	r.queue.Push(task, traits)
	r.scheduleRunLoop(traits)
}

func (r *SequencedTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraits(task, delay, DefaultTaskTraits())
}

func (r *SequencedTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	r.backend.PostDelayedInternal(task, delay, traits, r)
}

func (r *SequencedTaskRunner) scheduleRunLoop(traits TaskTraits) {
	r.mu.Lock()
	if !r.isRunning {
		r.isRunning = true
		r.mu.Unlock()
		r.backend.PostInternal(r.runLoop, traits)
	} else {
		r.mu.Unlock()
	}
}

func (r *SequencedTaskRunner) rePostSelf(traits TaskTraits) {
	r.backend.PostInternal(r.runLoop, traits)
}

func (r *SequencedTaskRunner) runLoop(ctx context.Context) {
	runCtx := context.WithValue(ctx, taskRunnerKey, r)
	
	// 1. Fetch batch (Time Slice)
	items := r.queue.PopUpTo(MaxTasksPerSlice)

	if len(items) == 0 {
		// [v2.1 Fix] Handle race condition where queue became non-empty
		// right after PopUpTo returned empty (unlikely but possible)
		var needRepost bool
		var nextTraits TaskTraits

		r.mu.Lock()
		if r.queue.IsEmpty() {
			r.isRunning = false
			needRepost = false
		} else {
			needRepost = true
		}
		r.mu.Unlock()

		if !needRepost { return }

		// Use PeekTraits to determine the next priority
		nextTraits, ok := r.queue.PeekTraits()
		if !ok { nextTraits = DefaultTaskTraits() }
		r.rePostSelf(nextTraits)
		return
	}

	// 2. Execute Batch
	for _, item := range items {
		func() {
			defer func() { recover() }()
			item.Task(runCtx)
		}()
	}

	// 3. Check Yielding (Fairness)
	var needRepost bool
	var nextTraits TaskTraits

	r.mu.Lock()
	if r.queue.IsEmpty() {
		r.isRunning = false
		needRepost = false
	} else {
		needRepost = true
	}
	r.mu.Unlock()

	if !needRepost { return }

	// Yield CPU: Re-queue self with correct priority
	nextTraits, ok := r.queue.PeekTraits()
	if !ok { nextTraits = DefaultTaskTraits() }
	r.rePostSelf(nextTraits)
}
Chapter 6: Dedicated Thread - SingleThreadTaskRunnerDesigned for Blocking IO or tasks requiring thread affinity. It uses its own channel, bypassing the global scheduler for execution but adhering to the interface.type SingleThreadTaskRunner struct {
	workQueue chan Task
	ctx       context.Context
	cancel    context.CancelFunc
	stopped   chan struct{}
	once      sync.Once
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

func (r *SingleThreadTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

func (r *SingleThreadTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	select {
	case <-r.ctx.Done():
		return
	case r.workQueue <- task:
	}
}

func (r *SingleThreadTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraits(task, delay, DefaultTaskTraits())
}

func (r *SingleThreadTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	select {
	case <-r.ctx.Done():
		return
	default:
		// Uses time.AfterFunc to inject task back into the channel
		time.AfterFunc(delay, func() {
			r.PostTaskWithTraits(task, traits)
		})
	}
}

func (r *SingleThreadTaskRunner) Stop() {
	r.once.Do(func() {
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
Chapter 7: Control Plane - JobManager (v2.3)The JobManager provides the Control Plane: Query, Cancel, and Schedule high-level jobs.

v2.3 Updates (Three-Layer Runner Architecture):
- Lock-Free Design: Use SequencedTaskRunner instead of mutexes for concurrency control
- Three-Layer Architecture: Separate Control, IO, and Execution concerns
- Non-Blocking Control: Control operations complete in <100μs (pure memory)
- Sequential IO: All database operations execute in order on ioRunner
- Scalable Execution: User jobs execute on dedicated executionRunner

7.1 Architecture Overview

Three-Layer Runner Design:

┌─────────────────────────────────────────────────────────┐
│  Layer 1: controlRunner (Control Plane)                 │
│  • Pure in-memory operations (sync.Map)                 │
│  • Handler registration                                 │
│  • activeJobs management                                │
│  • Scheduling decisions                                 │
│  • Response time: <100μs                                │
└────────────┬────────────────────────────────────────────┘
             │ Delegates
             ├──────────────┐
             ↓              ↓
┌────────────────────┐  ┌─────────────────────────────┐
│  Layer 2: ioRunner │  │  Layer 3: executionRunner   │
│  (IO Plane)        │  │  (Execution Plane)          │
│  • Database reads  │  │  • User job handlers        │
│  • Database writes │  │  • Business logic           │
│  • File operations │  │  • May be slow/blocking     │
│  • Network calls   │  │                             │
│  • Sequential      │  │                             │
└────────────────────┘  └─────────────────────────────┘

Key Benefits:
1. Control operations never block on IO
2. Database write ordering guaranteed by ioRunner sequencing
3. No locks needed (SequencedTaskRunner provides serialization)
4. Clear separation of concerns
5. Easy to test and reason about

7.2 Data Modelsimport (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "PENDING"
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
	JobStatusFailed    JobStatus = "FAILED"
	JobStatusCanceled  JobStatus = "CANCELED"
)

type JobEntity struct {
	ID        string
	Type      string
	ArgsData  []byte
	Status    JobStatus
	Result    string
	Priority  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type JobFilter struct {
	Status   JobStatus // Empty means all
	Type     string
	Limit    int
	Offset   int
}

7.3 Core Structuretype RawJobHandler func(ctx context.Context, args []byte) error

type JobManager struct {
	// Three-Layer Runner Architecture
	controlRunner   TaskRunner  // Layer 1: Fast control operations
	ioRunner        TaskRunner  // Layer 2: Sequential IO operations
	executionRunner TaskRunner  // Layer 3: User job execution

	// Concurrent-safe data structures (lock-free)
	handlers   sync.Map // map[string]RawJobHandler
	activeJobs sync.Map // map[string]*activeJobInfo

	store      JobStore
	serializer JobSerializer

	closed atomic.Bool
}

type activeJobInfo struct {
	cancel    context.CancelFunc
	jobEntity *JobEntity
	startTime time.Time
	dbSaved   atomic.Bool  // Track if saved to DB
}

func NewJobManager(
	controlRunner TaskRunner,
	ioRunner TaskRunner,
	executionRunner TaskRunner,
	store JobStore,
	serializer JobSerializer,
) *JobManager {
	return &JobManager{
		controlRunner:   controlRunner,
		ioRunner:        ioRunner,
		executionRunner: executionRunner,
		store:           store,
		serializer:      serializer,
	}
}

7.4 Handler Registration (Layer 1: controlRunner)// Generic Registration Helper
type TypedHandler[T any] func(ctx context.Context, args T) error

func RegisterHandler[T any](m *JobManager, jobType string, handler TypedHandler[T]) error {
	if m.closed.Load() {
		return fmt.Errorf("JobManager is closed")
	}

	// Wrap handler with serialization logic
	adapter := func(ctx context.Context, rawArgs []byte) error {
		var args T
		if err := m.serializer.Deserialize(rawArgs, &args); err != nil {
			return fmt.Errorf("deserialize failed: %w", err)
		}
		return handler(ctx, args)
	}

	// Post to controlRunner (fast, pure memory operation)
	errChan := make(chan error, 1)
	m.controlRunner.PostTask(func(ctx context.Context) {
		m.handlers.Store(jobType, adapter)
		errChan <- nil
	})

	return <-errChan
}

7.5 Submit Job Flow (Layer 1 → Layer 2 → Layer 3)func (m *JobManager) SubmitJob(ctx context.Context, id string, jobType string, args any, traits TaskTraits) error {
	return m.SubmitDelayedJob(ctx, id, jobType, args, 0, traits)
}

func (m *JobManager) SubmitDelayedJob(
	ctx context.Context,
	id string,
	jobType string,
	args any,
	delay time.Duration,
	traits TaskTraits,
) error {
	if m.closed.Load() {
		return fmt.Errorf("JobManager is closed")
	}

	// Serialize args (CPU-bound, do outside runners)
	argsBytes, err := m.serializer.Serialize(args)
	if err != nil {
		return err
	}

	entity := &JobEntity{
		ID:        id,
		Type:      jobType,
		ArgsData:  argsBytes,
		Status:    JobStatusPending,
		Priority:  int(traits.Priority),
		CreatedAt: time.Now(),
	}

	// Layer 1: Fast control logic
	errChan := make(chan error, 1)
	m.controlRunner.PostTask(func(_ context.Context) {
		errChan <- m.submitJobControl(ctx, entity, traits, delay)
	})

	return <-errChan
}

// submitJobControl: Layer 1 - Fast validation and scheduling
func (m *JobManager) submitJobControl(
	ctx context.Context,
	entity *JobEntity,
	traits TaskTraits,
	delay time.Duration,
) error {
	// 1. Fast duplicate check (memory only)
	if _, exists := m.activeJobs.Load(entity.ID); exists {
		return fmt.Errorf("job %s is already active", entity.ID)
	}

	// 2. Get handler (fast memory lookup)
	rawHandler, ok := m.handlers.Load(entity.Type)
	if !ok {
		return fmt.Errorf("handler for job type %s not found", entity.Type)
	}
	handler := rawHandler.(RawJobHandler)

	// 3. Create context and add to activeJobs (fast memory operation)
	jobCtx, cancel := context.WithCancel(context.Background())
	info := &activeJobInfo{
		cancel:    cancel,
		jobEntity: entity,
		startTime: time.Now(),
	}
	m.activeJobs.Store(entity.ID, info)

	// 4. Delegate to Layer 2 (IO operations)
	m.submitJobIO(ctx, entity, jobCtx, handler, traits, delay, info)

	return nil
}

// submitJobIO: Layer 2 - Database operations (on ioRunner)
func (m *JobManager) submitJobIO(
	ctx context.Context,
	entity *JobEntity,
	jobCtx context.Context,
	handler RawJobHandler,
	traits TaskTraits,
	delay time.Duration,
	info *activeJobInfo,
) {
	m.ioRunner.PostTask(func(_ context.Context) {
		// 1. Check DB for duplicates (may be slow)
		existing, _ := m.store.GetJob(ctx, entity.ID)
		if existing != nil && (existing.Status == JobStatusPending || existing.Status == JobStatusRunning) {
			// Duplicate found, rollback activeJobs
			m.controlRunner.PostTask(func(_ context.Context) {
				m.activeJobs.Delete(entity.ID)
				info.cancel()
			})
			return
		}

		// 2. Save to DB (may be slow)
		if err := m.store.SaveJob(ctx, entity); err != nil {
			// Save failed, rollback activeJobs
			m.controlRunner.PostTask(func(_ context.Context) {
				m.activeJobs.Delete(entity.ID)
				info.cancel()
			})
			return
		}

		// 3. Mark as saved
		info.dbSaved.Store(true)

		// 4. Schedule execution on Layer 3
		m.scheduleExecution(entity, jobCtx, handler, traits, delay)
	})
}

// scheduleExecution: Layer 3 - Execute user handler
func (m *JobManager) scheduleExecution(
	entity *JobEntity,
	jobCtx context.Context,
	handler RawJobHandler,
	traits TaskTraits,
	delay time.Duration,
) {
	taskWrapper := func(_ context.Context) {
		// Pre-execution check
		if jobCtx.Err() != nil {
			m.finalizeJob(entity.ID, JobStatusCanceled, "Canceled before execution")
			return
		}

		// Update to RUNNING (via ioRunner)
		m.updateStatusIO(entity.ID, JobStatusRunning, "")

		// Execute handler
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			err = handler(jobCtx, entity.ArgsData)
		}()

		// Determine final status
		status := JobStatusCompleted
		msg := ""
		if err != nil {
			if jobCtx.Err() != nil {
				status = JobStatusCanceled
				msg = "Job canceled"
			} else {
				status = JobStatusFailed
				msg = err.Error()
			}
		}

		m.finalizeJob(entity.ID, status, msg)
	}

	if delay > 0 {
		m.executionRunner.PostDelayedTaskWithTraits(taskWrapper, delay, traits)
	} else {
		m.executionRunner.PostTaskWithTraits(taskWrapper, traits)
	}
}

7.6 Cancel Job (Layer 1: controlRunner - Fast)func (m *JobManager) CancelJob(id string) error {
	if m.closed.Load() {
		return fmt.Errorf("JobManager is closed")
	}

	errChan := make(chan error, 1)
	m.controlRunner.PostTask(func(_ context.Context) {
		errChan <- m.cancelJobControl(id)
	})

	return <-errChan
}

func (m *JobManager) cancelJobControl(id string) error {
	raw, ok := m.activeJobs.Load(id)
	if !ok {
		return fmt.Errorf("job %s is not active", id)
	}

	info := raw.(*activeJobInfo)
	info.cancel() // Fast context cancellation

	return nil
}

7.7 Finalize Job (Layer 1 → Layer 2)func (m *JobManager) finalizeJob(id string, status JobStatus, msg string) {
	// Step 1: Clean up activeJobs (fast, on controlRunner)
	m.controlRunner.PostTask(func(_ context.Context) {
		m.finalizeJobControl(id, status, msg)
	})
}

func (m *JobManager) finalizeJobControl(id string, status JobStatus, msg string) {
	// Clean up activeJobs (fast memory operation)
	raw, ok := m.activeJobs.LoadAndDelete(id)
	if !ok {
		return
	}

	info := raw.(*activeJobInfo)
	info.cancel() // Cancel context

	// Step 2: Update DB status (delegate to ioRunner)
	m.updateStatusIO(id, status, msg)
}

func (m *JobManager) updateStatusIO(id string, status JobStatus, msg string) {
	m.ioRunner.PostTask(func(_ context.Context) {
		ctx := context.Background()
		if err := m.store.UpdateStatus(ctx, id, status, msg); err != nil {
			fmt.Printf("Failed to update status for job %s: %v\n", id, err)
		}
	})
}

7.8 Read Operations (Direct Access - Lock-free)// ListJobs - Direct read from store (allows slight delay)
func (m *JobManager) ListJobs(ctx context.Context, filter JobFilter) ([]*JobEntity, error) {
	return m.store.ListJobs(ctx, filter)
}

// GetJob - Direct read from store
func (m *JobManager) GetJob(ctx context.Context, id string) (*JobEntity, error) {
	return m.store.GetJob(ctx, id)
}

// GetActiveJobCount - Direct read from sync.Map
func (m *JobManager) GetActiveJobCount() int {
	count := 0
	m.activeJobs.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetActiveJobs - Returns snapshot of active jobs
func (m *JobManager) GetActiveJobs() []*JobEntity {
	var jobs []*JobEntity
	m.activeJobs.Range(func(key, value interface{}) bool {
		info := value.(*activeJobInfo)
		jobs = append(jobs, info.jobEntity)
		return true
	})
	return jobs
}

7.9 Lifecycle Managementfunc (m *JobManager) Start(ctx context.Context) error {
	errChan := make(chan error, 1)

	m.controlRunner.PostTask(func(_ context.Context) {
		errChan <- m.startRecovery(ctx)
	})

	return <-errChan
}

func (m *JobManager) startRecovery(ctx context.Context) error {
	// Delegate IO operations to ioRunner
	m.ioRunner.PostTask(func(_ context.Context) {
		m.doRecoveryIO(ctx)
	})

	return nil
}

func (m *JobManager) doRecoveryIO(ctx context.Context) {
	// 1. Mark RUNNING jobs as FAILED (interrupted by restart)
	runningJobs, err := m.store.ListJobs(ctx, JobFilter{Status: JobStatusRunning})
	if err != nil {
		fmt.Printf("Failed to list running jobs: %v\n", err)
		return
	}

	for _, job := range runningJobs {
		m.store.UpdateStatus(ctx, job.ID, JobStatusFailed, "Interrupted by restart")
	}

	// 2. Recover PENDING jobs
	jobs, err := m.store.GetRecoverableJobs(ctx)
	if err != nil {
		fmt.Printf("Failed to get recoverable jobs: %v\n", err)
		return
	}

	// 3. Schedule recovered jobs (via controlRunner)
	for _, job := range jobs {
		jobCopy := job

		m.controlRunner.PostTask(func(_ context.Context) {
			// Get handler
			rawHandler, ok := m.handlers.Load(jobCopy.Type)
			if !ok {
				fmt.Printf("Handler not found for job type %s\n", jobCopy.Type)
				return
			}
			handler := rawHandler.(RawJobHandler)

			// Create context and add to activeJobs
			jobCtx, cancel := context.WithCancel(context.Background())
			info := &activeJobInfo{
				cancel:    cancel,
				jobEntity: jobCopy,
				startTime: time.Now(),
			}
			info.dbSaved.Store(true) // Already in DB
			m.activeJobs.Store(jobCopy.ID, info)

			// Schedule execution
			traits := TaskTraits{Priority: TaskPriority(jobCopy.Priority)}
			m.scheduleExecution(jobCopy, jobCtx, handler, traits, 0)
		})
	}
}

func (m *JobManager) Shutdown(ctx context.Context) error {
	if !m.closed.CompareAndSwap(false, true) {
		return fmt.Errorf("already closed")
	}

	// 1. Cancel all active jobs (via controlRunner - fast)
	doneChan := make(chan struct{})
	m.controlRunner.PostTask(func(_ context.Context) {
		m.activeJobs.Range(func(key, value interface{}) bool {
			info := value.(*activeJobInfo)
			info.cancel()
			return true
		})
		close(doneChan)
	})

	select {
	case <-doneChan:
	case <-ctx.Done():
		return ctx.Err()
	}

	// 2. Wait for activeJobs to drain
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if m.GetActiveJobCount() == 0 {
				// 3. Shutdown runners in order
				m.controlRunner.Shutdown()
				m.ioRunner.Shutdown()
				m.executionRunner.Shutdown()
				return nil
			}
		}
	}
}

7.10 Usage Examplefunc main() {
	// Create thread pool
	pool := taskrunner.NewGoroutineThreadPool("main-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	// Create three runners with clear separation
	controlRunner := core.NewSequencedTaskRunner(pool)
	controlRunner.SetName("control-runner")

	ioRunner := core.NewSequencedTaskRunner(pool)
	ioRunner.SetName("io-runner")

	executionRunner := core.NewSequencedTaskRunner(pool)
	executionRunner.SetName("execution-runner")

	// Create JobManager
	store := &MemoryJobStore{}
	serializer := &JSONSerializer{}
	jobMgr := NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	// Start recovery
	jobMgr.Start(context.Background())

	// Register handlers (fast, <100μs)
	RegisterHandler(jobMgr, "email", func(ctx context.Context, args EmailArgs) error {
		return sendEmail(args.To, args.Subject, args.Body)
	})

	// Submit jobs (fast control path, returns immediately)
	jobMgr.SubmitJob(ctx, "job-1", "email", EmailArgs{
		To:      "user@example.com",
		Subject: "Hello",
		Body:    "World",
	}, DefaultTaskTraits())

	// Cancel job (fast, <100μs)
	jobMgr.CancelJob("job-1")

	// Query jobs (allows slight delay)
	jobs, _ := jobMgr.ListJobs(ctx, JobFilter{Status: JobStatusPending})

	// Graceful shutdown
	jobMgr.Shutdown(context.Background())
}
Chapter 8: Storage Layer - JobStore & SerializerInterfaces to decouple storage and serialization logic.type JobStore interface {
	SaveJob(ctx context.Context, job *JobEntity) error
	UpdateStatus(ctx context.Context, id string, status JobStatus, result string) error
	GetRecoverableJobs(ctx context.Context) ([]*JobEntity, error)

	// Control Plane
	GetJob(ctx context.Context, id string) (*JobEntity, error)
	ListJobs(ctx context.Context, filter JobFilter) ([]*JobEntity, error)
}

type JobSerializer interface {
	Serialize(args any) ([]byte, error)
	Deserialize(data []byte, target any) error
	Name() string
}
Example: MemoryJobStoretype MemoryJobStore struct {
	data sync.Map 
}

func (s *MemoryJobStore) GetJob(ctx context.Context, id string) (*JobEntity, error) {
	if v, ok := s.data.Load(id); ok {
		return v.(*JobEntity), nil
	}
	return nil, fmt.Errorf("not found")
}

func (s *MemoryJobStore) ListJobs(ctx context.Context, filter JobFilter) ([]*JobEntity, error) {
	var list []*JobEntity
	count := 0
	skipped := 0

	s.data.Range(func(key, value interface{}) bool {
		job := value.(*JobEntity)
		
		// Filter logic
		if filter.Status != "" && job.Status != filter.Status { return true }
		if filter.Type != "" && job.Type != filter.Type { return true }

		// Pagination
		if skipped < filter.Offset {
			skipped++
			return true
		}
		if filter.Limit > 0 && count >= filter.Limit {
			return false
		}

		list = append(list, job)
		count++
		return true
	})
	return list, nil
}
Summary: System Flow (Three-Layer Architecture)graph TD
    User -->|SubmitDelayedJob| JobMgr

    subgraph "Layer 1: controlRunner (< 100μs)"
        JobMgr -->|Fast Check| ActiveMap[activeJobs sync.Map]
        JobMgr -->|Get Handler| HandlerMap[handlers sync.Map]
        JobMgr -->|Add Job| ActiveMap
    end

    ActiveMap -->|Delegate| IORunner

    subgraph "Layer 2: ioRunner (Sequential IO)"
        IORunner -->|Check Duplicate| DB[(JobStore)]
        IORunner -->|Save Job| DB
        IORunner -->|Update Status| DB
    end

    IORunner -->|Schedule| ExecRunner

    subgraph "Layer 3: executionRunner (User Jobs)"
        ExecRunner -->|Wait Delay| DelayMgr[DelayManager]
        DelayMgr -->|Time Up| Scheduler[TaskScheduler]
        Scheduler -->|Signal| Worker[Worker Goroutines]
        Worker -->|Execute| Handler[User Handler]
    end

    Handler -->|Finalize| ControlRunner2[controlRunner]
    ControlRunner2 -->|Clean Up| ActiveMap
    ControlRunner2 -->|Update Status| IORunner

    User -->|CancelJob| CancelControl[controlRunner]
    CancelControl -->|ctx.Cancel| ActiveMap
    CancelControl -->|Fast Return| User

    style JobMgr fill:#e1f5ff
    style ActiveMap fill:#fff4e1
    style IORunner fill:#ffe1e1
    style ExecRunner fill:#e1ffe1

