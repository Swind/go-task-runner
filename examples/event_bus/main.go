package main

import (
	"context"
	"fmt"
	"sync/atomic"

	taskrunner "github.com/Swind/go-task-runner"
)

type EventType string

type Event struct {
	Type   EventType
	Source string // who published this event
	Data   string
}

// EventHandler is a function that handles an event
type EventHandler func(ctx context.Context, event Event)

// EventBus defines the interface for publishing and subscribing to events
type EventBus interface {
	// Publish publishes an event to all subscribers asynchronously
	// Returns immediately after queuing the event
	Publish(event Event)
	// Subscribe subscribes to events of a specific type
	// Returns immediately after queuing the subscription
	Subscribe(eventType EventType, handler EventHandler) string
	// Unsubscribe removes a subscription
	// Returns immediately after queuing the unsubscription
	Unsubscribe(subscriptionID string)
}

// TaskRunnerEventBus implements EventBus using go-task-runner
// All operations (Publish, Subscribe, Unsubscribe) are executed through the task runner
// to ensure thread-safety without additional mutexes
type TaskRunnerEventBus struct {
	runner        *taskrunner.SequencedTaskRunner
	subscriptions map[EventType][]subscription
	nextID        int64
}

type subscription struct {
	id      string
	typ     EventType
	handler EventHandler
}

// NewTaskRunnerEventBus creates a new event bus
func NewTaskRunnerEventBus(runner *taskrunner.SequencedTaskRunner) EventBus {
	return &TaskRunnerEventBus{
		runner:        runner,
		subscriptions: make(map[EventType][]subscription),
		nextID:        0,
	}
}

// Publish publishes an event to all subscribers asynchronously
// The publish operation itself is queued, and then each handler is executed
func (b *TaskRunnerEventBus) Publish(event Event) {
	// Queue the publish operation to ensure thread-safety
	b.runner.PostTask(func(ctx context.Context) {
		subs, ok := b.subscriptions[event.Type]
		if !ok || len(subs) == 0 {
			return
		}

		// Execute each handler
		// Handlers run in the same task runner, maintaining order
		for _, sub := range subs {
			b.runner.PostTask(func(ctx context.Context) {
				sub.handler(ctx, event)
			})
		}
	})
}

// Subscribe subscribes to events of a specific type
// Returns immediately with a subscription ID
func (b *TaskRunnerEventBus) Subscribe(eventType EventType, handler EventHandler) string {
	id := atomic.AddInt64(&b.nextID, 1)
	subID := generateID(int(id))

	// Capture id by value in the closure
	subIDStr := subID
	subHandler := handler

	// Queue the subscription operation
	b.runner.PostTask(func(ctx context.Context) {
		b.subscriptions[eventType] = append(b.subscriptions[eventType], subscription{
			id:      subIDStr,
			typ:     eventType,
			handler: subHandler,
		})
	})

	return subID
}

// Unsubscribe removes a subscription
// The unsubscription is queued and processed asynchronously
func (b *TaskRunnerEventBus) Unsubscribe(subscriptionID string) {
	b.runner.PostTask(func(ctx context.Context) {
		for typ, subs := range b.subscriptions {
			newSubs := make([]subscription, 0, len(subs))
			for _, sub := range subs {
				if sub.id != subscriptionID {
					newSubs = append(newSubs, sub)
				}
			}
			if len(newSubs) == 0 {
				delete(b.subscriptions, typ)
			} else {
				b.subscriptions[typ] = newSubs
			}
		}
	})
}

// generateID generates a unique subscription ID
func generateID(n int) string {
	// Simple ID generation - can be enhanced if needed
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[(n+i)%len(charset)]
	}
	return string(b)
}

func main() {
	// 1. Initialize the Global Thread Pool
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== Event Bus Example ===")
	fmt.Println("Demonstrating lock-free state management with SequencedTaskRunner")
	fmt.Println()

	// 2. Create a SequencedTaskRunner
	// All handlers execute sequentially on this single virtual thread
	// No locks needed - the runner guarantees exclusive access
	runner := taskrunner.CreateTaskRunner(taskrunner.TaskTraits{
		Priority: taskrunner.TaskPriorityUserVisible,
	})

	done := make(chan struct{})
	// Shared state accessed only by the sequenced runner - no locks needed!
	receivedEvents := make([]string, 0)
	receivedEventIndex := 0

	eventBus := NewTaskRunnerEventBus(runner)

	// Subscribe to Message events
	// All handlers run sequentially on the same runner
	eventBus.Subscribe("Message", func(ctx context.Context, event Event) {
		receivedEvents = append(receivedEvents, event.Data)
		if event.Data != fmt.Sprintf("Event %d", receivedEventIndex) {
			fmt.Printf("✗ Event order mismatch: expected Event %d, got %s\n", receivedEventIndex, event.Data)
			panic("Event order mismatch")
		}
		receivedEventIndex++
	})

	// Subscribe to Done event to signal completion
	eventBus.Subscribe("Done", func(ctx context.Context, event Event) {
		close(done)
	})

	// 3. Publish 1024 Message events
	for i := range 1024 {
		eventBus.Publish(Event{
			Type:   "Message",
			Source: "main",
			Data:   fmt.Sprintf("Event %d", i),
		})
	}

	// 4. Publish Done event
	eventBus.Publish(Event{
		Type:   "Done",
		Source: "main",
		Data:   "All events published",
	})

	// 5. Wait for completion
	<-done

	// 6. Verify
	fmt.Printf("Published: 1024 events\n")
	fmt.Printf("Received:  %d events\n", len(receivedEvents))

	if len(receivedEvents) == 1024 {
		fmt.Println("\n✓ All events received successfully!")
		fmt.Println("\nKey insight: No mutex needed!")
		fmt.Println("SequencedTaskRunner guarantees that only one task runs at a time,")
		fmt.Println("providing exclusive access to shared state without locks.")
	} else {
		fmt.Printf("\n✗ Expected 1024 events, got %d\n", len(receivedEvents))
	}
}
