# Task and Reply Pattern

Complete guide to the task-and-reply pattern for UI/background work separation.

## Overview

**The Pattern**: Execute heavy work on background runner, reply with result on UI runner.

**Use Cases**:
- UI + Background work separation
- Network requests with UI updates
- Heavy computation with result callbacks
- Cross-runner communication

---

## PostTaskAndReply Basics

### Simple Form

```go
bgRunner.PostTaskAndReply(
    func(ctx context.Context) {
        // Heavy work on background runner
        data := fetchFromNetwork()
    },
    func(ctx context.Context) {
        // Update UI on UI runner
        updateUI()
    },
    uiRunner,  // Reply runner
)
```

### With GetCurrentTaskRunner

```go
uiRunner.PostTask(func(ctx context.Context) {
    me := taskrunner.GetCurrentTaskRunner(ctx)

    bgRunner.PostTaskAndReply(
        taskFunc,
        replyFunc,
        me,  // Reply to current (UI) runner
    )
})
```

---

## PostTaskAndReplyWithResult[T]

### Generic Result Passing

```go
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (*Data, error) {
        // Task - returns result and error
        return fetchData(), nil
    },
    func(ctx context.Context, data *Data, err error) {
        // Reply - receives result and error
        if err != nil {
            showError(err)
            return
        }
        updateUI(data)
    },
    uiRunner,
)
```

### Type Safety Benefits

```go
// ✅ Type-safe - compiler checks types
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (int, error) {
        return 42, nil
    },
    func(ctx context.Context, result int, err error) {
        fmt.Printf("Result: %d\n", result)
    },
    uiRunner,
)

// ❌ Compile error - type mismatch
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (int, error) {
        return 42, nil
    },
    func(ctx context.Context, result string, err error) {  // Wrong type!
        // ...
    },
    uiRunner,
)
```

---

## Error Handling

### Pattern 1: Check Error in Reply

```go
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (*User, error) {
        user, err := db.QueryUser(id)
        if err != nil {
            return nil, err  // Return error
        }
        return user, nil
    },
    func(ctx context.Context, user *User, err error) {
        if err != nil {
            log.Printf("Error: %v", err)
            showErrorDialog(err)
            return
        }
        // Handle success
        displayUser(user)
    },
    uiRunner,
)
```

### Pattern 2: Task Panic

```go
bgRunner.PostTaskAndReply(
    func(ctx context.Context) {
        panic("task failed!")  // Task panics
    },
    func(ctx context.Context) {
        // Reply NEVER executes if task panics
        updateUI()
    },
    uiRunner,
)
```

**⚠️ Important**: If task panics, reply does NOT execute.

---

## Advanced Patterns

### Pattern 1: Different Priorities

```go
core.PostTaskAndReplyWithResultAndTraits(
    bgRunner,
    func(ctx context.Context) (int, error) {
        // Heavy background work (low priority)
        return compute(), nil
    },
    func(ctx context.Context, result int, err error) {
        // UI update (high priority)
        updateUI(result)
    },
    uiRunner,
    taskrunner.TraitsBestEffort(),   // Low priority task
    taskrunner.TraitsUserBlocking(), // High priority reply
)
```

### Pattern 2: Delayed Task and Reply

```go
core.PostDelayedTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (string, error) {
        // Runs after 1 second delay
        return "delayed result", nil
    },
    func(ctx context.Context, result string, err error) {
        updateUI(result)
    },
    uiRunner,
    1*time.Second,  // Delay
)
```

### Pattern 3: Chained Requests

```go
uiRunner.PostTask(func(ctx context.Context) {
    me := taskrunner.GetCurrentTaskRunner(ctx)

    // First request
    core.PostTaskAndReplyWithResult(
        bgRunner,
        func(ctx context.Context) (string, error) {
            return fetchUserID(), nil
        },
        func(ctx context.Context, userID string, err error) {
            if err != nil {
                showError(err)
                return
            }

            // Second request (chained)
            core.PostTaskAndReplyWithResult(
                bgRunner,
                func(ctx context.Context) (*Profile, error) {
                    return fetchProfile(userID), nil
                },
                func(ctx context.Context, profile *Profile, err error) {
                    if err != nil {
                        showError(err)
                        return
                    }
                    displayProfile(profile)
                },
                me,
            )
        },
        me,
    )
})
```

### Pattern 4: Reply to Same Runner

```go
runner := taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits())

runner.PostTask(func(ctx context.Context) {
    me := taskrunner.GetCurrentTaskRunner(ctx)

    // Task and reply both on same runner
    runner.PostTaskAndReply(
        func(ctx context.Context) {
            // First task
            doWork()
        },
        func(ctx context.Context) {
            // Second task (after first completes)
            doMoreWork()
        },
        me,  // Reply to same runner
    )
})
```

