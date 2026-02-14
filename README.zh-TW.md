# Go Task Runner（繁體中文）

[![CI](https://github.com/Swind/go-task-runner/workflows/CI/badge.svg)](https://github.com/Swind/go-task-runner/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Swind/go-task-runner)](https://goreportcard.com/report/github.com/Swind/go-task-runner)
![Go Version](https://img.shields.io/badge/Go-1.24%2B-%2300ADD8?logo=go)
[![License](https://img.shields.io/github/license/Swind/go-task-runner)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Swind/go-task-runner)](https://github.com/Swind/go-task-runner/releases/latest)

[English README](README.md)

### ⚠️ **免責聲明：僅供實驗 / 教學用途**

此函式庫屬於**實驗性實作**，主要用於教學與測試，**不建議直接用於正式生產環境**。

這是一個受 **Chromium Threading and Tasks** 設計啟發的 Go 多執行緒任務架構。

參考資料：[Threading and Tasks in Chrome](https://chromium.googlesource.com/chromium/src/+/main/docs/threading_and_tasks.md)

## 設計理念

此專案的核心是：把工作提交到虛擬執行緒（`TaskRunner`），而非直接手動管理 goroutine / channel。  
透過這層抽象，應用程式可以專注在業務邏輯，並讓底層統一處理併發細節。

核心概念如下：

- **Task Runners over Threads**：通常不直接建立裸 goroutine，而是建立 `TaskRunner` 並提交任務。
- **Sequential Consistency (Strands)**：`SequencedTaskRunner` 保證同一 runner 內任務序列化執行，可對該序列擁有的狀態採用 lock-free 寫法。
- **Task Traits**：描述任務「是什麼」（例如 `UserBlocking`、`BestEffort`），而不是描述「怎麼跑」。
- **Concurrency Safety**：透過執行期檢查維持序列規則，降低常見 race condition 風險。

## 功能特色

- **Goroutine Thread Pool**：提供高效率 worker pool。
- **Sequenced Task Runner**：同一序列中嚴格 FIFO。
- **Single Thread Task Runner**：固定 goroutine 執行，提供 thread affinity。
- **Delayed Tasks**：支援延遲排程。
- **Repeating Tasks**：固定間隔重複執行，並可停止。
- **Task and Reply Pattern**：在 A runner 執行 task，完成後在 B runner 回呼 reply（含型別安全回傳）。
- **Task Traits**：支援優先級導向排程。

## 安裝

```bash
go get github.com/Swind/go-task-runner
```

## 使用方式

說明：以下 code 片段聚焦在 API 重點；完整可執行程式請參考 `examples/*`。

重要同步說明：
- 當狀態只由單一 `SequencedTaskRunner`/`SingleThreadTaskRunner` 擁有並存取時，可採用 lock-free 寫法。
- 當資料跨 goroutine 或跨 runner 傳遞時，請使用明確同步（`WaitIdle`、`WaitShutdown`、channel 或 `sync/atomic`）。

### 1. 初始化全域 Thread Pool

```go
package main

import (
    "context"

    taskrunner "github.com/Swind/go-task-runner"
)

func main() {
    taskrunner.InitGlobalThreadPool(4)
    defer taskrunner.ShutdownGlobalThreadPool()

    runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
    _ = runner
}
```

### 2. 使用 SequencedTaskRunner

`SequencedTaskRunner` 是建議的預設模型。同一 runner 的任務會依序執行，適合保護同一份狀態並減少 lock 使用。

```go
runner.PostTask(func(ctx context.Context) {
    println("Doing work...")
})

runner.PostDelayedTask(func(ctx context.Context) {
    println("Runs 1 second later...")
}, 1*time.Second)
```

### 2.1 使用 SingleThreadTaskRunner

`SingleThreadTaskRunner` 保證任務都在同一個 dedicated goroutine 執行，適合：

- 阻塞式 IO
- 需要 TLS 的 CGO 呼叫
- 模擬 UI thread

```go
runner := taskrunner.NewSingleThreadTaskRunner()
defer runner.Stop()

runner.PostTask(func(ctx context.Context) {
    data := blockingRead()
    process(data)
})

runner.PostDelayedTask(func(ctx context.Context) {
    println("Runs on same goroutine after delay")
}, 1*time.Second)
```

更多可參考：[`examples/single_thread/main.go`](examples/single_thread/main.go)

### 2.2 使用 PostTaskAndReply 模式

`PostTaskAndReply` 可在背景 runner 執行工作，完成後把 reply 發回指定 runner（常見於 UI/背景工作切換）。

```go
uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
bgRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

uiRunner.PostTask(func(ctx context.Context) {
    me := taskrunner.GetCurrentTaskRunner(ctx)

    bgRunner.PostTaskAndReply(
        func(ctx context.Context) {
            loadDataFromServer()
        },
        func(ctx context.Context) {
            updateUI()
        },
        me,
    )
})
```

**帶回傳值（Generic）**

`PostTaskAndReplyWithResult` 目前位於 `core` package，請先匯入：

```go
import "github.com/Swind/go-task-runner/core"
```

再呼叫：

```go
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (*UserData, error) {
        return fetchUserFromDB(ctx)
    },
    func(ctx context.Context, user *UserData, err error) {
        if err != nil {
            showError(err)
            return
        }
        updateUserUI(user)
    },
    uiRunner,
)
```

更多可參考：[`examples/task_and_reply/main.go`](examples/task_and_reply/main.go)

### 3. 使用 Task Traits（優先級）

Priority 效果通常在「多個 runner 競爭同一個 worker pool」時最明顯。  
可參考：[`examples/mixed_priority/main.go`](examples/mixed_priority/main.go)

```go
runner.PostTaskWithTraits(func(ctx context.Context) {
    println("High priority work!")
}, taskrunner.TaskTraits{
    Priority: taskrunner.TaskPriorityUserBlocking,
})
```

### 4. Repeating Tasks（週期任務）

```go
handle := runner.PostRepeatingTask(func(ctx context.Context) {
    println("Runs every second")
}, 1*time.Second)
defer handle.Stop()

handle2 := runner.PostRepeatingTaskWithInitialDelay(
    task,
    2*time.Second,
    1*time.Second,
    taskrunner.DefaultTaskTraits(),
)
defer handle2.Stop()
```

更多可參考：[`examples/repeating_task/main.go`](examples/repeating_task/main.go)

### 5. 關閉與清理

```go
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

runner.Shutdown()

if runner.IsClosed() {
    println("Runner is closed")
}
```

更多可參考：[`examples/shutdown/main.go`](examples/shutdown/main.go)

## 範例程式

- [`examples/basic_sequence/main.go`](examples/basic_sequence/main.go)：基本序列執行與延遲任務。
- [`examples/delayed_task/main.go`](examples/delayed_task/main.go)：延遲任務排程。
- [`examples/repeating_task/main.go`](examples/repeating_task/main.go)：週期任務與停止機制。
- [`examples/task_and_reply/main.go`](examples/task_and_reply/main.go)：Task-Reply 模式（含 `core` 的 generic 回傳 helper）。
- [`examples/single_thread/main.go`](examples/single_thread/main.go)：單一執行緒 affinity 與同擁有者 lock-free 狀態管理。
- [`examples/mixed_priority/main.go`](examples/mixed_priority/main.go)：多 runner 競爭下的優先級行為。
- [`examples/shutdown/main.go`](examples/shutdown/main.go)：runner 生命週期與關閉語意。
- [`examples/custom_handlers/main.go`](examples/custom_handlers/main.go)：自訂 panic/metrics/rejection handler。
- [`examples/event_bus/main.go`](examples/event_bus/main.go)：以 sequenced runner 實作 event bus。
- [`examples/parallel_tasks/main.go`](examples/parallel_tasks/main.go)：parallel runner 功能與併發上限示範。

## 架構文件

深入架構請見 [`docs/DESIGN.md`](docs/DESIGN.md)，包含 `TaskScheduler`、`DelayManager`、`TaskQueue` 的互動說明。

## 授權

MIT License，詳見 [`LICENSE`](LICENSE)。
