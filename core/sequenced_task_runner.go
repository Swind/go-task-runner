package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type SequencedTaskRunner struct {
	threadPool    ThreadPool
	queue         TaskQueue
	mu            sync.Mutex
	isRunning     bool
	activeRunners int32 // atomic guard for concurrency assertion
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
	// Assertion: Ensure strictly one goroutine at a time
	if n := atomic.AddInt32(&r.activeRunners, 1); n > 1 {
		panic(fmt.Sprintf("SequencedTaskRunner: concurrent runLoop detected (count=%d)", n))
	}
	defer atomic.AddInt32(&r.activeRunners, -1)

	runCtx := context.WithValue(ctx, taskRunnerKey, r)

	// 1. Fetch SINGLE task
	item, ok := r.queue.Pop() // Changed from PopUpTo

	if !ok {
		// Queue completely empty
		r.mu.Lock()
		if r.queue.IsEmpty() {
			r.isRunning = false
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()

		var needRepost bool
		r.mu.Lock()
		if r.queue.IsEmpty() {
			r.isRunning = false
			needRepost = false
		} else {
			needRepost = true
		}
		r.mu.Unlock()

		if needRepost {
			nextTraits, _ := r.queue.PeekTraits() // Best effort peek
			r.rePostSelf(nextTraits)
		}
		return
	}

	// 2. Execute ONE task
	func() {
		defer func() { recover() }()
		item.Task(runCtx)
	}()

	// 3. Always repost if there are more tasks (Yield)
	// This ensures we yield to the Scheduler between every task
	var nextTraits TaskTraits
	var more bool

	r.mu.Lock()
	if r.queue.IsEmpty() {
		r.isRunning = false
		more = false
	} else {
		more = true
	}
	r.mu.Unlock()

	if more {
		nextTraits, _ = r.queue.PeekTraits()
		r.rePostSelf(nextTraits)
	}
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
