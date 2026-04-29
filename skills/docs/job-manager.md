# Job Manager

Durable background job processing with persistence, recovery, and retry — built on top of TaskRunners.

## Import

```go
import (
    taskrunner "github.com/Swind/go-task-runner"
    "github.com/Swind/go-task-runner/job"
)
```

---

## Architecture

```
SubmitJob()
    │
    ▼
controlRunner  ← JobManager control logic (UserBlocking priority)
    │
    ▼
ioRunner       ← Persistence to store (UserVisible priority)
    │
    ▼
executionRunner ← Handler execution (BestEffort priority)
```

Three separate TaskRunners isolate control logic, I/O, and execution — failures in one layer do not cascade.

---

## Job Stores

### MemoryJobStore (no persistence)

```go
store := job.NewMemoryJobStore()
```

Use for: testing, ephemeral queues, development.

### SQLiteJobStore (persistent)

```go
import _ "modernc.org/sqlite"

db, err := sql.Open("sqlite", "jobs.db")
if err != nil { log.Fatal(err) }
defer db.Close()

store, err := job.NewSQLiteJobStore(db)
if err != nil { log.Fatal(err) }
```

Use for: jobs that must survive process restarts.

---

## Setup

```go
taskrunner.InitGlobalThreadPool(4)
defer taskrunner.ShutdownGlobalThreadPool()

controlRunner  := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{Priority: taskrunner.TaskPriorityUserBlocking})
ioRunner       := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{Priority: taskrunner.TaskPriorityUserVisible})
executionRunner := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{Priority: taskrunner.TaskPriorityBestEffort})

manager := job.NewJobManager(
    controlRunner,
    ioRunner,
    executionRunner,
    store,
    job.NewJSONSerializer(),
)
manager.SetShutdownRunners(true)  // manager owns runners, shuts them down on Shutdown()
manager.SetLogger(job.NewDefaultLogger())
```

---

## Register Handlers

Handlers must be registered **before** `Start()`.

```go
type SendEmailArgs struct {
    To      string
    Subject string
    Body    string
}

ctx := context.Background()

err := job.RegisterHandler(manager, ctx, "send_email",
    func(ctx context.Context, args SendEmailArgs) error {
        return sendEmail(args.To, args.Subject, args.Body)
    },
)
```

Handler signature: `func(ctx context.Context, args T) error`

---

## Start

`Start` recovers any `PENDING` jobs from the store (important for SQLite persistence):

```go
if err := manager.Start(ctx); err != nil {
    log.Fatal(err)
}
```

---

## Submit Jobs

```go
// Immediate job
err := manager.SubmitJob(ctx, "email-001", "send_email",
    SendEmailArgs{To: "user@example.com", Subject: "Hi"},
    taskrunner.DefaultTaskTraits(),
)

// Delayed job (runs after 5 minutes)
err := manager.SubmitDelayedJob(ctx, "reminder-001", "send_email",
    SendEmailArgs{To: "user@example.com", Subject: "Reminder"},
    5*time.Minute,
    taskrunner.DefaultTaskTraits(),
)
```

Job IDs must be unique. `ErrJobAlreadyExists` is returned for duplicates.

---

## Job Status Flow

```
PENDING → RUNNING → COMPLETED
                 → FAILED
                 → CANCELLED
```

### List Jobs

```go
// All jobs
all, err := store.ListJobs(ctx, job.JobFilter{})

// Filter by status
pending, err := store.ListJobs(ctx, job.JobFilter{
    Status: job.JobStatusPending,
})

// Filter by type
emails, err := store.ListJobs(ctx, job.JobFilter{
    Type: "send_email",
})
```

### Get a Single Job

```go
entity, err := manager.GetJob(ctx, "email-001")
if err != nil {
    // job.ErrJobNotFound if not found
}
fmt.Printf("Status: %s, Result: %s\n", entity.Status, entity.Result)
```

---

## Cancel a Job

Only `PENDING` or `RUNNING` jobs can be cancelled.

```go
err := manager.CancelJob(ctx, "email-001")
```

---

## Retry Policy

Controls how IO operations (persistence) are retried on failure.

```go
manager.SetRetryPolicy(job.RetryPolicy{
    MaxAttempts: 5,
    InitialDelay: 100 * time.Millisecond,
    MaxDelay:     10 * time.Second,
    Multiplier:   2.0,
})

// Or use the default
manager.SetRetryPolicy(job.DefaultRetryPolicy())
```

---

## Shutdown

```go
if err := manager.Shutdown(ctx); err != nil {
    log.Printf("shutdown error: %v", err)
}
```

If `SetShutdownRunners(true)` was called, this also shuts down the three task runners.

---

## Crash Recovery

With SQLiteJobStore, jobs survive process restarts:

```go
// First run: submit jobs, process some, crash mid-flight
manager.Start(ctx)
manager.SubmitJob(ctx, "job-1", "send_email", args, traits)
// process crashes...

// Second run: Start() recovers PENDING jobs automatically
manager.Start(ctx)  // job-1 is recovered and re-executed
```

---

## Active Job Monitoring

```go
count := manager.GetActiveJobCount()
fmt.Printf("Active jobs: %d\n", count)

jobs := manager.GetActiveJobs()
for _, j := range jobs {
    fmt.Printf("  %s: %s\n", j.ID, j.Type)
}
```

---

## Common Patterns

### Idempotent Job IDs

Use deterministic IDs to prevent duplicate submissions:

```go
import "crypto/sha256"

func submitEmailJob(userID int, subject string) error {
    raw := fmt.Sprintf("email-%d-%s", userID, subject)
    hash := sha256.Sum256([]byte(raw))
    id := fmt.Sprintf("%x", hash[:8])

    err := manager.SubmitJob(ctx, id, "send_email", args, traits)
    if errors.Is(err, job.ErrJobAlreadyExists) {
        return nil  // already submitted
    }
    return err
}
```

### Multiple Job Types

```go
job.RegisterHandler(manager, ctx, "send_email", handleEmail)
job.RegisterHandler(manager, ctx, "process_payment", handlePayment)
job.RegisterHandler(manager, ctx, "send_notification", handleNotification)
```

---

## See Also

- [`templates/job-manager.go`](../templates/job-manager.go) — complete working example
- [`lifecycle.md`](lifecycle.md) — runner shutdown patterns
- [`runners.md`](runners.md) — runner selection guide
