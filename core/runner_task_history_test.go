package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type historyTestPool struct {
	tasks   chan Task
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	active  atomic.Int32
	workers int
}

func newHistoryTestPool(workers int) *historyTestPool {
	if workers < 1 {
		workers = 1
	}
	return &historyTestPool{workers: workers}
}

func (p *historyTestPool) Start(ctx context.Context) {
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.tasks = make(chan Task, 1024)
	for range p.workers {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case task := <-p.tasks:
					p.active.Add(1)
					task(p.ctx)
					p.active.Add(-1)
				case <-p.ctx.Done():
					return
				}
			}
		}()
	}
}

func (p *historyTestPool) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
}

func (p *historyTestPool) PostInternal(task Task, traits TaskTraits) {
	p.tasks <- task
}

func (p *historyTestPool) PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner) {
	time.AfterFunc(delay, func() {
		target.PostTaskWithTraits(task, traits)
	})
}

func (p *historyTestPool) ID() string            { return "history-test" }
func (p *historyTestPool) IsRunning() bool       { return p.ctx != nil }
func (p *historyTestPool) WorkerCount() int      { return p.workers }
func (p *historyTestPool) QueuedTaskCount() int  { return len(p.tasks) }
func (p *historyTestPool) ActiveTaskCount() int  { return int(p.active.Load()) }
func (p *historyTestPool) DelayedTaskCount() int { return 0 }

func TestSequencedTaskRunner_RecentTasks_NamedAndFallback(t *testing.T) {
	pool := newHistoryTestPool(1)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := NewSequencedTaskRunner(pool)
	runner.SetName("seq-obv")

	runner.PostTaskNamed("explicit-seq", func(ctx context.Context) {})
	runner.PostTask(func(ctx context.Context) {})

	delayedDone := make(chan struct{})
	runner.PostDelayedTaskNamed("delayed-seq", func(ctx context.Context) {
		close(delayedDone)
	}, 10*time.Millisecond)

	select {
	case <-delayedDone:
	case <-time.After(2 * time.Second):
		t.Fatal("delayed task did not run in time")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runner.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	recs := runner.RecentTasks(10)
	if len(recs) < 3 {
		t.Fatalf("RecentTasks count = %d, want >= 3", len(recs))
	}

	foundExplicit := false
	foundDelayed := false
	foundFallback := false
	for _, rec := range recs {
		if rec.Name == "explicit-seq" {
			foundExplicit = true
		}
		if rec.Name == "delayed-seq" {
			foundDelayed = true
		}
		if rec.Name != "explicit-seq" && rec.Name != "delayed-seq" && rec.Name != "" {
			foundFallback = true
		}
		if rec.Duration < 0 {
			t.Fatalf("negative duration: %+v", rec)
		}
	}

	if !foundExplicit || !foundDelayed || !foundFallback {
		t.Fatalf("missing expected records: explicit=%v delayed=%v fallback=%v records=%+v",
			foundExplicit, foundDelayed, foundFallback, recs)
	}

	stats := runner.Stats()
	if stats.LastTaskName == "" {
		t.Fatal("Stats.LastTaskName should not be empty")
	}
	if stats.LastTaskAt.IsZero() {
		t.Fatal("Stats.LastTaskAt should not be zero")
	}
}

func TestParallelTaskRunner_RecentTasks_PanicRecorded(t *testing.T) {
	pool := newHistoryTestPool(2)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := NewParallelTaskRunner(pool, 2)
	runner.SetName("par-obv")
	defer runner.Shutdown()

	runner.PostTaskNamed("ok-task", func(ctx context.Context) {})
	runner.PostTaskNamed("panic-task", func(ctx context.Context) { panic("boom") })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runner.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	recs := runner.RecentTasks(10)
	if len(recs) < 2 {
		t.Fatalf("RecentTasks count = %d, want >= 2", len(recs))
	}

	panicFound := false
	for _, rec := range recs {
		if rec.Name == "panic-task" {
			panicFound = true
			if !rec.Panicked {
				t.Fatalf("panic-task should be marked panicked: %+v", rec)
			}
		}
	}
	if !panicFound {
		t.Fatalf("panic-task record not found: %+v", recs)
	}
}

func TestSingleThreadTaskRunner_RecentTasks_LimitAndOrder(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	runner.SetName("single-obv")
	defer runner.Stop()

	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("task-%d", i)
		runner.PostTaskNamed(name, func(ctx context.Context) {})
	}
	runner.PostTask(func(ctx context.Context) {}) // unnamed, should get fallback name

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runner.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	recent2 := runner.RecentTasks(2)
	if len(recent2) != 2 {
		t.Fatalf("RecentTasks(2) len = %d, want 2", len(recent2))
	}

	all := runner.RecentTasks(0)
	if len(all) < 4 {
		t.Fatalf("RecentTasks(0) len = %d, want >= 4", len(all))
	}

	if all[0].FinishedAt.Before(all[1].FinishedAt) {
		t.Fatalf("RecentTasks should be newest-first: %+v", all[:2])
	}
	if all[0].Name == "" {
		t.Fatal("latest task name should not be empty")
	}
}
