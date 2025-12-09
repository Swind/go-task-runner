package main

import (
	"context"
	"fmt"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

func main() {
	// Initialize the global thread pool
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== Repeating Task Example ===")
	fmt.Println()

	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

	// Example 1: Simple repeating task
	fmt.Println("Example 1: Simple repeating task (every 500ms)")
	counter1 := 0
	handle1 := runner.PostRepeatingTask(func(ctx context.Context) {
		counter1++
		fmt.Printf("  [Task 1] Heartbeat #%d\n", counter1)
	}, 500*time.Millisecond)

	// Let it run for 2.5 seconds
	time.Sleep(2500 * time.Millisecond)
	handle1.Stop()
	fmt.Printf("  Stopped after %d executions\n\n", counter1)

	// Example 2: Repeating task with initial delay
	fmt.Println("Example 2: Repeating task with 1s initial delay, then every 300ms")
	counter2 := 0
	startTime := time.Now()
	handle2 := runner.PostRepeatingTaskWithInitialDelay(
		func(ctx context.Context) {
			counter2++
			elapsed := time.Since(startTime).Round(100 * time.Millisecond)
			fmt.Printf("  [Task 2] Execution #%d at %v\n", counter2, elapsed)
		},
		1*time.Second,   // initialDelay
		300*time.Millisecond, // interval
		taskrunner.DefaultTaskTraits(),
	)

	time.Sleep(2500 * time.Millisecond)
	handle2.Stop()
	fmt.Printf("  Stopped after %d executions\n\n", counter2)

	// Example 3: High priority repeating task
	fmt.Println("Example 3: High priority repeating task")
	counter3 := 0
	handle3 := runner.PostRepeatingTaskWithTraits(
		func(ctx context.Context) {
			counter3++
			fmt.Printf("  [Task 3] High priority execution #%d\n", counter3)
		},
		400*time.Millisecond,
		taskrunner.TaskTraits{
			Priority: taskrunner.TaskPriorityUserBlocking,
		},
	)

	time.Sleep(1500 * time.Millisecond)
	handle3.Stop()
	fmt.Printf("  Stopped after %d executions\n\n", counter3)

	// Example 4: Self-stopping task
	fmt.Println("Example 4: Task that stops itself after 5 executions")
	counter4 := 0
	var handle4 taskrunner.RepeatingTaskHandle
	handle4 = runner.PostRepeatingTask(func(ctx context.Context) {
		counter4++
		fmt.Printf("  [Task 4] Count: %d\n", counter4)
		if counter4 >= 5 {
			fmt.Println("  [Task 4] Stopping self...")
			handle4.Stop()
		}
	}, 200*time.Millisecond)

	// Wait for it to stop itself
	time.Sleep(1500 * time.Millisecond)
	fmt.Printf("  Final count: %d\n\n", counter4)

	// Example 5: Multiple concurrent repeating tasks
	fmt.Println("Example 5: Multiple concurrent repeating tasks")
	handles := make([]taskrunner.RepeatingTaskHandle, 3)
	for i := 0; i < 3; i++ {
		id := i + 1
		count := 0
		handles[i] = runner.PostRepeatingTask(func(ctx context.Context) {
			count++
			fmt.Printf("  [Worker %d] Tick #%d\n", id, count)
		}, time.Duration(200*(i+1))*time.Millisecond)
	}

	time.Sleep(1500 * time.Millisecond)

	// Stop all
	for _, h := range handles {
		h.Stop()
	}
	fmt.Println("  All tasks stopped")
	fmt.Println()

	fmt.Println("=== Example Finished ===")
}
