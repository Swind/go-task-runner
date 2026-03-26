package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/eventbus"
)

type UserCreated struct {
	ID   int
	Name string
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

func drainIdle(bus *eventbus.EventBus, ctx context.Context) {
	for i := 0; i < 10; i++ {
		if err := bus.WaitIdle(ctx); err != nil {
			break
		}
	}
}

func basicOrdering() {
	fmt.Println("--- 1. Basic Ordering ---")

	bus := eventbus.NewEventBus(taskrunner.GlobalThreadPool())
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var counter atomic.Int32
	const total = 1024

	eventbus.Subscribe[UserCreated](bus, func(ctx context.Context, event UserCreated) {
		if counter.Load() != int32(event.ID) {
			fmt.Printf("  ERROR: expected ID %d, got %d\n", counter.Load(), event.ID)
		}
		counter.Add(1)
	})

	bus.WaitIdle(ctx)

	for i := range total {
		bus.Publish(context.Background(), UserCreated{ID: i, Name: fmt.Sprintf("user-%d", i)})
	}

	drainIdle(bus, ctx)

	received := counter.Load()
	if received == total {
		fmt.Printf("  OK: received %d events in sequential order\n", received)
	} else {
		fmt.Printf("  ERROR: expected %d, received %d\n", total, received)
	}
	fmt.Println()
}

func reentrantPublish() {
	fmt.Println("--- 2. Reentrant Publish ---")

	bus := eventbus.NewEventBus(taskrunner.GlobalThreadPool())
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var order []string

	eventbus.Subscribe[UserCreated](bus, func(ctx context.Context, event UserCreated) {
		order = append(order, fmt.Sprintf("user-%d", event.ID))
		bus.Publish(ctx, OrderPlaced{UserID: event.ID, ProductID: event.ID * 100})
	})

	eventbus.Subscribe[OrderPlaced](bus, func(ctx context.Context, event OrderPlaced) {
		order = append(order, fmt.Sprintf("order-%d", event.ProductID))
	})

	bus.WaitIdle(ctx)

	bus.Publish(context.Background(), UserCreated{ID: 1, Name: "Alice"})
	bus.Publish(context.Background(), UserCreated{ID: 2, Name: "Bob"})

	drainIdle(bus, ctx)

	fmt.Printf("  Execution order: %v\n", order)
	fmt.Println("  Key insight: reentrant Publish() is safe because the SequencedTaskRunner")
	fmt.Println("  queues inner tasks without blocking — no lock re-entrancy deadlock.")
	fmt.Println()
}

func dynamicUnsubscribe() {
	fmt.Println("--- 3. Dynamic Unsubscribe ---")

	bus := eventbus.NewEventBus(taskrunner.GlobalThreadPool())
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var countA, countB atomic.Int32

	subA := eventbus.Subscribe[UserCreated](bus, func(ctx context.Context, event UserCreated) {
		countA.Add(1)
	})

	eventbus.Subscribe[UserCreated](bus, func(ctx context.Context, event UserCreated) {
		countB.Add(1)
	})

	bus.WaitIdle(ctx)

	bus.Publish(context.Background(), UserCreated{ID: 1, Name: "Alice"})
	drainIdle(bus, ctx)

	bus.Unsubscribe(subA)
	drainIdle(bus, ctx)

	bus.Publish(context.Background(), UserCreated{ID: 2, Name: "Bob"})
	drainIdle(bus, ctx)

	fmt.Printf("  Handler A fired: %d (expected 1)\n", countA.Load())
	fmt.Printf("  Handler B fired: %d (expected 2)\n", countB.Load())
	fmt.Println()
}
