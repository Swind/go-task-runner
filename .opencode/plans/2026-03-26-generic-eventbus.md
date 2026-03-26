# Generic EventBus Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a production-ready, type-safe EventBus package using Go generics, backed by SequencedTaskRunner for lock-free FIFO ordering with re-entrant publish safety.

**Architecture:** Single `SequencedTaskRunner` dispatches all events. `reflect.Type` keys index `map[reflect.Type][]subscriber`. Publish dispatches a task that iterates handlers and posts each as a runner task — guaranteeing FIFO order and re-entrant safety with zero mutexes.

**Tech Stack:** Go 1.24 generics, `reflect.Type`, `github.com/Swind/go-task-runner` core types.

---

## File Structure

```
eventbus/
  eventbus.go            // EventBus struct, Publish, Subscribe[T], Unsubscribe, Close, WaitIdle
  eventbus_test.go       // Functional tests: ordering, re-entrant, unsubscribe, concurrent
  eventbus_gc_test.go    // GC release tests: handler closures, pending events, bus itself, re-entrant
examples/event_bus/
  main.go                // Rewrite: calls eventbus package, 3 demo scenarios
```

---

## Task 1: EventBus Core Implementation

**Files:**
- Create: `eventbus/eventbus.go`

**Step 1: Create the eventbus package with core types**

```go
package eventbus

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/Swind/go-task-runner"
)

type subscriber struct {
	id      string
	handler any
}

type subLocator struct {
	eventType reflect.Type
	index     int
}

type EventBus struct {
	runner *taskrunner.SequencedTaskRunner
	subs   map[reflect.Type][]subscriber
	byID   map[string]subLocator
	nextID atomic.Int64
	closed atomic.Bool
}
```

**Step 2: Implement NewEventBus constructor**

```go
func NewEventBus(pool taskrunner.ThreadPool) *EventBus {
	return &EventBus{
		runner: taskrunner.NewSequencedTaskRunner(pool),
		subs:   make(map[reflect.Type][]subscriber),
		byID:   make(map[string]subLocator),
	}
}

func NewEventBusFromRunner(runner *taskrunner.SequencedTaskRunner) *EventBus {
	return &EventBus{
		runner: runner,
		subs:   make(map[reflect.Type][]subscriber),
		byID:   make(map[string]subLocator),
	}
}
```

**Step 3: Implement Subscribe[T]**

```go
func Subscribe[T any](b *EventBus, handler func(ctx context.Context, T)) string {
	id := fmt.Sprintf("sub-%d", b.nextID.Add(1))
	typ := reflect.TypeFor[T]()

	b.runner.PostTask(func(ctx context.Context) {
		b.subs[typ] = append(b.subs[typ], subscriber{id: id, handler: handler})
		b.byID[id] = subLocator{eventType: typ, index: len(b.subs[typ]) - 1}
	})

	return id
}
```

**Step 4: Implement Publish**

```go
func (b *EventBus) Publish(ctx context.Context, event any) {
	if b.closed.Load() {
		return
	}

	typ := reflect.TypeOf(event)

	b.runner.PostTask(func(rctx context.Context) {
		subs := b.subs[typ]
		for _, sub := range subs {
			handler := sub.handler
			b.runner.PostTask(func(rctx context.Context) {
				fn, ok := handler.(func(context.Context, any))
				if !ok {
					return
				}
				fn(rctx, event)
			})
		}
	})
}
```

Note: The dispatch task captures `event` and `typ` by value. Handler tasks capture `handler` by value. The dispatch task iterates `subs` snapshot — no concurrency issue since only the runner accesses `b.subs`.

**Step 5: Implement Unsubscribe (O(1))**

```go
func (b *EventBus) Unsubscribe(id string) {
	b.runner.PostTask(func(ctx context.Context) {
		loc, ok := b.byID[id]
		if !ok {
			return
		}
		subs := b.subs[loc.eventType]
		subs[loc.index] = subs[len(subs)-1]
		subs[len(subs)-1] = subscriber{}
		subs = subs[:len(subs)-1]

		if len(subs) == 0 {
			delete(b.subs, loc.eventType)
		} else {
			b.subs[loc.eventType] = subs
			b.byID[subs[loc.index].id] = subLocator{eventType: loc.eventType, index: loc.index}
		}
		delete(b.byID, id)
	})
}
```

