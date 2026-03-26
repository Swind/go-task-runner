# Redesign DESIGN.md Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace `docs/DESIGN.md` with a design-philosophy document (no code) and add auto-generated `docs/API.md` for interface reference.

**Architecture:** Split documentation into two files: DESIGN.md (prose + ASCII diagrams, architecture decisions) and API.md (auto-generated from Go AST). A generation script at `scripts/gen-api-doc.go` parses exported symbols from `core/` and root package and outputs markdown.

**Tech Stack:** Go `go/ast` + `go/token` + `go/types` for AST parsing. Markdown output.

---

### Task 1: Create API doc generation script

**Files:**
- Create: `scripts/gen-api-doc.go`
- Test: `scripts/gen-api-doc_test.go`

**Step 1: Write the generation script**

The script should:
- Accept a single argument: output file path (or `-` for stdout)
- Parse all `.go` files in `core/` and root package (exclude `*_test.go`, `scripts/`)
- Extract exported symbols organized by category:
  - **Interfaces** — name, method signatures
  - **Structs** — name, exported fields
  - **Constructors** — `New*` and `Default*` functions, full signature
  - **Functions** — exported package-level functions, full signature
  - **Types** — `type` aliases and named types (Task, TaskID, TaskPriority, QueuePolicy, etc.)
  - **Constants** — exported constants with values
- Output markdown grouped by package, then by category
- Use `go/doc` package (simpler than raw AST) to extract documentation comments

```go
// scripts/gen-api-doc.go
package main

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	// ... implementation
}
```

Key implementation details:
- Use `doc.New()` from `go/doc` to get filtered documentation (only exported symbols)
- Group by package: `taskrunner` (root) then `core`
- Within each package, group: Types (interfaces, structs, named types), Constructors, Functions, Constants
- For interfaces: list each method with full signature
- For structs: list exported fields
- Use code fences with `go` language tag
- Include doc comments above each symbol (trimmed)

**Step 2: Write a simple smoke test**

Create `scripts/gen-api-doc_test.go` that:
- Runs the script with `--output /dev/null` (or equivalent)
- Verifies it exits with code 0
- Verifies output contains expected package headers (`# Package: taskrunner`, `# Package: core`)

**Step 3: Run the test**

Run: `go test ./scripts/...`
Expected: PASS

**Step 4: Generate API.md and inspect**

Run: `go run scripts/gen-api-doc.go docs/API.md`
Then: `head -100 docs/API.md` to verify structure

**Step 5: Commit**

```bash
git add scripts/gen-api-doc.go scripts/gen-api-doc_test.go docs/API.md
git commit -m "docs: add auto-generated API.md and generation script"
```

---

### Task 2: Rewrite DESIGN.md — Sections 1-2 (Overview + Core Philosophy)

**Files:**
- Modify: `docs/DESIGN.md` (replace entirely)

**Step 1: Write the new DESIGN.md header, Overview, and Core Philosophy sections**

Replace the entire file with the new structure. Sections 1-2 content:

```
# Chromium-Inspired Threading Architecture in Go — Design Guide

For the JobManager-specific design, see [JOB_MANAGER.md](JOB_MANAGER.md).
For the complete API reference, see [API.md](API.md).

## 1. Overview

[One paragraph describing the project]

[ASCII diagram showing component relationships:
  TaskScheduler ← ThreadPool ← TaskRunners (Sequenced/Parallel/SingleThread)]

[Package structure: root (facade) + core/ (implementation)]

## 2. Core Philosophy

### Centralized Scheduling
[Why TaskScheduler is the sole decision-maker]

### Priority & Fairness
[TaskTraits priority levels, SequencedRunner one-at-a-time behavior]

### Separation of Duty
[Scheduler decides when, ThreadPool just executes]

### Plugin Extensibility
[PanicHandler, Metrics, RejectedTaskHandler via TaskSchedulerConfig]
```

**Step 2: Verify markdown renders correctly**

Open the file and visually inspect headers and ASCII diagram alignment.

**Step 3: Commit**

```bash
git add docs/DESIGN.md
git commit -m "docs: rewrite DESIGN.md overview and philosophy sections"
```

---

### Task 3: Rewrite DESIGN.md — Section 3 (The Brain — TaskScheduler)

**Files:**
- Modify: `docs/DESIGN.md`

**Step 1: Write Section 3**

Append to DESIGN.md:

```
## 3. The Brain — TaskScheduler

### Scheduling Model
[Pull model: single TaskQueue + signal channel, workers call GetWork()]
[Why single queue instead of per-priority queues]

### DelayManager
[Timer Pump pattern: min-heap + single timer + wakeup channel]
[Batch expired processing: processExpiredTasks collects all ready tasks under lock]
[Why batch: reduces lock contention vs one-at-a-time]

### Shutdown
[Shutdown: immediate, clears queue, stops DelayManager]
[ShutdownGraceful: waits for queued+active tasks with timeout, then force-clears]
[Why atomics for metrics: no lock contention on hot path]
```

**Step 2: Commit**

```bash
git add docs/DESIGN.md
git commit -m "docs: rewrite DESIGN.md TaskScheduler section"
```

---

### Task 4: Rewrite DESIGN.md — Section 4 (The Muscles — GoroutineThreadPool)

**Files:**
- Modify: `docs/DESIGN.md`

**Step 1: Write Section 4**

Append to DESIGN.md:

```
## 4. The Muscles — GoroutineThreadPool

### Worker Loop
[Dumb pull model: GetWork → execute → OnTaskStart/OnTaskEnd → recover panic]
[No scheduling logic, purely execution]

### ThreadPool as Interface
[Why ThreadPool is an interface: enables testing with mocks, different backends]
[10 methods: PostInternal, PostDelayedInternal, Start, Stop, ID, IsRunning, WorkerCount, QueuedTaskCount, ActiveTaskCount, DelayedTaskCount]

### Global Singleton
[InitGlobalThreadPool / GetGlobalThreadPool / ShutdownGlobalThreadPool]
[CreateTaskRunner convenience function]
[Why: most use cases need a single shared pool]

### Graceful Stop
[StopGraceful: waits for active tasks with timeout before stopping]
[Stop: immediate, cancels context]
```

**Step 2: Commit**

```bash
git add docs/DESIGN.md
git commit -m "docs: rewrite DESIGN.md ThreadPool section"
```

---

### Task 5: Rewrite DESIGN.md — Section 5 (Virtual Threads — TaskRunners)

**Files:**
- Modify: `docs/DESIGN.md`

**Step 1: Write Section 5**

Append to DESIGN.md:

```
## 5. Virtual Threads — TaskRunners

All three runners implement the TaskRunner interface (20 methods).
They provide sequential or bounded-concurrent execution guarantees
without requiring users to manage goroutines or channels directly.

### 5.1 SequencedTaskRunner

[Design intent: sequential execution with fairness]
[Lock-free CAS pattern: runningCount (0=idle, 1=running/scheduled)]
[ensureRunning: CAS 0→1 then post runLoop to thread pool]
[runLoop: pop one task, execute, check for more, either re-post or go idle]
[Why CAS instead of mutex: avoids holding mutex across post-and-return]
[Why yield after each task: allows scheduler to re-evaluate priorities]

### 5.2 ParallelTaskRunner

[Design intent: bounded concurrent execution with coordination]
[Internal SingleThreadTaskRunner as scheduler goroutine: serializes all scheduling decisions]
[Barrier task system: pendingBarrierTask + barrierTaskIDs for WaitIdle/FlushAsync]
[MaxConcurrency controls parallelism; scheduler goroutine dispatches tasks to thread pool]
[Why use SingleThreadTaskRunner as scheduler: simple serialization without additional locking]

### 5.3 SingleThreadTaskRunner

[Design intent: dedicated goroutine for blocking IO or thread affinity]
[Own channel + own goroutine, bypasses global scheduler]
[QueuePolicy: Drop (default), Reject (returns error), Wait (blocks until space)]
[Delayed tasks use time.AfterFunc directly (not DelayManager) for IO affinity]
[Shutdown vs Stop: Shutdown marks closed and signals waiters; Stop cancels context and waits for goroutine]
[RejectionCallback for custom handling when queue is full]
```

**Step 2: Commit**

```bash
git add docs/DESIGN.md
git commit -m "docs: rewrite DESIGN.md TaskRunners section"
```

---

### Task 6: Rewrite DESIGN.md — Section 6 (Task Patterns)

**Files:**
- Modify: `docs/DESIGN.md`

**Step 1: Write Section 6**

Append to DESIGN.md:

