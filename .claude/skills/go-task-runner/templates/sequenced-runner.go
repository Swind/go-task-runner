// Package main demonstrates SequencedTaskRunner basic usage
// SequencedTaskRunner guarantees FIFO execution - perfect for lock-free patterns
package main

import (
	"context"
	"fmt"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

// Example: State machine using SequencedTaskRunner
type StateMachine struct {
	runner       *taskrunner.SequencedTaskRunner
	currentState string // No mutex needed!
	transitions  int
}

func NewStateMachine() *StateMachine {
	return &StateMachine{
		runner:       taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits()),
		currentState: "idle",
	}
}

// TransitionTo changes state - sequential execution ensures no race
func (sm *StateMachine) TransitionTo(newState string) {
	sm.runner.PostTask(func(ctx context.Context) {
		fmt.Printf("State transition: %s -> %s\n", sm.currentState, newState)
		sm.currentState = newState
		sm.transitions++
	})
}

// GetState returns current state via callback
func (sm *StateMachine) GetState(callback func(string, int)) {
	sm.runner.PostTask(func(ctx context.Context) {
		callback(sm.currentState, sm.transitions)
	})
}

// Shutdown cleans up
func (sm *StateMachine) Shutdown() {
	sm.runner.Shutdown()
}

func main() {
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== SequencedTaskRunner Example ===\n")

	// Example 1: Basic sequential tasks
	fmt.Println("--- Example 1: Sequential Execution ---")
	runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
	defer runner.Shutdown()

	runner.PostTask(func(ctx context.Context) {
		fmt.Println("Task 1 executing")
	})

	runner.PostTask(func(ctx context.Context) {
		fmt.Println("Task 2 executing")
	})

	runner.PostTask(func(ctx context.Context) {
		fmt.Println("Task 3 executing")
	})

	time.Sleep(100 * time.Millisecond)

	// Example 2: Delayed task
	fmt.Println("\n--- Example 2: Delayed Task ---")
	runner.PostDelayedTask(func(ctx context.Context) {
		fmt.Println("Delayed task executed after 200ms")
	}, 200*time.Millisecond)

	time.Sleep(300 * time.Millisecond)

	// Example 3: State machine
	fmt.Println("\n--- Example 3: State Machine (Lock-Free) ---")
	sm := NewStateMachine()
	defer sm.Shutdown()

	// Post state transitions
	sm.TransitionTo("loading")
	sm.TransitionTo("ready")
	sm.TransitionTo("running")
	sm.TransitionTo("stopped")

	// Get final state
	time.Sleep(100 * time.Millisecond)
	done := make(chan struct{})
	sm.GetState(func(state string, transitions int) {
		fmt.Printf("\nFinal state: %s\n", state)
		fmt.Printf("Total transitions: %d\n", transitions)
		close(done)
	})
	<-done

	// Example 4: Context usage
	fmt.Println("\n--- Example 4: Context Usage ---")
	runner.PostTask(func(ctx context.Context) {
		me := taskrunner.GetCurrentTaskRunner(ctx)
		fmt.Printf("Running on: %s\n", me.Name())
	})

	time.Sleep(100 * time.Millisecond)
	fmt.Println("\n✓ All sequential tasks completed in FIFO order!")
}