**Step 6: Implement WaitIdle and Close**

```go
func (b *EventBus) WaitIdle(ctx context.Context) error {
	return b.runner.WaitIdle(ctx)
}

func (b *EventBus) Close() {
	b.closed.Store(true)
	b.runner.Shutdown()
}
```

**Step 7: Build check**

Run: `go build ./eventbus/...`
Expected: no errors

**Step 8: Commit**

```bash
git add eventbus/eventbus.go
git commit -m "feat(eventbus): add generic type-based EventBus with SequencedTaskRunner"
```

---

## Task 2: Functional Tests

**Files:**
- Create: `eventbus/eventbus_test.go`

**Step 1: Write helper — newTestPool**

```go
package eventbus_test

import (
	"context"
	"testing"
	"time"

	"github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/eventbus"
)

func newTestPool(t *testing.T) *taskrunner.GoroutineThreadPool {
	t.Helper()
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	return pool
}

func stopTestPool(t *testing.T, pool *taskrunner.GoroutineThreadPool) {
	t.Helper()
	pool.Stop()
}
```

**Step 2: Write TestEventBus_BasicPublishSubscribe**

BDD: Given an EventBus with a subscriber for `string`, when we publish "hello", then the handler receives "hello".

```go
func TestEventBus_BasicPublishSubscribe(t *testing.T) {
	// Arrange
	pool := newTestPool(t)
	defer stopTestPool(t, pool)

	bus := eventbus.NewEventBus(pool)
	defer bus.Close()

	received := make(chan string, 1)
	eventbus.Subscribe(bus, func(ctx context.Context, msg string) {
		received <- msg
	})

	// Act
	bus.Publish(context.Background(), "hello")

	// Assert
	select {
	case got := <-received:
		if got != "hello" {
			t.Errorf("received: got = %q, want = %q", got, "hello")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}
```

**Step 3: Write TestEventBus_FIFOOrdering**

BDD: Given 1024 events published sequentially, when all handlers execute, then events are received in exact publish order.

```go
func TestEventBus_FIFOOrdering(t *testing.T) {
	// Arrange
	pool := newTestPool(t)
	defer stopTestPool(t, pool)

	bus := eventbus.NewEventBus(pool)
	defer bus.Close()

	var mu sync.Mutex
	var received []int
	eventbus.Subscribe(bus, func(ctx context.Context, n int) {
		mu.Lock()
		received = append(received, n)
		mu.Unlock()
	})

	// Act
	for i := range 1024 {
		bus.Publish(context.Background(), i)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}

	// Assert
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1024 {
		t.Fatalf("received count: got = %d, want = 1024", len(received))
	}
	for i, v := range received {
		if v != i {
			t.Errorf("order[%d]: got = %d, want = %d", i, v, i)
			break
		}
	}
}
```

**Step 4: Write TestEventBus_ReentrantPublish**

BDD: Given a handler for `int` that publishes a `string` event, when event A is processed, then the re-entrant event B is queued and processed after A completes.

```go
func TestEventBus_ReentrantPublish(t *testing.T) {
	// Arrange
	pool := newTestPool(t)
	defer stopTestPool(t, pool)

	bus := eventbus.NewEventBus(pool)
	defer bus.Close()

	var order []string
	eventbus.Subscribe(bus, func(ctx context.Context, n int) {
		order = append(order, fmt.Sprintf("int-%d", n))
		if n == 1 {
			bus.Publish(ctx, "from-handler")
		}
	})
	eventbus.Subscribe(bus, func(ctx context.Context, s string) {
		order = append(order, fmt.Sprintf("str-%s", s))
	})

	// Act
	bus.Publish(context.Background(), 1)
	bus.Publish(context.Background(), 2)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := bus.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}

	// Assert
	expected := []string{"int-1", "int-2", "str-from-handler"}
	if !reflect.DeepEqual(order, expected) {
		t.Errorf("order: got = %v, want = %v", order, expected)
	}
}
```

