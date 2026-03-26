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

type HandlerFunc[T any] func(ctx context.Context, event T)

func Subscribe[T any](b *EventBus, handler HandlerFunc[T]) string {
	id := fmt.Sprintf("sub-%d", b.nextID.Add(1))
	typ := reflect.TypeFor[T]()

	b.runner.PostTask(func(ctx context.Context) {
		b.subs[typ] = append(b.subs[typ], subscriber{id: id, handler: handler})
		b.byID[id] = subLocator{eventType: typ, index: len(b.subs[typ]) - 1}
	})

	return id
}

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
				reflect.ValueOf(handler).Call([]reflect.Value{
					reflect.ValueOf(rctx),
					reflect.ValueOf(event),
				})
			})
		}
	})
}

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

func (b *EventBus) WaitIdle(ctx context.Context) error {
	return b.runner.WaitIdle(ctx)
}

func (b *EventBus) Close() {
	b.closed.Store(true)
	b.runner.Shutdown()
}
