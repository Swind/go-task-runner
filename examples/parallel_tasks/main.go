package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

func main() {
	// 1. Initialize the Global Thread Pool
	taskrunner.InitGlobalThreadPool(8)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== ParallelTaskRunner Example ===")

	// Example 1: Basic Parallel Execution with Concurrency Limit
	basicParallelExample()

	// Example 2: Priority Queue
	priorityExample()

	// Example 3: FlushAsync
	flushAsyncExample()

	// Example 4: Repeating Tasks
	repeatingTaskExample()

	// Example 5: Task and Reply Pattern
	taskAndReplyExample()

	fmt.Println("\n=== All Examples Finished ===")
}

// basicParallelExample demonstrates parallel execution with a concurrency limit
func basicParallelExample() {
	fmt.Println("--- Example 1: Basic Parallel Execution ---")

	// Create a ParallelTaskRunner with max 3 concurrent tasks
	runner := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		3, // maxConcurrency: only 3 tasks run simultaneously
	)
	defer runner.Shutdown()

	var wg sync.WaitGroup
	var mu sync.Mutex
	executionOrder := []string{}

	// Post 10 tasks, but only 3 will run at a time
	for i := 1; i <= 10; i++ {
		id := i
		wg.Add(1)
		runner.PostTask(func(ctx context.Context) {
			defer wg.Done()

			mu.Lock()
			executionOrder = append(executionOrder, fmt.Sprintf("Task%d", id))
			mu.Unlock()

			fmt.Printf("Task %d started\n", id)
			time.Sleep(200 * time.Millisecond) // Simulate work
			fmt.Printf("Task %d completed\n", id)
		})
	}

	// Wait for all tasks to complete
	runner.FlushAsync(func() {
		fmt.Println("FlushAsync callback: All 10 tasks completed!")
	})

	wg.Wait()

	fmt.Printf("Execution order: %v\n\n", executionOrder)
}

// priorityExample demonstrates priority-based task scheduling
func priorityExample() {
	fmt.Println("--- Example 2: Priority Queue ---")

	runner := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		2, // Only 2 tasks at a time to see priority effect
	)
	defer runner.Shutdown()

	// Post tasks with different priorities
	for _, task := range []struct {
		name     string
		priority taskrunner.TaskPriority
	}{
		{"LowPriority-1", taskrunner.TaskPriorityBestEffort},
		{"HighPriority-1", taskrunner.TaskPriorityUserBlocking},
		{"LowPriority-2", taskrunner.TaskPriorityBestEffort},
		{"NormalPriority-1", taskrunner.TaskPriorityUserVisible},
		{"HighPriority-2", taskrunner.TaskPriorityUserBlocking},
	} {
		task := task // capture loop variable
		runner.PostTaskWithTraits(func(ctx context.Context) {
			fmt.Printf("Running: %s\n", task.name)
			time.Sleep(100 * time.Millisecond)
		}, taskrunner.TaskTraits{Priority: task.priority})
	}

	runner.WaitIdle(context.Background())
	fmt.Println("All priority tasks completed")
}

// flushAsyncExample demonstrates FlushAsync barrier semantics
func flushAsyncExample() {
	fmt.Println("--- Example 3: FlushAsync ---")

	runner := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		4,
	)
	defer runner.Shutdown()

	var mu sync.Mutex
	events := []string{}

	// Post first batch of tasks
	for i := 1; i <= 3; i++ {
		id := i
		runner.PostTask(func(ctx context.Context) {
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			events = append(events, fmt.Sprintf("Task%d", id))
			mu.Unlock()
		})
	}

	// FlushAsync: callback runs after ALL prior tasks complete
	runner.FlushAsync(func() {
		mu.Lock()
		events = append(events, "FlushCallback")
		mu.Unlock()
		fmt.Println("FlushAsync callback executed!")
	})

	// Post second batch immediately
	for i := 4; i <= 6; i++ {
		id := i
		runner.PostTask(func(ctx context.Context) {
			mu.Lock()
			events = append(events, fmt.Sprintf("Task%d", id))
			mu.Unlock()
		})
	}

	runner.WaitIdle(context.Background())

	fmt.Printf("Event order: %v\n", events)
	fmt.Println("Notice: FlushCallback appears AFTER Task1-3, BEFORE Task4-6")
}

// repeatingTaskExample demonstrates repeating tasks
func repeatingTaskExample() {
	fmt.Println("--- Example 4: Repeating Task ---")

	runner := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		2,
	)
	defer runner.Shutdown()

	count := 0
	var mu sync.Mutex

	// Post a repeating task that runs every 100ms
	handle := runner.PostRepeatingTask(func(ctx context.Context) {
		mu.Lock()
		count++
		fmt.Printf("Repeating task execution #%d\n", count)
		mu.Unlock()
	}, 100*time.Millisecond)

	// Let it run 3 times
	time.Sleep(350 * time.Millisecond)

	// Stop the repeating task
	handle.Stop()
	fmt.Println("Repeating task stopped")

	// Wait to verify it stopped
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	finalCount := count
	mu.Unlock()

	fmt.Printf("Total executions: %d (should be 3-4)\n\n", finalCount)
}

// taskAndReplyExample demonstrates task and reply pattern
func taskAndReplyExample() {
	fmt.Println("--- Example 5: Task and Reply ---")

	bgRunner := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		4,
	)
	defer bgRunner.Shutdown()

	uiRunner := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		1, // UI thread: sequential execution
	)
	defer uiRunner.Shutdown()

	// Post task to background runner, reply to UI runner
	bgRunner.PostTaskAndReply(
		func(ctx context.Context) {
			// Background task: fetch data
			fmt.Println("Background: Fetching data...")
			time.Sleep(100 * time.Millisecond)
			fmt.Println("Background: Data fetched!")
		},
		func(ctx context.Context) {
			// UI task: update interface
			fmt.Println("UI: Updating interface with fetched data")
		},
		uiRunner, // Reply runs on UI runner
	)

	bgRunner.WaitIdle(context.Background())
	uiRunner.WaitIdle(context.Background())

	fmt.Println("Task and reply pattern completed")
}
