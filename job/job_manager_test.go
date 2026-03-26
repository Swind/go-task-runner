package job_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/job"
)

// =============================================================================
// JobManager Tests
// =============================================================================

func setupJobManager(t *testing.T) (*job.JobManager, func()) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = manager.Shutdown(ctx)
		pool.Stop()
	}

	return manager, cleanup
}

// TestJobManager_RegisterHandler verifies handler registration
// Given: A JobManager and a handler function
// When: Handler is registered for a job type
// Then: Registration completes successfully
func TestJobManager_RegisterHandler(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	handlerCalled := atomic.Bool{}
	handler := func(ctx context.Context, args EmailArgs) error {
		handlerCalled.Store(true)
		return nil
	}

	// Act
	if err := job.RegisterHandler(manager, context.Background(), "email", handler); err != nil {
		t.Fatalf("RegisterHandler failed: %v", err)
	}

	// Wait for registration
	time.Sleep(50 * time.Millisecond)
	// Note: Handler is stored internally for later use
}

// TestJobManager_SubmitAndExecute verifies job submission and execution
// Given: A JobManager with registered handler
// When: Job is submitted
// Then: Handler is called with correct args, job status becomes COMPLETED
func TestJobManager_SubmitAndExecute(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionDone := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		if args.To != "user@example.com" {
			t.Errorf("To = %s, want user@example.com", args.To)
		}
		close(executionDone)
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Act
	args := EmailArgs{To: "user@example.com"}
	if err := manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits()); err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for execution
	select {
	case <-executionDone:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Job execution timed out")
	}

	// Assert - Job status is COMPLETED
	time.Sleep(100 * time.Millisecond)
	ctx := context.Background()
	retrievedJob, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if retrievedJob.Status != job.JobStatusCompleted {
		t.Errorf("Status = %s, want COMPLETED", retrievedJob.Status)
	}
}

// TestJobManager_SubmitDelayedJob verifies delayed job execution
// Given: A JobManager with registered handler
// When: Job is submitted with 200ms delay
// Then: Job executes after delay elapses
func TestJobManager_SubmitDelayedJob(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	startTime := time.Now()
	executionDone := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionDone)
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Act - Submit delayed job
	delay := 200 * time.Millisecond
	args := EmailArgs{To: "user@example.com"}
	if err := manager.SubmitDelayedJob(context.Background(), "job1", "email", args, delay, taskrunner.DefaultTaskTraits()); err != nil {
		t.Fatalf("SubmitDelayedJob failed: %v", err)
	}

	// Wait for execution
	select {
	case <-executionDone:
		elapsed := time.Since(startTime)
		if elapsed < delay {
			t.Errorf("elapsed = %v, want >=%v", elapsed, delay)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Delayed job execution timed out")
	}
}

// TestJobManager_CancelJob verifies job cancellation
// Given: A running job with a handler that waits for context cancellation
// When: CancelJob is called
// Then: Handler context is cancelled, job status becomes CANCELED
func TestJobManager_CancelJob(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionStarted := make(chan struct{})
	executionDone := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionStarted)
		<-ctx.Done()
		close(executionDone)
		return ctx.Err()
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Act - Submit job
	args := EmailArgs{To: "user@example.com"}
	_ = manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())

	// Wait for execution to start
	select {
	case <-executionStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("Job execution did not start")
	}

	// Act - Cancel job
	if err := manager.CancelJob(context.Background(), "job1"); err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}

	// Wait for cancellation
	select {
	case <-executionDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Job cancellation timed out")
	}

	// Assert - Job status is CANCELED
	time.Sleep(100 * time.Millisecond)
	ctx := context.Background()
	j, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusCanceled {
		t.Errorf("Status = %s, want CANCELED", j.Status)
	}
}

// TestJobManager_JobFailure verifies failed job handling
// Given: A handler that returns error
// When: Job executes
// Then: Job status becomes FAILED, error message stored
func TestJobManager_JobFailure(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionDone := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionDone)
		return fmt.Errorf("simulated failure")
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Act - Submit job
	args := EmailArgs{To: "user@example.com"}
	_ = manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())

	// Wait for execution
	select {
	case <-executionDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Job execution timed out")
	}

	// Assert - Job status is FAILED
	time.Sleep(100 * time.Millisecond)
	ctx := context.Background()
	j, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusFailed {
		t.Errorf("Status = %s, want FAILED", j.Status)
	}
	if j.Result != "simulated failure" {
		t.Errorf("Result = %s, want 'simulated failure'", j.Result)
	}
}

// TestJobManager_DuplicateSubmission verifies duplicate prevention
// Given: A running job
// When: Second submission with same ID is attempted
// Then: Second submission is rejected with error
func TestJobManager_DuplicateSubmission(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionStarted := make(chan struct{})
	unblock := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionStarted)
		<-unblock
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Act - Submit first job
	args := EmailArgs{To: "user@example.com"}
	if err := manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits()); err != nil {
		t.Fatalf("First SubmitJob failed: %v", err)
	}

	// Wait for execution to start
	select {
	case <-executionStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("Job execution did not start")
	}

	// Act - Try duplicate submission
	err := manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())

	// Assert
	if err == nil {
		t.Error("Duplicate submission succeeded, want error")
	}

	close(unblock)
}

// TestJobManager_GetActiveJobs verifies active job tracking
// Given: A JobManager with blocking handler
// When: Multiple jobs are submitted
// Then: GetActiveJobCount returns correct count
func TestJobManager_GetActiveJobs(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	unblock := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		<-unblock
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Act - Submit 3 jobs
	for i := 0; i < 3; i++ {
		args := EmailArgs{To: "user@example.com"}
		_ = manager.SubmitJob(context.Background(), fmt.Sprintf("job%d", i), "email", args, taskrunner.DefaultTaskTraits())
	}

	time.Sleep(100 * time.Millisecond)

	// Assert
	activeCount := manager.GetActiveJobCount()
	if activeCount != 3 {
		t.Errorf("GetActiveJobCount() = %d, want 3", activeCount)
	}

	activeJobs := manager.GetActiveJobs()
	if len(activeJobs) != 3 {
		t.Errorf("len(GetActiveJobs()) = %d, want 3", len(activeJobs))
	}

	close(unblock)
}

