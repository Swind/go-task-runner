package core

import "time"

// TaskExecutionRecord captures a completed task execution event.
type TaskExecutionRecord struct {
	TaskID     TaskID
	Name       string
	RunnerName string
	RunnerType string
	Priority   TaskPriority
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Panicked   bool
}

// RunnerStats represents runtime observability state for a task runner.
type RunnerStats struct {
	Name           string
	Type           string
	Pending        int
	Running        int
	Rejected       int64
	Closed         bool
	BarrierPending bool
	LastTaskName   string
	LastTaskAt     time.Time
}

// PoolStats represents runtime observability state for a thread pool.
type PoolStats struct {
	ID      string
	Workers int
	Queued  int
	Active  int
	Delayed int
	Running bool
}
