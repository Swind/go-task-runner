package taskrunner_test

import (
	"context"
	"fmt"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

// ExampleCreateTaskRunner demonstrates the basic usage with only one import.
func ExampleCreateTaskRunner() {
	// Initialize global thread pool
	taskrunner.InitGlobalThreadPool(2)
	defer taskrunner.ShutdownGlobalThreadPool()

	// Create a sequenced task runner
	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	done := make(chan struct{})

	// Post sequential tasks
	runner.PostTask(func(ctx context.Context) {
		fmt.Println("Task 1")
	})

	runner.PostTask(func(ctx context.Context) {
		fmt.Println("Task 2")
	})

	runner.PostTask(func(ctx context.Context) {
		fmt.Println("Task 3")
		close(done)
	})

	<-done
	time.Sleep(10 * time.Millisecond) // Allow output to flush

	// Output:
	// Task 1
	// Task 2
	// Task 3
}

// ExampleTaskTraits demonstrates using task priorities with a single import.
func ExampleTaskTraits() {
	taskrunner.InitGlobalThreadPool(1)
	defer taskrunner.ShutdownGlobalThreadPool()

	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	done := make(chan struct{})

	// High priority task
	runner.PostTaskWithTraits(func(ctx context.Context) {
		fmt.Println("High priority")
	}, taskrunner.TaskTraits{
		Priority: taskrunner.TaskPriorityUserBlocking,
	})

	// Default priority task
	runner.PostTask(func(ctx context.Context) {
		fmt.Println("Normal priority")
		close(done)
	})

	<-done
	time.Sleep(10 * time.Millisecond)

	// Output:
	// High priority
	// Normal priority
}
