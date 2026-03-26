package eventbus_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/eventbus"
)

type gcObject struct {
	data []byte
}

func forceGC() {
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForFinalizers(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) bool {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// TestEventBus_GC_HandlerClosureCapturedObjects verifies handler-captured objects are GC'd
// Given: 50 handlers for int, each capturing a heavyObj with finalizer
// When: 50 events are published, bus waits idle, and is closed
// Then: all 50 captured objects are garbage collected
func TestEventBus_GC_HandlerClosureCapturedObjects(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	bus := eventbus.NewEventBus(pool)

	const numHandlers = 50
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numHandlers)

	// Act
	func() {
		for i := 0; i < numHandlers; i++ {
			obj := &gcObject{data: make([]byte, 10*1024)}
			runtime.SetFinalizer(obj, func(o *gcObject) {
				finalizerCount.Add(1)
				wg.Done()
			})
			eventbus.Subscribe(bus, func(_ context.Context, _ int) {
				_ = obj.data[0]
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := bus.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle after subscribe: %v", err)
		}

		for i := 0; i < numHandlers; i++ {
			bus.Publish(ctx, i)
		}

		for i := 0; i < 10; i++ {
			if err := bus.WaitIdle(ctx); err != nil {
				break
			}
		}

		bus.Close()
	}()

	bus = nil
	forceGC()

	// Assert
	if !waitForFinalizers(t, &wg, 3*time.Second) {
		collected := finalizerCount.Load()
		t.Errorf("timeout: only %d/%d handler objects were GC'd", collected, numHandlers)
		return
	}
	collected := finalizerCount.Load()
	if collected != numHandlers {
		t.Errorf("handler objects GC'd: got = %d, want = %d", collected, numHandlers)
	}
}

// TestEventBus_GC_ShutdownClearsPendingEvents verifies pending event objects are GC'd on Close
// Given: 50 handlers for int, each capturing a pendingObj
// When: 50 events are published and bus is closed without WaitIdle
// Then: all 50 captured objects are garbage collected
func TestEventBus_GC_ShutdownClearsPendingEvents(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 1)
	pool.Start(context.Background())
	defer pool.Stop()

	bus := eventbus.NewEventBus(pool)

	const numHandlers = 50
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numHandlers)

	// Act
	func() {
		for i := 0; i < numHandlers; i++ {
			obj := &gcObject{data: make([]byte, 1024)}
			runtime.SetFinalizer(obj, func(o *gcObject) {
				finalizerCount.Add(1)
				wg.Done()
			})
			eventbus.Subscribe(bus, func(_ context.Context, _ int) {
				_ = obj.data[0]
			})
		}

		ctx := context.Background()
		if err := bus.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle after subscribe: %v", err)
		}

		for i := 0; i < numHandlers; i++ {
			bus.Publish(ctx, i)
		}

		time.Sleep(20 * time.Millisecond)
		bus.Close()
	}()

	bus = nil
	forceGC()

	// Assert
	if !waitForFinalizers(t, &wg, 3*time.Second) {
		collected := finalizerCount.Load()
		t.Errorf("timeout: only %d/%d pending objects were GC'd", collected, numHandlers)
		return
	}
	collected := finalizerCount.Load()
	if collected != numHandlers {
		t.Errorf("pending objects GC'd: got = %d, want = %d", collected, numHandlers)
	}
}

// TestEventBus_GC_BusItself verifies the EventBus is GC'd after Close
// Given: an EventBus with a handler that has processed events
// When: bus is closed and all references are dropped
// Then: the EventBus is garbage collected
func TestEventBus_GC_BusItself(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Act
	func() {
		bus := eventbus.NewEventBus(pool)
		runtime.SetFinalizer(bus, func(b *eventbus.EventBus) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		eventbus.Subscribe(bus, func(_ context.Context, _ string) {})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := bus.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle after subscribe: %v", err)
		}

		bus.Publish(ctx, "event")
		for i := 0; i < 10; i++ {
			if err := bus.WaitIdle(ctx); err != nil {
				break
			}
		}

		bus.Close()
	}()

	forceGC()

	// Assert
	if !waitForFinalizers(t, &wg, 2*time.Second) {
		t.Error("timeout: EventBus was not GC'd")
		return
	}
	if !finalizerCalled.Load() {
		t.Error("EventBus GC'd: got = false, want = true")
	}
}

