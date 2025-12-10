package core

import (
	"context"
	"time"
)

// Task is the unit of work (Closure)
type Task func(ctx context.Context)

// =============================================================================
// TaskTraits: Define task attributes (priority, blocking behavior, etc.)
// =============================================================================

type TaskPriority int

const (
	// TaskPriorityBestEffort: Lowest priority
	TaskPriorityBestEffort TaskPriority = iota

	// TaskPriorityUserVisible: Default priority
	TaskPriorityUserVisible

	// TaskPriorityUserBlocking: Highest priority
	// `UserBlocking` means the task may block the main thread.
	// If main thread is blocked, the UI will be unresponsive.
	// The user experience will be affected if the task blocks the main thread.
	TaskPriorityUserBlocking
)

type TaskTraits struct {
	Priority TaskPriority
	MayBlock bool
	Category string
}

func DefaultTaskTraits() TaskTraits {
	return TaskTraits{Priority: TaskPriorityUserVisible}
}

func TraitsUserBlocking() TaskTraits {
	return TaskTraits{Priority: TaskPriorityUserBlocking}
}

func TraitsBestEffort() TaskTraits {
	return TaskTraits{Priority: TaskPriorityBestEffort}
}

func TraitsUserVisible() TaskTraits {
	return TaskTraits{Priority: TaskPriorityUserVisible}
}

type TaskRunner interface {
	PostTask(task Task)
	PostTaskWithTraits(task Task, traits TaskTraits)
	PostDelayedTask(task Task, delay time.Duration)

	// [v2.1 New] Support delayed tasks with specific traits
	PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits)

	// [v2.2 New] Support repeating tasks
	PostRepeatingTask(task Task, interval time.Duration) RepeatingTaskHandle
	PostRepeatingTaskWithTraits(task Task, interval time.Duration, traits TaskTraits) RepeatingTaskHandle
	PostRepeatingTaskWithInitialDelay(task Task, initialDelay, interval time.Duration, traits TaskTraits) RepeatingTaskHandle

	// [v2.3 New] Support task and reply pattern
	// PostTaskAndReply executes task on this runner, then posts reply to replyRunner
	PostTaskAndReply(task Task, reply Task, replyRunner TaskRunner)
	// PostTaskAndReplyWithTraits allows specifying traits for both task and reply
	PostTaskAndReplyWithTraits(task Task, taskTraits TaskTraits, reply Task, replyTraits TaskTraits, replyRunner TaskRunner)

	// Lifecycle management
	Shutdown()
	IsClosed() bool
}

// =============================================================================
// RepeatingTaskHandle: Control repeating task lifecycle
// =============================================================================

// RepeatingTaskHandle controls the lifecycle of a repeating task.
type RepeatingTaskHandle interface {
	// Stop stops the repeating task. It will not interrupt a currently executing task,
	// but will prevent future executions from being scheduled.
	Stop()

	// IsStopped returns true if the task has been stopped.
	IsStopped() bool
}

// =============================================================================
// Context Helper
// =============================================================================
type taskRunnerKeyType struct{}

var taskRunnerKey taskRunnerKeyType

func GetCurrentTaskRunner(ctx context.Context) TaskRunner {
	if v := ctx.Value(taskRunnerKey); v != nil {
		return v.(TaskRunner)
	}
	return nil
}

// =============================================================================
// Task and Reply Pattern with Generic Return Values
// =============================================================================

// TaskWithResult defines a task that returns a result of type T and an error.
// This is used with PostTaskAndReplyWithResult to pass data from task to reply.
type TaskWithResult[T any] func(ctx context.Context) (T, error)

// ReplyWithResult defines a reply callback that receives a result of type T and an error.
// This is the counterpart to TaskWithResult, receiving the values returned by the task.
type ReplyWithResult[T any] func(ctx context.Context, result T, err error)
