package main

import (
	"context"
	"fmt"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

func main() {
	taskrunner.InitGlobalThreadPool(2)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== Delayed Task Example ===")
	runner := taskrunner.CreateTaskRunner(core.DefaultTaskTraits())

	start := time.Now()

	// Post multiple delayed tasks
	delays := []time.Duration{
		200 * time.Millisecond,
		500 * time.Millisecond,
		100 * time.Millisecond, // Should run first
	}

	done := make(chan struct{}, len(delays))

	for i, d := range delays {
		id, delay := i, d // Capture sequence
		runner.PostDelayedTask(func(ctx context.Context) {
			elapsed := time.Since(start)
			fmt.Printf("Task %d executed after %v (Expected ~%v)\n", id, elapsed, delay)
			done <- struct{}{}
		}, delay)
	}

	// Wait for all
	for i := 0; i < len(delays); i++ {
		<-done
	}
	fmt.Println("=== Example Finished ===")
}