// TestJobManager_Recovery verifies job recovery after restart
// Given: PENDING jobs in store from previous run
// When: Manager starts
// Then: Pending jobs are recovered and executed
func TestJobManager_Recovery(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionCount := atomic.Int32{}
	handler := func(ctx context.Context, args EmailArgs) error {
		executionCount.Add(1)
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Note: This test requires access to internal store
	t.Skip("Skipping recovery test - requires internal store access")
}

// TestJobManager_Shutdown verifies manager shutdown behavior
// Given: A JobManager with active jobs
// When: Shutdown is called
// Then: Shutdown waits for active jobs, new submissions fail
func TestJobManager_Shutdown(t *testing.T) {
	manager, _ := setupJobManager(t)
	// Don't defer cleanup - testing shutdown explicitly

	type EmailArgs struct {
		To string `json:"to"`
	}

	handler := func(ctx context.Context, args EmailArgs) error {
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Act - Submit job
	args := EmailArgs{To: "user@example.com"}
	_ = manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())

	time.Sleep(200 * time.Millisecond)

	// Act - Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := manager.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Assert - Cannot submit new job after shutdown
	if err := manager.SubmitJob(context.Background(), "job2", "email", args, taskrunner.DefaultTaskTraits()); err == nil {
		t.Error("SubmitJob after Shutdown() succeeded, want error")
	}
}

// TestJobManager_HandlerNotFound verifies submission without handler
// Given: A JobManager without registered handlers
// When: Job is submitted
// Then: Submission fails with error
func TestJobManager_HandlerNotFound(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	// Act - Try to submit without registering handler
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())

	// Assert
	if err == nil {
		t.Error("SubmitJob without handler succeeded, want error")
	}
}

