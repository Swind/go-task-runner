package taskrunner

import "github.com/Swind/go-task-runner/core"

// Re-export commonly used types from core package for convenience.
// This allows users to import only the taskrunner package for most use cases.

// Task is the unit of work (Closure)
type Task = core.Task

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

// RepeatingTaskHandle controls the lifecycle of a repeating task
type RepeatingTaskHandle = core.RepeatingTaskHandle

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

// ThreadPool is re-exported for type compatibility
type ThreadPool = core.ThreadPool

// TaskWithResult and ReplyWithResult for generic PostTaskAndReply pattern
type TaskWithResult[T any] = core.TaskWithResult[T]
type ReplyWithResult[T any] = core.ReplyWithResult[T]

// GetCurrentTaskRunner retrieves the current TaskRunner from context
var GetCurrentTaskRunner = core.GetCurrentTaskRunner
