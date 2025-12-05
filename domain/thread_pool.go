package domain

import (
	"context"
	"time"
)

type WorkSource interface {
	GetWork(stopCh <-chan struct{}) (Task, bool)
}

// =============================================================================
// ThreadPool: Define task execution interface
// =============================================================================
type ThreadPool interface {
	PostInternal(task Task, traits TaskTraits)
	PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner)

	Start(ctx context.Context)
	Stop()

	ID() string
	IsRunning() bool

	WorkerCount() int
	QueuedTaskCount() int  // In queue
	ActiveTaskCount() int  // Executing
	DelayedTaskCount() int // Delayed
}