// TestJobManager_ConcurrentSubmissions verifies concurrent job submissions
// Given: 20 goroutines submitting jobs
// When: All jobs submit concurrently
// Then: All jobs execute successfully
func TestJobManager_ConcurrentSubmissions(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionCount := atomic.Int32{}
	handler := func(ctx context.Context, args EmailArgs) error {
		executionCount.Add(1)
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Act - Submit 20 jobs concurrently
	const jobCount = 20
	for i := 0; i < jobCount; i++ {
		go func(id int) {
			args := EmailArgs{To: "user@example.com"}
			_ = manager.SubmitJob(context.Background(), fmt.Sprintf("job%d", id), "email", args, taskrunner.DefaultTaskTraits())
		}(i)
	}

	// Wait for all executions
	done := make(chan struct{})
	go func() {
		for {
			if executionCount.Load() == jobCount {
				close(done)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatalf("Timed out: executed %d/%d", executionCount.Load(), jobCount)
	}
}

// =============================================================================
// Retry Behavior Tests
// =============================================================================

// FailingJobStore simulates transient failures
type FailingJobStore struct {
	*job.MemoryJobStore
	failCount   atomic.Int32
	maxFailures int
	recovered   atomic.Bool
}

type DuplicateGetJobStore struct {
	*job.MemoryJobStore
}

func NewDuplicateGetJobStore() *DuplicateGetJobStore {
	return &DuplicateGetJobStore{
		MemoryJobStore: job.NewMemoryJobStore(),
	}
}

func (s *DuplicateGetJobStore) GetJob(ctx context.Context, id string) (*job.JobEntity, error) {
	return &job.JobEntity{
		ID:     id,
		Status: job.JobStatusPending,
	}, nil
}

func (s *DuplicateGetJobStore) CreateJob(ctx context.Context, j *job.JobEntity) error {
	return job.ErrJobAlreadyExists
}

type SaveFailJobStore struct {
	*job.MemoryJobStore
}

func NewSaveFailJobStore() *SaveFailJobStore {
	return &SaveFailJobStore{
		MemoryJobStore: job.NewMemoryJobStore(),
	}
}

func (s *SaveFailJobStore) SaveJob(ctx context.Context, j *job.JobEntity) error {
	return fmt.Errorf("save failed intentionally")
}

func (s *SaveFailJobStore) CreateJob(ctx context.Context, j *job.JobEntity) error {
	return fmt.Errorf("save failed intentionally")
}

type BlockingRecoveryStore struct {
	*job.MemoryJobStore
}

func NewBlockingRecoveryStore() *BlockingRecoveryStore {
	return &BlockingRecoveryStore{
		MemoryJobStore: job.NewMemoryJobStore(),
	}
}

func (s *BlockingRecoveryStore) ListJobs(ctx context.Context, filter job.JobFilter) ([]*job.JobEntity, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// LegacyOnlyJobStore emulates a JobStore implementation without DurableJobStore.CreateJob.
type LegacyOnlyJobStore struct {
	*job.MemoryJobStore
}

func NewLegacyOnlyJobStore() *LegacyOnlyJobStore {
	return &LegacyOnlyJobStore{MemoryJobStore: job.NewMemoryJobStore()}
}

// ListJobsFailStore fails ListJobs during recovery.
type ListJobsFailStore struct {
	*job.MemoryJobStore
}

func NewListJobsFailStore() *ListJobsFailStore {
	return &ListJobsFailStore{MemoryJobStore: job.NewMemoryJobStore()}
}

func (s *ListJobsFailStore) ListJobs(ctx context.Context, filter job.JobFilter) ([]*job.JobEntity, error) {
	return nil, fmt.Errorf("list jobs failed intentionally")
}

// RecoverableFailStore fails GetRecoverableJobs during recovery.
type RecoverableFailStore struct {
	*job.MemoryJobStore
}

func NewRecoverableFailStore() *RecoverableFailStore {
	return &RecoverableFailStore{MemoryJobStore: job.NewMemoryJobStore()}
}

func (s *RecoverableFailStore) GetRecoverableJobs(ctx context.Context) ([]*job.JobEntity, error) {
	return nil, fmt.Errorf("recoverable jobs failed intentionally")
}

// AlwaysFailCreateStore simulates persistent creation failures.
type AlwaysFailCreateStore struct {
	*job.MemoryJobStore
}

func NewAlwaysFailCreateStore() *AlwaysFailCreateStore {
	return &AlwaysFailCreateStore{MemoryJobStore: job.NewMemoryJobStore()}
}

func (s *AlwaysFailCreateStore) CreateJob(ctx context.Context, j *job.JobEntity) error {
	return fmt.Errorf("create failed intentionally")
}

func NewFailingJobStore(maxFailures int) *FailingJobStore {
	return &FailingJobStore{
		MemoryJobStore: job.NewMemoryJobStore(),
		maxFailures:    maxFailures,
	}
}

func (s *FailingJobStore) UpdateStatus(ctx context.Context, id string, status job.JobStatus, result string) error {
	if s.failCount.Load() < int32(s.maxFailures) {
		s.failCount.Add(1)
		return fmt.Errorf("simulated transient failure")
	}
	s.recovered.Store(true)
	return s.MemoryJobStore.UpdateStatus(ctx, id, status, result)
}

// TestJobManager_RetrySuccess verifies retry mechanism with transient failures
// Given: A store that fails twice then succeeds
// When: Job is submitted with retry policy
// Then: Status update succeeds after retries
func TestJobManager_RetrySuccess(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	store := NewFailingJobStore(2)
	serializer := job.NewJSONSerializer()

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	manager.SetRetryPolicy(job.RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffRatio: 1.5,
	})

	logger := job.NewDefaultLogger()
	manager.SetLogger(logger)

	type EmailArgs struct {
		To string
	}

	handlerCalled := atomic.Bool{}
	handler := func(ctx context.Context, args EmailArgs) error {
		handlerCalled.Store(true)
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Act - Submit job
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for execution and status update
	time.Sleep(500 * time.Millisecond)

	// Assert
	if !handlerCalled.Load() {
		t.Error("Handler not called")
	}

	if !store.recovered.Load() {
		t.Error("Store did not recover after retries")
	}

	j, err := manager.GetJob(context.Background(), "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusCompleted {
		t.Errorf("Status = %s, want COMPLETED", j.Status)
	}
}

// TestJobManager_SubmitJobIO_DuplicateInStoreRollsBackActiveJobs verifies duplicate detection rolls back active tracking
// Given: A manager backed by a store that reports duplicate jobs
// When: SubmitJob is called for an existing ID
// Then: No handler runs and active job count is rolled back to zero
func TestJobManager_SubmitJobIO_DuplicateInStoreRollsBackActiveJobs(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	manager := job.NewJobManager(
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		NewDuplicateGetJobStore(),
		job.NewJSONSerializer(),
	)

	type Args struct {
		Name string `json:"name"`
	}

	// Arrange
	handlerRan := atomic.Bool{}
	_ = job.RegisterHandler(manager, context.Background(), "dup", func(ctx context.Context, args Args) error {
		handlerRan.Store(true)
		return nil
	})

	// Act
	err := manager.SubmitJob(context.Background(), "job-dup", "dup", Args{Name: "x"}, taskrunner.DefaultTaskTraits())
	if err == nil {
		t.Fatalf("SubmitJob() = nil, want duplicate error")
	}

	time.Sleep(200 * time.Millisecond)

	// Assert
	if manager.GetActiveJobCount() != 0 {
		t.Fatalf("active jobs = %d, want 0 after duplicate rollback", manager.GetActiveJobCount())
	}
	if handlerRan.Load() {
		t.Fatal("handler should not run when duplicate exists in store")
	}
}

// TestJobManager_SubmitJobIO_SaveFailureRollsBackActiveJobs verifies save failure rolls back active tracking
// Given: A manager backed by a store that fails SaveJob
// When: SubmitJob is called
// Then: No handler runs and active job count is rolled back to zero
func TestJobManager_SubmitJobIO_SaveFailureRollsBackActiveJobs(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	manager := job.NewJobManager(
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		NewSaveFailJobStore(),
		job.NewJSONSerializer(),
	)

	type Args struct {
		Name string `json:"name"`
	}

	// Arrange
	handlerRan := atomic.Bool{}
	_ = job.RegisterHandler(manager, context.Background(), "savefail", func(ctx context.Context, args Args) error {
		handlerRan.Store(true)
		return nil
	})

	// Act
	err := manager.SubmitJob(context.Background(), "job-save-fail", "savefail", Args{Name: "x"}, taskrunner.DefaultTaskTraits())
	if err == nil {
		t.Fatalf("SubmitJob() = nil, want save failure error")
	}

	time.Sleep(200 * time.Millisecond)

	// Assert
	if manager.GetActiveJobCount() != 0 {
		t.Fatalf("active jobs = %d, want 0 after save failure rollback", manager.GetActiveJobCount())
	}
	if handlerRan.Load() {
		t.Fatal("handler should not run when save fails")
	}
}

// TestJobManager_RegisterHandler_AfterShutdown verifies handler registration is blocked after shutdown
// Given: A shut down job manager
// When: RegisterHandler is called
// Then: Registration fails with an error
func TestJobManager_RegisterHandler_AfterShutdown(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	// Act
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := manager.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Act
	err := job.RegisterHandler(manager, context.Background(), "x", func(ctx context.Context, args struct{}) error { return nil })

	// Assert
	if err == nil {
		t.Fatal("RegisterHandler() should fail after shutdown")
	}
}

func TestJobManager_UnregisterHandler(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type NoArgs struct{}
	_ = job.RegisterHandler(manager, context.Background(), "test", func(ctx context.Context, args NoArgs) error {
		return nil
	})

	if err := manager.UnregisterHandler(context.Background(), "test"); err != nil {
		t.Fatalf("UnregisterHandler failed: %v", err)
	}

	err := manager.SubmitJob(context.Background(), "j1", "test", NoArgs{}, taskrunner.DefaultTaskTraits())
	if err == nil {
		t.Fatal("expected error for unregistered handler")
	}
}

// TestJobManager_SubmitDelayedJob_SerializeError verifies delayed submit fails on non-serializable arguments
// Given: A job manager with registered handler
// When: SubmitDelayedJob is called with non-JSON-serializable args
// Then: Submission returns serialization error
func TestJobManager_SubmitDelayedJob_SerializeError(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	// Arrange
	_ = job.RegisterHandler(manager, context.Background(), "x", func(ctx context.Context, args map[string]string) error { return nil })
	time.Sleep(50 * time.Millisecond)

	// Act
	err := manager.SubmitDelayedJob(
		context.Background(),
		"job-bad-args",
		"x",
		make(chan int), // channel is not JSON serializable
		0,
		taskrunner.DefaultTaskTraits(),
	)

	// Assert
	if err == nil {
		t.Fatal("SubmitDelayedJob() should fail on serialization error")
	}
}

// TestJobManager_CancelJob_NotFoundAndClosed verifies cancel behavior for missing and closed states
// Given: A running manager and then a closed manager
// When: CancelJob is called for missing ID and after shutdown
// Then: Both calls return errors
func TestJobManager_CancelJob_NotFoundAndClosed(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	// Act and Assert
	if err := manager.CancelJob(context.Background(), "missing"); err == nil {
		t.Fatal("CancelJob() should fail for missing job")
	}

	// Act
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := manager.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Assert
	if err := manager.CancelJob(context.Background(), "anything"); err == nil {
		t.Fatal("CancelJob() should fail when manager is closed")
	}
}

// TestJobManager_Start_ContextCanceled verifies start fails fast with canceled context
// Given: A manager using a blocking recovery store
// When: Start is called with an already-canceled context
// Then: Start returns context cancellation error
func TestJobManager_Start_ContextCanceled(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	manager := job.NewJobManager(
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		NewBlockingRecoveryStore(),
		job.NewJSONSerializer(),
	)

	// Act
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Assert
	if err := manager.Start(ctx); err == nil {
		t.Fatal("Start() should return context cancellation error")
	}
}

// TestJobManager_Shutdown_AlreadyClosedAndTimeout verifies timeout and repeated shutdown error paths
// Given: A manager with an active blocking job
// When: Shutdown is called with short timeout, then called again after close
// Then: First call times out and second call reports already closed
func TestJobManager_Shutdown_AlreadyClosedAndTimeout(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type Args struct {
		Val string `json:"val"`
	}
	block := make(chan struct{})
	_ = job.RegisterHandler(manager, context.Background(), "block", func(ctx context.Context, args Args) error {
		<-block
		return nil
	})
	time.Sleep(50 * time.Millisecond)

	// Act
	_ = manager.SubmitJob(context.Background(), "job-timeout", "block", Args{Val: "x"}, taskrunner.DefaultTaskTraits())

	// Act
	shortCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := manager.Shutdown(shortCtx)

	// Assert
	if err == nil {
		t.Fatal("Shutdown() should return context timeout when active jobs do not drain")
	}

	// Act
	close(block)
	time.Sleep(100 * time.Millisecond)

	// Manager is now already marked closed due first shutdown call.
	// Act
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	// Assert
	if err2 := manager.Shutdown(ctx2); err2 == nil {
		t.Fatal("second Shutdown() should return already closed error")
	}
}

// TestJobManager_RetryExhausted verifies retry exhaustion handling
// Given: A store that always fails
// When: Job is submitted with limited retries
// Then: Error handler is called after retries exhausted
func TestJobManager_RetryExhausted(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	store := NewFailingJobStore(100)
	serializer := job.NewJSONSerializer()

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	manager.SetRetryPolicy(job.RetryPolicy{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		BackoffRatio: 1.5,
	})

	errorHandlerCalled := atomic.Bool{}
	var lastError atomic.Value
	manager.SetErrorHandler(func(jobID string, operation string, err error) {
		errorHandlerCalled.Store(true)
		lastError.Store(err)
	})

	logger := job.NewDefaultLogger()
	manager.SetLogger(logger)

	type EmailArgs struct {
		To string
	}

	handler := func(ctx context.Context, args EmailArgs) error {
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Act
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Assert
	if !errorHandlerCalled.Load() {
		t.Error("Error handler not called after retry exhaustion")
	}

	if lastError.Load() == nil {
		t.Error("lastError = nil, want non-nil")
	}

	_, _ = manager.GetJob(context.Background(), "job1")
}

// TestJobManager_RetryPolicyConfiguration verifies retry policy configuration
// Given: A JobManager
// When: Custom retry policy is set
// Then: GetRetryPolicy returns configured policy
func TestJobManager_RetryPolicyConfiguration(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	// Assert - Default policy
	defaultPolicy := manager.GetRetryPolicy()
	if defaultPolicy.MaxRetries != 3 {
		t.Errorf("Default MaxRetries = %d, want 3", defaultPolicy.MaxRetries)
	}

	// Act - Set custom policy
	customPolicy := job.RetryPolicy{
		MaxRetries:   5,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		BackoffRatio: 3.0,
	}
	manager.SetRetryPolicy(customPolicy)

	// Assert
	retrieved := manager.GetRetryPolicy()
	if retrieved.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", retrieved.MaxRetries)
	}
	if retrieved.InitialDelay != 200*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 200ms", retrieved.InitialDelay)
	}
}

// TestJobManager_NoRetry verifies disabling retry behavior
// Given: A JobManager with NoRetry policy
// When: Operation fails
// Then: No retries are attempted
func TestJobManager_NoRetry(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	store := NewFailingJobStore(1)
	serializer := job.NewJSONSerializer()

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	manager.SetRetryPolicy(job.NoRetry())

	logger := job.NewDefaultLogger()
	manager.SetLogger(logger)

	type EmailArgs struct {
		To string
	}

	handler := func(ctx context.Context, args EmailArgs) error {
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Act
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Assert - Only one attempt was made
	if store.failCount.Load() != 1 {
		t.Errorf("failCount = %d, want 1 (no retries)", store.failCount.Load())
	}
}

// =============================================================================
// Context Propagation Tests
// =============================================================================

// TestJobManager_ContextPropagation_ParentCancelsJob verifies parent context cancellation
// Given: A job submitted with cancellable parent context
// When: Parent context is cancelled
// Then: Job handler receives cancellation, job becomes CANCELED
func TestJobManager_ContextPropagation_ParentCancelsJob(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionStarted := make(chan struct{})
	executionEnded := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionStarted)
		<-ctx.Done()
		close(executionEnded)
		return ctx.Err()
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	parentCtx, parentCancel := context.WithCancel(context.Background())

	// Act - Submit job with parent context
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(parentCtx, "job1", "email", args, taskrunner.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for job to start
	select {
	case <-executionStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("Job did not start")
	}

	// Act - Cancel parent context
	parentCancel()

	// Wait for cancellation
	select {
	case <-executionEnded:
	case <-time.After(1 * time.Second):
		t.Fatal("Job not cancelled by parent context")
	}

	// Assert
	time.Sleep(100 * time.Millisecond)
	j, err := manager.GetJob(context.Background(), "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusCanceled {
		t.Errorf("Status = %s, want CANCELED", j.Status)
	}
}

// TestJobManager_ContextPropagation_ParentTimeout verifies parent context timeout
// Given: A job submitted with timeout parent context
// When: Parent context times out
// Then: Job handler receives timeout, job becomes CANCELED
func TestJobManager_ContextPropagation_ParentTimeout(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionStarted := make(chan struct{})
	executionEnded := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionStarted)
		select {
		case <-time.After(5 * time.Second):
			return nil
		case <-ctx.Done():
			close(executionEnded)
			return ctx.Err()
		}
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	parentCtx, parentCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer parentCancel()

	// Act - Submit job with timeout
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(parentCtx, "job1", "email", args, taskrunner.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for start
	select {
	case <-executionStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Job did not start")
	}

	// Wait for timeout cancellation
	select {
	case <-executionEnded:
	case <-time.After(1 * time.Second):
		t.Fatal("Job not cancelled by timeout")
	}

	// Assert
	time.Sleep(100 * time.Millisecond)
	j, err := manager.GetJob(context.Background(), "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusCanceled {
		t.Errorf("Status = %s, want CANCELED", j.Status)
	}
}

// =============================================================================
// Duplicate Prevention Tests (Issue #6)
// =============================================================================

// TestJobManager_DuplicatePrevention_Concurrent verifies concurrent duplicate prevention
// Given: 10 goroutines submitting same job ID
// When: All submissions happen concurrently
// Then: Only one submission succeeds, others are rejected
func TestJobManager_DuplicatePrevention_Concurrent(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	unblockHandler := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		<-unblockHandler
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	const concurrentSubmissions = 10
	args := EmailArgs{To: "user@example.com"}
	successCount := atomic.Int32{}
	var firstErr atomic.Value

	var wg sync.WaitGroup
	wg.Add(concurrentSubmissions)

	// Act - Concurrent submissions
	for i := 0; i < concurrentSubmissions; i++ {
		go func() {
			defer wg.Done()
			err := manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())
			if err == nil {
				successCount.Add(1)
			} else if firstErr.Load() == nil {
				firstErr.Store(err)
			}
		}()
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	// Assert - Only one succeeded
	count := successCount.Load()
	if count != 1 {
		t.Errorf("successCount = %d, want 1", count)
	}

	if firstErr.Load() == nil {
		t.Error("firstErr = nil, want error (duplicates rejected)")
	}

	close(unblockHandler)
}

// TestJobManager_DuplicatePrevention_Sequential verifies sequential duplicate prevention
// Given: A running job
// When: Second submission with same ID is attempted
// Then: Second submission is rejected
func TestJobManager_DuplicatePrevention_Sequential(t *testing.T) {
	// Arrange
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	unblock := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		<-unblock
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Act - Submit first job
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("First SubmitJob failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Act - Try duplicate submission
	err = manager.SubmitJob(context.Background(), "job1", "email", args, taskrunner.DefaultTaskTraits())

	// Assert
	if err == nil {
		t.Error("Duplicate submission succeeded, want error")
	}

	close(unblock)
}

// TestJobManager_DuplicatePrevention_DatabaseLevel verifies database-level duplicate check
// Given: A PENDING job in database but not in activeJobs
// When: New job with same ID is submitted
// Then: Submission is rejected, handler is not called
func TestJobManager_DuplicatePrevention_DatabaseLevel(t *testing.T) {
	// Arrange - Create store with existing job
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	existingJob := &job.JobEntity{
		ID:       "job1",
		Type:     "email",
		ArgsData: []byte(`{"to":"user@example.com"}`),
		Status:   job.JobStatusPending,
		Priority: 1,
	}
	ctx := context.Background()
	_ = store.SaveJob(ctx, existingJob)

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	type EmailArgs struct {
		To string `json:"to"`
	}

	handlerCalled := atomic.Bool{}
	handler := func(ctx context.Context, args EmailArgs) error {
		handlerCalled.Store(true)
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Act - Try to submit duplicate
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(ctx, "job1", "email", args, taskrunner.DefaultTaskTraits())
	if err == nil {
		t.Fatal("SubmitJob() = nil, want duplicate error")
	}

	time.Sleep(300 * time.Millisecond)

	// Assert - Handler was not called
	if handlerCalled.Load() {
		t.Error("Handler called for duplicate job, want false")
	}

	// Assert - Original job still PENDING
	j, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusPending {
		t.Errorf("Status = %s, want PENDING (original)", j.Status)
	}

}

// =============================================================================
// JobManager.ListJobs() Tests
// =============================================================================

// TestJobManager_ListJobs_FilterByStatus verifies status filtering
// Given: Jobs with different statuses
// When: ListJobs is called with status filter
// Then: Only matching jobs are returned
func TestJobManager_ListJobs_FilterByStatus(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	ctx := context.Background()

	job1 := &job.JobEntity{ID: "job1", Type: "test", Status: job.JobStatusPending}
	job2 := &job.JobEntity{ID: "job2", Type: "test", Status: job.JobStatusCompleted}
	job3 := &job.JobEntity{ID: "job3", Type: "test", Status: job.JobStatusPending}

	_ = store.SaveJob(ctx, job1)
	_ = store.SaveJob(ctx, job2)
	_ = store.SaveJob(ctx, job3)

	// Act - List pending jobs
	jobs, err := manager.ListJobs(ctx, job.JobFilter{Status: job.JobStatusPending})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}

	// Assert
	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2", len(jobs))
	}

	for _, j := range jobs {
		if j.Status != job.JobStatusPending {
			t.Errorf("Job status = %s, want PENDING", j.Status)
		}
	}
}

// TestJobManager_ListJobs_FilterByType verifies type filtering
// Given: Jobs with different types
// When: ListJobs is called with type filter
// Then: Only matching type jobs are returned
func TestJobManager_ListJobs_FilterByType(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	ctx := context.Background()

	job1 := &job.JobEntity{ID: "job1", Type: "email", Status: job.JobStatusPending}
	job2 := &job.JobEntity{ID: "job2", Type: "sms", Status: job.JobStatusPending}
	job3 := &job.JobEntity{ID: "job3", Type: "email", Status: job.JobStatusPending}

	_ = store.SaveJob(ctx, job1)
	_ = store.SaveJob(ctx, job2)
	_ = store.SaveJob(ctx, job3)

	// Act - List email jobs
	jobs, err := manager.ListJobs(ctx, job.JobFilter{Type: "email"})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}

	// Assert
	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2", len(jobs))
	}

	for _, j := range jobs {
		if j.Type != "email" {
			t.Errorf("Job type = %s, want email", j.Type)
		}
	}
}

// TestJobManager_ListJobs_WithLimitAndOffset verifies pagination
// Given: 10 jobs in store
// When: ListJobs is called with limit and offset
// Then: Correct pagination results are returned
func TestJobManager_ListJobs_WithLimitAndOffset(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	ctx := context.Background()

	// Add 10 jobs
	for i := 0; i < 10; i++ {
		j := &job.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "test",
			Status: job.JobStatusPending,
		}
		_ = store.SaveJob(ctx, j)
	}

	// Act - Test limit
	jobs, err := manager.ListJobs(ctx, job.JobFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListJobs with limit failed: %v", err)
	}

	// Assert
	if len(jobs) != 5 {
		t.Errorf("len(jobs) with limit=5 = %d, want 5", len(jobs))
	}

	// Act - Test offset
	jobs, err = manager.ListJobs(ctx, job.JobFilter{Offset: 5, Limit: 3})
	if err != nil {
		t.Fatalf("ListJobs with offset failed: %v", err)
	}

	// Assert
	if len(jobs) != 3 {
		t.Errorf("len(jobs) with offset=5,limit=3 = %d, want 3", len(jobs))
	}
}

// =============================================================================
// JobManager.Start() Recovery Tests
// =============================================================================

// TestJobManager_Start_RecoveryFromPendingJobs verifies pending job recovery
// Given: A PENDING job in store from previous run
// When: Manager is started
// Then: Pending job is recovered and executed
func TestJobManager_Start_RecoveryFromPendingJobs(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	type EmailArgs struct {
		To string `json:"to"`
	}

	handlerCalled := atomic.Bool{}
	handler := func(ctx context.Context, args EmailArgs) error {
		handlerCalled.Store(true)
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	// Create PENDING job in store (simulating previous run)
	ctx := context.Background()
	existingJob := &job.JobEntity{
		ID:       "recovery-job-1",
		Type:     "email",
		ArgsData: []byte(`{"to":"recovery@example.com"}`),
		Status:   job.JobStatusPending,
		Priority: 1,
	}
	_ = store.SaveJob(ctx, existingJob)

	// Act - Start manager (should trigger recovery)
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Assert - Handler was called
	if !handlerCalled.Load() {
		t.Error("Handler not called for recovered job")
	}

	// Assert - Job status is COMPLETED
	j, err := manager.GetJob(ctx, "recovery-job-1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusCompleted {
		t.Errorf("Status = %s, want COMPLETED", j.Status)
	}

	// Cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = manager.Shutdown(shutdownCtx)
}

// TestJobManager_Start_ConvertsRunningToFailed verifies orphaned RUNNING job handling
// Given: A RUNNING job in store (from crashed process)
// When: Manager is started
// Then: RUNNING job is marked as FAILED
func TestJobManager_Start_ConvertsRunningToFailed(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	// Create RUNNING job (simulating crash)
	ctx := context.Background()
	runningJob := &job.JobEntity{
		ID:       "running-job-1",
		Type:     "email",
		ArgsData: []byte(`{"to":"test@example.com"}`),
		Status:   job.JobStatusRunning,
		Priority: 1,
	}
	_ = store.SaveJob(ctx, runningJob)

	// Act - Start manager
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Assert - Job status is FAILED
	j, err := manager.GetJob(ctx, "running-job-1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusFailed {
		t.Errorf("Status = %s, want FAILED", j.Status)
	}
	if j.Result != "Interrupted by restart" {
		t.Errorf("Result = %s, want 'Interrupted by restart'", j.Result)
	}

	// Cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = manager.Shutdown(shutdownCtx)
}

// TestJobManager_Start_WaitsForRecovery verifies Start() only returns after recovery IO completes.
// Given: A RUNNING job in store from a previous run
// When: Start is called
// Then: Start returns after RUNNING job is converted to FAILED
func TestJobManager_Start_WaitsForRecovery(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	ctx := context.Background()
	runningJob := &job.JobEntity{
		ID:       "running-job-sync-start",
		Type:     "email",
		ArgsData: []byte(`{"to":"test@example.com"}`),
		Status:   job.JobStatusRunning,
		Priority: 1,
	}
	_ = store.SaveJob(ctx, runningJob)

	// Act
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Assert - no extra sleep needed; Start should be synchronous for recovery IO
	j, err := manager.GetJob(ctx, "running-job-sync-start")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusFailed {
		t.Errorf("Status = %s, want FAILED", j.Status)
	}
	if j.Result != "Interrupted by restart" {
		t.Errorf("Result = %s, want 'Interrupted by restart'", j.Result)
	}

	// Cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = manager.Shutdown(shutdownCtx)
}

// TestJobManager_SubmitJob_LegacyStoreFallback verifies durable-ack fallback path for legacy stores
// Given: A JobStore implementation without DurableJobStore.CreateJob
// When: SubmitJob is called
// Then: Job is persisted and executed successfully
func TestJobManager_SubmitJob_LegacyStoreFallback(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	store := NewLegacyOnlyJobStore()
	manager := job.NewJobManager(
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		store,
		job.NewJSONSerializer(),
	)

	type Args struct {
		Val string `json:"val"`
	}

	done := make(chan struct{}, 1)
	_ = job.RegisterHandler(manager, context.Background(), "legacy", func(ctx context.Context, args Args) error {
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	})

	// Act
	if err := manager.SubmitJob(context.Background(), "legacy-1", "legacy", Args{Val: "ok"}, taskrunner.DefaultTaskTraits()); err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Assert
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("handler did not execute")
	}
}

// TestJobManager_SubmitJob_ContextTimeoutDuringRetry verifies SubmitJob returns context error during retry backoff
// Given: A store that always fails CreateJob and a short timeout context
// When: SubmitJob triggers retry with delay
// Then: SubmitJob returns context timeout error
func TestJobManager_SubmitJob_ContextTimeoutDuringRetry(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	store := NewAlwaysFailCreateStore()
	manager := job.NewJobManager(
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		store,
		job.NewJSONSerializer(),
	)

	manager.SetRetryPolicy(job.RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		BackoffRatio: 1.0,
	})

	type Args struct {
		Val string `json:"val"`
	}
	_ = job.RegisterHandler(manager, context.Background(), "retry-timeout", func(ctx context.Context, args Args) error { return nil })

	// Act
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := manager.SubmitJob(ctx, "retry-timeout-1", "retry-timeout", Args{Val: "x"}, taskrunner.DefaultTaskTraits())

	// Assert
	if err == nil {
		t.Fatal("SubmitJob() = nil, want context timeout error")
	}
}

