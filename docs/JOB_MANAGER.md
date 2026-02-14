# JobManager Design

This document describes the current `core.JobManager` architecture and runtime behavior.

## Overview

`JobManager` uses a three-layer runner model:

- `controlRunner`: fast in-memory control operations (registration, dedupe checks, active job map updates).
- `ioRunner`: serialized persistence operations (`CreateJob`, `SaveJob`, `UpdateStatus`, recovery reads).
- `executionRunner`: user handler execution (can be slow/blocking).

This separation keeps control-path latency low while preserving ordered persistence behavior.

## Durable-Ack Submission

`SubmitJob` / `SubmitDelayedJob` use durable-ack semantics:

1. Serialize user args.
2. Post to `controlRunner` for validation and active-map admission.
3. Persist on `ioRunner`.
4. Return success to caller only after persistence succeeds.
5. Schedule handler execution on `executionRunner`.

If persistence fails, active-map state is rolled back and submission returns an error.

## Persistence Abstraction

`JobManager` depends on `JobStore` and optionally `DurableJobStore`:

- `JobStore`: baseline interface (`SaveJob`, `UpdateStatus`, `GetJob`, `ListJobs`, `GetRecoverableJobs`, `DeleteJob`).
- `DurableJobStore`: optional interface with atomic `CreateJob`.

Behavior:

- If store implements `DurableJobStore`, `CreateJob` is used for atomic create semantics.
- Otherwise, manager uses compatibility fallback (`GetJob` + `SaveJob`).

`MemoryJobStore` implements both and is the default in-memory reference implementation.

## Concurrency Model

Core state coordination is lock-minimized by design:

- `handlers` and `activeJobs` use `sync.Map` for concurrent-safe access.
- Critical sequencing is guaranteed by `controlRunner` (single sequence), avoiding mutexes for check-and-add style logic.
- Runtime config (`retryPolicy`, `logger`, `errorHandler`) uses `cfgMu` (`sync.RWMutex`) because config may be changed/read by different goroutines.

This means extra `sync.Mutex` is generally not needed for job lifecycle flow itself; synchronization is primarily structural via runner sequencing.

## Lifecycle

### Start / Recovery

`Start` performs recovery in two phases:

- Mark persisted `RUNNING` jobs as `FAILED` (interrupted by restart).
- Re-enqueue recoverable `PENDING` jobs by routing through control/execution flow.

### Cancel

`CancelJob` is handled on `controlRunner`, cancels job context, and lets normal finalize path update final status.

### Shutdown

`Shutdown`:

1. Marks manager closed.
2. Cancels active jobs.
3. Waits for active set to drain.
4. Shuts down runners in order (`control` -> `io` -> `execution`).

## Read APIs

Read paths are exposed as explicit functions:

- `ListJobs(ctx, filter)`
- `GetJob(ctx, id)`
- `GetActiveJobCount()`
- `GetActiveJobs()`

Persistence-backed reads go through store APIs; active snapshot reads use the in-memory active map.

## Retry and Error Handling

IO calls run through retry policy logic with configurable:

- retry policy (max retries, delay strategy)
- logger
- optional terminal error handler callback

`ErrJobAlreadyExists` is treated as a non-retriable duplicate condition.
