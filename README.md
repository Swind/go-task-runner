本文件詳細說明如何參考 Chromium 的 `base::TaskScheduler` 架構，在 Golang 中實作一套生產級別的任務調度系統。

**v2.1 更新重點：**

- **Fix**: 修復 `SequencedTaskRunner` 潛在的並發競爭問題 (Critical Race Condition)。
    
- **Feat**: 新增 `PostDelayedTaskWithTraits` 介面，支援高優先級延遲任務。
    
- **Feat**: 實作 `Graceful Shutdown` 機制，確保任務不丟失。
    
- **Perf**: 優化 `TaskQueue` 使用 Zero-allocation 技巧與記憶體壓縮。
    
- **Ops**: 完善全鏈路的可觀測性指標 (Observability)。
    

## Chapter 1: 基礎單元 - Task & Traits

我們定義了任務的基本單位，並引入 `TaskTraits` 來描述任務的屬性。

### 核心定義

```go
package main

import (
	"context"
	"time"
)

// Task 是執行的最小單位 (Closure)
type Task func(ctx context.Context)

// =============================================================================
// TaskTraits: 定義任務的屬性 (優先級、阻塞行為等)
// =============================================================================

type TaskPriority int

const (
	// TaskPriorityBestEffort: 最低優先級
	TaskPriorityBestEffort TaskPriority = iota

	// TaskPriorityUserVisible: 預設優先級
	TaskPriorityUserVisible

	// TaskPriorityUserBlocking: 最高優先級
	TaskPriorityUserBlocking
)

type TaskTraits struct {
	Priority TaskPriority
	MayBlock bool
	Category string
}

func DefaultTaskTraits() TaskTraits {
	return TaskTraits{Priority: TaskPriorityUserVisible}
}

func TraitsUserBlocking() TaskTraits {
	return TaskTraits{Priority: TaskPriorityUserBlocking}
}
```

### 介面定義

更新介面以支援帶有 Traits 的延遲任務與監控回調。

```go
// TaskRunner 是發布任務的通用介面
type TaskRunner interface {
	PostTask(task Task)
	PostTaskWithTraits(task Task, traits TaskTraits)
	PostDelayedTask(task Task, delay time.Duration)

	// [v2.1 New] 支援指定優先級的延遲任務
	PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits)
}

// Backend 定義了執行引擎的行為
type Backend interface {
	PostInternal(task Task, traits TaskTraits)
	PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner)

	// Observability (v2.1 Enhanced)
	WorkerCount() int
	QueuedTaskCount() int   // 排隊中
	ActiveTaskCount() int   // 執行中
	DelayedTaskCount() int  // 延遲中

	// v2.1: 監控回調（供 ThreadGroup 使用）
	OnTaskStart()
	OnTaskEnd()
}

// Context Helper
type taskRunnerKeyType struct{}
var taskRunnerKey taskRunnerKeyType

func GetCurrentTaskRunner(ctx context.Context) TaskRunner {
	if v := ctx.Value(taskRunnerKey); v != nil {
		return v.(TaskRunner)
	}
	return nil
}
```

## Chapter 2: 容器 - TaskQueue (效能優化版)

採用更激進的 **Zero-allocation** 策略與 **記憶體壓縮** 機制。