// TestJobManager_Start_ListJobsError verifies Start propagates ListJobs errors
// Given: A recovery store that fails ListJobs
// When: Start is called
// Then: Start returns error
func TestJobManager_Start_ListJobsError(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	manager := job.NewJobManager(
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		NewListJobsFailStore(),
		job.NewJSONSerializer(),
	)

	// Act
	err := manager.Start(context.Background())

	// Assert
	if err == nil {
		t.Fatal("Start() = nil, want list jobs error")
	}
}

// TestJobManager_Start_GetRecoverableJobsError verifies Start propagates GetRecoverableJobs errors
// Given: A recovery store that fails GetRecoverableJobs
// When: Start is called
// Then: Start returns error
func TestJobManager_Start_GetRecoverableJobsError(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	manager := job.NewJobManager(
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		NewRecoverableFailStore(),
		job.NewJSONSerializer(),
	)

	// Act
	err := manager.Start(context.Background())

	// Assert
	if err == nil {
		t.Fatal("Start() = nil, want recoverable jobs error")
	}
}

// TestJobManager_Start_MissingHandlerMarksJobFailed verifies missing handler during recovery marks job as FAILED
// Given: A recoverable PENDING job with unregistered type
// When: Start performs recovery
// Then: Job is marked FAILED with missing handler message
func TestJobManager_Start_MissingHandlerMarksJobFailed(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 2)
	pool.Start(context.Background())
	defer pool.Stop()

	store := job.NewMemoryJobStore()
	manager := job.NewJobManager(
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		taskrunner.NewSequencedTaskRunner(pool),
		store,
		job.NewJSONSerializer(),
	)

	ctx := context.Background()
	_ = store.SaveJob(ctx, &job.JobEntity{
		ID:       "missing-handler-job",
		Type:     "unknown-type",
		ArgsData: []byte(`{}`),
		Status:   job.JobStatusPending,
		Priority: 1,
	})

	// Act
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Assert
	j, err := manager.GetJob(ctx, "missing-handler-job")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusFailed {
		t.Fatalf("Status = %s, want FAILED", j.Status)
	}
	if j.Result != "Missing handler during recovery" {
		t.Fatalf("Result = %q, want %q", j.Result, "Missing handler during recovery")
	}
}

