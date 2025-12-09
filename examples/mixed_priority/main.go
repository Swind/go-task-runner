package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

func main() {
	// Use only 1 worker to clearly demonstrate priority preemption/ordering effects
	taskrunner.InitGlobalThreadPool(1)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== Mixed Priority Example ===")

	runner := taskrunner.CreateTaskRunner(core.DefaultTaskTraits())
	var wg sync.WaitGroup

	// 1. Saturate the worker with a low priority task
	wg.Add(1)
	runner.PostTaskWithTraits(func(ctx context.Context) {
		defer wg.Done()
		fmt.Println("Generic Low Priority Task Started (Blocking)")
		time.Sleep(500 * time.Millisecond)
		fmt.Println("Generic Low Priority Task Finished")
	}, core.TraitsBestEffort())

	// 2. Post a High Priority task immediately after
	// Since the worker is blocked, this will be queued.
	// But it should be picked up before any other BestEffort tasks if we had more.

	// Note: Within a SequencedTaskRunner, STRICT FIFO is observed.
	// Priority only affects when the *RunLoop* itself gets scheduled by the Scheduler.
	// To demonstrate Priority Scheduler, we strictly speaking need *multiple* runners or use global PostTask if available.
	// But here we use SequencedTaskRunner. If we post these to the SAME runner, they run sequentially (FIFO).
	// To show Preemption/Priority scheduling, we should use two DIFFERENT runners.

	runnerHigh := taskrunner.CreateTaskRunner(core.TraitsUserBlocking())

	wg.Add(1)
	runnerHigh.PostTaskWithTraits(func(ctx context.Context) {
		defer wg.Done()
		fmt.Println(">>> High Priority Runner Task Executed!")
	}, core.TraitsUserBlocking())

	// Wait for completion
	wg.Wait()
	fmt.Println("=== Example Finished ===")
}
