package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// JobManager - Three-Layer Runner Architecture
// =============================================================================

// RawJobHandler is the internal handler type that works with raw bytes
type RawJobHandler func(ctx context.Context, args []byte) error

// TypedHandler is a generic handler type for type-safe job handlers
type TypedHandler[T any] func(ctx context.Context, args T) error

// ErrorHandler is called when an IO operation fails after all retries
type ErrorHandler func(jobID string, operation string, err error)

// JobManager manages job lifecycle using a three-layer runner architecture:
// - Layer 1 (controlRunner): Fast control operations (<100μs, pure memory)
// - Layer 2 (ioRunner): Sequential IO operations (database, file, network)
// - Layer 3 (executionRunner): User job execution (may be slow/blocking)
type JobManager struct {
	// Three-Layer Runner Architecture
	controlRunner   TaskRunner // Layer 1: Fast control operations
	ioRunner        TaskRunner // Layer 2: Sequential IO operations
	executionRunner TaskRunner // Layer 3: User job execution

	// Concurrent-safe data structures (lock-free)
	handlers   sync.Map // map[string]RawJobHandler
	activeJobs sync.Map // map[string]*activeJobInfo

	store      JobStore
	serializer JobSerializer

	// Error handling and retry
	retryPolicy  RetryPolicy
	logger       Logger
	errorHandler ErrorHandler

	closed atomic.Bool
}

// activeJobInfo tracks information about an active job
type activeJobInfo struct {
	cancel    context.CancelFunc
	jobEntity *JobEntity
	startTime time.Time
	dbSaved   atomic.Bool // Track if saved to DB
}

// NewJobManager creates a new JobManager with the three-layer runner architecture
func NewJobManager(
	controlRunner TaskRunner,
	ioRunner TaskRunner,
	executionRunner TaskRunner,
	store JobStore,
	serializer JobSerializer,
) *JobManager {
	return &JobManager{
		controlRunner:   controlRunner,
		ioRunner:        ioRunner,
		executionRunner: executionRunner,
		store:           store,
		serializer:      serializer,
		retryPolicy:     DefaultRetryPolicy(),
		logger:          NewNoOpLogger(), // Default: no logging
	}
}

// =============================================================================
// Configuration Methods
// =============================================================================

// SetRetryPolicy sets the retry policy for IO operations
func (m *JobManager) SetRetryPolicy(policy RetryPolicy) {
	m.retryPolicy = policy
}

// GetRetryPolicy returns the current retry policy
func (m *JobManager) GetRetryPolicy() RetryPolicy {
	return m.retryPolicy
}

// SetLogger sets the logger for JobManager
func (m *JobManager) SetLogger(logger Logger) {
	m.logger = logger
}

// SetErrorHandler sets a custom error handler for failed IO operations
func (m *JobManager) SetErrorHandler(handler ErrorHandler) {
	m.errorHandler = handler
}

// =============================================================================
// Handler Registration (Layer 1: controlRunner)
// =============================================================================

// RegisterHandler registers a type-safe handler for a job type.
// The handler will be called with deserialized arguments of type T.
func RegisterHandler[T any](m *JobManager, jobType string, handler TypedHandler[T]) error {
	if m.closed.Load() {
		return fmt.Errorf("JobManager is closed")
	}

	// Wrap handler with serialization logic
	var adapter RawJobHandler = func(ctx context.Context, rawArgs []byte) error {
		var args T
		if err := m.serializer.Deserialize(rawArgs, &args); err != nil {
			return fmt.Errorf("deserialize failed: %w", err)
		}
		return handler(ctx, args)
	}

	// Post to controlRunner (fast, pure memory operation)
	errChan := make(chan error, 1)
	m.controlRunner.PostTask(func(ctx context.Context) {
		m.handlers.Store(jobType, adapter)
		errChan <- nil
	})

	return <-errChan
}

// =============================================================================
// Submit Job Flow (Layer 1 → Layer 2 → Layer 3)
// =============================================================================

// SubmitJob submits a job for immediate execution
func (m *JobManager) SubmitJob(ctx context.Context, id string, jobType string, args any, traits TaskTraits) error {
	return m.SubmitDelayedJob(ctx, id, jobType, args, 0, traits)
}

