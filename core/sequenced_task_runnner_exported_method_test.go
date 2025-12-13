package core

import (
	"context"
	"sync/atomic"
)

func (r *SequencedTaskRunner) ExportedEnsureRunning(traits TaskTraits) {
	r.ensureRunning(traits)
}

func (r *SequencedTaskRunner) GetRunningCount() int32 {
	return atomic.LoadInt32(&r.runningCount)
}

func (r *SequencedTaskRunner) SetRunningCount(count int32) {
	atomic.StoreInt32(&r.runningCount, count)
}

func (r *SequencedTaskRunner) ExportedRunLoop(ctx context.Context) {
	r.runLoop(ctx)
}
