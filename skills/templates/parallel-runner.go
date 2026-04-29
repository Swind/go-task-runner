// Package main demonstrates ParallelTaskRunner for controlled parallelism
// Use this when you need to run multiple tasks concurrently with a maximum limit
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
	taskrunner.InitGlobalThreadPool(50)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== ParallelTaskRunner Example ===\n")

	// Example 1: Basic parallel execution
	fmt.Println("--- Example 1: Controlled Concurrency (Max 5) ---")
	runner := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		5, // Max 5 tasks run concurrently
	)
	defer runner.Shutdown()

	var wg sync.WaitGroup
	for i := 1; i <= 10; i++ {
		i := i
		wg.Add(1)
		runner.PostTask(func(ctx context.Context) {
			defer wg.Done()
			fmt.Printf("Task %d started\n", i)
			time.Sleep(200 * time.Millisecond)
			fmt.Printf("Task %d completed\n", i)
		})
	}

	wg.Wait()
	fmt.Println("All tasks completed\n")

	// Example 2: WaitIdle usage
	fmt.Println("--- Example 2: WaitIdle ---")
	runner2 := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		3,
	)
	defer runner2.Shutdown()

	for i := 1; i <= 5; i++ {
		i := i
		runner2.PostTask(func(ctx context.Context) {
			fmt.Printf("Task %d executing\n", i)
			time.Sleep(100 * time.Millisecond)
		})
	}

	// Wait until all tasks complete
	if err := runner2.WaitIdle(context.Background()); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println("Runner is idle\n")

	// Example 3: FlushAsync (barrier semantics)
	fmt.Println("--- Example 3: FlushAsync (Barrier) ---")
	runner3 := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		3,
	)
	defer runner3.Shutdown()

	// Batch 1
	for i := 1; i <= 3; i++ {
		i := i
		runner3.PostTask(func(ctx context.Context) {
			fmt.Printf("Batch 1 - Task %d\n", i)
			time.Sleep(50 * time.Millisecond)
		})
	}

	// Barrier - callback runs after batch 1 completes
	runner3.FlushAsync(func() {
		fmt.Println(">>> Flush callback: Batch 1 complete! <<<")
	})

	// Batch 2
	for i := 4; i <= 6; i++ {
		i := i
		runner3.PostTask(func(ctx context.Context) {
			fmt.Printf("Batch 2 - Task %d\n", i)
		})
	}

	if err := runner3.WaitIdle(context.Background()); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println()

	// Example 4: High concurrency batch processing
	fmt.Println("--- Example 4: Batch Processing (100 tasks, 50 concurrent) ---")
	batchRunner := taskrunner.NewParallelTaskRunner(
		taskrunner.GlobalThreadPool(),
		50, // 50 concurrent tasks
	)
	defer batchRunner.Shutdown()

	var completed atomic.Int32
	start := time.Now()

	var batchWg sync.WaitGroup
	for i := 1; i <= 100; i++ {
		batchWg.Add(1)
		batchRunner.PostTask(func(ctx context.Context) {
			defer batchWg.Done()
			time.Sleep(100 * time.Millisecond) // Simulate work
			count := completed.Add(1)
			if count%10 == 0 {
				fmt.Printf("Completed: %d/100\n", count)
			}
		})
	}

	batchWg.Wait()
	duration := time.Since(start)

	fmt.Printf("\n✓ 100 tasks completed in %.2f seconds\n", duration.Seconds())
	fmt.Printf("Expected ~200ms (2 batches of 50 tasks @ 100ms each)\n")

	if duration < 300*time.Millisecond {
		fmt.Println("✓ PASS: Parallel execution working!")
	}
}
