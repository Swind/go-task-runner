package prometheus

import (
	"context"
	"testing"
	"time"

	"github.com/Swind/go-task-runner/core"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type runnerStub struct {
	stats core.RunnerStats
}

func (s runnerStub) Stats() core.RunnerStats { return s.stats }

type poolStub struct {
	stats core.PoolStats
}

func (s poolStub) Stats() core.PoolStats { return s.stats }

func TestSnapshotPoller_CollectsRunnerAndPoolStats(t *testing.T) {
	reg := prom.NewRegistry()
	poller, err := NewSnapshotPoller(reg, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("NewSnapshotPoller failed: %v", err)
	}

	poller.AddRunner("runner-a", runnerStub{stats: core.RunnerStats{
		Type:     "sequenced",
		Pending:  3,
		Running:  1,
		Rejected: 2,
		Closed:   true,
	}})
	poller.AddPool("pool-a", poolStub{stats: core.PoolStats{
		Queued:  4,
		Active:  2,
		Delayed: 1,
		Workers: 8,
		Running: true,
	}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poller.Start(ctx)
	defer poller.Stop()

	assertEventually(t, 2*time.Second, func() bool {
		pending := testutil.ToFloat64(poller.runnerPending.WithLabelValues("runner-a", "sequenced"))
		active := testutil.ToFloat64(poller.poolActive.WithLabelValues("pool-a"))
		return pending == 3 && active == 2
	})

	if got := testutil.ToFloat64(poller.runnerClosed.WithLabelValues("runner-a", "sequenced")); got != 1 {
		t.Fatalf("runner closed gauge = %v, want 1", got)
	}
	if got := testutil.ToFloat64(poller.poolRunning.WithLabelValues("pool-a")); got != 1 {
		t.Fatalf("pool running gauge = %v, want 1", got)
	}
}

func TestSnapshotPoller_StartStop_Idempotent(t *testing.T) {
	reg := prom.NewRegistry()
	poller, err := NewSnapshotPoller(reg, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("NewSnapshotPoller failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poller.Start(ctx)
	poller.Start(ctx)
	poller.Stop()
	poller.Stop()
}

func assertEventually(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
