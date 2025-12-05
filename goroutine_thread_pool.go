package main

import (
	"context"
	"fmt"
	"sync"
)

// GoroutineThreadPool 管理一組 worker goroutines
// 負責從 WorkSource 拉取任務並執行
type GoroutineThreadPool struct {
	id        string
	workers   int
	scheduler *PriorityTaskScheduler
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.RWMutex
}

// NewThreadGroup 創建一個新的 ThreadGroup
func NewThreadGroup(id string, workers int) *GoroutineThreadPool {
	return &GoroutineThreadPool{
		id:        id,
		workers:   workers,
		scheduler: NewPriorityTaskScheduler(workers),
	}
}

// NewThreadGroupDefault 創建一個使用預設 ID 的 ThreadGroup
func NewThreadGroupDefault(workers int) *GoroutineThreadPool {
	return NewThreadGroup(fmt.Sprintf("pool-%d", workers), workers)
}

// Start 啟動所有 worker goroutines
func (tg *GoroutineThreadPool) Start(ctx context.Context) {
	tg.runningMu.Lock()
	defer tg.runningMu.Unlock()

	if tg.running {
		return // 已經在運行
	}

	tg.ctx, tg.cancel = context.WithCancel(ctx)
	tg.running = true

	for i := 0; i < tg.workers; i++ {
		tg.wg.Add(1)
		go tg.workerLoop(i, tg.ctx)
	}
}

// Stop 停止線程池
func (tg *GoroutineThreadPool) Stop() {
	tg.runningMu.Lock()
	if !tg.running {
		tg.runningMu.Unlock()
		return
	}
	tg.runningMu.Unlock()

	// Shutdown scheduler to stop accepting new tasks
	tg.scheduler.Shutdown()
	if tg.cancel != nil {
		tg.cancel()
	}
	tg.Join()

	tg.runningMu.Lock()
	tg.running = false
	tg.runningMu.Unlock()
}

// ID 返回線程池的 ID
func (tg *GoroutineThreadPool) ID() string {
	return tg.id
}

// IsRunning 返回線程池是否正在運行
func (tg *GoroutineThreadPool) IsRunning() bool {
	tg.runningMu.RLock()
	defer tg.runningMu.RUnlock()
	return tg.running
}

// workerLoop 是每個 worker 的主循環
func (tg *GoroutineThreadPool) workerLoop(id int, ctx context.Context) {
	defer tg.wg.Done()
	stopCh := ctx.Done()

	for {
		// 從 WorkSource 拉取任務
		task, ok := tg.scheduler.GetWork(stopCh)
		if !ok {
			// WorkSource 已關閉或 context 已取消
			return
		}

		// 透過介面更新 Active Metrics
		tg.scheduler.OnTaskStart()

		// 執行任務，並捕獲 panic
		func() {
			defer func() {
				tg.scheduler.OnTaskEnd()
				if r := recover(); r != nil {
					// TODO: 可以增加更好的錯誤處理，例如回調
					fmt.Printf("[Worker %d] Panic: %v\n", id, r)
				}
			}()
			task(ctx)
		}()
	}
}

// Join 等待所有 worker goroutines 結束
func (tg *GoroutineThreadPool) Join() {
	tg.wg.Wait()
}

// WorkerCount 返回 worker 數量
func (tg *GoroutineThreadPool) WorkerCount() int {
	return tg.workers
}