// SubmitDelayedJob submits a job with a delay before execution
func (m *JobManager) SubmitDelayedJob(
	ctx context.Context,
	id string,
	jobType string,
	args any,
	delay time.Duration,
	traits TaskTraits,
) error {
	if m.closed.Load() {
		return fmt.Errorf("JobManager is closed")
	}

	// Serialize args (CPU-bound, do outside runners)
	argsBytes, err := m.serializer.Serialize(args)
	if err != nil {
		return err
	}

	// Capture all data for the task
	entity := &JobEntity{
		ID:        id,
		Type:      jobType,
		ArgsData:  argsBytes,
		Status:    JobStatusPending,
		Priority:  int(traits.Priority),
		CreatedAt: time.Now(),
	}

	// Capture parent context for closure
	// Note: PostTask's ctx parameter is the runLoop's context, not the parent context
	// We need to capture the parent context separately to pass it to submitJobControl
	parentCtx := ctx

	// Submit to controlRunner and wait for result
	// By running the entire submit logic on controlRunner, we ensure
	// sequential execution and atomic check-and-add without needing a mutex
	resultChan := make(chan error, 1)
	m.controlRunner.PostTask(func(_ context.Context) {
		resultChan <- m.submitJobControl(parentCtx, entity, traits, delay)
	})
	return <-resultChan
}

// submitJobControl: Layer 1 - Fast validation and scheduling
// This runs on controlRunner, which ensures sequential execution.
// The sequential execution guarantees atomic check-and-add without needing a mutex.
func (m *JobManager) submitJobControl(
	ctx context.Context,
	entity *JobEntity,
	traits TaskTraits,
	delay time.Duration,
) error {
	// 1. Fast duplicate check (memory only)
	// Since this runs sequentially on controlRunner, by the time we check activeJobs
	// and add to it, no other submitJobControl can be running concurrently
	if _, exists := m.activeJobs.Load(entity.ID); exists {
		return fmt.Errorf("job %s is already active", entity.ID)
	}

	// 2. Get handler (fast memory lookup)
	rawHandler, ok := m.handlers.Load(entity.Type)
	if !ok {
		return fmt.Errorf("handler for job type %s not found", entity.Type)
	}
	handler := rawHandler.(RawJobHandler)

	// 3. Create context derived from parent and add to activeJobs
	// When parent context is cancelled, jobCtx will also be cancelled
	jobCtx, cancel := context.WithCancel(ctx)
	info := &activeJobInfo{
		cancel:    cancel,
		jobEntity: entity,
		startTime: time.Now(),
	}
	m.activeJobs.Store(entity.ID, info)

	// 4. Delegate to Layer 2 (IO operations)
	m.submitJobIO(ctx, entity, jobCtx, handler, traits, delay, info)

	return nil
}

// submitJobIO: Layer 2 - Database operations (on ioRunner)
func (m *JobManager) submitJobIO(
	ctx context.Context,
	entity *JobEntity,
	jobCtx context.Context,
	handler RawJobHandler,
	traits TaskTraits,
	delay time.Duration,
	info *activeJobInfo,
) {
	m.ioRunner.PostTask(func(_ context.Context) {
		// 1. Check DB for duplicates (may be slow)
		existing, _ := m.store.GetJob(ctx, entity.ID)
		if existing != nil && (existing.Status == JobStatusPending || existing.Status == JobStatusRunning) {
			// Duplicate found, rollback activeJobs
			m.controlRunner.PostTask(func(_ context.Context) {
				m.activeJobs.Delete(entity.ID)
				info.cancel()
			})
			return
		}

		// 2. Save to DB (may be slow)
		if err := m.store.SaveJob(ctx, entity); err != nil {
			// Save failed, rollback activeJobs
			m.controlRunner.PostTask(func(_ context.Context) {
				m.activeJobs.Delete(entity.ID)
				info.cancel()
			})
			return
		}

		// 3. Mark as saved
		info.dbSaved.Store(true)

		// 4. Schedule execution on Layer 3
		m.scheduleExecution(entity, jobCtx, handler, traits, delay)
	})
}