**Step 5: Write TestEventBus_Unsubscribe**

BDD: Given 2 subscribers, when one unsubscribes, then only the remaining subscriber receives events.

```go
func TestEventBus_Unsubscribe(t *testing.T) {
	// Arrange
	pool := newTestPool(t)
	defer stopTestPool(t, pool)

	bus := eventbus.NewEventBus(pool)
	defer bus.Close()

	var countA, countB atomic.Int32
	idA := eventbus.Subscribe(bus, func(ctx context.Context, _ int) {
		countA.Add(1)
	})
	eventbus.Subscribe(bus, func(ctx context.Context, _ int) {
		countB.Add(1)
	})

	bus.Publish(context.Background(), 1)
	bus.WaitIdle(context.Background())

	// Act
	bus.Unsubscribe(idA)
	bus.WaitIdle(context.Background())
	bus.Publish(context.Background(), 2)
	bus.WaitIdle(context.Background())

	// Assert
	if got := countA.Load(); got != 1 {
		t.Errorf("handlerA count: got = %d, want = 1", got)
	}
	if got := countB.Load(); got != 2 {
		t.Errorf("handlerB count: got = %d, want = 2", got)
	}
}
```

**Step 6: Write TestEventBus_MultipleEventTypes**

BDD: Given subscribers for `int` and `string`, when each event type is published, then only the matching subscribers fire.

```go
func TestEventBus_MultipleEventTypes(t *testing.T) {
	// Arrange
	pool := newTestPool(t)
	defer stopTestPool(t, pool)

	bus := eventbus.NewEventBus(pool)
	defer bus.Close()

	var ints []int
	var strs []string
	eventbus.Subscribe(bus, func(ctx context.Context, n int) { ints = append(ints, n) })
	eventbus.Subscribe(bus, func(ctx context.Context, s string) { strs = append(strs, s) })

	// Act
	bus.Publish(context.Background(), 42)
	bus.Publish(context.Background(), "hello")
	bus.WaitIdle(context.Background())

	// Assert
	if len(ints) != 1 || ints[0] != 42 {
		t.Errorf("ints: got = %v, want = [42]", ints)
	}
	if len(strs) != 1 || strs[0] != "hello" {
		t.Errorf("strs: got = %v, want = [hello]", strs)
	}
}
```

**Step 7: Write TestEventBus_CloseRejectsPublish**

BDD: Given a closed EventBus, when Publish is called, then no handlers execute.

```go
func TestEventBus_CloseRejectsPublish(t *testing.T) {
	// Arrange
	pool := newTestPool(t)
	defer stopTestPool(t, pool)

	bus := eventbus.NewEventBus(pool)
	eventbus.Subscribe(bus, func(ctx context.Context, _ int) {
		t.Error("handler should not be called after Close")
	})

	// Act
	bus.Close()
	bus.Publish(context.Background(), 1)
	time.Sleep(50 * time.Millisecond)

	// Assert - no panic, no handler call
}
```

**Step 8: Write TestEventBus_ConcurrentPublish (race detector)**

```go
func TestEventBus_ConcurrentPublish(t *testing.T) {
	// Arrange
	pool := newTestPool(t)
	defer stopTestPool(t, pool)

	bus := eventbus.NewEventBus(pool)
	defer bus.Close()

	var count atomic.Int32
	eventbus.Subscribe(bus, func(ctx context.Context, _ int) {
		count.Add(1)
	})

	// Act - publish from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(context.Background(), 1)
		}()
	}
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bus.WaitIdle(ctx)

	// Assert
	if got := count.Load(); got != 100 {
		t.Errorf("count: got = %d, want = 100", got)
	}
}
```

**Step 9: Run all functional tests**

