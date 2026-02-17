package main

import (
	"context"
	"fmt"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
	obs "github.com/Swind/go-task-runner/observability/prometheus"
	prom "github.com/prometheus/client_golang/prometheus"
)

func main() {
	reg := prom.NewRegistry()

	exporter, err := obs.NewMetricsExporter("taskrunner", reg, obs.ExporterOptions{})
	if err != nil {
		panic(err)
	}

	poller, err := obs.NewSnapshotPoller(reg, 50*time.Millisecond)
	if err != nil {
		panic(err)
	}

	config := &core.TaskSchedulerConfig{
		PanicHandler:        &core.DefaultPanicHandler{},
		Metrics:             exporter,
		RejectedTaskHandler: &core.DefaultRejectedTaskHandler{},
	}

	pool := taskrunner.NewPriorityGoroutineThreadPoolWithConfig("metrics-pool", 2, config)
	pool.Start(context.Background())
	defer pool.Stop()

	runner := taskrunner.NewSequencedTaskRunner(pool)
	runner.SetName("metrics-seq")

	poller.AddPool("metrics-pool", pool)
	poller.AddRunner("metrics-seq", runner)
	poller.Start(context.Background())
	defer poller.Stop()

	runner.PostTaskNamed("quick-task", func(ctx context.Context) {
		time.Sleep(5 * time.Millisecond)
	})
	runner.PostTask(func(ctx context.Context) {
		time.Sleep(2 * time.Millisecond)
	})

	if err := runner.WaitIdle(context.Background()); err != nil {
		panic(err)
	}

	mf, err := reg.Gather()
	if err != nil {
		panic(err)
	}
	fmt.Printf("exported metric families: %d\n", len(mf))
}
