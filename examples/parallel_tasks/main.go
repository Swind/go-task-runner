package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
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

	// Example 6: High Concurrency Performance Test
	highConcurrencyExample()

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

	if err := runner.WaitIdle(context.Background()); err != nil {
		fmt.Printf("Error waiting for idle: %v\n", err)
	}
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

	if err := runner.WaitIdle(context.Background()); err != nil {
		fmt.Printf("Error waiting for idle: %v\n", err)
	}

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

	if err := bgRunner.WaitIdle(context.Background()); err != nil {
		fmt.Printf("Error waiting for background runner: %v\n", err)
	}
	if err := uiRunner.WaitIdle(context.Background()); err != nil {
		fmt.Printf("Error waiting for UI runner: %v\n", err)
	}

	fmt.Println("Task and reply pattern completed")
}

// highConcurrencyExample demonstrates high concurrency performance
// 100 tasks, each sleeping 1 second, with 50 concurrent workers
// Should complete in ~2 seconds (2 batches of 50 tasks)
func highConcurrencyExample() {
	fmt.Println("\n--- Example 6: High Concurrency Performance Test ---")
	fmt.Println("Running 100 tasks (each 1s) with 50 concurrent workers")

	// Create a dedicated thread pool with 50 workers for this test
	pool := taskrunner.NewGoroutineThreadPool("HighConcurrencyPool", 50)
	pool.Start(context.Background())
	defer pool.Stop()

	// Create a ParallelTaskRunner with 50 concurrent tasks
	runner := taskrunner.NewParallelTaskRunner(
		pool,
		50, // maxConcurrency: 50 tasks run simultaneously
	)
	defer runner.Shutdown()

	var completed sync.WaitGroup
	var completedCount int32

	// Record start time
	startTime := time.Now()

	// Post 100 tasks, each sleeping 1 second
	for i := 1; i <= 100; i++ {
		id := i
		completed.Add(1)
		runner.PostTask(func(ctx context.Context) {
			defer completed.Done()

			// Simulate 1 second of work
			time.Sleep(1 * time.Second)

			// Track completion
			count := atomic.AddInt32(&completedCount, 1)

			if (id-1)%10 == 0 {
				fmt.Printf("  Task %d completed (total: %d/100)\n", id, count)
			}
		})
	}

	// Wait in idle to ensure all tasks are posted
	err := runner.WaitIdle(context.Background())
	if err != nil {
		fmt.Printf("Error waiting for idle: %v\n", err)
	}

	// completed should be done when all tasks finish
	completed.Wait()
	duration := time.Since(startTime)

	// Verify results
	fmt.Printf("\n✓ All 100 tasks completed!\n")
	fmt.Printf("  Execution time: %.2f seconds\n", duration.Seconds())

	// Verify completion time (should be ~2 seconds with 50 concurrent workers)
	expectedTime := 2 * time.Second
	maxTime := 3 * time.Second

	if duration < maxTime {
		fmt.Printf("  ✓ PASS: Completed within 3 seconds (expected ~%.1fs)\n", expectedTime.Seconds())
		fmt.Printf("  Performance: %.1fx faster than sequential execution\n", 100.0/duration.Seconds())
	} else {
		fmt.Printf("  ✗ FAIL: Took longer than 3 seconds (expected ~%.1fs)\n", expectedTime.Seconds())
	}

	fmt.Println()
}