Run: `go test -v -race -run 'TestEventBus_' ./eventbus/`
Expected: all PASS

**Step 10: Commit**

```bash
git add eventbus/eventbus_test.go
git commit -m "test(eventbus): add functional tests for ordering, re-entrant, unsubscribe"
```

---

## Task 3: GC Release Tests

**Files:**
- Create: `eventbus/eventbus_gc_test.go`

Pattern: Follow the project's existing `_gc_test.go` convention — `runtime.SetFinalizer` on objects with `[]byte` allocation, `sync.WaitGroup` to count finalizer calls, `runtime.GC()` x5 loop, timeout assertion.

**Step 1: Write TestEventBus_GC_HandlerClosureCapturedObjects**

BDD: Given 50 subscribers with objects captured in handler closures, when all events are processed and references dropped, then all captured objects are GC'd.

```go
func TestEventBus_GC_HandlerClosureCapturedObjects(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	bus := eventbus.NewEventBus(pool)

	const numSubs = 50
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numSubs)

	type heavyObj struct {
		ID   string
		Data []byte
	}

	func() {
		for i := range numSubs {
			obj := &heavyObj{
				ID:   fmt.Sprintf("handler-%d", i),
				Data: make([]byte, 10*1024),
			}
			runtime.SetFinalizer(obj, func(o *heavyObj) {
				finalizerCount.Add(1)
				wg.Done()
			})
			eventbus.Subscribe(bus, func(ctx context.Context, _ int) {
				_ = obj.ID
			})
		}

		for i := range numSubs {
			bus.Publish(context.Background(), i)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := bus.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle: %v", err)
		}

		bus.Close()
		bus = nil
	}()

	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if got := finalizerCount.Load(); got != numSubs {
			t.Errorf("handler objects GC'd: got = %d, want = %d", got, numSubs)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("timeout: only %d/%d handler objects GC'd", finalizerCount.Load(), numSubs)
	}
}
```

**Step 2: Write TestEventBus_GC_ShutdownClearsPendingEvents**

BDD: Given events queued with captured objects, when the bus is closed while events are pending, then all captured objects are GC'd.

```go
func TestEventBus_GC_ShutdownClearsPendingEvents(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 1)
	pool.Start(context.Background())
	defer pool.Stop()

	bus := eventbus.NewEventBus(pool)

	const numPending = 50
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numPending)

	type pendingObj struct {
		ID   string
		Data []byte
	}

	func() {
		for i := range numPending {
			obj := &pendingObj{
				ID:   fmt.Sprintf("pending-%d", i),
				Data: make([]byte, 1024),
			}
			runtime.SetFinalizer(obj, func(o *pendingObj) {
				finalizerCount.Add(1)
				wg.Done()
			})
			eventbus.Subscribe(bus, func(ctx context.Context, _ int) {
				_ = obj.ID
			})
		}

		for i := range numPending {
			bus.Publish(context.Background(), i)
		}

		time.Sleep(20 * time.Millisecond)
		bus.Close()
		bus = nil
	}()

	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if got := finalizerCount.Load(); got != numPending {
			t.Errorf("pending event objects GC'd: got = %d, want = %d", got, numPending)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("timeout: only %d/%d pending event objects GC'd", finalizerCount.Load(), numPending)
	}
}
```

**Step 3: Write TestEventBus_GC_BusItself**

BDD: Given an EventBus that has processed events and been closed, when all references are dropped, then the EventBus is GC'd.

```go
func TestEventBus_GC_BusItself(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	var finalizerCalled atomic.Bool

	func() {
		bus := eventbus.NewEventBus(pool)

		runtime.SetFinalizer(bus, func(b *eventbus.EventBus) {
			finalizerCalled.Store(true)
		})

		eventbus.Subscribe(bus, func(ctx context.Context, _ int) {})
		bus.Publish(context.Background(), 1)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		bus.WaitIdle(ctx)

		bus.Close()
		bus = nil
	}()

	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	if !finalizerCalled.Load() {
		t.Error("EventBus GC'd: got = false, want = true")
	}
}
```

