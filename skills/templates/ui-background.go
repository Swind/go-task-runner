// Package main demonstrates Task and Reply pattern for UI/Background work
// This pattern cleanly separates heavy computation from UI updates
package main

import (
	"context"
	"fmt"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// Data represents fetched data
type Data struct {
	Value string
}

func main() {
	taskrunner.InitGlobalThreadPool(8)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== Task and Reply Pattern Example ===\n")

	// Create UI and Background runners
	uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
	defer uiRunner.Shutdown()

	bgRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())
	defer bgRunner.Shutdown()

	// Example 1: Basic PostTaskAndReply
	fmt.Println("--- Example 1: Basic Task and Reply ---")
	uiRunner.PostTask(func(ctx context.Context) {
		me := taskrunner.GetCurrentTaskRunner(ctx)

		bgRunner.PostTaskAndReply(
			func(ctx context.Context) {
				// Heavy work on background runner
				fmt.Println("Background: Fetching data...")
				time.Sleep(200 * time.Millisecond)
				fmt.Println("Background: Data fetched!")
			},
			func(ctx context.Context) {
				// Update UI on UI runner
				fmt.Println("UI: Updating interface with data")
			},
			me, // Reply to UI runner
		)
	})

	time.Sleep(300 * time.Millisecond)

	// Example 2: PostTaskAndReplyWithResult (with error handling)
	fmt.Println("\n--- Example 2: Task and Reply with Result ---")
	uiRunner.PostTask(func(ctx context.Context) {
		me := taskrunner.GetCurrentTaskRunner(ctx)

		core.PostTaskAndReplyWithResult(
			bgRunner,
			func(ctx context.Context) (*Data, error) {
				// Simulate heavy computation
				fmt.Println("Background: Computing result...")
				time.Sleep(150 * time.Millisecond)
				return &Data{Value: "Computed Result"}, nil
			},
			func(ctx context.Context, data *Data, err error) {
				// Handle result on UI runner
				if err != nil {
					fmt.Printf("UI: Error occurred: %v\n", err)
					return
				}
				fmt.Printf("UI: Received data: %s\n", data.Value)
			},
			me,
		)
	})

	time.Sleep(250 * time.Millisecond)

	// Example 3: Multiple chained requests
	fmt.Println("\n--- Example 3: Chained Requests ---")
	uiRunner.PostTask(func(ctx context.Context) {
		me := taskrunner.GetCurrentTaskRunner(ctx)

		// First request
		core.PostTaskAndReplyWithResult(
			bgRunner,
			func(ctx context.Context) (string, error) {
				fmt.Println("Background: Fetching user ID...")
				time.Sleep(100 * time.Millisecond)
				return "user_123", nil
			},
			func(ctx context.Context, userID string, err error) {
				if err != nil {
					fmt.Printf("UI: Error: %v\n", err)
					return
				}
				fmt.Printf("UI: Got user ID: %s\n", userID)

				// Second request (chained)
				core.PostTaskAndReplyWithResult(
					bgRunner,
					func(ctx context.Context) (string, error) {
						fmt.Printf("Background: Fetching profile for %s...\n", userID)
						time.Sleep(100 * time.Millisecond)
						return "User Profile Data", nil
					},
					func(ctx context.Context, profile string, err error) {
						if err != nil {
							fmt.Printf("UI: Error: %v\n", err)
							return
						}
						fmt.Printf("UI: Got profile: %s\n", profile)
					},
					me,
				)
			},
			me,
		)
	})

	time.Sleep(350 * time.Millisecond)

	// Example 4: Different priorities
	fmt.Println("\n--- Example 4: Different Priorities for Task and Reply ---")
	uiRunner.PostTask(func(ctx context.Context) {
		me := taskrunner.GetCurrentTaskRunner(ctx)

		// Background task with low priority, reply with high priority
		core.PostTaskAndReplyWithResultAndTraits(
			bgRunner,
			func(ctx context.Context) (int, error) {
				fmt.Println("Background: Heavy computation (low priority)...")
				time.Sleep(150 * time.Millisecond)
				return 42, nil
			},
			taskrunner.TraitsBestEffort(),    // Low priority for background work
			func(ctx context.Context, result int, err error) {
				fmt.Printf("UI: Result = %d (high priority reply)\n", result)
			},
			taskrunner.TraitsUserBlocking(),  // High priority for UI update
			me,
		)
	})

	time.Sleep(250 * time.Millisecond)

	fmt.Println("\n✓ All task and reply patterns completed!")
}
