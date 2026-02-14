package taskrunner

import (
	"context"
	"testing"
	"time"

	"github.com/Swind/go-task-runner/core"
)

// TestPoolConstructorsAndSchedulerAccessors verifies wrapper pool constructors expose scheduler state
// Given: Pool constructors with default and custom scheduler config
// When: Each pool is created and inspected
// Then: Each exposes a non-nil scheduler and zero delayed tasks
func TestPoolConstructorsAndSchedulerAccessors(t *testing.T) {
	// Arrange
	cfg := &core.TaskSchedulerConfig{
		PanicHandler:        &core.DefaultPanicHandler{},
		Metrics:             &core.NilMetrics{},
		RejectedTaskHandler: &core.DefaultRejectedTaskHandler{},
	}

	// Act
	p1 := NewGoroutineThreadPoolWithConfig("cfg-pool", 1, cfg)
	p2 := NewPriorityGoroutineThreadPool("prio-pool", 1)
	p3 := NewPriorityGoroutineThreadPoolWithConfig("prio-cfg-pool", 1, cfg)

	// Assert
	for _, p := range []*GoroutineThreadPool{p1, p2, p3} {
		if p.GetScheduler() == nil {
			t.Fatalf("GetScheduler() returned nil for pool %q", p.ID())
		}
		if p.DelayedTaskCount() != 0 {
			t.Fatalf("DelayedTaskCount() = %d, want 0 for fresh pool", p.DelayedTaskCount())
		}
	}
}

// TestTypeWrappersAndGlobalPoolAccessor verifies top-level wrappers return usable instances
// Given: An initialized global pool
// When: Type wrapper constructors and GlobalThreadPool accessor are called
// Then: Wrappers return non-nil runners and tasks execute through shared pool
func TestTypeWrappersAndGlobalPoolAccessor(t *testing.T) {
	// Arrange
	InitGlobalThreadPool(1)
	defer ShutdownGlobalThreadPool()

	// Act
	gp := GlobalThreadPool()

	// Assert
	if gp == nil {
		t.Fatal("GlobalThreadPool() returned nil")
	}

	// Act
	seq := NewSequencedTaskRunner(gp)

	// Assert
	if seq == nil {
		t.Fatal("NewSequencedTaskRunner() returned nil")
	}

	// Act
	single := NewSingleThreadTaskRunner()

	// Assert
	if single == nil {
		t.Fatal("NewSingleThreadTaskRunner() returned nil")
	}
	defer single.Stop()

	// Act
	par := NewParallelTaskRunner(gp, 1)

	// Assert
	if par == nil {
		t.Fatal("NewParallelTaskRunner() returned nil")
	}
	par.Shutdown()

	// Act
	done := make(chan struct{}, 1)
	seq.PostTask(func(ctx context.Context) {
		select {
		case done <- struct{}{}:
		default:
		}
	})

	// Assert
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("sequenced runner wrapper task did not execute")
	}
}
