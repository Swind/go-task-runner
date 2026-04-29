// Package main demonstrates basic go-task-runner setup with global thread pool
package main

import (
	"context"
	"fmt"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

func main() {
	// Step 1: Initialize global thread pool
	// Worker count should match your CPU cores or workload needs
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	// Step 2: Create a task runner
	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
	defer runner.Shutdown()

	// Step 3: Post tasks
	fmt.Println("Posting tasks...")

	// Basic task
	runner.PostTask(func(ctx context.Context) {
		fmt.Println("Task 1 executed")
	})

	// Delayed task
	runner.PostDelayedTask(func(ctx context.Context) {
		fmt.Println("Task 2 executed after 100ms delay")
	}, 100*time.Millisecond)

	// Task with context
	runner.PostTask(func(ctx context.Context) {
		// Get current runner from context
		me := taskrunner.GetCurrentTaskRunner(ctx)
		fmt.Printf("Task 3 executing on runner: %s\n", me.Name())
	})

	// Wait for tasks to complete
	time.Sleep(200 * time.Millisecond)
	fmt.Println("All tasks completed")
}