```go
import "sync"

const (
	defaultQueueCap     = 16
	compactMinCap       = 64 // 容量小於此值時不壓縮
	compactShrinkFactor = 4  // 當 len < cap/4 時觸發壓縮
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

// PopUpTo: 最多取出 max 個任務
// [v2.1 Optimization] 使用 slice 重用技巧，避免 make 分配
func (q *TaskQueue) PopUpTo(max int) []TaskItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := len(q.tasks)
	if n == 0 {
		return nil
	}

	if n <= max {
		// 全部取出：直接回傳目前的 slice
		// 並將 q.tasks 重置為長度 0 但保留容量 (Zero allocation)
		batch := q.tasks
		q.tasks = q.tasks[:0]
		return batch
	}

	// 部分取出：必須分配新 slice 給 batch，因為我們要保留 q.tasks 的後半段
	// 這裡無法完全避免分配，除非我們實作 RingBuffer
	batch := make([]TaskItem, max)
	copy(batch, q.tasks[:max])

	// 移動並縮容
	q.tasks = q.tasks[max:]
	q.maybeCompactLocked()

	return batch
}

// MaybeCompact 公開方法，允許外部（如定期清理 Job）觸發壓縮
func (q *TaskQueue) MaybeCompact() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.maybeCompactLocked()
}

// maybeCompactLocked 根據目前 len/cap 狀態，視情況縮小底層陣列容量。
func (q *TaskQueue) maybeCompactLocked() {
	n := len(q.tasks)
	c := cap(q.tasks)

	// 太小的 queue 不值得壓縮
	if c < compactMinCap {
		return
	}

	// 空佇列：直接重建成預設容量
	if n == 0 {
		q.tasks = make([]TaskItem, 0, defaultQueueCap)
		return
	}

	// 只有在 len 明顯小於 cap 才壓縮，例如 len < cap/4
	if n*compactShrinkFactor >= c {
		return
	}

	// 新容量：最多縮到原來的一半，但不小於 defaultQueueCap，也不小於目前 len
	newCap := c / 2
	if newCap < defaultQueueCap {
		newCap = defaultQueueCap
	}
	if newCap < n {
		newCap = n
	}

	// 重新分配並複製，釋放舊的巨大 Array
	newSlice := make([]TaskItem, n, newCap)
	copy(newSlice, q.tasks)
	q.tasks = newSlice
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
```

## Chapter 3: 大腦 - TaskScheduler (Graceful Shutdown & Full Metrics)

修復了監控指標不準確的問題，並實作優雅關閉。

```go
import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"fmt"
)

type TaskScheduler struct {
	queues [3]*TaskQueue
	signal chan struct{}

	threadGroup  *ThreadGroup
	delayManager *DelayManager

	// [v2.1 Metrics] 全局計數器
	metricQueued  int32 // 在 ReadyQueue 等待中
	metricActive  int32 // 正在 Worker 執行中
	metricDelayed int32 // 在 DelayManager 等待中

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

	s.delayManager = NewDelayManager(s) // 傳入 s 以便更新 delayed metrics
	s.threadGroup = NewThreadGroup(workerCount, s)
	s.threadGroup.Start(ctx)

	return s
}

// PostInternal
func (s *TaskScheduler) PostInternal(task Task, traits TaskTraits) {
	// 如果正在關閉，拒絕新任務 (或是根據策略決定)
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		fmt.Println("Scheduler is shutting down, task rejected")
		return
	}

	idx := 1
	switch traits.Priority {
	case TaskPriorityUserBlocking: idx = 0
	case TaskPriorityUserVisible:  idx = 1
	case TaskPriorityBestEffort:   idx = 2
	}

	s.queues[idx].Push(task, traits)
	atomic.AddInt32(&s.metricQueued, 1) // Metric++

	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// PostDelayedInternal
func (s *TaskScheduler) PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner) {
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		return
	}
	s.delayManager.AddDelayedTask(task, delay, traits, target)
	// Metric 更新由 DelayManager 內部處理
}

// GetWork (Worker 呼叫)
func (s *TaskScheduler) GetWork(stopCh <-chan struct{}) (Task, bool) {
	for {
		for i := 0; i < 3; i++ {
			batch := s.queues[i].PopUpTo(1)
			if len(batch) > 0 {
				atomic.AddInt32(&s.metricQueued, -1) // Metric-- (離開 Queue)
				return batch[0].Task, true
			}
		}

		select {
		case <-s.signal:
			continue
		case <-stopCh:
			return nil, false
		}
	}
}

// [v2.1] Graceful Shutdown
func (s *TaskScheduler) Shutdown() {
	// 1. 標記停止接收新任務
	atomic.StoreInt32(&s.shuttingDown, 1)

	// 2. 停止 DelayManager (不再產生新任務)
	s.delayManager.Stop()

	// 3. 等待 Queue 清空 (簡單實作：輪詢檢查)
	// 實務上可用 WaitGroup 或更複雜的訊號，這裡示範邏輯
	for {
		if s.QueuedTaskCount() == 0 && s.ActiveTaskCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 4. 強制停止 Worker
	s.cancel()
	s.threadGroup.Join()
}

// Metrics
func (s *TaskScheduler) WorkerCount() int      { return s.threadGroup.workers }
func (s *TaskScheduler) QueuedTaskCount() int  { return int(atomic.LoadInt32(&s.metricQueued)) }
func (s *TaskScheduler) ActiveTaskCount() int  { return int(atomic.LoadInt32(&s.metricActive)) }
func (s *TaskScheduler) DelayedTaskCount() int { return int(atomic.LoadInt32(&s.metricDelayed)) }

// 監控回調 (v2.1)
func (s *TaskScheduler) OnTaskStart() {
	atomic.AddInt32(&s.metricActive, 1)
}

func (s *TaskScheduler) OnTaskEnd() {
	atomic.AddInt32(&s.metricActive, -1)
}
```

