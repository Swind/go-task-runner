# Go Task Runner 架構評估報告

## 概述

本文檔對 Go Task Runner 項目的架構設計進行了全面評估，識別了潛在問題並提供改進建議。

## 架構優勢

### 1. 清晰的分層設計
- **三層 JobManager 架構**：控制層、IO 層、執行層分離清晰
- **TaskRunner 抽象**：統一的接口支持多種實現（Sequenced、SingleThread）
- **ThreadPool 接口**：解耦調度器和執行引擎

### 2. 靈活的任務模型
- **優先級系統**：BestEffort、UserVisible、UserBlocking 三級優先級
- **延遲任務**：支持延遲執行
- **重複任務**：支持定時重複任務
- **Task-and-Reply 模式**：支持跨 Runner 的數據傳遞

### 3. 並發安全設計
- **SequencedTaskRunner**：使用 CAS 原子操作和單一 runLoop 保證順序執行
- **同步容器**：sync.Map 用於併發安全的數據結構
- **零拷貝優化**：FIFO 隊列使用 slice 切片減少分配

---

## 潛在問題

### 1. SingleThreadTaskRunner 的 Channel 沒有背壓控制

**位置**: `core/single_thread_task_runner.go:48`

```go
workQueue: make(chan Task, 100), // 固定緩衝大小
```

**問題**:
- Channel 緩衝大小固定為 100，無法根據負載動態調整
- 大量快速提交任務時可能造成內存壓力
- 沒有背壓反饋機制，調用者無法知道隊列已滿

**影響**:
- 高負載情況下可能導致內存溢出
- 任務丟失或延遲不可預測

**建議**:
```go
type SingleThreadTaskRunner struct {
    workQueue      chan Task
    maxQueueSize  int        // 可配置最大隊列大小
    rejectedCount atomic.Int64 // 統計拒絕次數
}

// 增加 Drop 或 Wait 策略選項
type QueuePolicy int
const (
    QueuePolicyDrop QueuePolicy = iota
    QueuePolicyWait
    QueuePolicyReject
)

func (r *SingleThreadTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) error {
    select {
    case <-r.ctx.Done():
        return fmt.Errorf("runner closed")
    case r.workQueue <- task:
        return nil
    default:
        // 隊列滿，根據策略處理
        switch r.queuePolicy {
        case QueuePolicyDrop:
            r.rejectedCount.Add(1)
            return fmt.Errorf("queue full, task dropped")
        case QueuePolicyReject:
            return fmt.Errorf("queue full, rejected")
        case QueuePolicyWait:
            select {
            case <-r.ctx.Done():
                return fmt.Errorf("runner closed")
            case r.workQueue <- task:
                return nil
            }
        }
    }
}
```

---

### 2. JobManager 的錯誤處理不一致

**位置**: `core/job_manager.go:326-330`

```go
func (m *JobManager) updateStatusIO(id string, status JobStatus, msg string) {
    m.ioRunner.PostTask(func(_ context.Context) {
        ctx := context.Background()
        if err := m.store.UpdateStatus(ctx, id, status, msg); err != nil {
            // Log error but don't fail
            _ = err  // 錯誤被忽略！
        }
    })
}
```

**問題**:
- 狀態更新失敗被完全忽略
- 沒有重試機制
- 沒有通知機制告知調用者

**影響**:
- 數據庫狀態不一致
- 無法追蹤錯誤發生的原因

**建議**:
```go
type JobManagerConfig struct {
    ErrorHandler func(jobID string, err error)
    RetryPolicy RetryPolicy
}

type RetryPolicy struct {
    MaxRetries    int
    InitialDelay  time.Duration
    MaxDelay     time.Duration
    BackoffRatio float64
}

func (m *JobManager) updateStatusIO(id string, status JobStatus, msg string) {
    m.ioRunner.PostTask(func(_ context.Context) {
        var lastErr error
        for attempt := 0; attempt <= m.retryPolicy.MaxRetries; attempt++ {
            ctx := context.Background()
            if err := m.store.UpdateStatus(ctx, id, status, msg); err != nil {
                lastErr = err
                if attempt < m.retryPolicy.MaxRetries {
                    delay := calculateBackoff(attempt, m.retryPolicy)
                    time.Sleep(delay)
                    continue
                }
            } else {
                return // 成功
            }
        }
        // 所有重試都失敗
        if m.errorHandler != nil {
            m.errorHandler(id, fmt.Errorf("update status failed: %w", lastErr))
        }
    })
}
```

