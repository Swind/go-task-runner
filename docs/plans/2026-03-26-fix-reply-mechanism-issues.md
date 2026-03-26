# Fix PostTaskAndReply Mechanism Issues Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix 5 issues in the PostTaskAndReply mechanism: duplicated panic recovery logic, nil replyRunner inconsistency, unused reply traits in `postTaskAndReplyInternal`, hardcoded `fmt.Printf` panic handling, and missing re-export of generic functions.

**Architecture:** Extract a shared `executeTaskWithRecovery` helper that uses the thread pool's PanicHandler (when available) instead of raw `fmt.Printf`. Refactor both the normal and delayed paths to use this helper. Fix nil replyRunner semantics to be consistent. Re-export generic functions from root package.

**Tech Stack:** Go 1.21+, existing `core` package patterns

---

### Task 1: Extract shared panic recovery helper

**Files:**
- Modify: `core/task_and_reply.go:1-55`

**Step 1: Write the failing test**

Add a test in `core/task_and_reply_test.go` that verifies panic recovery in the delayed path also uses the pool's PanicHandler (matching the non-delayed path behavior). This test will fail because the delayed path currently uses `fmt.Printf`.

```go
func TestPostDelayedTaskAndReplyWithResult_PanicUsesHandler(t *testing.T) {
	panicHandler := NewTestPanicHandler()
	pool := NewGoroutineThreadPool(WithTaskSchedulerConfig(
		&TaskSchedulerConfig{PanicHandler: panicHandler},
	))
	pool.Start()
	defer pool.Stop()

	targetRunner := NewSequencedTaskRunner(pool)
	replyRunner := NewSequencedTaskRunner(pool)
	var replyExecuted atomic.Bool

	PostDelayedTaskAndReplyWithResult(
		targetRunner,
		func(ctx context.Context) (int, error) {
			panic("delayed boom")
		},
		10*time.Millisecond,
		func(ctx context.Context, result int, err error) {
			replyExecuted.Store(true)
		},
		replyRunner,
	)

	targetRunner.WaitIdle(context.Background(), 3*time.Second)

	if panicHandler.CallCount() != 1 {
		t.Errorf("panic handler call count = %d, want 1", panicHandler.CallCount())
	}
	if replyExecuted.Load() {
		t.Error("reply should not execute after task panic")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestPostDelayedTaskAndReplyWithResult_PanicUsesHandler ./core/`
Expected: FAIL — panic handler call count = 0 (delayed path uses fmt.Printf, not PanicHandler)

**Step 3: Implement `executeTaskWithRecovery` helper**

Add to `core/task_and_reply.go`, before the existing `postTaskAndReplyInternalWithTraits` function:

```go
type panicRecoveryContext struct {
	handler    PanicHandler
	runnerName string
	workerID   int
}

func getPanicRecovery(tp ThreadPool, runnerName string) panicRecoveryContext {
	if tp == nil {
		return panicRecoveryContext{}
	}
	type schedulerAccessor interface {
		GetScheduler() *TaskScheduler
	}
	sa, ok := tp.(schedulerAccessor)
	if !ok || sa.GetScheduler() == nil {
		return panicRecoveryContext{}
	}
	return panicRecoveryContext{
		handler:    sa.GetScheduler().GetPanicHandler(),
		runnerName: runnerName,
		workerID:   -1,
	}
}

func executeTaskWithRecovery(ctx context.Context, task Task, prc panicRecoveryContext) (panicked bool) {
	panicked = true
	func() {
		defer func() {
			if r := recover(); r != nil {
				if prc.handler != nil {
					prc.handler.HandlePanic(ctx, prc.runnerName, prc.workerID, r, debug.Stack())
				} else {
					fmt.Printf("[TaskAndReply] Task panicked, reply will not run: %v\n", r)
				}
			}
		}()
		task(ctx)
		panicked = false
	}()
	return panicked
}
```

Add `"runtime/debug"` to imports.

**Step 4: Refactor `postTaskAndReplyInternalWithTraits` to use the helper**

Replace the inline panic recovery block (lines 33-52) with:

```go
func postTaskAndReplyInternalWithTraits(
	targetRunner TaskRunner,
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	if replyRunner == nil {
		targetRunner.PostTaskWithTraits(task, taskTraits)
		return
	}

	prc := getPanicRecovery(targetRunner.GetThreadPool(), targetRunner.Name())

	wrappedTask := func(ctx context.Context) {
		if !executeTaskWithRecovery(ctx, task, prc) {
			replyRunner.PostTaskWithTraits(reply, replyTraits)
		}
	}

	targetRunner.PostTaskWithTraits(wrappedTask, taskTraits)
}
```

