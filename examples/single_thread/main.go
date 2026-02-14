package main

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

func main() {
	fmt.Println("=== SingleThreadTaskRunner Examples ===")
	fmt.Println()

	// Example 1: Thread Affinity - All tasks on same goroutine
	fmt.Println("Example 1: Thread Affinity Guarantee")
	{
		runner := taskrunner.NewSingleThreadTaskRunner()

		goroutineID := func() uint64 {
			b := make([]byte, 64)
			b = b[:runtime.Stack(b, false)]
			var id uint64
			for i := len("goroutine "); i < len(b); i++ {
				if b[i] >= '0' && b[i] <= '9' {
					id = id*10 + uint64(b[i]-'0')
				} else {
					break
				}
			}
			return id
		}

		var firstID uint64
		for i := 0; i < 5; i++ {
			id := i
			runner.PostTask(func(ctx context.Context) {
				gid := goroutineID()
				if id == 0 {
					firstID = gid
					fmt.Printf("  Task %d: goroutine %d (first)\n", id, gid)
				} else {
					fmt.Printf("  Task %d: goroutine %d (same=%v)\n", id, gid, gid == firstID)
				}
			})
		}

		time.Sleep(100 * time.Millisecond)
		runner.Stop()
	}
	fmt.Println()

	// Example 2: Blocking IO Simulation
	fmt.Println("Example 2: Blocking IO Operations")
	{
		runner := taskrunner.NewSingleThreadTaskRunner()

		// Simulate blocking network reads
		runner.PostTask(func(ctx context.Context) {
			fmt.Println("  Reading from socket (blocking)...")
			time.Sleep(100 * time.Millisecond)
			fmt.Println("  ✓ Socket read complete")
		})

		// Simulate blocking file IO
		runner.PostTask(func(ctx context.Context) {
			fmt.Println("  Writing to file (blocking)...")
			time.Sleep(100 * time.Millisecond)
			fmt.Println("  ✓ File write complete")
		})

		// Simulate blocking database query
		runner.PostTask(func(ctx context.Context) {
			fmt.Println("  Database query (blocking)...")
			time.Sleep(100 * time.Millisecond)
			fmt.Println("  ✓ Query complete")
		})

		time.Sleep(400 * time.Millisecond)
		runner.Stop()
	}
	fmt.Println()

	// Example 3: Delayed Tasks
	fmt.Println("Example 3: Delayed Tasks on Single Thread")
	{
		runner := taskrunner.NewSingleThreadTaskRunner()

		fmt.Println("  Posting delayed tasks...")
		runner.PostDelayedTask(func(ctx context.Context) {
			fmt.Println("  Task delayed by 100ms executed")
		}, 100*time.Millisecond)

		runner.PostDelayedTask(func(ctx context.Context) {
			fmt.Println("  Task delayed by 200ms executed")
		}, 200*time.Millisecond)

		time.Sleep(300 * time.Millisecond)
		runner.Stop()
	}
	fmt.Println()

	// Example 4: Repeating Tasks
	fmt.Println("Example 4: Repeating Tasks on Single Thread")
	{
		runner := taskrunner.NewSingleThreadTaskRunner()

		var counter atomic.Int32
		handle := runner.PostRepeatingTask(func(ctx context.Context) {
			count := counter.Add(1)
			fmt.Printf("  Heartbeat #%d\n", count)
		}, 100*time.Millisecond)

		time.Sleep(450 * time.Millisecond)
		fmt.Println("  Stopping repeating task...")
		handle.Stop()

		time.Sleep(200 * time.Millisecond)
		fmt.Printf("  Final count: %d (stopped)\n", counter.Load())

		runner.Stop()
	}
	fmt.Println()

	// Example 5: Sequential State Management (Lock-free)
	fmt.Println("Example 5: Lock-free State Management")
	{
		runner := taskrunner.NewSingleThreadTaskRunner()

		// No locks needed - guaranteed sequential execution
		type State struct {
			counter int
			items   []string
		}

		state := &State{}

		// Multiple tasks modifying shared state without locks
		for i := 0; i < 5; i++ {
			id := i
			runner.PostTask(func(ctx context.Context) {
				state.counter++
				state.items = append(state.items, fmt.Sprintf("item-%d", id))
				fmt.Printf("  State: counter=%d, items=%d\n", state.counter, len(state.items))
			})
		}

		if err := runner.WaitIdle(context.Background()); err != nil {
			fmt.Printf("  WaitIdle error: %v\n", err)
		}
		fmt.Printf("  Final state: counter=%d, items=%v\n", state.counter, state.items)

		runner.Stop()
	}
	fmt.Println()

	// Example 6: Graceful Shutdown
	fmt.Println("Example 6: Graceful Shutdown")
	{
		runner := taskrunner.NewSingleThreadTaskRunner()

		// Post multiple tasks
		for i := 0; i < 5; i++ {
			id := i
			runner.PostTask(func(ctx context.Context) {
				fmt.Printf("  Task %d executed\n", id)
				time.Sleep(50 * time.Millisecond)
			})
		}

		// Let some tasks execute
		time.Sleep(150 * time.Millisecond)

		fmt.Println("  Calling Shutdown()...")
		runner.Shutdown()

		if runner.IsClosed() {
			fmt.Println("  ✓ Runner is closed")
		}

		// Posting tasks after shutdown has no effect
		runner.PostTask(func(ctx context.Context) {
			fmt.Println("  This won't execute")
		})

		time.Sleep(100 * time.Millisecond)
	}
	fmt.Println()

	// Example 7: Repeating Task with Initial Delay
	fmt.Println("Example 7: Repeating Task with Initial Delay")
	{
		runner := taskrunner.NewSingleThreadTaskRunner()

		fmt.Println("  Starting repeating task with 200ms initial delay...")
		var counter atomic.Int32
		handle := runner.PostRepeatingTaskWithInitialDelay(
			func(ctx context.Context) {
				count := counter.Add(1)
				fmt.Printf("  Execution #%d\n", count)
			},
			200*time.Millisecond, // Initial delay
			100*time.Millisecond, // Interval
			taskrunner.DefaultTaskTraits(),
		)

		time.Sleep(500 * time.Millisecond)
		handle.Stop()

		fmt.Printf("  Stopped after %d executions\n", counter.Load())
		runner.Stop()
	}
	fmt.Println()

	// Example 8: Panic Recovery
	fmt.Println("Example 8: Panic Recovery")
	{
		runner := taskrunner.NewSingleThreadTaskRunner()

		runner.PostTask(func(ctx context.Context) {
			fmt.Println("  Task 1: Normal execution")
		})

		runner.PostTask(func(ctx context.Context) {
			fmt.Println("  Task 2: About to panic...")
			panic("simulated panic")
		})

		runner.PostTask(func(ctx context.Context) {
			fmt.Println("  Task 3: Executed despite previous panic")
		})

		time.Sleep(100 * time.Millisecond)
		runner.Stop()
	}
	fmt.Println()

	fmt.Println("=== Examples Finished ===")
}