---

### 3. SequencedTaskRunner 的雙重檢查邏輯重複

**位置**: `core/sequenced_task_runner.go:116-177`

**問題**:
- `runLoop` 中有兩處類似的「空隊列檢查 → double-check → 可能有新任務」邏輯
- 代碼重複，容易出錯
- 維護困難

**影響**:
- 代碼可讀性差
- 容易引入 bug

**建議**:
```go
// 提取公共邏輯
func (r *SequencedTaskRunner) ensureRunLoopIfNeeded(traits TaskTraits) {
    r.queueMu.Lock()
    hasMore := !r.queue.IsEmpty()
    if hasMore {
        var nextTraits TaskTraits
        nextTraits, _ = r.queue.PeekTraits()
        r.queueMu.Unlock()
        r.ensureRunning(nextTraits)
    } else {
        r.queueMu.Unlock()
    }
}

func (r *SequencedTaskRunner) runLoop(ctx context.Context) {
    if atomic.LoadInt32(&r.runningCount) != 1 {
        panic("invalid runningCount")
    }

    runCtx := context.WithValue(ctx, taskRunnerKey, r)

    // 1. Fetch and execute task
    item, ok := r.fetchOneTask()
    if !ok {
        atomic.StoreInt32(&r.runningCount, 0)
        r.ensureRunLoopIfNeeded(DefaultTaskTraits())
        return
    }

    // 2. Execute with panic recovery
    r.executeWithRecovery(runCtx, item.Task)

    // 3. Schedule next if needed
    r.ensureRunLoopIfNeeded(r.peekTraits())
}
```

---

### 4. TaskScheduler Shutdown 語義不明確

**位置**: `core/task_scheduler.go:92-101`

```go
func (s *TaskScheduler) Shutdown() {
    atomic.StoreInt32(&s.shuttingDown, 1)
    s.delayManager.Stop()
    s.queue.Clear()  // 直接清空隊列！
}
```

**問題**:
- `queue.Clear()` 直接丟棄所有隊列中的任務
- 沒有提供優雅關閉選項
- 任務沒有機會執行清理邏輯

**影響**:
- 數據可能丟失
- 資源可能無法正確釋放

**建議**:
```go
type ShutdownMode int
const (
    ShutdownModeGraceful ShutdownMode = iota // 等待現有任務完成
    ShutdownModeForceful                  // 立即停止，丟棄隊列
)

func (s *TaskScheduler) Shutdown(mode ShutdownMode, timeout time.Duration) error {
    atomic.StoreInt32(&s.shuttingDown, 1)
    s.delayManager.Stop()

    if mode == ShutdownModeGraceful {
        // 等待隊列排空
        deadline := time.After(timeout)
        ticker := time.NewTicker(100 * time.Millisecond)
        defer ticker.Stop()

        for {
            select {
            case <-deadline:
                s.queue.Clear() // 超時，強制清空
                return fmt.Errorf("shutdown timeout, forced")
            case <-ticker.C:
                if s.QueuedTaskCount() == 0 && s.ActiveTaskCount() == 0 {
                    return nil
                }
            }
        }
    } else {
        // 立即清空
        s.queue.Clear()
        return nil
    }
}
```

---

### 5. DelayManager 的時間精度問題

**位置**: `core/delay_manager.go:96-136`

**問題**:
- 使用單個 `time.Timer` 管理所有延遲任務
- 高頻率添加/刪除延遲任務時，Timer 會頻繁重置
- 時間精度依賴於 OS 調度器和 Go runtime