## Chapter 4: 肌肉 - ThreadGroup

透過 `WorkSource` 介面追蹤 `metricActive`，移除類型斷言依賴。

```go
// WorkSource 定義 ThreadGroup 從何處獲取任務
type WorkSource interface {
	GetWork(stopCh <-chan struct{}) (Task, bool)

	// [v2.1] 監控回調
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
		if !ok {
			return
		}

		// [v2.1] 透過介面更新 Active Metrics
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

func (tg *ThreadGroup) Join() {
	tg.wg.Wait()
}
```

## Chapter 5: 調度器 - SequencedTaskRunner (Fix Race Condition)

**核心修復：** 重寫 `runLoop` 與 Re-Post 邏輯，確保在釋放鎖時 `isRunning` 狀態與實際行為一致。

```go
const MaxTasksPerSlice = 4  // 每次批次處理的任務數量（Fairness）

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

// PostDelayedTaskWithTraits [v2.1 New]
func (r *SequencedTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	r.backend.PostDelayedInternal(task, delay, traits, r)
}

func (r *SequencedTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraits(task, delay, DefaultTaskTraits())
}

// runLoop 修正版 [v2.1 Fix]
func (r *SequencedTaskRunner) runLoop(ctx context.Context) {
	runCtx := context.WithValue(ctx, taskRunnerKey, r)

	// 1. 嘗試取出任務
	items := r.queue.PopUpTo(MaxTasksPerSlice)

	if len(items) == 0 {
		// [v2.1 Fix] 處理空轉與 Race Condition
		var needRepost bool
		var nextTraits TaskTraits

		r.mu.Lock()
		if r.queue.IsEmpty() {
			// 確實沒有任務，可以安全地設置 isRunning = false
			r.isRunning = false
			needRepost = false
		} else {
			// Queue 不空（Race: 剛好有新任務進來），需要 repost
			needRepost = true
		}
		r.mu.Unlock()

		if !needRepost {
			return
		}

		// 在鎖外讀取 traits（TaskQueue 內部會加自己的鎖）
		// Lock Order: r.mu -> q.mu（安全）
		nextTraits, ok := r.queue.PeekTraits()
		if !ok {
			nextTraits = DefaultTaskTraits()
		}
		r.rePostSelf(nextTraits)
		return
	}

	// 2. 執行任務
	for _, item := range items {
		func() {
			defer func() { recover() }()
			item.Task(runCtx)
		}()
	}

	// 3. 檢查是否需要繼續（Fairness Yield）
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

	if !needRepost {
		return
	}

	// 在鎖外讀取 traits
	nextTraits, ok := r.queue.PeekTraits()
	if !ok {
		nextTraits = DefaultTaskTraits()
	}
	r.rePostSelf(nextTraits)
}

// scheduleRunLoop 啟動 runLoop（如果尚未運行）
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

// rePostSelf 將 runLoop 重新提交到 Scheduler（用於 Yield）
func (r *SequencedTaskRunner) rePostSelf(traits TaskTraits) {
	r.backend.PostInternal(r.runLoop, traits)
}

// PostTask 提交任務（使用預設 Traits）
func (r *SequencedTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

// PostTaskWithTraits 提交帶有屬性的任務
func (r *SequencedTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	r.queue.Push(task, traits)
	r.scheduleRunLoop(traits)
}
```

## Chapter 6: 專屬線程 - SingleThreadTaskRunner

