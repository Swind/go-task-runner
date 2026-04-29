# EventBus

Type-safe, lock-free event publish/subscribe built on SequencedTaskRunner.

## Import

```go
import (
    taskrunner "github.com/Swind/go-task-runner"
    "github.com/Swind/go-task-runner/eventbus"
)
```

---

## Core API

### Create

```go
// Requires an initialized thread pool
taskrunner.InitGlobalThreadPool(4)
defer taskrunner.ShutdownGlobalThreadPool()

bus := eventbus.NewEventBus(taskrunner.GlobalThreadPool())
defer bus.Close()
```

Or attach to an existing pool:

```go
pool := taskrunner.NewGoroutineThreadPool("my-pool", 4)
pool.Start(context.Background())
defer pool.Stop()

bus := eventbus.NewEventBus(pool)
defer bus.Close()
```

### Subscribe

Returns a subscription ID used for unsubscribing.

```go
type UserCreated struct {
    ID   int
    Name string
}

subID := eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
    fmt.Printf("User created: %s\n", event.Name)
})
```

### Publish

Publish is **non-blocking** — it enqueues the event and returns immediately.

```go
bus.Publish(context.Background(), UserCreated{ID: 1, Name: "Alice"})
```

### Unsubscribe

```go
bus.Unsubscribe(subID)
```

### WaitIdle

Block until all pending events and handlers have finished.

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := bus.WaitIdle(ctx); err != nil {
    log.Printf("timeout waiting for idle: %v", err)
}
```

### Close

```go
bus.Close()  // stops accepting new publishes, shuts down internal runner
```

---

## Key Properties

### Type Safety

Each subscription is typed — the compiler enforces that `Publish` and `Subscribe` use matching types:

```go
// ✅ Correct — types match
eventbus.Subscribe(bus, func(ctx context.Context, event OrderPlaced) { ... })
bus.Publish(ctx, OrderPlaced{...})

// OrderPlaced handlers are NOT called for UserCreated events
bus.Publish(ctx, UserCreated{...})
```

### Sequential Ordering

All handlers for a given event type execute in subscription order, one at a time.

```go
counter := 0  // No mutex needed — handlers are sequential
eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
    counter++  // Lock-free!
})
```

### Reentrant Publish

Calling `bus.Publish` from inside a handler is safe — it enqueues without blocking:

```go
eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
    // Publishing from inside a handler is safe
    bus.Publish(ctx, OrderPlaced{UserID: event.ID})
})
```

---

## Multiple Event Types

```go
type UserCreated struct{ ID int; Name string }
type OrderPlaced struct{ UserID int; ProductID int }
type PaymentProcessed struct{ OrderID string; Amount float64 }

eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
    createUserProfile(event.ID)
})

eventbus.Subscribe(bus, func(ctx context.Context, event OrderPlaced) {
    reserveInventory(event.ProductID)
})

eventbus.Subscribe(bus, func(ctx context.Context, event PaymentProcessed) {
    sendReceipt(event.OrderID)
})
```

---

## Dynamic Unsubscribe

```go
subA := eventbus.Subscribe(bus, handlerA)
subB := eventbus.Subscribe(bus, handlerB)

bus.Publish(ctx, SomeEvent{})
bus.WaitIdle(ctx)  // both A and B fired

// Remove A
bus.Unsubscribe(subA)

bus.Publish(ctx, SomeEvent{})
bus.WaitIdle(ctx)  // only B fires
```

---

## Common Mistakes

### ❌ Forgetting bus.Close()

```go
// ❌ Bad - internal SequencedTaskRunner leaks
bus := eventbus.NewEventBus(pool)
// forgot defer bus.Close()
```

```go
// ✅ Good
bus := eventbus.NewEventBus(pool)
defer bus.Close()
```

### ❌ Expecting Synchronous Delivery

```go
// ❌ Bad - event may not have fired yet
bus.Publish(ctx, UserCreated{ID: 1})
// NOT guaranteed to have executed here
fmt.Println("User handled")
```

```go
// ✅ Good - wait for delivery
bus.Publish(ctx, UserCreated{ID: 1})
bus.WaitIdle(ctx)
fmt.Println("User handled")
```

### ❌ Mutexes Inside Handlers

```go
// ❌ Bad - mutex is unnecessary; handlers are sequential
var mu sync.Mutex
var count int
eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
    mu.Lock()
    count++
    mu.Unlock()
})
```

```go
// ✅ Good - lock-free
var count int
eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
    count++  // Sequential execution = no race
})
```

---

## See Also

- [`templates/eventbus.go`](../templates/eventbus.go) — complete working example
- [`lock-free-patterns.md`](lock-free-patterns.md) — understanding sequential execution
- [`runners.md`](runners.md) — SequencedTaskRunner internals
