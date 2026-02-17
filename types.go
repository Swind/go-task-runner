package taskrunner

import (
	"github.com/Swind/go-task-runner/core"
)

// Re-export commonly used types from core package for convenience.
// This allows users to import only the taskrunner package for most use cases.

// Task is the unit of work (Closure)
type Task = core.Task

// TaskID is a unique identifier for tasks
type TaskID = core.TaskID

// TaskTraits defines task attributes (priority, blocking behavior, etc.)
type TaskTraits = core.TaskTraits

// TaskPriority defines the priority levels for tasks
type TaskPriority = core.TaskPriority

// TaskRunner is the interface for posting tasks
type TaskRunner = core.TaskRunner

// SequencedTaskRunner ensures sequential execution of tasks
type SequencedTaskRunner = core.SequencedTaskRunner

// SingleThreadTaskRunner ensures all tasks execute on the same dedicated goroutine
type SingleThreadTaskRunner = core.SingleThreadTaskRunner

// ParallelTaskRunner executes up to maxConcurrency tasks simultaneously
type ParallelTaskRunner = core.ParallelTaskRunner

// RepeatingTaskHandle controls the lifecycle of a repeating task
type RepeatingTaskHandle = core.RepeatingTaskHandle

// Observability snapshot types
type RunnerStats = core.RunnerStats
type PoolStats = core.PoolStats
type TaskExecutionRecord = core.TaskExecutionRecord

// Priority constants
const (
	TaskPriorityBestEffort   TaskPriority = core.TaskPriorityBestEffort
	TaskPriorityUserVisible  TaskPriority = core.TaskPriorityUserVisible
	TaskPriorityUserBlocking TaskPriority = core.TaskPriorityUserBlocking
)

// Convenience functions for creating TaskTraits
var (
	DefaultTaskTraits  = core.DefaultTaskTraits
	TraitsUserBlocking = core.TraitsUserBlocking
	TraitsBestEffort   = core.TraitsBestEffort
	TraitsUserVisible  = core.TraitsUserVisible
)

// NewSequencedTaskRunner creates a new SequencedTaskRunner with the given thread pool.
// This is re-exported for advanced users who want to create runners with custom pools.
func NewSequencedTaskRunner(pool ThreadPool) *SequencedTaskRunner {
	return core.NewSequencedTaskRunner(pool)
}

// NewSingleThreadTaskRunner creates a new SingleThreadTaskRunner with a dedicated goroutine.
// Use this for blocking IO operations, CGO calls with thread-local storage, or UI thread simulation.
func NewSingleThreadTaskRunner() *SingleThreadTaskRunner {
	return core.NewSingleThreadTaskRunner()
}

// NewParallelTaskRunner creates a new ParallelTaskRunner with the specified concurrency limit.
// This runner executes up to maxConcurrency tasks simultaneously.
// Panics if maxConcurrency is less than 1.
func NewParallelTaskRunner(threadPool ThreadPool, maxConcurrency int) *ParallelTaskRunner {
	return core.NewParallelTaskRunner(threadPool, maxConcurrency)
}

// ThreadPool is re-exported for type compatibility
type ThreadPool = core.ThreadPool

// TaskWithResult and ReplyWithResult for generic PostTaskAndReply pattern
type TaskWithResult[T any] = core.TaskWithResult[T]
type ReplyWithResult[T any] = core.ReplyWithResult[T]

// GetCurrentTaskRunner retrieves the current TaskRunner from context
var GetCurrentTaskRunner = core.GetCurrentTaskRunner

// GlobalThreadPool returns the global thread pool instance as a ThreadPool interface.
// This is a convenience wrapper for use with ParallelTaskRunner.
// It panics if InitGlobalThreadPool has not been called.
func GlobalThreadPool() ThreadPool {
	return GetGlobalThreadPool()
}
