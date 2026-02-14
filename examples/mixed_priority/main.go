package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

func main() {
	// Use 1 worker so the two runners must compete for the same worker.
	// This demonstrates scheduler-level priority across runners.
	taskrunner.InitGlobalThreadPool(1)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== Mixed Priority Example ===")

	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
	var wg sync.WaitGroup

	// 1. Saturate the worker with a low priority task
	wg.Add(1)
	runner.PostTaskWithTraits(func(ctx context.Context) {
		defer wg.Done()
		fmt.Println("Generic Low Priority Task Started (Blocking)")
		time.Sleep(500 * time.Millisecond)
		fmt.Println("Generic Low Priority Task Finished")
	}, taskrunner.TraitsBestEffort())

	// 2. Post a high-priority task on a different runner.
	// SequencedTaskRunner itself is FIFO; priority differences are visible when
	// multiple runners compete for scheduler/worker time.

	runnerHigh := taskrunner.CreateTaskRunner(taskrunner.TraitsUserBlocking())

	wg.Add(1)
	runnerHigh.PostTaskWithTraits(func(ctx context.Context) {
		defer wg.Done()
		fmt.Println(">>> High Priority Runner Task Executed!")
	}, taskrunner.TraitsUserBlocking())

	// Wait for completion
	wg.Wait()
	fmt.Println("=== Example Finished ===")
}