**Step 5: Refactor delayed path to use the helper**

Replace `delayedWrapper` in `PostDelayedTaskAndReplyWithResultAndTraits` (lines 239-256) with:

```go
delayedWrapper := func(ctx context.Context) {
	prc := getPanicRecovery(targetRunner.GetThreadPool(), targetRunner.Name())
	if !executeTaskWithRecovery(ctx, wrappedTask, prc) && replyRunner != nil {
		replyRunner.PostTaskWithTraits(wrappedReply, replyTraits)
	}
}
```

**Step 6: Run test to verify it passes**

Run: `go test -v -run TestPostDelayedTaskAndReplyWithResult_PanicUsesHandler ./core/`
Expected: PASS

**Step 7: Run all existing tests**

Run: `go test -v ./core/...`
Expected: All PASS (existing panic behavior tests still pass because `DefaultPanicHandler` also prints to stdout)

**Step 8: Commit**

```bash
git add core/task_and_reply.go core/task_and_reply_test.go
git commit -m "refactor(task-and-reply): extract shared panic recovery helper"
```

---

### Task 2: Fix nil replyRunner inconsistency

**Files:**
- Modify: `core/task_and_reply.go:27-31` (nil early-return path)

**Step 1: Write the failing test**

Add a test verifying that when `replyRunner == nil` and the task panics, the panic is still recovered via PanicHandler (not bubbled up):

```go
func TestPostTaskAndReply_NilReplyRunner_PanicRecovered(t *testing.T) {
	panicHandler := NewTestPanicHandler()
	pool := NewGoroutineThreadPool(WithTaskSchedulerConfig(
		&TaskSchedulerConfig{PanicHandler: panicHandler},
	))
	pool.Start()
	defer pool.Stop()

	targetRunner := NewSequencedTaskRunner(pool)

	targetRunner.PostTaskAndReply(
		func(ctx context.Context) {
			panic("nil reply runner boom")
		},
		func(ctx context.Context) {},
		nil,
	)

	targetRunner.WaitIdle(context.Background(), 3*time.Second)

	if panicHandler.CallCount() != 1 {
		t.Errorf("panic handler call count = %d, want 1", panicHandler.CallCount())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestPostTaskAndReply_NilReplyRunner_PanicRecovered ./core/`
Expected: FAIL — currently the nil path posts task directly without any wrapping, so panic goes to runner's own recovery (which does use PanicHandler), but this test is about making the *code path* consistent. If this test passes already, that's fine — the fix is about structural consistency, not behavioral change. Verify the behavior is the same.

**Step 3: Fix the nil path to also use wrapped execution**

Change the nil early-return to wrap the task with recovery too:

```go
func postTaskAndReplyInternalWithTraits(
	targetRunner TaskRunner,
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyTraits TaskTraits,
	replyRunner TaskRunner,
) {
	prc := getPanicRecovery(targetRunner.GetThreadPool(), targetRunner.Name())

	if replyRunner == nil {
		wrappedTask := func(ctx context.Context) {
			executeTaskWithRecovery(ctx, task, prc)
		}
		targetRunner.PostTaskWithTraits(wrappedTask, taskTraits)
		return
	}

	wrappedTask := func(ctx context.Context) {
		if !executeTaskWithRecovery(ctx, task, prc) {
			replyRunner.PostTaskWithTraits(reply, replyTraits)
		}
	}

	targetRunner.PostTaskWithTraits(wrappedTask, taskTraits)
}
```

**Step 4: Run test**

Run: `go test -v -run TestPostTaskAndReply_NilReplyRunner ./core/`
Expected: PASS

**Step 5: Run all tests**

Run: `go test -v ./core/...`
Expected: All PASS

**Step 6: Commit**

```bash
git add core/task_and_reply.go core/task_and_reply_test.go
git commit -m "fix(task-and-reply): consistent panic recovery for nil replyRunner"
```

---

### Task 3: Fix `postTaskAndReplyInternal` unused reply traits parameter

**Files:**
- Modify: `core/task_and_reply.go:57-73`

**Step 1: Rename and simplify the function**

The `traits` parameter is used for the task, and reply always gets `DefaultTaskTraits()`. Rename to make intent explicit and remove the misleading comment:

