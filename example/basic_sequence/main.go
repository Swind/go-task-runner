package main

import (
	"context"
	"fmt"
	"time"

	taskrunner "swind/go-task-runner"
	"swind/go-task-runner/domain"
)

func main() {
	// 1. Initialize the Global Thread Pool
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== Basic Sequence Example ===")

	// 2. Create a SequencedTaskRunner
	// This runner acts like a virtual thread. Tasks posted to it are sequential.
	runner := taskrunner.CreateTaskRunner(domain.TaskTraits{
		Priority: domain.TaskPriorityUserVisible,
	})

	done := make(chan struct{})

	// 3. Post sequence of tasks
	for i := 1; i <= 3; i++ {
		id := i
		runner.PostTask(func(ctx context.Context) {
			fmt.Printf("Task %d running on worker (Simulated Thread)\n", id)
			time.Sleep(100 * time.Millisecond) // Simulate work
		})
	}

	// 4. Post a delayed task
	runner.PostDelayedTask(func(ctx context.Context) {
		fmt.Println("Delayed Task executed!")
		close(done)
	}, 500*time.Millisecond)

	<-done
	fmt.Println("=== Example Finished ===")
}
