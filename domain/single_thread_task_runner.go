package domain

import (
	"context"
	"time"
)

type SingleThreadTaskRunner struct {
	workQueue chan Task
	quit      chan struct{}
}

func NewSingleThreadTaskRunner() *SingleThreadTaskRunner {
	return &SingleThreadTaskRunner{
		workQueue: make(chan Task, 16),
		quit:      make(chan struct{}),
	}
}

func (r *SingleThreadTaskRunner) Start() {
	go r.runLoop()
}

func (r *SingleThreadTaskRunner) Stop() {
	close(r.quit)
}

func (r *SingleThreadTaskRunner) runLoop() {
	for {
		select {
		case task := <-r.workQueue:
			func() {
				defer func() { recover() }()
				task(context.Background())
			}()
		case <-r.quit:
			return
		}
	}
}

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
	// For SingleThreadRunner, we can use time.AfterFunc to inject directly into workQueue
	// This avoids dependency on TaskScheduler's Global Timer, keeping SingleThread independent
	time.AfterFunc(delay, func() {
		r.PostTask(task)
	})
}

// [v2.1] Support Traits
func (r *SingleThreadTaskRunner) PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits) {
	time.AfterFunc(delay, func() {
		r.PostTaskWithTraits(task, traits)
	})
}
