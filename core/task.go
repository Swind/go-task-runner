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