// scheduleExecution: Layer 3 - Execute user handler
func (m *JobManager) scheduleExecution(
	entity *JobEntity,
	jobCtx context.Context,
	handler RawJobHandler,
	traits TaskTraits,
	delay time.Duration,
) {
	taskWrapper := func(_ context.Context) {
		// Pre-execution check
		if jobCtx.Err() != nil {
			m.finalizeJob(entity.ID, JobStatusCanceled, "Canceled before execution")
			return
		}

		// Update to RUNNING (via ioRunner)
		m.updateStatusIO(entity.ID, JobStatusRunning, "")

		// Execute handler
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			err = handler(jobCtx, entity.ArgsData)
		}()

		// Determine final status
		status := JobStatusCompleted
		msg := ""
		if err != nil {
			if jobCtx.Err() != nil {
				status = JobStatusCanceled
				msg = "Job canceled"
			} else {
				status = JobStatusFailed
				msg = err.Error()
			}
		}

		m.finalizeJob(entity.ID, status, msg)
	}

	if delay > 0 {
		m.executionRunner.PostDelayedTaskWithTraits(taskWrapper, delay, traits)
	} else {
		m.executionRunner.PostTaskWithTraits(taskWrapper, traits)
	}
}

// =============================================================================
// Cancel Job (Layer 1: controlRunner - Fast)
// =============================================================================

// CancelJob cancels an active job
func (m *JobManager) CancelJob(id string) error {
	if m.closed.Load() {
		return fmt.Errorf("JobManager is closed")
	}

	errChan := make(chan error, 1)
	m.controlRunner.PostTask(func(_ context.Context) {
		errChan <- m.cancelJobControl(id)
	})

	return <-errChan
}

func (m *JobManager) cancelJobControl(id string) error {
	raw, ok := m.activeJobs.Load(id)
	if !ok {
		return fmt.Errorf("job %s is not active", id)
	}

	info := raw.(*activeJobInfo)
	info.cancel() // Fast context cancellation

	return nil
}

// =============================================================================
// Finalize Job (Layer 1 → Layer 2)
// =============================================================================

func (m *JobManager) finalizeJob(id string, status JobStatus, msg string) {
	// Step 1: Clean up activeJobs (fast, on controlRunner)
	m.controlRunner.PostTask(func(_ context.Context) {
		m.finalizeJobControl(id, status, msg)
	})
}

func (m *JobManager) finalizeJobControl(id string, status JobStatus, msg string) {
	// Clean up activeJobs (fast memory operation)
	raw, ok := m.activeJobs.LoadAndDelete(id)
	if !ok {
		return
	}

	info := raw.(*activeJobInfo)
	info.cancel() // Cancel context

	// Step 2: Update DB status (delegate to ioRunner)
	m.updateStatusIO(id, status, msg)
}

// =============================================================================
// Retry Logic
// =============================================================================

// retryIOOperation executes an IO operation with retry logic
// Returns the last error if all retries fail
func (m *JobManager) retryIOOperation(
	ctx context.Context,
	operation string,
	jobID string,
	fn func(context.Context) error,
) error {
	var lastErr error
	for attempt := 0; attempt <= m.retryPolicy.MaxRetries; attempt++ {
		if err := fn(ctx); err == nil {
			// Success
			if attempt > 0 {
				m.logger.Debug("IO operation succeeded after retry",
					F("operation", operation),
					F("jobID", jobID),
					F("attempt", attempt))
			}
			return nil
		} else {
			lastErr = err
			// Log retry attempt
			m.logger.Warn("IO operation failed, retrying",
				F("operation", operation),
				F("jobID", jobID),
				F("attempt", attempt),
				F("maxRetries", m.retryPolicy.MaxRetries),
				F("error", err))

			if attempt < m.retryPolicy.MaxRetries {
				// Calculate delay and wait
				delay := m.retryPolicy.calculateDelay(attempt)
				m.logger.Debug("Waiting before retry",
					F("delay", delay),
					F("attempt", attempt+1))
				time.Sleep(delay)
			}
		}
	}

	// All retries failed
	m.logger.Error("IO operation failed after all retries",
		F("operation", operation),
		F("jobID", jobID),
		F("totalAttempts", m.retryPolicy.MaxRetries+1),
		F("error", lastErr))

	// Call error handler if set
	if m.errorHandler != nil {
		m.errorHandler(jobID, operation, lastErr)
	}

	return lastErr
}

func (m *JobManager) updateStatusIO(id string, status JobStatus, msg string) {
	m.ioRunner.PostTask(func(_ context.Context) {
		ctx := context.Background()
		// Use retry logic for status updates
		_ = m.retryIOOperation(ctx, "UpdateStatus", id, func(ctx context.Context) error {
			return m.store.UpdateStatus(ctx, id, status, msg)
		})
		// Error already logged and handled by retryIOOperation
	})
}