**Step 4: Write TestEventBus_GC_ReentrantPublishClosures**

BDD: Given a handler that publishes new events (capturing objects), when all processing completes and references are dropped, then all re-entrant closure objects are GC'd.

```go
func TestEventBus_GC_ReentrantPublishClosures(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	bus := eventbus.NewEventBus(pool)

	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(20)

	type reentrantObj struct {
		ID   string
		Data []byte
	}

	func() {
		// Subscribe for int events — handler publishes string events
		eventbus.Subscribe(bus, func(ctx context.Context, n int) {
			if n < 10 {
				obj := &reentrantObj{
					ID:   fmt.Sprintf("reentrant-%d", n),
					Data: make([]byte, 1024),
				}
				runtime.SetFinalizer(obj, func(o *reentrantObj) {
					finalizerCount.Add(1)
					wg.Done()
				})
				captured := obj
				bus.Publish(ctx, fmt.Sprintf("trigger-%d", n))
				_ = captured.ID
			}
		})

		// Subscribe for string events — handler captures object
		eventbus.Subscribe(bus, func(ctx context.Context, s string) {
			obj := &reentrantObj{
				ID:   fmt.Sprintf("str-handler-%s", s),
				Data: make([]byte, 1024),
			}
			runtime.SetFinalizer(obj, func(o *reentrantObj) {
				finalizerCount.Add(1)
				wg.Done()
			})
			_ = obj.ID
		})

		// Act - publish 10 int events that trigger 10 re-entrant string events
		for i := range 10 {
			bus.Publish(context.Background(), i)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := bus.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle: %v", err)
		}

		bus.Close()
		bus = nil
	}()

	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("re-entrant closure objects GC'd: %d/20", finalizerCount.Load())
	case <-time.After(3 * time.Second):
		t.Errorf("timeout: only %d/20 re-entrant closure objects GC'd", finalizerCount.Load())
	}
}
```

**Step 5: Write TestEventBus_GC_UnsubscribeReleasesHandler**

BDD: Given a subscriber with a captured object, when the subscriber is unsubscribed and bus is closed, then the captured object is GC'd.

```go
func TestEventBus_GC_UnsubscribeReleasesHandler(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	bus := eventbus.NewEventBus(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	type handlerObj struct {
		ID   string
		Data []byte
	}

	func() {
		obj := &handlerObj{
			ID:   "to-unsubscribe",
			Data: make([]byte, 50*1024),
		}
		runtime.SetFinalizer(obj, func(o *handlerObj) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		id := eventbus.Subscribe(bus, func(ctx context.Context, _ int) {
			_ = obj.ID
		})

		// Act - unsubscribe then close
		bus.WaitIdle(context.Background())
		bus.Unsubscribe(id)
		bus.WaitIdle(context.Background())
		bus.Close()
		bus = nil
	}()

	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !finalizerCalled.Load() {
			t.Error("unsubscribed handler object GC'd: got = false, want = true")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: unsubscribed handler object was not GC'd")
	}
}
```

**Step 6: Run all GC tests**

Run: `go test -v -run 'TestEventBus_GC' ./eventbus/`
Expected: all PASS

**Step 7: Commit**

```bash
git add eventbus/eventbus_gc_test.go
git commit -m "test(eventbus): add GC release tests for handler closures, pending events, bus, re-entrant"
```

---

## Task 4: Rewrite Example

**Files:**
- Modify: `examples/event_bus/main.go`

**Step 1: Rewrite main.go to use eventbus package**

3 demo scenarios:
1. Basic FIFO ordering (1024 events)
2. Re-entrant publish (handler publishes new event type)
3. Unsubscribe (dynamic removal)

Use `WaitIdle` instead of `time.Sleep` / `<-done` for synchronization. No panics for assertions.

