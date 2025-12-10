package core

import (
	"context"
	"fmt"
	"time"
)

// =============================================================================
// PostTaskAndReply Internal Helpers
// =============================================================================

// postTaskAndReplyInternalWithTraits is the core implementation for PostTaskAndReply pattern.
// It wraps the task and reply to ensure proper execution order:
// 1. Execute task on targetRunner
// 2. If task completes successfully (no panic), post reply to replyRunner
//
// This function allows specifying different traits for task and reply separately.
func postTaskAndReplyInternalWithTraits(
	targetRunner TaskRunner,
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	if replyRunner == nil {
		// No reply runner specified, just execute the task
		targetRunner.PostTaskWithTraits(task, taskTraits)
		return
	}

	wrappedTask := func(ctx context.Context) {
		// Track whether task panicked
		panicked := true

		// Execute task with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[TaskAndReply] Task panicked, reply will not run: %v\n", r)
				}
			}()
			task(ctx)
			panicked = false
		}()

		// If task completed successfully, post reply to replyRunner
		if !panicked {
			replyRunner.PostTaskWithTraits(reply, replyTraits)
		}
	}

	targetRunner.PostTaskWithTraits(wrappedTask, taskTraits)
}

// postTaskAndReplyInternal is a simplified version that uses the same traits for both task and reply.
func postTaskAndReplyInternal(
	targetRunner TaskRunner,
	task Task,
	reply Task,
	replyRunner TaskRunner,
	traits TaskTraits,
) {
	postTaskAndReplyInternalWithTraits(
		targetRunner,
		task,
		traits,
		reply,
		DefaultTaskTraits(), // Reply uses default traits
		replyRunner,
	)
}

// =============================================================================
// Generic PostTaskAndReply with Result
// =============================================================================

// PostTaskAndReplyWithResult executes a task that returns a result of type T and an error,
// then passes that result to a reply callback on the replyRunner.
//
// This function uses closure capture to safely pass the result across goroutines.
// The captured variables (result and err) will escape to the heap, ensuring thread safety.
//
// Execution guarantee (Happens-Before):
// - The task ALWAYS completes before the reply starts
// - The reply ALWAYS sees the final values written by the task
// - This is guaranteed by the sequential execution in wrappedTask
//
// Example:
//
//	PostTaskAndReplyWithResult(
//	    backgroundRunner,
//	    func(ctx context.Context) (int, error) {
//	        return len("Hello"), nil
//	    },
//	    func(ctx context.Context, length int, err error) {
//	        fmt.Printf("Length: %d\n", length)
//	    },
//	    uiRunner,
//	)
func PostTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	reply ReplyWithResult[T],
	replyRunner TaskRunner,
) {
	PostTaskAndReplyWithResultAndTraits(
		targetRunner,
		task,
		DefaultTaskTraits(),
		reply,
		DefaultTaskTraits(),
		replyRunner,
	)
}

// PostTaskAndReplyWithResultAndTraits is the full-featured version that allows specifying
// different traits for the task and reply separately.
//
// This is useful when:
// - Task is background work (BestEffort) but reply is UI update (UserVisible/UserBlocking)
// - Task has different priority requirements than the reply
//
// Example:
//
//	PostTaskAndReplyWithResultAndTraits(
//	    backgroundRunner,
//	    func(ctx context.Context) (*UserData, error) {
//	        return fetchUserFromDB(ctx)
//	    },
//	    TraitsBestEffort(),        // Background work, low priority
//	    func(ctx context.Context, user *UserData, err error) {
//	        updateUI(user)
//	    },
//	    TraitsUserVisible(),       // UI update, higher priority
//	    uiRunner,
//	)
func PostTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	taskTraits TaskTraits,
	reply ReplyWithResult[T],
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	// Declare shared variables to capture result and error
	// These will escape to heap due to closure capture, ensuring thread safety
	var result T
	var err error

	// Wrap task: executes and captures result/error
	wrappedTask := func(ctx context.Context) {
		result, err = task(ctx)
	}

	// Wrap reply: receives result/error
	// By the Happens-Before guarantee, when this executes, the variables
	// have been written by wrappedTask, so access is safe
	wrappedReply := func(ctx context.Context) {
		reply(ctx, result, err)
	}

	// Use the internal helper to handle execution order
	postTaskAndReplyInternalWithTraits(
		targetRunner,
		wrappedTask,
		taskTraits,
		wrappedReply,
		replyTraits,
		replyRunner,
	)
}

// =============================================================================
// Delayed Task and Reply
// =============================================================================

// PostDelayedTaskAndReplyWithResult is similar to PostTaskAndReplyWithResult,
// but delays the execution of the task.
//
// The reply is NOT delayed - it executes immediately after the task completes.
// Only the initial task execution is delayed by the specified duration.
//
// Example:
//
//	PostDelayedTaskAndReplyWithResult(
//	    runner,
//	    func(ctx context.Context) (string, error) {
//	        return "delayed result", nil
//	    },
//	    2*time.Second,  // Wait 2 seconds before starting task
//	    func(ctx context.Context, result string, err error) {
//	        fmt.Println(result)  // Executes immediately after task completes
//	    },
//	    replyRunner,
//	)
func PostDelayedTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	delay time.Duration,
	reply ReplyWithResult[T],
	replyRunner TaskRunner,
) {
	PostDelayedTaskAndReplyWithResultAndTraits(
		targetRunner,
		task,
		delay,
		DefaultTaskTraits(),
		reply,
		DefaultTaskTraits(),
		replyRunner,
	)
}

// PostDelayedTaskAndReplyWithResultAndTraits is the full-featured delayed version
// with separate traits for task and reply.
func PostDelayedTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task TaskWithResult[T],
	delay time.Duration,
	taskTraits TaskTraits,
	reply ReplyWithResult[T],
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	var result T
	var err error

	wrappedTask := func(ctx context.Context) {
		result, err = task(ctx)
	}

	wrappedReply := func(ctx context.Context) {
		reply(ctx, result, err)
	}

	// Create a delayed task that will execute task then reply
	delayedWrapper := func(ctx context.Context) {
		// Execute task
		panicked := true
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[DelayedTaskAndReply] Task panicked: %v\n", r)
				}
			}()
			wrappedTask(ctx)
			panicked = false
		}()

		// Post reply if task succeeded
		if !panicked && replyRunner != nil {
			replyRunner.PostTaskWithTraits(wrappedReply, replyTraits)
		}
	}

	// Post the delayed wrapper
	targetRunner.PostDelayedTaskWithTraits(delayedWrapper, delay, taskTraits)
}
