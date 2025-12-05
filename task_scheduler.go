package main

import (
	"fmt"
	"swind/go-task-runner/domain"
	"sync/atomic"
	"time"
)

type PriorityTaskScheduler struct {
	queues      [3]domain.TaskQueue
	signal      chan struct{}
	workerCount int

	delayManager *domain.DelayManager

	metricQueued  int32 // Waiting in ReadyQueue
	metricActive  int32 // Executing in Worker
	metricDelayed int32 // Waiting in DelayManager

	// Lifecycle
	shuttingDown int32 // atomic flag
}

func NewPriorityTaskScheduler(workerCount int) *PriorityTaskScheduler {
	s := &PriorityTaskScheduler{
		signal:      make(chan struct{}, workerCount*2),
		workerCount: workerCount,
	}

	for i := 0; i < 3; i++ {
		s.queues[i] = domain.NewFIFOTaskQueue()
	}

	s.delayManager = domain.NewDelayManager()
	return s
}

// PostInternal
func (s *PriorityTaskScheduler) PostInternal(task domain.Task, traits domain.TaskTraits) {
	// If shutting down, reject new tasks (or decide based on policy)
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		fmt.Println("Scheduler is shutting down, task rejected")
		return
	}

	idx := 1
	switch traits.Priority {
	case domain.TaskPriorityUserBlocking:
		idx = 0
	case domain.TaskPriorityUserVisible:
		idx = 1
	case domain.TaskPriorityBestEffort:
		idx = 2
	}

	s.queues[idx].Push(task, traits)
	atomic.AddInt32(&s.metricQueued, 1) // Metric++

	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// PostDelayedInternal
func (s *PriorityTaskScheduler) PostDelayedInternal(task domain.Task, delay time.Duration, traits domain.TaskTraits, target domain.TaskRunner) {
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		return
	}
	s.delayManager.AddDelayedTask(task, delay, traits, target)
	// Metric update handled inside DelayManager
}

// GetWork (Called by Worker)
func (s *PriorityTaskScheduler) GetWork(stopCh <-chan struct{}) (domain.Task, bool) {
	for {
		for i := 0; i < 3; i++ {
			batch := s.queues[i].PopUpTo(1)
			if len(batch) > 0 {
				atomic.AddInt32(&s.metricQueued, -1) // Metric-- (Left Queue)
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

func (s *PriorityTaskScheduler) Shutdown() {
	// 1. Mark as shutting down to stop accepting new tasks
	atomic.StoreInt32(&s.shuttingDown, 1)

	// 2. Stop DelayManager (no more new tasks generated)
	s.delayManager.Stop()
}

// Metrics
func (s *PriorityTaskScheduler) WorkerCount() int     { return s.workerCount }
func (s *PriorityTaskScheduler) QueuedTaskCount() int { return int(atomic.LoadInt32(&s.metricQueued)) }
func (s *PriorityTaskScheduler) ActiveTaskCount() int { return int(atomic.LoadInt32(&s.metricActive)) }
func (s *PriorityTaskScheduler) DelayedTaskCount() int {
	return int(atomic.LoadInt32(&s.metricDelayed))
}

func (s *PriorityTaskScheduler) OnTaskStart() {
	atomic.AddInt32(&s.metricActive, 1)
}

func (s *PriorityTaskScheduler) OnTaskEnd() {
	atomic.AddInt32(&s.metricActive, -1)
}
