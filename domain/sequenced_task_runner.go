package domain

import (
	"context"
	"sync"
	"time"
)

const MaxTasksPerSlice = 4 // Process up to 4 tasks per slice (Fairness)

type SequencedTaskRunner struct {
	threadPool ThreadPool
	queue      TaskQueue
	mu         sync.Mutex
	isRunning  bool
}

func NewSequencedTaskRunner(threadPool ThreadPool) *SequencedTaskRunner {
	return &SequencedTaskRunner{
		threadPool: threadPool,
		queue:      NewFIFOTaskQueue(),
	}
}

func (r *SequencedTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	r.threadPool.PostDelayedInternal(task, delay, traits, r)
}

func (r *SequencedTaskRunner) PostDelayedTask(task Task, delay time.Duration) {
	r.PostDelayedTaskWithTraits(task, delay, DefaultTaskTraits())
}

func (r *SequencedTaskRunner) runLoop(ctx context.Context) {
	runCtx := context.WithValue(ctx, taskRunnerKey, r)

	// 1. Try to fetch tasks
	items := r.queue.PopUpTo(MaxTasksPerSlice)

	if len(items) == 0 {
		// Handle spin and Race Condition
		var needRepost bool
		var nextTraits TaskTraits

		r.mu.Lock()
		if r.queue.IsEmpty() {
			// Indeed empty, allow to set isRunning = false
			r.isRunning = false
			needRepost = false
		} else {
			// Queue not empty (Race: new task just arrived), need repost
			needRepost = true
		}
		r.mu.Unlock()

		if !needRepost {
			return
		}

		// Read traits outside lock (TaskQueue has its own lock)
		// Lock Order: r.mu -> q.mu (Safe)
		nextTraits, ok := r.queue.PeekTraits()
		if !ok {
			nextTraits = DefaultTaskTraits()
		}
		r.rePostSelf(nextTraits)
		return
	}

	// 2. Execute tasks
	for _, item := range items {
		func() {
			defer func() { recover() }()
			item.Task(runCtx)
		}()
	}

	// 3. Check if need to continue (Fairness Yield)
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

	nextTraits, ok := r.queue.PeekTraits()
	if !ok {
		nextTraits = DefaultTaskTraits()
	}
	r.rePostSelf(nextTraits)
}

// scheduleRunLoop starts runLoop (if not already running)
func (r *SequencedTaskRunner) scheduleRunLoop(traits TaskTraits) {
	r.mu.Lock()
	if !r.isRunning {
		r.isRunning = true
		r.mu.Unlock()
		r.threadPool.PostInternal(r.runLoop, traits)
	} else {
		r.mu.Unlock()
	}
}

// rePostSelf re-submits runLoop to Scheduler (for Yield)
func (r *SequencedTaskRunner) rePostSelf(traits TaskTraits) {
	r.threadPool.PostInternal(r.runLoop, traits)
}

// PostTask submits task (using default Traits)
func (r *SequencedTaskRunner) PostTask(task Task) {
	r.PostTaskWithTraits(task, DefaultTaskTraits())
}

// PostTaskWithTraits submits task with traits
func (r *SequencedTaskRunner) PostTaskWithTraits(task Task, traits TaskTraits) {
	r.queue.Push(task, traits)
	r.scheduleRunLoop(traits)
}
