package core

// RunnerStats represents runtime observability state for a task runner.
type RunnerStats struct {
	Name           string
	Type           string
	Pending        int
	Running        int
	Rejected       int64
	Closed         bool
	BarrierPending bool
}

// PoolStats represents runtime observability state for a thread pool.
type PoolStats struct {
	ID      string
	Workers int
	Queued  int
	Active  int
	Delayed int
	Running bool
}
