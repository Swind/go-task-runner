package prometheus

import (
	"context"
	"sync"
	"time"

	"github.com/Swind/go-task-runner/core"
	prom "github.com/prometheus/client_golang/prometheus"
)

// RunnerSnapshotProvider provides current runner stats snapshots.
type RunnerSnapshotProvider interface {
	Stats() core.RunnerStats
}

// PoolSnapshotProvider provides current pool stats snapshots.
type PoolSnapshotProvider interface {
	Stats() core.PoolStats
}

// SnapshotPoller periodically exports runner/pool Stats() snapshots into Prometheus gauges.
type SnapshotPoller struct {
	interval time.Duration

	runnersMu sync.RWMutex
	runners   map[string]RunnerSnapshotProvider

	poolsMu sync.RWMutex
	pools   map[string]PoolSnapshotProvider

	runnerPending  *prom.GaugeVec
	runnerRunning  *prom.GaugeVec
	runnerRejected *prom.GaugeVec
	runnerClosed   *prom.GaugeVec

	poolQueued  *prom.GaugeVec
	poolActive  *prom.GaugeVec
	poolDelayed *prom.GaugeVec
	poolWorkers *prom.GaugeVec
	poolRunning *prom.GaugeVec

	stateMu sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewSnapshotPoller creates a snapshot poller and registers its collectors.
func NewSnapshotPoller(reg prom.Registerer, interval time.Duration) (*SnapshotPoller, error) {
	if reg == nil {
		reg = prom.DefaultRegisterer
	}
	if interval <= 0 {
		interval = time.Second
	}

	runnerPending := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: "taskrunner",
		Name:      "runner_pending",
		Help:      "Number of pending tasks per runner.",
	}, []string{"runner", "type"})
	runnerRunning := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: "taskrunner",
		Name:      "runner_running",
		Help:      "Number of running tasks per runner.",
	}, []string{"runner", "type"})
	runnerRejected := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: "taskrunner",
		Name:      "runner_rejected_total",
		Help:      "Runner rejected task count snapshot.",
	}, []string{"runner", "type"})
	runnerClosed := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: "taskrunner",
		Name:      "runner_closed",
		Help:      "Runner closed state (1=closed, 0=open).",
	}, []string{"runner", "type"})

	poolQueued := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: "taskrunner",
		Name:      "pool_queued",
		Help:      "Queued tasks per pool.",
	}, []string{"pool"})
	poolActive := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: "taskrunner",
		Name:      "pool_active",
		Help:      "Active tasks per pool.",
	}, []string{"pool"})
	poolDelayed := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: "taskrunner",
		Name:      "pool_delayed",
		Help:      "Delayed tasks per pool.",
	}, []string{"pool"})
	poolWorkers := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: "taskrunner",
		Name:      "pool_workers",
		Help:      "Worker count per pool.",
	}, []string{"pool"})
	poolRunning := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: "taskrunner",
		Name:      "pool_running",
		Help:      "Pool running state (1=running, 0=stopped).",
	}, []string{"pool"})

	var err error
	if runnerPending, err = registerCollector(reg, runnerPending); err != nil {
		return nil, err
	}
	if runnerRunning, err = registerCollector(reg, runnerRunning); err != nil {
		return nil, err
	}
	if runnerRejected, err = registerCollector(reg, runnerRejected); err != nil {
		return nil, err
	}
	if runnerClosed, err = registerCollector(reg, runnerClosed); err != nil {
		return nil, err
	}
	if poolQueued, err = registerCollector(reg, poolQueued); err != nil {
		return nil, err
	}
	if poolActive, err = registerCollector(reg, poolActive); err != nil {
		return nil, err
	}
	if poolDelayed, err = registerCollector(reg, poolDelayed); err != nil {
		return nil, err
	}
	if poolWorkers, err = registerCollector(reg, poolWorkers); err != nil {
		return nil, err
	}
	if poolRunning, err = registerCollector(reg, poolRunning); err != nil {
		return nil, err
	}

	return &SnapshotPoller{
		interval:       interval,
		runners:        make(map[string]RunnerSnapshotProvider),
		pools:          make(map[string]PoolSnapshotProvider),
		runnerPending:  runnerPending,
		runnerRunning:  runnerRunning,
		runnerRejected: runnerRejected,
		runnerClosed:   runnerClosed,
		poolQueued:     poolQueued,
		poolActive:     poolActive,
		poolDelayed:    poolDelayed,
		poolWorkers:    poolWorkers,
		poolRunning:    poolRunning,
	}, nil
}

// AddRunner adds or replaces a runner snapshot provider by name.
func (p *SnapshotPoller) AddRunner(name string, provider RunnerSnapshotProvider) {
	if p == nil || provider == nil {
		return
	}
	name = normalizeLabel(name, "runner")
	p.runnersMu.Lock()
	p.runners[name] = provider
	p.runnersMu.Unlock()
}

// AddPool adds or replaces a pool snapshot provider by name.
func (p *SnapshotPoller) AddPool(name string, provider PoolSnapshotProvider) {
	if p == nil || provider == nil {
		return
	}
	name = normalizeLabel(name, "pool")
	p.poolsMu.Lock()
	p.pools[name] = provider
	p.poolsMu.Unlock()
}

// Start begins periodic polling; repeated calls are no-ops.
func (p *SnapshotPoller) Start(ctx context.Context) {
	if p == nil {
		return
	}

	p.stateMu.Lock()
	if p.running {
		p.stateMu.Unlock()
		return
	}
	pollCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})
	p.running = true
	p.stateMu.Unlock()

	go p.loop(pollCtx)
}

// Stop stops periodic polling; repeated calls are safe.
func (p *SnapshotPoller) Stop() {
	if p == nil {
		return
	}

	p.stateMu.Lock()
	if !p.running {
		p.stateMu.Unlock()
		return
	}
	cancel := p.cancel
	done := p.done
	p.stateMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	p.stateMu.Lock()
	p.running = false
	p.cancel = nil
	p.done = nil
	p.stateMu.Unlock()
}

func (p *SnapshotPoller) loop(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.collectOnce()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.collectOnce()
		}
	}
}

func (p *SnapshotPoller) collectOnce() {
	p.runnersMu.RLock()
	for name, provider := range p.runners {
		stats := provider.Stats()
		typeLabel := normalizeLabel(stats.Type, "unknown")
		p.runnerPending.WithLabelValues(name, typeLabel).Set(float64(stats.Pending))
		p.runnerRunning.WithLabelValues(name, typeLabel).Set(float64(stats.Running))
		p.runnerRejected.WithLabelValues(name, typeLabel).Set(float64(stats.Rejected))
		if stats.Closed {
			p.runnerClosed.WithLabelValues(name, typeLabel).Set(1)
		} else {
			p.runnerClosed.WithLabelValues(name, typeLabel).Set(0)
		}
	}
	p.runnersMu.RUnlock()

	p.poolsMu.RLock()
	for name, provider := range p.pools {
		stats := provider.Stats()
		p.poolQueued.WithLabelValues(name).Set(float64(stats.Queued))
		p.poolActive.WithLabelValues(name).Set(float64(stats.Active))
		p.poolDelayed.WithLabelValues(name).Set(float64(stats.Delayed))
		p.poolWorkers.WithLabelValues(name).Set(float64(stats.Workers))
		if stats.Running {
			p.poolRunning.WithLabelValues(name).Set(1)
		} else {
			p.poolRunning.WithLabelValues(name).Set(0)
		}
	}
	p.poolsMu.RUnlock()
}
