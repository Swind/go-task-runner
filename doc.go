// Package taskrunner provides a Chromium-inspired task scheduling architecture for Go.
//
// This library implements a threading model where developers post tasks to virtual threads
// (TaskRunners) rather than managing goroutines directly. The core design is inspired by
// Chromium's Threading and Tasks system.
//
// # Quick Start
//
// Initialize the global thread pool at application startup:
//
//	taskrunner.InitGlobalThreadPool(4) // 4 workers
//	defer taskrunner.ShutdownGlobalThreadPool()
//
// Create a SequencedTaskRunner for sequential task execution:
//
//	runner := taskrunner.CreateTaskRunner(core.DefaultTaskTraits())
//	runner.PostTask(func(ctx context.Context) {
//		// Your code here - guaranteed sequential execution
//	})
//
// # Key Concepts
//
// TaskRunner: Interface for posting tasks. Tasks posted to a SequencedTaskRunner
// execute sequentially, eliminating the need for locks on resources owned by that runner.
//
// TaskTraits: Describes task attributes including priority (BestEffort, UserVisible, UserBlocking).
// Priority determines when the sequence gets scheduled, not the order within a sequence.
//
// GoroutineThreadPool: The execution engine managing worker goroutines that pull
// and execute tasks from the scheduler.
//
// # Thread Safety
//
// SequencedTaskRunner provides strict FIFO execution guarantees with runtime assertions.
// Tasks within a sequence never run concurrently, allowing lock-free programming
// for resources owned by that sequence.
//
// # Example
//
//	import (
//		"context"
//		taskrunner "github.com/Swind/go-task-runner"
//		"github.com/Swind/go-task-runner/core"
//	)
//
//	func main() {
//		taskrunner.InitGlobalThreadPool(4)
//		defer taskrunner.ShutdownGlobalThreadPool()
//
//		runner := taskrunner.CreateTaskRunner(core.DefaultTaskTraits())
//
//		// Tasks execute sequentially
//		runner.PostTask(func(ctx context.Context) {
//			println("Task 1")
//		})
//		runner.PostTask(func(ctx context.Context) {
//			println("Task 2")
//		})
//
//		// Delayed task
//		runner.PostDelayedTask(func(ctx context.Context) {
//			println("Task 3 - delayed")
//		}, 1*time.Second)
//	}
//
// For more details, see https://github.com/Swind/go-task-runner
package taskrunner