// =============================================================================
// Finalize vs Shutdown Race Condition
// =============================================================================

// slowUpdateJobStore wraps MemoryJobStore and makes UpdateStatus block until
// a signal is received, simulating slow IO that exposes the finalize/shutdown race.
type slowUpdateJobStore struct {
	*job.MemoryJobStore
	updateCalled atomic.Bool
	unblock      chan struct{}
}

func newSlowUpdateJobStore() *slowUpdateJobStore {
	return &slowUpdateJobStore{
		MemoryJobStore: job.NewMemoryJobStore(),
		unblock:        make(chan struct{}),
	}
}

func (s *slowUpdateJobStore) UpdateStatus(ctx context.Context, id string, status job.JobStatus, result string) error {
	s.updateCalled.Store(true)
	<-s.unblock
	return s.MemoryJobStore.UpdateStatus(ctx, id, status, result)
}

// TestJobManager_FinalizeStatusPersistedAfterShutdown verifies that a job's final
// status update is not lost when Shutdown races with finalizeJobControl.
// Given: A job with a slow UpdateStatus store
// When: Job completes and Shutdown is called immediately
// Then: The COMPLETED status is persisted to the store before Shutdown returns
func TestJobManager_FinalizeStatusPersistedAfterShutdown(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	store := newSlowUpdateJobStore()
	serializer := job.NewJSONSerializer()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	type Args struct {
		Val string `json:"val"`
	}

	handlerDone := make(chan struct{})
	_ = job.RegisterHandler(manager, context.Background(), "finalize-test", func(ctx context.Context, args Args) error {
		close(handlerDone)
		return nil
	})

	// Act - Submit job
	err := manager.SubmitJob(context.Background(), "finalize-job", "finalize-test", Args{Val: "ok"}, taskrunner.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for handler to complete (so finalizeJobControl runs)
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not complete")
	}

	// Wait for the slow UpdateStatus to be called
	deadline := time.After(2 * time.Second)
	for {
		if store.updateCalled.Load() {
			break
		}
		select {
		case <-time.After(5 * time.Millisecond):
		case <-deadline:
			t.Fatal("UpdateStatus was never called")
		}
	}

	// Now call Shutdown — this must wait for the pending status update
	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownDone <- manager.Shutdown(ctx)
	}()

	// Assert - Shutdown must NOT return before status is persisted
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned before status was persisted: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	// Unblock the slow UpdateStatus
	close(store.unblock)

	// Assert - Shutdown completes and status is persisted
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timed out")
	}

	j, err := store.GetJob(context.Background(), "finalize-job")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != job.JobStatusCompleted {
		t.Errorf("Status = %s, want COMPLETED (status update was lost)", j.Status)
	}
}

