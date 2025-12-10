package main

import (
	"context"
	"fmt"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

func main() {
	// Initialize global thread pool
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== PostTaskAndReply Examples ===")
	fmt.Println()

	// Example 1: Basic PostTaskAndReply
	fmt.Println("Example 1: Basic UI/Background Pattern")
	{
		uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
		bgRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

		uiRunner.PostTask(func(ctx context.Context) {
			fmt.Println("  [UI] Button clicked: Start loading data...")
			me := taskrunner.GetCurrentTaskRunner(ctx)

			// Post to background, reply back to UI
			bgRunner.PostTaskAndReply(
				func(ctx context.Context) {
					fmt.Println("  [BG] Loading data from server...")
					time.Sleep(200 * time.Millisecond)
					fmt.Println("  [BG] Data loaded!")
				},
				func(ctx context.Context) {
					fmt.Println("  [UI] Update UI with loaded data")
				},
				me, // Reply back to UI runner
			)
		})

		time.Sleep(400 * time.Millisecond)
	}
	fmt.Println()

	// Example 2: PostTaskAndReplyWithResult - Int Result
	fmt.Println("Example 2: Calculate and Display Result (Int)")
	{
		uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
		calcRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

		uiRunner.PostTask(func(ctx context.Context) {
			me := taskrunner.GetCurrentTaskRunner(ctx)

			core.PostTaskAndReplyWithResult(
				calcRunner,
				func(ctx context.Context) (int, error) {
					fmt.Println("  [Calc] Computing length of 'Hello World'...")
					time.Sleep(100 * time.Millisecond)
					return len("Hello World"), nil
				},
				func(ctx context.Context, length int, err error) {
					if err != nil {
						fmt.Printf("  [UI] Error: %v\n", err)
						return
					}
					fmt.Printf("  [UI] Length is: %d\n", length)
				},
				me,
			)
		})

		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println()

	// Example 3: PostTaskAndReplyWithResult - String Result
	fmt.Println("Example 3: Fetch and Display User Name")
	{
		uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
		dbRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

		uiRunner.PostTask(func(ctx context.Context) {
			me := taskrunner.GetCurrentTaskRunner(ctx)

			core.PostTaskAndReplyWithResult(
				dbRunner,
				func(ctx context.Context) (string, error) {
					fmt.Println("  [DB] Fetching user name from database...")
					time.Sleep(150 * time.Millisecond)
					return "Alice", nil
				},
				func(ctx context.Context, name string, err error) {
					if err != nil {
						fmt.Printf("  [UI] Error: %v\n", err)
						return
					}
					fmt.Printf("  [UI] Welcome, %s!\n", name)
				},
				me,
			)
		})

		time.Sleep(350 * time.Millisecond)
	}
	fmt.Println()

	// Example 4: PostTaskAndReplyWithResult - Struct Result
	fmt.Println("Example 4: Fetch User Data (Struct)")
	{
		type UserData struct {
			Name  string
			Email string
			Age   int
		}

		uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
		apiRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

		uiRunner.PostTask(func(ctx context.Context) {
			me := taskrunner.GetCurrentTaskRunner(ctx)

			core.PostTaskAndReplyWithResult(
				apiRunner,
				func(ctx context.Context) (*UserData, error) {
					fmt.Println("  [API] Fetching user data from API...")
					time.Sleep(200 * time.Millisecond)
					return &UserData{
						Name:  "Bob",
						Email: "bob@example.com",
						Age:   25,
					}, nil
				},
				func(ctx context.Context, user *UserData, err error) {
					if err != nil {
						fmt.Printf("  [UI] Error: %v\n", err)
						return
					}
					fmt.Printf("  [UI] User: %s (%s), Age: %d\n", user.Name, user.Email, user.Age)
				},
				me,
			)
		})

		time.Sleep(400 * time.Millisecond)
	}
	fmt.Println()

	// Example 5: Error Handling
	fmt.Println("Example 5: Error Handling")
	{
		uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
		netRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

		uiRunner.PostTask(func(ctx context.Context) {
			me := taskrunner.GetCurrentTaskRunner(ctx)

			core.PostTaskAndReplyWithResult(
				netRunner,
				func(ctx context.Context) (string, error) {
					fmt.Println("  [Net] Connecting to server...")
					time.Sleep(100 * time.Millisecond)
					return "", fmt.Errorf("connection timeout")
				},
				func(ctx context.Context, data string, err error) {
					if err != nil {
						fmt.Printf("  [UI] Show error dialog: %v\n", err)
						return
					}
					fmt.Printf("  [UI] Data: %s\n", data)
				},
				me,
			)
		})

		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println()

	// Example 6: WithTraits - Different Priorities
	fmt.Println("Example 6: Using Different Traits for Task and Reply")
	{
		uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
		bgRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

		uiRunner.PostTask(func(ctx context.Context) {
			me := taskrunner.GetCurrentTaskRunner(ctx)

			core.PostTaskAndReplyWithResultAndTraits(
				bgRunner,
				func(ctx context.Context) (string, error) {
					fmt.Println("  [BG] (BestEffort) Processing heavy computation...")
					time.Sleep(150 * time.Millisecond)
					return "Result", nil
				},
				taskrunner.TraitsBestEffort(), // Background work - low priority
				func(ctx context.Context, result string, err error) {
					fmt.Printf("  [UI] (UserBlocking) Update UI immediately: %s\n", result)
				},
				taskrunner.TraitsUserBlocking(), // UI update - high priority
				me,
			)
		})

		time.Sleep(350 * time.Millisecond)
	}
	fmt.Println()

	// Example 7: Delayed Task and Reply
	fmt.Println("Example 7: Delayed Task and Reply")
	{
		uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
		timerRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

		uiRunner.PostTask(func(ctx context.Context) {
			me := taskrunner.GetCurrentTaskRunner(ctx)

			fmt.Println("  [UI] Setting up delayed notification...")
			core.PostDelayedTaskAndReplyWithResult(
				timerRunner,
				func(ctx context.Context) (string, error) {
					return "Reminder: Meeting in 5 minutes!", nil
				},
				200*time.Millisecond, // Delay 200ms
				func(ctx context.Context, message string, err error) {
					fmt.Printf("  [UI] Show notification: %s\n", message)
				},
				me,
			)
		})

		time.Sleep(400 * time.Millisecond)
	}
	fmt.Println()

	// Example 8: Multiple Sequential Requests
	fmt.Println("Example 8: Chained Requests")
	{
		uiRunner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())
		apiRunner := taskrunner.CreateTaskRunner(taskrunner.TraitsBestEffort())

		uiRunner.PostTask(func(ctx context.Context) {
			me := taskrunner.GetCurrentTaskRunner(ctx)

			// First request: Get user ID
			core.PostTaskAndReplyWithResult(
				apiRunner,
				func(ctx context.Context) (int, error) {
					fmt.Println("  [API] Fetching user ID...")
					time.Sleep(100 * time.Millisecond)
					return 123, nil
				},
				func(ctx context.Context, userID int, err error) {
					if err != nil {
						fmt.Printf("  [UI] Error: %v\n", err)
						return
					}
					fmt.Printf("  [UI] Got user ID: %d\n", userID)

					// Second request: Get user details using the ID
					core.PostTaskAndReplyWithResult(
						apiRunner,
						func(ctx context.Context) (string, error) {
							fmt.Printf("  [API] Fetching details for user %d...\n", userID)
							time.Sleep(100 * time.Millisecond)
							return "Alice (Premium User)", nil
						},
						func(ctx context.Context, details string, err error) {
							if err != nil {
								fmt.Printf("  [UI] Error: %v\n", err)
								return
							}
							fmt.Printf("  [UI] User details: %s\n", details)
						},
						me,
					)
				},
				me,
			)
		})

		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println()

	// Example 9: SingleThreadTaskRunner with PostTaskAndReply
	fmt.Println("Example 9: SingleThreadTaskRunner (Thread Affinity)")
	{
		uiThread := taskrunner.NewSingleThreadTaskRunner()
		defer uiThread.Stop()

		bgThread := taskrunner.NewSingleThreadTaskRunner()
		defer bgThread.Stop()

		uiThread.PostTask(func(ctx context.Context) {
			fmt.Println("  [UI-Thread] User clicked button")

			core.PostTaskAndReplyWithResult(
				bgThread,
				func(ctx context.Context) (string, error) {
					fmt.Println("  [BG-Thread] Blocking IO operation...")
					time.Sleep(150 * time.Millisecond)
					return "Data from blocking IO", nil
				},
				func(ctx context.Context, data string, err error) {
					fmt.Printf("  [UI-Thread] Received: %s\n", data)
				},
				uiThread,
			)
		})

		time.Sleep(350 * time.Millisecond)
	}
	fmt.Println()

	// Example 10: Same Runner (Reply to Self)
	fmt.Println("Example 10: Reply to Same Runner")
	{
		runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

		runner.PostTask(func(ctx context.Context) {
			me := taskrunner.GetCurrentTaskRunner(ctx)

			fmt.Println("  [Runner] Starting task...")
			core.PostTaskAndReplyWithResult(
				me, // Task runs on same runner
				func(ctx context.Context) (int, error) {
					fmt.Println("  [Runner] Computing...")
					time.Sleep(100 * time.Millisecond)
					return 42, nil
				},
				func(ctx context.Context, result int, err error) {
					fmt.Printf("  [Runner] Got result: %d\n", result)
				},
				me, // Reply also on same runner
			)
		})

		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println()

	fmt.Println("=== Examples Finished ===")
}
