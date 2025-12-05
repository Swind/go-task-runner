package domain

import (
	"fmt"
	"sync/atomic"
	"time"
)

type TaskScheduler struct {
	queue       TaskQueue
	signal      chan struct{}
	workerCount int

	delayManager *DelayManager

	metricQueued  int32 // Waiting in ReadyQueue
	metricActive  int32 // Executing in Worker
	metricDelayed int32 // Waiting in DelayManager

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

// PostInternal
func (s *TaskScheduler) PostInternal(task Task, traits TaskTraits) {
	// If shutting down, reject new tasks (or decide based on policy)
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		fmt.Println("Scheduler is shutting down, task rejected")
		return
	}

	s.queue.Push(task, traits)
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
	// Metric update handled inside DelayManager
}

// GetWork (Called by Worker)
func (s *TaskScheduler) GetWork(stopCh <-chan struct{}) (Task, bool) {
	for {
		// Try to pop one task
		if item, ok := s.queue.Pop(); ok {
			atomic.AddInt32(&s.metricQueued, -1) // Metric-- (Left Queue)
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

func (s *TaskScheduler) Shutdown() {
	// 1. Mark as shutting down to stop accepting new tasks
	atomic.StoreInt32(&s.shuttingDown, 1)

	// 2. Stop DelayManager (no more new tasks generated)
	s.delayManager.Stop()
}

// Metrics
func (s *TaskScheduler) WorkerCount() int     { return s.workerCount }
func (s *TaskScheduler) QueuedTaskCount() int { return int(atomic.LoadInt32(&s.metricQueued)) }
func (s *TaskScheduler) ActiveTaskCount() int { return int(atomic.LoadInt32(&s.metricActive)) }
func (s *TaskScheduler) DelayedTaskCount() int {
	return int(atomic.LoadInt32(&s.metricDelayed))
}

func (s *TaskScheduler) OnTaskStart() {
	atomic.AddInt32(&s.metricActive, 1)
}

func (s *TaskScheduler) OnTaskEnd() {
	atomic.AddInt32(&s.metricActive, -1)
}