// =============================================================================
// Context Cancel During IO Submission Tests
// =============================================================================

// blockingCreateStore is a DurableJobStore whose CreateJob blocks until signaled.
type blockingCreateStore struct {
	*job.MemoryJobStore
	allowCreate chan struct{}
	createDone  chan struct{}
	updateCount atomic.Int32
}

func newBlockingCreateStore() *blockingCreateStore {
	return &blockingCreateStore{
		MemoryJobStore: job.NewMemoryJobStore(),
		allowCreate:    make(chan struct{}),
		createDone:     make(chan struct{}),
	}
}

func (s *blockingCreateStore) CreateJob(ctx context.Context, j *job.JobEntity) error {
	<-s.allowCreate
	err := s.MemoryJobStore.CreateJob(ctx, j)
	close(s.createDone)
	return err
}

func (s *blockingCreateStore) UpdateStatus(ctx context.Context, id string, status job.JobStatus, msg string) error {
	s.updateCount.Add(1)
	return s.MemoryJobStore.UpdateStatus(ctx, id, status, msg)
}

// TestJobManager_SubmitJob_ContextCanceledDuringIO_PreventsExecution verifies that
// when the parent context is canceled while IO persistence is in progress,
// the job handler is NOT called and activeJobs is cleaned up.
//
// Given: A JobManager with a DurableJobStore that blocks during CreateJob
// When: Parent context is canceled while CreateJob is blocked
// Then: SubmitJob returns an error, handler is never called, activeJobs is cleaned up
func TestJobManager_SubmitJob_ContextCanceledDuringIO_PreventsExecution(t *testing.T) {
	// Arrange
	store := newBlockingCreateStore()
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)
	serializer := job.NewJSONSerializer()

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = manager.Shutdown(ctx)
		pool.Stop()
	}()

	type EmailArgs struct {
		To string `json:"to"`
	}

	handlerCalled := atomic.Bool{}
	handler := func(ctx context.Context, args EmailArgs) error {
		handlerCalled.Store(true)
		return nil
	}

	_ = job.RegisterHandler(manager, context.Background(), "email", handler)

	parentCtx, parentCancel := context.WithCancel(context.Background())

	submitDone := make(chan error, 1)
	go func() {
		args := EmailArgs{To: "user@example.com"}
		submitDone <- manager.SubmitJob(parentCtx, "cancel-during-io", "email", args, taskrunner.DefaultTaskTraits())
	}()

	// Wait for CreateJob to be called (blocked)
	select {
	case <-store.createDone:
		t.Fatal("CreateJob completed before we canceled context (test timing issue)")
	case <-time.After(200 * time.Millisecond):
	}

	// Act - Cancel parent context while IO is in progress
	parentCancel()

	// Allow CreateJob to proceed so the ioRunner task can complete
	close(store.allowCreate)

	// Assert - SubmitJob returns context error
	select {
	case err := <-submitDone:
		if err == nil {
			t.Fatal("SubmitJob should return error when context is canceled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubmitJob did not return")
	}

	// Assert - Handler was never called
	time.Sleep(200 * time.Millisecond)
	if handlerCalled.Load() {
		t.Error("Handler should not be called when context is canceled during IO")
	}

	// Assert - activeJobs does not contain the job
	if manager.GetActiveJobCount() != 0 {
		t.Errorf("active jobs = %d, want 0 after context cancel during IO", manager.GetActiveJobCount())
	}

	// Assert - no execution was scheduled (no status updates beyond CreateJob)
	// Wait for any async operations to settle
	time.Sleep(500 * time.Millisecond)
	count := store.updateCount.Load()
	if count != 0 {
		t.Errorf("UpdateStatus called %d times, want 0 (scheduleExecution should not be reached)", count)
	}
}