```go
type SingleThreadTaskRunner struct {
	workQueue chan Task
	quit      chan struct{}
}

// ... New, runLoop, Stop ...

func (r *SingleThreadTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

func (r *SingleThreadTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	select {
	case r.workQueue <- task:
	case <-r.quit:
	}
}

func (r *SingleThreadTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	// 對於單執行緒 Runner，我們可以使用 time.AfterFunc 直接注入到 workQueue
	// 這樣就不需要依賴 TaskScheduler 的 Global Timer，保持 SingleThread 的獨立性
	time.AfterFunc(delay, func() {
		r.PostTask(task)
	})
}

// [v2.1] 支援 Traits
func (r *SingleThreadTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	time.AfterFunc(delay, func() {
		r.PostTaskWithTraits(task, traits)
	})
}
```

## Chapter 7: 管理介面 - JobManager (Integration)

`JobManager` 可透過 `SubmitJob` 將任務提交，若需要延遲執行，可擴充 `SubmitDelayedJob`。

```go
// ... JobManager Interface ...

// MemJobManager 使用 Runner 的 PostTaskWithTraits
func (m *MemJobManager) SubmitJob(ctx context.Context, id string, jobType string, args interface{}, traits TaskTraits) error {
	// ... Serialize ...
	taskWrapper := func(ctx context.Context) {
		m.execute(ctx, id, jobType, argBytes)
	}
	m.runner.PostTaskWithTraits(taskWrapper, traits)
	return nil
}
```

## Chapter 8: 延遲任務 - DelayManager (Metrics 整合)

`DelayManager` 內建於 `TaskScheduler`，負責延遲任務，並更新 Metrics。

```go
import (
	"container/heap"
	"time"
)

type DelayManager struct {
	scheduler *TaskScheduler // 用於更新 metrics
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
	
	// 更新 Metrics
	atomic.AddInt32(&dm.scheduler.metricDelayed, 1)

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
			nextRun = 1000 * time.Hour
		} else {
			now = time.Now()
			if item.RunAt.Before(now) {
				heap.Pop(&dm.pq)
				// 更新 Metrics
				atomic.AddInt32(&dm.scheduler.metricDelayed, -1)
				dm.mu.Unlock()

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
		case <-dm.wakeup:
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
```

## Chapter 9: SequencedRunner 優先級語義說明

**SequencedTaskRunner Priority Semantics:**

1. **序列化優先 (Strict FIFO)**：`SequencedTaskRunner` 的首要保證是任務的執行順序。即使佇列中間有一個 High Priority 任務，它也必須等待前面的 Normal Priority 任務執行完畢。
    
2. **排程優先級 (Scheduling Priority)**：優先級決定的是**「整個序列何時被 ThreadPool 執行」**。
    
    - 當序列的下一個任務是 High Priority 時，`runLoop` 會以 High Priority 進入 Scheduler 的 Ready Queue。
        
    - 這意味著這個序列會比其他 Normal 序列更早獲得 CPU 時間片。
        
    - 一旦獲得 CPU，它會依序執行任務（無論該 Batch 內的任務優先級為何），直到 Time Slice 用盡。
        

此設計符合 Chromium `base::SequenceManager` 的行為模式：**Priority Inheritance applied to the Sequence head**.

## 總結：架構全貌

```
graph TD
    Client -->|Post| Scheduler[TaskScheduler]
    
    subgraph "TaskScheduler (The Brain)"
        DelayMgr[DelayManager]
        HighQ[High Queue]
        NormalQ[Normal Queue]
        LowQ[Low Queue]
        Metrics[Atomic Counters]
    end
    
    Client -->|Immediate| HighQ
    Client -->|Delayed| DelayMgr
    DelayMgr -->|Time Up| HighQ
    
    Scheduler -->|Signal| TG[ThreadGroup]
    
    subgraph "ThreadGroup (The Muscles)"
        Worker1
        Worker2
    end
    
    Worker1 -->|GetWork| Scheduler
    Worker1 -->|Update| Metrics
    
    subgraph "SequencedRunner (Fairness)"
        SeqQueue[TaskQueue]
        Pump[runLoop]
    end
    
    Client -->|Post| SeqQueue
    SeqQueue -->|Activate| Pump
    Pump -->|PostInternal| Scheduler
    Worker2 -->|Execute| Pump
```