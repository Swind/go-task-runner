package core

import (
	"context"
	"testing"
	"time"
)

type blockedThreadPool struct {
	tasks chan Task
}

func newBlockedThreadPool() *blockedThreadPool {
	return &blockedThreadPool{tasks: make(chan Task, 1024)}
}

func (p *blockedThreadPool) PostInternal(task Task, traits TaskTraits) {
	p.tasks <- task
}

func (p *blockedThreadPool) PostDelayedInternal(task Task, delay time.Duration, traits TaskTraits, target TaskRunner) {
	time.AfterFunc(delay, func() {
		target.PostTaskWithTraits(task, traits)
	})
}

func (p *blockedThreadPool) Start(ctx context.Context) {}
func (p *blockedThreadPool) Stop()                     {}
func (p *blockedThreadPool) ID() string                { return "blocked" }
func (p *blockedThreadPool) IsRunning() bool           { return true }
func (p *blockedThreadPool) WorkerCount() int          { return 1 }
func (p *blockedThreadPool) QueuedTaskCount() int      { return len(p.tasks) }
func (p *blockedThreadPool) ActiveTaskCount() int      { return 0 }
func (p *blockedThreadPool) DelayedTaskCount() int     { return 0 }

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

func TestSequencedTaskRunner_Stats(t *testing.T) {
	pool := newBlockedThreadPool()
	runner := NewSequencedTaskRunner(pool)
	runner.SetName("seq-test")

	runner.PostTask(func(ctx context.Context) {})

	stats := runner.Stats()
	if stats.Name != "seq-test" || stats.Type != "sequenced" {
		t.Fatalf("unexpected stats identity: %+v", stats)
	}
	if stats.Pending != 1 {
		t.Fatalf("pending = %d, want 1", stats.Pending)
	}
	if stats.Running != 1 {
		t.Fatalf("running = %d, want 1", stats.Running)
	}
	if stats.Closed {
		t.Fatal("closed = true, want false")
	}

	runner.Shutdown()
	stats = runner.Stats()
	if !stats.Closed {
		t.Fatal("closed = false after shutdown, want true")
	}
	if stats.Pending != 0 {
		t.Fatalf("pending after shutdown = %d, want 0", stats.Pending)
	}
}

func TestParallelTaskRunner_Stats(t *testing.T) {
	pool := newBlockedThreadPool()
	runner := NewParallelTaskRunner(pool, 1)
	runner.SetName("par-test")
	defer runner.Shutdown()

	for range 3 {
		runner.PostTask(func(ctx context.Context) {})
	}

	waitForCondition(t, time.Second, func() bool {
		return runner.RunningTaskCount() == 1
	})

	stats := runner.Stats()
	if stats.Name != "par-test" || stats.Type != "parallel" {
		t.Fatalf("unexpected stats identity: %+v", stats)
	}
	if stats.Running != 1 {
		t.Fatalf("running = %d, want 1", stats.Running)
	}
	if stats.Pending < 1 {
		t.Fatalf("pending = %d, want >= 1", stats.Pending)
	}
	if stats.Closed {
		t.Fatal("closed = true, want false")
	}

	runner.Shutdown()
	stats = runner.Stats()
	if !stats.Closed {
		t.Fatal("closed = false after shutdown, want true")
	}
	if stats.Pending != 0 {
		t.Fatalf("pending after shutdown = %d, want 0", stats.Pending)
	}
}

func TestSingleThreadTaskRunner_Stats(t *testing.T) {
	runner := NewSingleThreadTaskRunner()
	runner.SetName("single-test")
	defer runner.Stop()

	entered := make(chan struct{})
	unblock := make(chan struct{})

	runner.PostTask(func(ctx context.Context) {
		close(entered)
		<-unblock
	})

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("task did not start in time")
	}

	stats := runner.Stats()
	if stats.Name != "single-test" || stats.Type != "single-thread" {
		t.Fatalf("unexpected stats identity: %+v", stats)
	}
	if stats.Running != 1 {
		t.Fatalf("running = %d, want 1", stats.Running)
	}
	if stats.Rejected != runner.RejectedCount() {
		t.Fatalf("rejected = %d, want %d", stats.Rejected, runner.RejectedCount())
	}
	if stats.Closed {
		t.Fatal("closed = true, want false")
	}

	close(unblock)
	if err := runner.WaitIdle(context.Background()); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}

	stats = runner.Stats()
	if stats.Running != 0 {
		t.Fatalf("running after idle = %d, want 0", stats.Running)
	}
}
