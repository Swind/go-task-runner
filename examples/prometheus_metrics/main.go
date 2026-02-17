package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
	obs "github.com/Swind/go-task-runner/observability/prometheus"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	server := &http.Server{Addr: ":2112", Handler: mux}

	go func() {
		_ = server.ListenAndServe()
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	for i := range 8 {
		runner.PostTaskNamed(fmt.Sprintf("demo-task-%d", i), func(ctx context.Context) {
			time.Sleep(5 * time.Millisecond)
		})
	}

	if err := runner.WaitIdle(context.Background()); err != nil {
		panic(err)
	}

	fmt.Println("Prometheus endpoint is up at http://127.0.0.1:2112/metrics")
	fmt.Println("Try: curl -s http://127.0.0.1:2112/metrics | grep '^taskrunner_'")

	// Keep demo alive briefly so local users can scrape metrics.
	time.Sleep(2 * time.Second)
}