```
## 6. Task Patterns

### 6.1 Task-and-Reply

[Generic result passing: TaskWithResult[T] returns (T, error), ReplyWithResult[T] receives result]
[PostTaskAndReplyWithResult and variants]
[Panic semantics: if task panics, reply is NOT executed]
[Root package wrapper functions in reply.go]

### 6.2 Repeating Tasks

[Fixed-delay semantics (not fixed-rate): interval starts after previous execution completes]
[PostRepeatingTask / PostRepeatingTaskWithTraits / PostRepeatingTaskWithInitialDelay]
[RepeatingTaskHandle: Stop() cancels, IsStopped() checks state]
[All three runners implement repeating task variants]

### 6.3 Named Tasks

[PostTaskNamed / PostTaskWithTraitsNamed variants]
[Debug and metadata purpose; name stored in task context]

### 6.4 Context Propagation

[taskRunnerKey injected into context by all three runners' runLoop]
[GetCurrentTaskRunner(ctx) retrieves the runner that is executing the current task]
[Enables tasks to post back to their own runner without holding a reference]
```

**Step 2: Commit**

```bash
git add docs/DESIGN.md
git commit -m "docs: rewrite DESIGN.md task patterns section"
```

---

### Task 7: Rewrite DESIGN.md — Section 7 (Observability & Lifecycle)

**Files:**
- Modify: `docs/DESIGN.md`

**Step 1: Write Section 7**

Append to DESIGN.md:

```
## 7. Observability & Lifecycle

### 7.1 Stats

[RunnerStats: Name, Type, Pending, Running, Rejected, Closed, BarrierPending]
[PoolStats: ID, Workers, Queued, Active, Delayed, Running]
[Stats() method on all runners and pool]

### 7.2 Synchronization

[WaitIdle(ctx): blocks until queue is empty and no tasks are executing]
[FlushAsync(callback): posts a barrier callback that runs after all in-flight tasks complete]
[WaitShutdown(ctx): blocks until the runner has fully stopped]
[Differences: WaitIdle is for "drain", FlushAsync is for "barrier callback", WaitShutdown is for "teardown"]

### 7.3 Shutdown Semantics

[SequencedTaskRunner: Shutdown() — marks closed, drains queue, waits for runLoop]
[ParallelTaskRunner: Shutdown() — marks closed, drains queue, waits for all workers]
[SingleThreadTaskRunner: Shutdown() — marks closed, signals waiters (does NOT cancel context); Stop() — cancels context, waits for goroutine]

### 7.4 Plugin Architecture

[TaskSchedulerConfig: PanicHandler, Metrics, RejectedTaskHandler]
[DefaultPanicHandler: logs panic info + stack trace]
[NilMetrics: no-op implementation (default)]
[DefaultRejectedTaskHandler: logs rejection]
[Prometheus MetricsExporter in observability/prometheus/ as extension point]
```

**Step 2: Commit**

```bash
git add docs/DESIGN.md
git commit -m "docs: rewrite DESIGN.md observability and lifecycle section"
```

---

### Task 8: Final verification

**Step 1: Regenerate API.md to ensure it's current**

Run: `go run scripts/gen-api-doc.go docs/API.md`

**Step 2: Verify DESIGN.md structure**

Run: `grep "^## " docs/DESIGN.md`
Expected output:
```
## 1. Overview
## 2. Core Philosophy
## 3. The Brain — TaskScheduler
## 4. The Muscles — GoroutineThreadPool
## 5. Virtual Threads — TaskRunners
## 6. Task Patterns
## 7. Observability & Lifecycle
```

**Step 3: Run all tests to ensure nothing broke**

Run: `go test ./...`
Expected: all PASS

**Step 4: Run lint**

Run: `golangci-lint run --timeout=5m core/... .`
Expected: no errors (scripts/ is excluded from lint scope)

**Step 5: Cross-check API.md against code**

Verify API.md contains these key symbols:
- `TaskRunner` interface
- `ThreadPool` interface
- `WorkSource` interface
- `PanicHandler`, `Metrics`, `RejectedTaskHandler` interfaces
- `TaskQueue` interface
- `RepeatingTaskHandle` interface
- All three runner constructors
- `GoroutineThreadPool` methods
- `TaskScheduler` constructors and methods
- `RunnerStats`, `PoolStats` structs

**Step 6: Final commit (if any cleanup needed)**

```bash
git add -A
git commit -m "docs: complete DESIGN.md rewrite and API.md generation"
```