// TestEventBus_GC_ReentrantPublishClosures verifies objects captured in re-entrant publish chains are GC'd
// Given: int handler publishes string events (capturing reentrantObj), string handler captures reentrantObj
// When: 10 int events trigger 10 re-entrant string events (20 total objects)
// Then: all 20 captured objects are garbage collected
func TestEventBus_GC_ReentrantPublishClosures(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	bus := eventbus.NewEventBus(pool)

	const numEvents = 10
	const totalObjects = numEvents * 2
	var finalizerCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(totalObjects)

	// Act
	func() {
		for i := 0; i < numEvents; i++ {
			intObj := &gcObject{data: make([]byte, 10*1024)}
			runtime.SetFinalizer(intObj, func(o *gcObject) {
				finalizerCount.Add(1)
				wg.Done()
			})

			strObj := &gcObject{data: make([]byte, 10*1024)}
			runtime.SetFinalizer(strObj, func(o *gcObject) {
				finalizerCount.Add(1)
				wg.Done()
			})

			eventbus.Subscribe(bus, func(_ context.Context, _ int) {
				_ = intObj.data[0]
				bus.Publish(context.Background(), "reentrant")
			})
			eventbus.Subscribe(bus, func(_ context.Context, _ string) {
				_ = strObj.data[0]
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := bus.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle after subscribe: %v", err)
		}

		for i := 0; i < numEvents; i++ {
			bus.Publish(ctx, i)
		}

		for i := 0; i < 10; i++ {
			if err := bus.WaitIdle(ctx); err != nil {
				break
			}
		}

		bus.Close()
	}()

	bus = nil
	forceGC()

	// Assert
	if !waitForFinalizers(t, &wg, 3*time.Second) {
		collected := finalizerCount.Load()
		t.Errorf("timeout: only %d/%d reentrant objects were GC'd", collected, totalObjects)
		return
	}
	collected := finalizerCount.Load()
	if collected != totalObjects {
		t.Errorf("reentrant objects GC'd: got = %d, want = %d", collected, totalObjects)
	}
}

// TestEventBus_GC_UnsubscribeReleasesHandler verifies unsubscribed handler closures release captured objects
// Given: a handler for int capturing a handlerObj with finalizer
// When: event is published (handler fires), handler is unsubscribed, bus is closed
// Then: the captured object is garbage collected
func TestEventBus_GC_UnsubscribeReleasesHandler(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("gc-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	bus := eventbus.NewEventBus(pool)

	var finalizerCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Act
	func() {
		obj := &gcObject{data: make([]byte, 50*1024)}
		runtime.SetFinalizer(obj, func(o *gcObject) {
			finalizerCalled.Store(true)
			wg.Done()
		})

		id := eventbus.Subscribe(bus, func(_ context.Context, _ int) {
			_ = obj.data[0]
		})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := bus.WaitIdle(ctx); err != nil {
			t.Fatalf("WaitIdle after subscribe: %v", err)
		}

		bus.Publish(ctx, 1)
		for i := 0; i < 10; i++ {
			if err := bus.WaitIdle(ctx); err != nil {
				break
			}
		}

		bus.Unsubscribe(id)
		for i := 0; i < 10; i++ {
			if err := bus.WaitIdle(ctx); err != nil {
				break
			}
		}

		bus.Close()
	}()

	bus = nil
	forceGC()

	// Assert
	if !waitForFinalizers(t, &wg, 2*time.Second) {
		t.Error("timeout: handler object was not GC'd after unsubscribe")
		return
	}
	if !finalizerCalled.Load() {
		t.Error("handler object GC'd: got = false, want = true")
	}
}
