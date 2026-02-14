package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

func main() {
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== Runner Shutdown Example ===")
	fmt.Println()

	// Example 1: Basic shutdown
	fmt.Println("Example 1: Basic shutdown clears pending tasks")
	{
		runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

		var executed atomic.Int32
		for i := 0; i < 10; i++ {
			id := i
			runner.PostTask(func(ctx context.Context) {
				time.Sleep(50 * time.Millisecond)
				count := executed.Add(1)
				fmt.Printf("  Task %d executed (count=%d)\n", id, count)
			})
		}

		// Let one or two tasks execute
		time.Sleep(100 * time.Millisecond)

		// Shutdown - remaining tasks won't execute
		runner.Shutdown()
		fmt.Printf("  Shutdown called. Executed: %d/10\n", executed.Load())

		time.Sleep(200 * time.Millisecond)
		fmt.Printf("  Final count: %d/10 (remaining tasks cleared)\n", executed.Load())
	}
	fmt.Println()

	// Example 2: Shutdown stops repeating tasks automatically
	fmt.Println("Example 2: Shutdown automatically stops repeating tasks")
	{
		runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

		var counter1 atomic.Int32
		runner.PostRepeatingTask(func(ctx context.Context) {
			count := counter1.Add(1)
			fmt.Printf("  Repeating task 1: #%d\n", count)
		}, 100*time.Millisecond)

		var counter2 atomic.Int32
		runner.PostRepeatingTask(func(ctx context.Context) {
			count := counter2.Add(1)
			fmt.Printf("  Repeating task 2: #%d\n", count)
		}, 150*time.Millisecond)

		// Let them run for a while
		time.Sleep(500 * time.Millisecond)

		fmt.Println("  Calling Shutdown()...")
		runner.Shutdown()

		beforeCount1 := counter1.Load()
		beforeCount2 := counter2.Load()

		// Wait to verify they stopped
		time.Sleep(400 * time.Millisecond)

		fmt.Printf("  Task 1: %d -> %d (stopped)\n", beforeCount1, counter1.Load())
		fmt.Printf("  Task 2: %d -> %d (stopped)\n", beforeCount2, counter2.Load())
	}
	fmt.Println()

	// Example 3: Module lifecycle management
	fmt.Println("Example 3: Module lifecycle management")
	{
		type Module struct {
			name   string
			runner *taskrunner.SequencedTaskRunner
		}

		startModule := func(name string) *Module {
			m := &Module{
				name:   name,
				runner: taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits()),
			}

			// Module heartbeat
			m.runner.PostRepeatingTask(func(ctx context.Context) {
				fmt.Printf("  [%s] Heartbeat\n", m.name)
			}, 200*time.Millisecond)

			// Module work
			m.runner.PostTask(func(ctx context.Context) {
				fmt.Printf("  [%s] Doing work...\n", m.name)
			})

			return m
		}

		stopModule := func(m *Module) {
			fmt.Printf("  Stopping module: %s\n", m.name)
			m.runner.Shutdown()
		}

		// Start multiple modules
		module1 := startModule("Auth")
		module2 := startModule("Database")
		module3 := startModule("Cache")

		// Let them run
		time.Sleep(600 * time.Millisecond)

		// Stop individual modules
		stopModule(module1)
		time.Sleep(300 * time.Millisecond)

		stopModule(module2)
		time.Sleep(300 * time.Millisecond)

		stopModule(module3)
		fmt.Println("  All modules stopped")
	}
	fmt.Println()

	// Example 4: Check if runner is closed
	fmt.Println("Example 4: Checking runner state")
	{
		runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

		fmt.Printf("  Before shutdown: IsClosed() = %v\n", runner.IsClosed())

		runner.Shutdown()

		fmt.Printf("  After shutdown:  IsClosed() = %v\n", runner.IsClosed())

		// Posting tasks after shutdown has no effect
		runner.PostTask(func(ctx context.Context) {
			fmt.Println("  This won't execute")
		})

		time.Sleep(100 * time.Millisecond)
		fmt.Println("  No tasks executed (as expected)")
	}
	fmt.Println()

	fmt.Println("=== Example Finished ===")
}