**Use Case**: Sequential task chaining with callback.

---

## Common Patterns

### UI/Background Architecture

```go
type App struct {
    uiRunner *taskrunner.SequencedTaskRunner
    bgRunner *taskrunner.SequencedTaskRunner
}

func (app *App) FetchAndDisplay() {
    app.uiRunner.PostTask(func(ctx context.Context) {
        me := taskrunner.GetCurrentTaskRunner(ctx)

        // Show loading
        app.showLoading()

        core.PostTaskAndReplyWithResult(
            app.bgRunner,
            func(ctx context.Context) (*Data, error) {
                // Heavy network work
                return fetchFromAPI()
            },
            func(ctx context.Context, data *Data, err error) {
                // Update UI
                app.hideLoading()
                if err != nil {
                    app.showError(err)
                    return
                }
                app.displayData(data)
            },
            me,
        )
    })
}
```

### Multiple Background Tasks

```go
uiRunner.PostTask(func(ctx context.Context) {
    me := taskrunner.GetCurrentTaskRunner(ctx)

    var wg sync.WaitGroup

    // Task 1
    wg.Add(1)
    core.PostTaskAndReplyWithResult(
        bgRunner,
        func(ctx context.Context) (*Data1, error) {
            defer wg.Done()
            return fetchData1(), nil
        },
        func(ctx context.Context, data *Data1, err error) {
            handleData1(data, err)
        },
        me,
    )

    // Task 2
    wg.Add(1)
    core.PostTaskAndReplyWithResult(
        bgRunner,
        func(ctx context.Context) (*Data2, error) {
            defer wg.Done()
            return fetchData2(), nil
        },
        func(ctx context.Context, data *Data2, err error) {
            handleData2(data, err)
        },
        me,
    )

    // Wait for both to complete
    wg.Wait()
})
```

---

## Common Mistakes

### ❌ Mistake 1: Forgetting GetCurrentTaskRunner

```go
// ❌ Bad - 'me' not defined
bgRunner.PostTaskAndReply(
    taskFunc,
    replyFunc,
    me,  // Error: undefined
)
```

```go
// ✅ Good
uiRunner.PostTask(func(ctx context.Context) {
    me := taskrunner.GetCurrentTaskRunner(ctx)
    bgRunner.PostTaskAndReply(taskFunc, replyFunc, me)
})
```

### ❌ Mistake 2: Reply-to-Self Deadlock

```go
// ❌ Bad - potential deadlock
runner.PostTaskAndReply(
    func(ctx context.Context) {
        runner.PostTask(...)  // Nested post
    },
    replyFunc,
    runner,  // Reply to self
)
```

**Fix**: Avoid complex nesting; keep task-and-reply simple.

### ❌ Mistake 3: Closure Variable Capture

```go
// ❌ Bad - captures loop variable
for _, item := range items {
    bgRunner.PostTaskAndReply(
        func(ctx context.Context) {
            process(item)  // Wrong item!
        },
        replyFunc,
        uiRunner,
    )
}
```

```go
// ✅ Good - capture variable
for _, item := range items {
    item := item  // Capture
    bgRunner.PostTaskAndReply(
        func(ctx context.Context) {
            process(item)  // Correct
        },
        replyFunc,
        uiRunner,
    )
}
```

### ❌ Mistake 4: Ignoring Error in Reply

```go
// ❌ Bad - error ignored
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (*Data, error) {
        return fetchData()
    },
    func(ctx context.Context, data *Data, err error) {
        updateUI(data)  // What if err != nil?
    },
    uiRunner,
)
```

```go
// ✅ Good - check error first
core.PostTaskAndReplyWithResult(
    bgRunner,
    func(ctx context.Context) (*Data, error) {
        return fetchData()
    },
    func(ctx context.Context, data *Data, err error) {
        if err != nil {
            showError(err)
            return
        }
        updateUI(data)
    },
    uiRunner,
)
```

---

## Summary

**PostTaskAndReply**: Basic pattern without result

**PostTaskAndReplyWithResult[T]**: Type-safe result passing

**Key Points**:
- Always use `GetCurrentTaskRunner(ctx)` for reply runner
- Check errors in reply callback
- Avoid complex nesting
- Capture loop variables
- Task panic prevents reply execution

**See Also**:
- [../templates/ui-background.go](../templates/ui-background.go) - Complete examples
- [runners.md](runners.md) - Runner selection
- [pitfalls.md](pitfalls.md) - Common mistakes
