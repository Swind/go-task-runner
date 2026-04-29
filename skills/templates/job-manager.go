// Package main demonstrates JobManager with MemoryJobStore.
// Swap MemoryJobStore for SQLiteJobStore to get persistence and crash recovery.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/job"
)

// Define job argument types as plain structs (must be JSON-serializable).
type SendEmailArgs struct {
	To      string
	Subject string
	Body    string
}

func main() {
	fmt.Println("=== JobManager Example ===")

	// 1. Choose a store.
	//    MemoryJobStore: no persistence, good for testing / ephemeral queues.
	//    SQLiteJobStore: survives restarts (see examples/sqlite_job_store/).
	store := job.NewMemoryJobStore()

	// 2. Initialize the global thread pool.
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	// 3. Create three task runners — one per layer.
	controlRunner := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{
		Priority: taskrunner.TaskPriorityUserBlocking,
	})
	ioRunner := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{
		Priority: taskrunner.TaskPriorityUserVisible,
	})
	executionRunner := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{
		Priority: taskrunner.TaskPriorityBestEffort,
	})

	// 4. Build the JobManager.
	manager := job.NewJobManager(
		controlRunner,
		ioRunner,
		executionRunner,
		store,
		job.NewJSONSerializer(),
	)
	manager.SetShutdownRunners(true) // manager shuts down runners on Shutdown()
	manager.SetLogger(job.NewDefaultLogger())

	ctx := context.Background()

	// 5. Register handlers BEFORE Start().
	if err := job.RegisterHandler(manager, ctx, "send_email",
		func(ctx context.Context, args SendEmailArgs) error {
			fmt.Printf("  Sending email to %s: %q\n", args.To, args.Subject)
			time.Sleep(100 * time.Millisecond) // simulate work
			fmt.Printf("  Sent to %s\n", args.To)
			return nil
		},
	); err != nil {
		log.Fatalf("RegisterHandler: %v", err)
	}

	// 6. Start recovers any PENDING jobs from the store (key for SQLite).
	if err := manager.Start(ctx); err != nil {
		log.Fatalf("Start: %v", err)
	}

	// 7. Submit jobs — each gets a unique ID.
	jobs := []SendEmailArgs{
		{To: "alice@example.com", Subject: "Welcome!"},
		{To: "bob@example.com", Subject: "Invoice"},
	}
	for i, args := range jobs {
		id := fmt.Sprintf("email-%d", i+1)
		if err := manager.SubmitJob(ctx, id, "send_email", args, taskrunner.DefaultTaskTraits()); err != nil {
			log.Printf("SubmitJob %s: %v", id, err)
			continue
		}
		fmt.Printf("[main] Submitted %s → %s\n", id, args.To)
	}

	// 8. Wait for execution then shut down.
	time.Sleep(500 * time.Millisecond)

	if err := manager.Shutdown(ctx); err != nil {
		log.Printf("Shutdown: %v", err)
	}

	// 9. Show final state.
	all, _ := store.ListJobs(ctx, job.JobFilter{})
	fmt.Println("\nFinal job states:")
	for _, j := range all {
		fmt.Printf("  id=%-10s status=%-10s result=%s\n", j.ID, j.Status, j.Result)
	}

	fmt.Println("=== Done ===")
}
