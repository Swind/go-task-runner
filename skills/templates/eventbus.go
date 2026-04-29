// Package main demonstrates the EventBus — type-safe, lock-free publish/subscribe
// built on SequencedTaskRunner.
package main

import (
	"context"
	"fmt"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/eventbus"
)

// Define event types as plain structs.
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

	bus := eventbus.NewEventBus(taskrunner.GlobalThreadPool())
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- Subscribe ---

	// Counter is lock-free: handlers execute sequentially.
	var userCount int
	eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
		userCount++
		fmt.Printf("UserCreated: id=%d name=%s (total=%d)\n", event.ID, event.Name, userCount)
	})

	// Second subscriber for UserCreated.
	eventbus.Subscribe(bus, func(ctx context.Context, event UserCreated) {
		// Reentrant publish is safe — enqueues without blocking.
		bus.Publish(ctx, OrderPlaced{UserID: event.ID, ProductID: event.ID * 100})
	})

	subOrder := eventbus.Subscribe(bus, func(ctx context.Context, event OrderPlaced) {
		fmt.Printf("OrderPlaced: userID=%d productID=%d\n", event.UserID, event.ProductID)
	})

	// Wait until subscriptions are registered before publishing.
	bus.WaitIdle(ctx)

	// --- Publish ---

	bus.Publish(context.Background(), UserCreated{ID: 1, Name: "Alice"})
	bus.Publish(context.Background(), UserCreated{ID: 2, Name: "Bob"})

	// Wait for all handlers to complete.
	bus.WaitIdle(ctx)
	fmt.Printf("Users processed: %d\n", userCount)

	// --- Dynamic Unsubscribe ---

	bus.Unsubscribe(subOrder)
	bus.WaitIdle(ctx)

	bus.Publish(context.Background(), UserCreated{ID: 3, Name: "Carol"})
	bus.WaitIdle(ctx)
	// OrderPlaced handler no longer fires for Carol's order.

	fmt.Println("Done")
}
