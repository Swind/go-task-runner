package eventbus_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/eventbus"
)

func newTestPool(t *testing.T) (taskrunner.ThreadPool, context.Context) {
	t.Helper()
	ctx := context.Background()
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(ctx)
	t.Cleanup(func() { pool.Stop() })
	return pool, ctx
}

func mustWaitIdle(t *testing.T, bus *eventbus.EventBus, ctx context.Context) {
	t.Helper()
	if err := bus.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
}

// drainIdle calls WaitIdle repeatedly until all nested task levels complete.
// EventBus.Publish creates two task levels (dispatch + handler), and re-entrant
// publishing adds more. Multiple WaitIdle rounds ensure every level drains.
func drainIdle(t *testing.T, bus *eventbus.EventBus, ctx context.Context) {
	t.Helper()
	for i := 0; i < 10; i++ {
		if err := bus.WaitIdle(ctx); err != nil {
			break
		}
	}
}

func TestEventBus_BasicPublishSubscribe(t *testing.T) {
	// Arrange
	pool, ctx := newTestPool(t)
	bus := eventbus.NewEventBus(pool)

	received := atomic.Value{}
	eventbus.Subscribe(bus, func(_ context.Context, s string) {
		received.Store(s)
	})
	mustWaitIdle(t, bus, ctx)

	// Act
	bus.Publish(ctx, "hello")
	drainIdle(t, bus, ctx)

	// Assert
	v, ok := received.Load().(string)
	if !ok {
		t.Fatal("expected string value, got nothing")
	}
	if v != "hello" {
		t.Fatalf("expected %q, got %q", "hello", v)
	}
}

func TestEventBus_FIFOOrdering(t *testing.T) {
	// Arrange
	pool, ctx := newTestPool(t)
	bus := eventbus.NewEventBus(pool)

	done := make(chan struct{})
	var received []int
	eventbus.Subscribe(bus, func(_ context.Context, n int) {
		received = append(received, n)
		if n == 1023 {
			close(done)
		}
	})
	mustWaitIdle(t, bus, ctx)

	// Act
	for i := 0; i < 1024; i++ {
		bus.Publish(ctx, i)
	}

	// Assert — synchronize via channel so race detector sees happens-before
	<-done
	if len(received) != 1024 {
		t.Fatalf("expected 1024 events, got %d", len(received))
	}
	for i, v := range received {
		if v != i {
			t.Fatalf("at index %d: expected %d, got %d", i, i, v)
		}
	}
}

func TestEventBus_ReentrantPublish(t *testing.T) {
	// Arrange
	pool, ctx := newTestPool(t)
	bus := eventbus.NewEventBus(pool)

	done := make(chan struct{})
	var order []string
	eventbus.Subscribe(bus, func(_ context.Context, n int) {
		order = append(order, fmt.Sprintf("int-%d", n))
		if n == 1 {
			bus.Publish(ctx, "from-handler")
		}
	})
	eventbus.Subscribe(bus, func(_ context.Context, s string) {
		order = append(order, fmt.Sprintf("str-%s", s))
		if s == "from-handler" {
			close(done)
		}
	})
	mustWaitIdle(t, bus, ctx)

	// Act
	bus.Publish(ctx, 1)
	bus.Publish(ctx, 2)

	// Assert — synchronize via channel
	<-done
	expected := []string{"int-1", "int-2", "str-from-handler"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range order {
		if v != expected[i] {
			t.Fatalf("at index %d: expected %q, got %q", i, expected[i], v)
		}
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	// Arrange
	pool, ctx := newTestPool(t)
	bus := eventbus.NewEventBus(pool)

	var countA, countB atomic.Int32
	idA := eventbus.Subscribe(bus, func(_ context.Context, n int) {
		countA.Add(1)
	})
	eventbus.Subscribe(bus, func(_ context.Context, n int) {
		countB.Add(1)
	})
	mustWaitIdle(t, bus, ctx)

	// Act
	bus.Publish(ctx, 1)
	drainIdle(t, bus, ctx)

	bus.Unsubscribe(idA)
	drainIdle(t, bus, ctx)

	bus.Publish(ctx, 2)
	drainIdle(t, bus, ctx)

	// Assert
	a, b := countA.Load(), countB.Load()
	if a != 1 || b != 2 {
		t.Fatalf("expected A=1 B=2, got A=%d B=%d", a, b)
	}
}

func TestEventBus_MultipleEventTypes(t *testing.T) {
	// Arrange
	pool, ctx := newTestPool(t)
	bus := eventbus.NewEventBus(pool)

	var (
		intCount atomic.Int32
		strCount atomic.Int32
		intSeen  atomic.Int64
		strSeen  atomic.Int64
	)

	eventbus.Subscribe(bus, func(_ context.Context, n int) {
		intCount.Add(1)
		intSeen.Add(int64(n))
	})
	eventbus.Subscribe(bus, func(_ context.Context, s string) {
		strCount.Add(1)
		switch s {
		case "hello":
			strSeen.Add(1)
		case "world":
			strSeen.Add(2)
		}
	})
	mustWaitIdle(t, bus, ctx)

	// Act
	bus.Publish(ctx, 42)
	bus.Publish(ctx, "hello")
	bus.Publish(ctx, 99)
	bus.Publish(ctx, "world")
	drainIdle(t, bus, ctx)

	// Assert
	if c := intCount.Load(); c != 2 {
		t.Fatalf("expected 2 int events, got %d", c)
	}
	if s := intSeen.Load(); s != 141 {
		t.Fatalf("expected int sum 141, got %d", s)
	}
	if c := strCount.Load(); c != 2 {
		t.Fatalf("expected 2 string events, got %d", c)
	}
	if s := strSeen.Load(); s != 3 {
		t.Fatalf("expected str flag 3, got %d", s)
	}
}

func TestEventBus_CloseRejectsPublish(t *testing.T) {
	// Arrange
	pool, ctx := newTestPool(t)
	bus := eventbus.NewEventBus(pool)

	eventbus.Subscribe(bus, func(_ context.Context, s string) {
		t.Error("handler should not be called after Close")
	})
	mustWaitIdle(t, bus, ctx)

	// Act
	bus.Close()
	bus.Publish(ctx, "should not deliver")
}

func TestEventBus_ConcurrentPublish(t *testing.T) {
	// Arrange
	pool, ctx := newTestPool(t)
	bus := eventbus.NewEventBus(pool)

	var count atomic.Int32
	eventbus.Subscribe(bus, func(_ context.Context, n int) {
		count.Add(1)
	})
	mustWaitIdle(t, bus, ctx)

	// Act
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				bus.Publish(ctx, j)
			}
		}()
	}
	wg.Wait()
	drainIdle(t, bus, ctx)

	// Assert
	if c := count.Load(); c != 100 {
		t.Fatalf("expected 100 events, got %d", c)
	}
}
