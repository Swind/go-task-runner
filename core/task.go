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

// =============================================================================
// TaskRunner: Define task submission interface
// =============================================================================
type TaskRunner interface {
	PostTask(task Task)
	PostTaskWithTraits(task Task, traits TaskTraits)
	PostDelayedTask(task Task, delay time.Duration)

	// [v2.1 New] Support delayed tasks with specific traits
	PostDelayedTaskWithTraits(task Task, delay time.Duration, traits TaskTraits)
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
