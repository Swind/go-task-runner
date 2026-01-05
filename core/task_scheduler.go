package core

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

	metricQueued int32 // Waiting in ReadyQueue
	metricActive int32 // Executing in Worker

	// Handlers and Metrics
	panicHandler        PanicHandler
	metrics             Metrics
	rejectedTaskHandler RejectedTaskHandler

	// Lifecycle
	shuttingDown int32 // atomic flag
}

func NewPriorityTaskScheduler(workerCount int) *TaskScheduler {
	return NewPriorityTaskSchedulerWithConfig(workerCount, DefaultTaskSchedulerConfig())
}

func NewPriorityTaskSchedulerWithConfig(workerCount int, config *TaskSchedulerConfig) *TaskScheduler {
	s := &TaskScheduler{
		signal:      make(chan struct{}, workerCount*2),
		workerCount: workerCount,
	}

	// Use single PriorityTaskQueue which handles priority ordering internally
	s.queue = NewPriorityTaskQueue()
	s.delayManager = NewDelayManager()

	// Apply config
	if config != nil {
		s.panicHandler = config.PanicHandler
		s.metrics = config.Metrics
		s.rejectedTaskHandler = config.RejectedTaskHandler
	}

	// Use defaults if not provided
	if s.panicHandler == nil {
		s.panicHandler = &DefaultPanicHandler{}
	}
	if s.metrics == nil {
		s.metrics = &NilMetrics{}
	}
	if s.rejectedTaskHandler == nil {
		s.rejectedTaskHandler = &DefaultRejectedTaskHandler{}
	}

	return s
}

func NewFIFOTaskScheduler(workerCount int) *TaskScheduler {
	return NewFIFOTaskSchedulerWithConfig(workerCount, DefaultTaskSchedulerConfig())
}

func NewFIFOTaskSchedulerWithConfig(workerCount int, config *TaskSchedulerConfig) *TaskScheduler {
	s := &TaskScheduler{
		signal:      make(chan struct{}, workerCount*2),
		workerCount: workerCount,
	}

	// Use single FIFOTaskQueue which handles priority ordering internally
	s.queue = NewFIFOTaskQueue()
	s.delayManager = NewDelayManager()

	// Apply config
	if config != nil {
		s.panicHandler = config.PanicHandler
		s.metrics = config.Metrics
		s.rejectedTaskHandler = config.RejectedTaskHandler
	}

	// Use defaults if not provided
	if s.panicHandler == nil {
		s.panicHandler = &DefaultPanicHandler{}
	}
	if s.metrics == nil {
		s.metrics = &NilMetrics{}
	}
	if s.rejectedTaskHandler == nil {
		s.rejectedTaskHandler = &DefaultRejectedTaskHandler{}
	}

	return s
}

// PostInternal
func (s *TaskScheduler) PostInternal(task Task, traits TaskTraits) {
	// If shutting down, reject new tasks (or decide based on policy)
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		s.rejectedTaskHandler.HandleRejectedTask("TaskScheduler", "shutting down")
		s.metrics.RecordTaskRejected("TaskScheduler", "shutting down")
		return
	}

	s.queue.Push(task, traits)
	atomic.AddInt32(&s.metricQueued, 1) // Metric++

	select {
	case s.signal <- struct{}{}:
	default:
		// Signal channel full, but task is already queued
		// This is not an error, just a optimization hint
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

	// 3. Clear queue to release all task references (including runLoop bound methods)
	s.queue.Clear()
}

// ShutdownGraceful waits for all queued and active tasks to complete
// Returns error if timeout is exceeded before tasks complete
func (s *TaskScheduler) ShutdownGraceful(timeout time.Duration) error {
	// 1. Mark as shutting down to stop accepting new tasks
	atomic.StoreInt32(&s.shuttingDown, 1)

	// 2. Stop DelayManager (no more new tasks generated)
	s.delayManager.Stop()

	// 3. Wait for queues to drain and active tasks to complete
	deadline := time.After(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			// Timeout exceeded, force clear remaining queues
			s.queue.Clear()
			return fmt.Errorf("shutdown graceful timeout after %v, forced clearing", timeout)
		case <-ticker.C:
			// Check if all work is done
			if s.QueuedTaskCount() == 0 && s.ActiveTaskCount() == 0 {
				return nil
			}
		}
	}
}

// Metrics
func (s *TaskScheduler) WorkerCount() int     { return s.workerCount }
func (s *TaskScheduler) QueuedTaskCount() int { return int(atomic.LoadInt32(&s.metricQueued)) }
func (s *TaskScheduler) ActiveTaskCount() int { return int(atomic.LoadInt32(&s.metricActive)) }
func (s *TaskScheduler) DelayedTaskCount() int {
	return s.delayManager.TaskCount()
}

func (s *TaskScheduler) OnTaskStart() {
	atomic.AddInt32(&s.metricActive, 1)
}

func (s *TaskScheduler) OnTaskEnd() {
	atomic.AddInt32(&s.metricActive, -1)
}

// GetPanicHandler returns the panic handler for this scheduler
func (s *TaskScheduler) GetPanicHandler() PanicHandler {
	return s.panicHandler
}

// GetMetrics returns the metrics collector for this scheduler
func (s *TaskScheduler) GetMetrics() Metrics {
	return s.metrics
}
