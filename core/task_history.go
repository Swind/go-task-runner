package core

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"time"
)

const defaultTaskHistoryCapacity = 100

type executionHistory struct {
	mu    sync.Mutex
	items []TaskExecutionRecord
	head  int
	count int
}

func newExecutionHistory(capacity int) executionHistory {
	if capacity < 1 {
		capacity = defaultTaskHistoryCapacity
	}
	return executionHistory{items: make([]TaskExecutionRecord, capacity)}
}

func (h *executionHistory) Add(record TaskExecutionRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.items) == 0 {
		return
	}

	h.items[h.head] = record
	h.head = (h.head + 1) % len(h.items)
	if h.count < len(h.items) {
		h.count++
	}
}

func (h *executionHistory) Recent(limit int) []TaskExecutionRecord {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.count == 0 {
		return nil
	}

	if limit <= 0 || limit > h.count {
		limit = h.count
	}

	out := make([]TaskExecutionRecord, 0, limit)
	for i := range limit {
		idx := (h.head - 1 - i + len(h.items)) % len(h.items)
		out = append(out, h.items[idx])
	}
	return out
}

func (h *executionHistory) Last() (TaskExecutionRecord, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.count == 0 {
		return TaskExecutionRecord{}, false
	}

	idx := (h.head - 1 + len(h.items)) % len(h.items)
	return h.items[idx], true
}

func resolveTaskName(task Task, explicit string) string {
	if explicit != "" {
		return explicit
	}

	if task == nil {
		return "anonymous"
	}

	v := reflect.ValueOf(task)
	if v.Kind() != reflect.Func {
		return "anonymous"
	}

	pc := v.Pointer()
	if pc == 0 {
		return "anonymous"
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "anonymous"
	}

	name := fn.Name()
	if name == "" {
		return "anonymous"
	}
	return name
}

func wrapObservedTask(
	task Task,
	explicitName string,
	traits TaskTraits,
	runnerName string,
	runnerType string,
	record func(TaskExecutionRecord),
) Task {
	taskID := GenerateTaskID()
	name := resolveTaskName(task, explicitName)

	if runnerName == "" {
		runnerName = runnerType
	}

	return func(ctx context.Context) {
		startedAt := time.Now()
		panicked := false

		defer func() {
			if rec := recover(); rec != nil {
				panicked = true
				finishedAt := time.Now()
				record(TaskExecutionRecord{
					TaskID:     taskID,
					Name:       name,
					RunnerName: runnerName,
					RunnerType: runnerType,
					Priority:   traits.Priority,
					StartedAt:  startedAt,
					FinishedAt: finishedAt,
					Duration:   finishedAt.Sub(startedAt),
					Panicked:   panicked,
				})
				panic(rec)
			}

			finishedAt := time.Now()
			record(TaskExecutionRecord{
				TaskID:     taskID,
				Name:       name,
				RunnerName: runnerName,
				RunnerType: runnerType,
				Priority:   traits.Priority,
				StartedAt:  startedAt,
				FinishedAt: finishedAt,
				Duration:   finishedAt.Sub(startedAt),
				Panicked:   panicked,
			})
		}()

		if task == nil {
			panic(fmt.Sprintf("task %s is nil", taskID.String()))
		}
		task(ctx)
	}
}