// =============================================================================
// Read Operations (Direct Access - Lock-free)
// =============================================================================

// ListJobs returns jobs matching the filter (allows slight delay from recent writes)
func (m *JobManager) ListJobs(ctx context.Context, filter JobFilter) ([]*JobEntity, error) {
	return m.store.ListJobs(ctx, filter)
}

// GetJob retrieves a job by ID
func (m *JobManager) GetJob(ctx context.Context, id string) (*JobEntity, error) {
	return m.store.GetJob(ctx, id)
}

// GetActiveJobCount returns the number of active jobs
func (m *JobManager) GetActiveJobCount() int {
	count := 0
	m.activeJobs.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}

// GetActiveJobs returns a snapshot of active jobs
func (m *JobManager) GetActiveJobs() []*JobEntity {
	var jobs []*JobEntity
	m.activeJobs.Range(func(key, value any) bool {
		info := value.(*activeJobInfo)
		jobs = append(jobs, info.jobEntity)
		return true
	})
	return jobs
}

// =============================================================================
// Lifecycle Management
// =============================================================================

// Start initializes the JobManager and recovers unfinished jobs
func (m *JobManager) Start(ctx context.Context) error {
	errChan := make(chan error, 1)

	m.controlRunner.PostTask(func(_ context.Context) {
		errChan <- m.startRecovery(ctx)
	})

	return <-errChan
}

func (m *JobManager) startRecovery(ctx context.Context) error {
	// Delegate IO operations to ioRunner
	m.ioRunner.PostTask(func(_ context.Context) {
		m.doRecoveryIO(ctx)
	})

	return nil
}

func (m *JobManager) doRecoveryIO(ctx context.Context) {
	// 1. Mark RUNNING jobs as FAILED (interrupted by restart)
	runningJobs, err := m.store.ListJobs(ctx, JobFilter{Status: JobStatusRunning})
	if err != nil {
		m.logger.Error("Failed to list running jobs during recovery", F("error", err))
		return
	}

	for _, job := range runningJobs {
		// Use retry logic for status updates during recovery
		_ = m.retryIOOperation(ctx, "RecoveryUpdateStatus", job.ID, func(ctx context.Context) error {
			return m.store.UpdateStatus(ctx, job.ID, JobStatusFailed, "Interrupted by restart")
		})
	}

	// 2. Recover PENDING jobs
	jobs, err := m.store.GetRecoverableJobs(ctx)
	if err != nil {
		return
	}

	// 3. Schedule recovered jobs (via controlRunner)
	for _, job := range jobs {
		jobCopy := job

		m.controlRunner.PostTask(func(_ context.Context) {
			// Get handler
			rawHandler, ok := m.handlers.Load(jobCopy.Type)
			if !ok {
				return
			}
			handler := rawHandler.(RawJobHandler)

			// Create context and add to activeJobs
			jobCtx, cancel := context.WithCancel(context.Background())
			info := &activeJobInfo{
				cancel:    cancel,
				jobEntity: jobCopy,
				startTime: time.Now(),
			}
			info.dbSaved.Store(true) // Already in DB
			m.activeJobs.Store(jobCopy.ID, info)

			// Schedule execution
			traits := TaskTraits{Priority: TaskPriority(jobCopy.Priority)}
			m.scheduleExecution(jobCopy, jobCtx, handler, traits, 0)
		})
	}
}

// Shutdown gracefully shuts down the JobManager
func (m *JobManager) Shutdown(ctx context.Context) error {
	if !m.closed.CompareAndSwap(false, true) {
		return fmt.Errorf("already closed")
	}

	// 1. Cancel all active jobs (via controlRunner - fast)
	doneChan := make(chan struct{})
	m.controlRunner.PostTask(func(_ context.Context) {
		m.activeJobs.Range(func(key, value any) bool {
			info := value.(*activeJobInfo)
			info.cancel()
			return true
		})
		close(doneChan)
	})

	select {
	case <-doneChan:
	case <-ctx.Done():
		return ctx.Err()
	}

	// 2. Wait for activeJobs to drain
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if m.GetActiveJobCount() == 0 {
				// 3. Shutdown runners in order
				m.controlRunner.Shutdown()
				m.ioRunner.Shutdown()
				m.executionRunner.Shutdown()
				return nil
			}
		}
	}
}