**影響**:
- 大量延遲任務時性能下降
- 可能出現定時器抖動

**建議**:
- 考慮使用時間輪（Time Wheel）算法（如 Kafka 或 Netty 實現）
- 分桶管理：短期任務和長期任務分開管理

---

### 6. JobManager 的重複檢查競態條件

**位置**: `core/job_manager.go:149-152`

```go
// 1. Fast duplicate check (memory only)
if _, exists := m.activeJobs.Load(entity.ID); exists {
    return fmt.Errorf("job %s is already active", entity.ID)
}
```

**問題**:
- 只檢查 `activeJobs`，沒有檢查數據庫
- 如果程序重啟，數據庫中的 PENDING 任務會被重複執行

**影響**:
- 可能導致任務重複執行

**建議**:
```go
// submitJobControl 應該先查詢數據庫
func (m *JobManager) submitJobControl(
    ctx context.Context,
    entity *JobEntity,
    traits TaskTraits,
    delay time.Duration,
) error {
    // 1. 檢查 activeJobs（快速路徑）
    if _, exists := m.activeJobs.Load(entity.ID); exists {
        return fmt.Errorf("job %s is already active", entity.ID)
    }

    // 2. 檢查數據庫（通過 ioRunner 異步檢查）
    checkDone := make(chan bool, 1)
    m.ioRunner.PostTask(func(_ context.Context) {
        existing, _ := m.store.GetJob(ctx, entity.ID)
        if existing != nil && (existing.Status == JobStatusPending ||
                             existing.Status == JobStatusRunning) {
            checkDone <- true // 重複
        } else {
            checkDone <- false
        }
    })

    // 等待檢查結果
    if <-checkDone {
        return fmt.Errorf("job %s already in database", entity.ID)
    }

    // ... 繼續後續邏輯
}
```

---

### 7. 沒有監控和觀測性

**問題**:
- 缺少指標收集（metrics）
- 缺少分佈式追蹤（tracing）
- 缺少結構化日誌

**影響**:
- 生產環境難以診斷問題
- 無法監控系統健康狀況

**建議**:
```go
// 使用標準接口
type MetricsCollector interface {
    RecordTaskExecution(runnerID, taskType string, duration time.Duration, success bool)
    RecordQueueSize(runnerID string, size int)
    RecordTaskRejection(reason string)
}

type TracingConfig struct {
    Enabled bool
    // 可集成 OpenTelemetry
}

// 在 SequencedTaskRunner 中添加
func (r *SequencedTaskRunner) runLoop(ctx context.Context) {
    start := time.Now()
    // ... 執行任務 ...
    r.metrics.RecordTaskExecution(r.id, "task", time.Since(start), success)
}
```

---

### 8. TaskTraits.Category 沒有被使用

**位置**: `core/task.go:35`

```go
type TaskTraits struct {
    Priority TaskPriority
    MayBlock bool      // 沒有被使用！
    Category string    // 沒有被使用！
}
```

**問題**:
- `MayBlock` 字段定義了但從未使用
- `Category` 字段也沒有被利用
- 這些字段會讓使用者誤會它們有實際作用

**建議**:
```go
// 方案 1: 移除未使用的字段
type TaskTraits struct {
    Priority TaskPriority
}

// 方案 2: 實現它們的功能
func (s *TaskScheduler) GetWork(stopCh <-chan struct{}) (Task, bool) {
    for {
        for i := 0; i < 3; i++ {
            // 根據 MayBlock 調度到不同的 worker
            batch := s.queues[i].PopUpTo(1)
            if len(batch) > 0 {
                // 如果 MayBlock=true，使用 IO-dedicated workers
                // 否則使用 CPU workers
                return batch[0].Task, true
            }
        }
        // ...
    }
}
```

---

### 9. Context 傳播不完整

**位置**: 多處

**問題**:
- `JobManager` 在創建 job 時使用 `context.Background()`，而不是調用者的 context
- 任務執行時無法被外部取消