func TestJobManager_SetShutdownRunners_SkipsRunnerShutdown(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	// Act
	manager.SetShutdownRunners(false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := manager.Shutdown(ctx)

	// Assert
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	if controlRunner.IsClosed() {
		t.Error("controlRunner should not be closed when SetShutdownRunners(false)")
	}
	if ioRunner.IsClosed() {
		t.Error("ioRunner should not be closed when SetShutdownRunners(false)")
	}
	if executionRunner.IsClosed() {
		t.Error("executionRunner should not be closed when SetShutdownRunners(false)")
	}

	controlRunner.Shutdown()
	ioRunner.Shutdown()
	executionRunner.Shutdown()
}

func TestJobManager_SetShutdownRunners_DefaultShutsDownRunners(t *testing.T) {
	// Arrange
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	controlRunner := taskrunner.NewSequencedTaskRunner(pool)
	ioRunner := taskrunner.NewSequencedTaskRunner(pool)
	executionRunner := taskrunner.NewSequencedTaskRunner(pool)

	store := job.NewMemoryJobStore()
	serializer := job.NewJSONSerializer()

	manager := job.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	// Act - default behavior (no SetShutdownRunners call)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := manager.Shutdown(ctx)

	// Assert
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	if !controlRunner.IsClosed() {
		t.Error("controlRunner should be closed by default")
	}
	if !ioRunner.IsClosed() {
		t.Error("ioRunner should be closed by default")
	}
	if !executionRunner.IsClosed() {
		t.Error("executionRunner should be closed by default")
	}
}