```go
func postTaskAndReplyInternal(
	targetRunner TaskRunner,
	task Task,
	taskTraits TaskTraits,
	reply Task,
	replyRunner TaskRunner,
) {
	postTaskAndReplyInternalWithTraits(
		targetRunner,
		task,
		taskTraits,
		reply,
		DefaultTaskTraits(),
		replyRunner,
	)
}
```

Update all three callers (Sequenced, Parallel, SingleThread) to pass named parameter `taskTraits:`:

Sequenced (`core/sequenced_task_runner.go:422`):
```go
postTaskAndReplyInternal(r, task, DefaultTaskTraits(), reply, replyRunner)
```

Parallel (`core/parallel_task_runner.go:360`):
```go
postTaskAndReplyInternal(r, task, DefaultTaskTraits(), reply, replyRunner)
```

SingleThread (`core/single_thread_task_runner.go:453`):
```go
postTaskAndReplyInternal(r, task, DefaultTaskTraits(), reply, replyRunner)
```

**Step 2: Run all tests**

Run: `go test -v ./core/...`
Expected: All PASS

**Step 3: Commit**

```bash
git add core/task_and_reply.go core/sequenced_task_runner.go core/parallel_task_runner.go core/single_thread_task_runner.go
git commit -m "refactor(task-and-reply): clarify postTaskAndReplyInternal signature"
```

---

### Task 4: Re-export generic PostTaskAndReply functions from root package

**Files:**
- Modify: `pool.go` (or create a new `reply.go` in root)
- Modify: `types.go`

**Step 1: Add function aliases in root package**

Create a `reply.go` file in the root package with re-exports:

```go
package taskrunner

import (
	"time"

	core "github.com/Swind/go-task-runner/core"
)

func PostTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task core.TaskWithResult[T],
	reply core.ReplyWithResult[T],
	replyRunner TaskRunner,
) {
	core.PostTaskAndReplyWithResult(targetRunner, task, reply, replyRunner)
}

func PostTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task core.TaskWithResult[T],
	taskTraits core.TaskTraits,
	reply core.ReplyWithResult[T],
	replyTraits core.TaskTraits,
	replyRunner TaskRunner,
) {
	core.PostTaskAndReplyWithResultAndTraits(targetRunner, task, taskTraits, reply, replyTraits, replyRunner)
}

func PostDelayedTaskAndReplyWithResult[T any](
	targetRunner TaskRunner,
	task core.TaskWithResult[T],
	delay time.Duration,
	reply core.ReplyWithResult[T],
	replyRunner TaskRunner,
) {
	core.PostDelayedTaskAndReplyWithResult(targetRunner, task, delay, reply, replyRunner)
}

func PostDelayedTaskAndReplyWithResultAndTraits[T any](
	targetRunner TaskRunner,
	task core.TaskWithResult[T],
	delay time.Duration,
	taskTraits core.TaskTraits,
	reply core.ReplyWithResult[T],
	replyTraits core.TaskTraits,
	replyRunner TaskRunner,
) {
	core.PostDelayedTaskAndReplyWithResultAndTraits(targetRunner, task, delay, taskTraits, reply, replyTraits, replyRunner)
}
```

**Step 2: Run all tests**

Run: `go test -v ./...`
Expected: All PASS

**Step 3: Run examples to verify nothing breaks**

Run: `go run examples/task_and_reply/main.go`
Expected: All examples execute successfully

**Step 4: Commit**

```bash
git add reply.go
git commit -m "feat(reply): re-export generic PostTaskAndReply functions from root package"
```

---

### Task 5: Run full verification

**Step 1: Run full test suite with race detector**

Run: `go test -race -v ./core/...`
Expected: All PASS

**Step 2: Run CI-mode tests**

Run: `go test -tags=ci -v -coverprofile=coverage.out ./...`
Expected: All PASS

**Step 3: Run linter**

Run: `golangci-lint run --timeout=5m core/... .`
Expected: No errors

**Step 4: Commit (if any lint fixes were needed)**

---

## Summary

| Task | Issue | Scope |
|------|-------|-------|
| 1 | Extract shared `executeTaskWithRecovery` helper | Issues #2 + #3 (duplicated panic recovery + fmt.Printf) |
| 2 | Fix nil replyRunner inconsistency | Issue #5 |
| 3 | Clarify `postTaskAndReplyInternal` signature | Issue #1 |
| 4 | Re-export generic functions from root | Issue #4 |
| 5 | Full verification | — |