```go
package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/eventbus"
)

type UserCreated struct {
	ID    int
	Name  string
}

type OrderPlaced struct {
	UserID    int
	ProductID int
}

func main() {
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== EventBus Examples ===")
	fmt.Println()

	basicOrdering()
	reentrantPublish()
	dynamicUnsubscribe()

	fmt.Println("=== All Examples Finished ===")
}

func basicOrdering() {
	fmt.Println("--- Example 1: FIFO Ordering ---")

	bus := eventbus.NewEventBus(taskrunner.GlobalThreadPool())
	defer bus.Close()

	var count atomic.Int32
	eventbus.Subscribe(bus, func(ctx context.Context, e UserCreated) {
		if atomic.AddInt32(&count, 1) != e.ID {
			fmt.Printf("  ERROR: order mismatch, expected %d got event %d\n", count, e.ID)
		}
	})

	for i := 1; i <= 1024; i++ {
		bus.Publish(context.Background(), UserCreated{ID: i, Name: fmt.Sprintf("User%d", i)})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.WaitIdle(ctx); err != nil {
		fmt.Printf("  ERROR: WaitIdle: %v\n", err)
		return
	}

	fmt.Printf("  Received %d/1024 events in order\n", count.Load())
	fmt.Println()
}

func reentrantPublish() {
	fmt.Println("--- Example 2: Re-entrant Publish ---")

	bus := eventbus.NewEventBus(taskrunner.GlobalThreadPool())
	defer bus.Close()

	var log []string

	eventbus.Subscribe(bus, func(ctx context.Context, e UserCreated) {
		log = append(log, fmt.Sprintf("user-%d", e.ID))
		bus.Publish(ctx, OrderPlaced{UserID: e.ID, ProductID: e.ID * 100})
	})
	eventbus.Subscribe(bus, func(ctx context.Context, e OrderPlaced) {
		log = append(log, fmt.Sprintf("order-%d", e.ProductID))
	})

	bus.Publish(context.Background(), UserCreated{ID: 1})
	bus.Publish(context.Background(), UserCreated{ID: 2})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	bus.WaitIdle(ctx)

	fmt.Printf("  Execution order: %v\n", log)
	fmt.Println("  Key: handler can publish without locks, events queue in order")
	fmt.Println()
}

func dynamicUnsubscribe() {
	fmt.Println("--- Example 3: Dynamic Unsubscribe ---")

	bus := eventbus.NewEventBus(taskrunner.GlobalThreadPool())
	defer bus.Close()

	var countA, countB atomic.Int32

	idA := eventbus.Subscribe(bus, func(ctx context.Context, e UserCreated) {
		countA.Add(1)
	})
	eventbus.Subscribe(bus, func(ctx context.Context, e UserCreated) {
		countB.Add(1)
	})

	bus.Publish(context.Background(), UserCreated{ID: 1})
	bus.WaitIdle(context.Background())

	bus.Unsubscribe(idA)
	bus.WaitIdle(context.Background())

	bus.Publish(context.Background(), UserCreated{ID: 2})
	bus.WaitIdle(context.Background())

	fmt.Printf("  Handler A (unsubscribed after event 1): %d events\n", countA.Load())
	fmt.Printf("  Handler B (active throughout): %d events\n", countB.Load())
	fmt.Println()
}
```

**Step 2: Run the example**

Run: `go run examples/event_bus/main.go`
Expected: all 3 examples complete with correct output, no panics

**Step 3: Commit**

```bash
git add examples/event_bus/main.go
git commit -m "feat(examples): rewrite event_bus example to use eventbus package"
```

---

## Task 5: Final Verification

**Step 1: Run all tests with race detector**

Run: `go test -v -race ./eventbus/`
Expected: all PASS

**Step 2: Run full project tests**

Run: `go test ./...`
Expected: all PASS (no regressions)

**Step 3: Run CI-mode tests**

Run: `go test -tags=ci -v -coverprofile=coverage.out ./...`
Expected: all PASS

**Step 4: Lint**

Run: `golangci-lint run --timeout=5m eventbus/...`
Expected: no errors

**Step 5: Run the example**

Run: `go run examples/event_bus/main.go`
Expected: clean output, no panics