**影響**:
- 無法實現請求級別的超時控制
- 無實現取消傳播

**建議**:
```go
type JobEntity struct {
    // ...
    Ctx    context.Context
    Cancel context.CancelFunc
}

func (m *JobManager) SubmitJob(
    parentCtx context.Context, // 改為接收父 context
    id string,
    jobType string,
    args any,
    traits TaskTraits,
) error {
    // 使用父 context 創建 job context
    jobCtx, cancel := context.WithCancel(parentCtx)

    entity := &JobEntity{
        // ...
    }

    // ...
    info := &activeJobInfo{
        cancel:    cancel,
        jobEntity: entity,
        // ...
    }
}
```

---

### 10. PriorityTaskQueue 的序列號可能溢出

**位置**: `core/queue.go:208`

```go
nextSequence uint64
// ...
q.nextSequence++  // 沒有溢出處理
```

**問題**:
- `uint64` 理論上可能溢出（雖然極不可能）
- 溢出後，順序保證可能失效

**建議**:
```go
// 使用 int64 並檢測溢出
type PriorityTaskQueue struct {
    // ...
    nextSequence int64
}

func (q *PriorityTaskQueue) Push(t Task, traits TaskTraits) {
    q.mu.Lock()
    defer q.mu.Unlock()

    item := &priorityItem{
        TaskItem: TaskItem{Task: t, Traits: traits},
        sequence: q.nextSequence,
    }

    if q.nextSequence == math.MaxInt64 {
        // 溢出處理：重置為 0（假舊任務已執行完）
        // 或者使用更高精度的 ID
        q.nextSequence = 0
    } else {
        q.nextSequence++
    }

    heap.Push(&q.pq, item)
}
```

---

## 改進建議

### 高優先級

1. **實現 Shutdown 語義區分**
   - 增加 `ShutdownMode`：優雅關閉 vs 強制關閉
   - 支持超時控制

2. **改進錯誤處理**
   - JobManager 的狀態更新增加重試機制
   - 添加統一的錯誤處理和日誌

3. **增加監控能力**
   - 添加 metrics 收集接口
   - 支持結構化日誌

### 中優先級

4. **重構 SequencedTaskRunner**
   - 提取重複的雙重檢查邏輯
   - 簡化 runLoop 流程

5. **實現 Channel 背壓控制**
   - SingleThreadTaskRunner 的隊列策略可配置
   - 提供拒絕、丟棄、等待選項

6. **使用調用者的 Context**
   - JobManager 支持傳遞父 context
   - 實現取消傳播

### 低優先級

7. **實現未使用的字段**
   - `MayBlock` 和 `Category` 要麼實現功能，要麼移除
   - 清晰的 API 文檔說明

8. **優化 DelayManager**
   - 評估時間輪算法的適用性
   - 大量延遲任務時的性能優化

9. **序列號溢出處理**
   - 雖然不太可能發生，但應該防禦性編程

---

## 測試建議

### 1. 增加並發測試
- 測試高負載下的行為
- 測試競態條件
- 使用 `go test -race` 檢測數據競爭

### 2. 壓力測試
- 測試大量任務提交
- 測試大量延遲任務
- 測試內存使用

### 3. 崩潰恢復測試
- 測試 goroutine panic 後的恢復
- 測試進程重啟後的狀態恢復

### 4. 基準測試
- 測試各種操作的延遲
- 測試吞吐量

---

## 總結

Go Task Runner 的架構設計整體上非常優秀，清晰的三層架構和靈活的任務模型使其易於理解和使用。但存在一些可以改進的地方：

**需要立即處理的問題**:
- Shutdown 語義不明確
- JobManager 錯誤處理不一致

**中期改進方向**:
- 增加監控能力
- 改進錯誤處理和重試機制
- 實現背壓控制

**長期優化**:
- 延遲任務算法優化
- 更好的並發性能

建議優先解決高優先級問題，然後逐步實現其他改進建議。每次改進都應該有對應的測試覆蓋。
