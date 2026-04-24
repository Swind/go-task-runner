package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/job"
)

// SendEmailArgs holds arguments for the "send_email" job type.
type SendEmailArgs struct {
	To      string
	Subject string
	Body    string
}

func main() {
	fmt.Println("=== SQLite JobStore Example ===")

	// 1. Open an SQLite database (file-based so jobs survive restarts).
	db, err := sql.Open("sqlite", "jobs.db")
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	// 2. Create the SQLite-backed store and initialize the table.
	store, err := job.NewSQLiteJobStore(db)
	if err != nil {
		log.Fatalf("NewSQLiteJobStore: %v", err)
	}

	// 3. Initialize the global thread pool and create three task runners
	//    for the JobManager's three-layer architecture.
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	controlRunner := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{Priority: taskrunner.TaskPriorityUserBlocking})
	ioRunner := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{Priority: taskrunner.TaskPriorityUserVisible})
	executionRunner := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{Priority: taskrunner.TaskPriorityBestEffort})

	// 4. Build the JobManager wired to the SQLite store.
	manager := job.NewJobManager(
		controlRunner,
		ioRunner,
		executionRunner,
		store,
		job.NewJSONSerializer(),
	)
	manager.SetShutdownRunners(true)
	manager.SetLogger(job.NewDefaultLogger())

	ctx := context.Background()

	// 5. Register a handler for the "send_email" job type.
	if err := job.RegisterHandler(manager, ctx, "send_email",
		func(ctx context.Context, args SendEmailArgs) error {
			fmt.Printf("  [handler] Sending email to %s: %q\n", args.To, args.Subject)
			time.Sleep(200 * time.Millisecond) // simulate sending
			fmt.Printf("  [handler] Email sent to %s\n", args.To)
			return nil
		},
	); err != nil {
		log.Fatalf("RegisterHandler: %v", err)
	}

	// 6. Start the manager (triggers recovery of any PENDING jobs from prior runs).
	if err := manager.Start(ctx); err != nil {
		log.Fatalf("Start: %v", err)
	}

	// 7. Submit three jobs. Each is persisted to SQLite before SubmitJob returns.
	jobs := []SendEmailArgs{
		{To: "alice@example.com", Subject: "Welcome!", Body: "Hello Alice"},
		{To: "bob@example.com", Subject: "Invoice", Body: "Please pay"},
		{To: "carol@example.com", Subject: "Newsletter", Body: "Monthly update"},
	}

	for i, args := range jobs {
		id := fmt.Sprintf("email-%d", i+1)
		if err := manager.SubmitJob(ctx, id, "send_email", args, taskrunner.DefaultTaskTraits()); err != nil {
			log.Printf("SubmitJob %s: %v", id, err)
			continue
		}
		fmt.Printf("[main] Submitted job %s → %s\n", id, args.To)
	}

	// 8. List all jobs in the store to show they were persisted.
	fmt.Println("\n[main] Jobs in SQLite store:")
	all, err := store.ListJobs(ctx, job.JobFilter{})
	if err != nil {
		log.Fatalf("ListJobs: %v", err)
	}
	for _, j := range all {
		fmt.Printf("  id=%-10s type=%-12s status=%s\n", j.ID, j.Type, j.Status)
	}

	// 9. Wait for execution to finish, then shut down.
	fmt.Println("\n[main] Waiting for jobs to complete…")
	time.Sleep(1 * time.Second)

	if err := manager.Shutdown(ctx); err != nil {
		log.Printf("Shutdown: %v", err)
	}

	// 10. Show final state.
	fmt.Println("\n[main] Final job states in SQLite store:")
	all, _ = store.ListJobs(ctx, job.JobFilter{})
	for _, j := range all {
		fmt.Printf("  id=%-10s status=%-10s result=%s\n", j.ID, j.Status, j.Result)
	}

	fmt.Println("\n=== Example Finished ===")
	fmt.Println("(jobs.db persists — re-run to see recovery in action)")
}
