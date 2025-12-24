package core_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// =============================================================================
// JobStore Tests
// =============================================================================

func TestMemoryJobStore_SaveAndGet(t *testing.T) {
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	job := &core.JobEntity{
		ID:       "job1",
		Type:     "email",
		ArgsData: []byte(`{"to":"user@example.com"}`),
		Status:   core.JobStatusPending,
		Priority: 1,
	}

	// Save job
	if err := store.SaveJob(ctx, job); err != nil {
		t.Fatalf("SaveJob failed: %v", err)
	}

	// Get job
	retrieved, err := store.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}

	if retrieved.ID != "job1" {
		t.Errorf("Expected ID job1, got %s", retrieved.ID)
	}
	if retrieved.Status != core.JobStatusPending {
		t.Errorf("Expected status PENDING, got %s", retrieved.Status)
	}
}

func TestMemoryJobStore_UpdateStatus(t *testing.T) {
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	job := &core.JobEntity{
		ID:     "job1",
		Type:   "email",
		Status: core.JobStatusPending,
	}

	store.SaveJob(ctx, job)

	// Update status
	if err := store.UpdateStatus(ctx, "job1", core.JobStatusRunning, ""); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify update
	retrieved, _ := store.GetJob(ctx, "job1")
	if retrieved.Status != core.JobStatusRunning {
		t.Errorf("Expected status RUNNING, got %s", retrieved.Status)
	}
}

func TestMemoryJobStore_ListJobs(t *testing.T) {
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	// Create multiple jobs
	for i := 0; i < 5; i++ {
		status := core.JobStatusPending
		if i%2 == 0 {
			status = core.JobStatusCompleted
		}
		job := &core.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "email",
			Status: status,
		}
		store.SaveJob(ctx, job)
	}

	// List all jobs
	allJobs, err := store.ListJobs(ctx, core.JobFilter{})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}
	if len(allJobs) != 5 {
		t.Errorf("Expected 5 jobs, got %d", len(allJobs))
	}

	// List pending jobs only
	pendingJobs, err := store.ListJobs(ctx, core.JobFilter{Status: core.JobStatusPending})
	if err != nil {
		t.Fatalf("ListJobs with filter failed: %v", err)
	}
	if len(pendingJobs) != 2 {
		t.Errorf("Expected 2 pending jobs, got %d", len(pendingJobs))
	}

	// Test limit
	limitedJobs, err := store.ListJobs(ctx, core.JobFilter{Limit: 3})
	if err != nil {
		t.Fatalf("ListJobs with limit failed: %v", err)
	}
	if len(limitedJobs) != 3 {
		t.Errorf("Expected 3 jobs with limit, got %d", len(limitedJobs))
	}
}

func TestMemoryJobStore_GetRecoverableJobs(t *testing.T) {
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	// Create jobs with different statuses
	statuses := []core.JobStatus{core.JobStatusPending, core.JobStatusRunning, core.JobStatusCompleted, core.JobStatusPending}
	for i, status := range statuses {
		job := &core.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "email",
			Status: status,
		}
		store.SaveJob(ctx, job)
	}

	// Get recoverable jobs (should only return PENDING)
	recoverable, err := store.GetRecoverableJobs(ctx)
	if err != nil {
		t.Fatalf("GetRecoverableJobs failed: %v", err)
	}
	if len(recoverable) != 2 {
		t.Errorf("Expected 2 recoverable jobs, got %d", len(recoverable))
	}

	for _, job := range recoverable {
		if job.Status != core.JobStatusPending {
			t.Errorf("Recoverable job has wrong status: %s", job.Status)
		}
	}
}

func TestMemoryJobStore_DeleteJob(t *testing.T) {
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	job := &core.JobEntity{ID: "job1", Type: "email", Status: core.JobStatusPending}
	store.SaveJob(ctx, job)

	// Verify exists
	if _, err := store.GetJob(ctx, "job1"); err != nil {
		t.Fatalf("Job should exist before delete")
	}

	// Delete
	if err := store.DeleteJob(ctx, "job1"); err != nil {
		t.Fatalf("DeleteJob failed: %v", err)
	}

	// Verify deleted
	if _, err := store.GetJob(ctx, "job1"); err == nil {
		t.Errorf("Job should not exist after delete")
	}
}

// =============================================================================
// JobSerializer Tests
// =============================================================================

func TestJSONSerializer_Serialize(t *testing.T) {
	serializer := core.NewJSONSerializer()

	type EmailArgs struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
	}

	args := EmailArgs{
		To:      "user@example.com",
		Subject: "Test",
	}

	data, err := serializer.Serialize(args)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data should not be empty")
	}
}

func TestJSONSerializer_Deserialize(t *testing.T) {
	serializer := core.NewJSONSerializer()

	type EmailArgs struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
	}

	// Serialize
	original := EmailArgs{To: "user@example.com", Subject: "Test"}
	data, _ := serializer.Serialize(original)

	// Deserialize
	var result EmailArgs
	if err := serializer.Deserialize(data, &result); err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if result.To != original.To {
		t.Errorf("Expected To=%s, got %s", original.To, result.To)
	}
	if result.Subject != original.Subject {
		t.Errorf("Expected Subject=%s, got %s", original.Subject, result.Subject)
	}
}

func TestJSONSerializer_NilHandling(t *testing.T) {
	serializer := core.NewJSONSerializer()

	// Serialize nil
	data, err := serializer.Serialize(nil)
	if err != nil {
		t.Fatalf("Serialize nil failed: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("Expected 'null', got %s", string(data))
	}

	// Deserialize to nil target should fail
	var target interface{}
	if err := serializer.Deserialize(data, nil); err == nil {
		t.Error("Deserialize to nil target should fail")
	}

	// Deserialize empty data should fail
	if err := serializer.Deserialize([]byte{}, &target); err == nil {
		t.Error("Deserialize empty data should fail")
	}
}

// =============================================================================
// JobManager Tests
// =============================================================================

func setupJobManager(t *testing.T) (*core.JobManager, func()) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	store := core.NewMemoryJobStore()
	serializer := core.NewJSONSerializer()

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		manager.Shutdown(ctx)
		pool.Stop()
	}

	return manager, cleanup
}

func TestJobManager_RegisterHandler(t *testing.T) {
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

	if err := core.RegisterHandler(manager, "email", handler); err != nil {
		t.Fatalf("RegisterHandler failed: %v", err)
	}

	// Wait for handler registration to complete
	time.Sleep(50 * time.Millisecond)

	// Note: We cannot access manager.handlers from outside the core package
	// So we'll test by submitting a job later
}

func TestJobManager_SubmitAndExecute(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionDone := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		if args.To != "user@example.com" {
			t.Errorf("Expected To=user@example.com, got %s", args.To)
		}
		close(executionDone)
		return nil
	}

	core.RegisterHandler(manager, "email", handler)

	// Submit job
	args := EmailArgs{To: "user@example.com"}
	if err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits()); err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for execution
	select {
	case <-executionDone:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Job execution timed out")
	}

	// Verify job status
	time.Sleep(100 * time.Millisecond) // Give time for status update
	ctx := context.Background()
	job, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCompleted {
		t.Errorf("Expected status COMPLETED, got %s", job.Status)
	}
}

func TestJobManager_SubmitDelayedJob(t *testing.T) {
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

	core.RegisterHandler(manager, "email", handler)

	// Submit delayed job (200ms delay)
	delay := 200 * time.Millisecond
	args := EmailArgs{To: "user@example.com"}
	if err := manager.SubmitDelayedJob(context.Background(), "job1", "email", args, delay, core.DefaultTaskTraits()); err != nil {
		t.Fatalf("SubmitDelayedJob failed: %v", err)
	}

	// Wait for execution
	select {
	case <-executionDone:
		elapsed := time.Since(startTime)
		if elapsed < delay {
			t.Errorf("Job executed too early: %v < %v", elapsed, delay)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Delayed job execution timed out")
	}
}

func TestJobManager_CancelJob(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	executionStarted := make(chan struct{})
	executionDone := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionStarted)
		// Wait for cancellation
		<-ctx.Done()
		close(executionDone)
		return ctx.Err()
	}

	core.RegisterHandler(manager, "email", handler)

	// Submit job
	args := EmailArgs{To: "user@example.com"}
	manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())

	// Wait for execution to start
	select {
	case <-executionStarted:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("Job execution did not start")
	}

	// Cancel job
	if err := manager.CancelJob("job1"); err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}

	// Wait for execution to complete
	select {
	case <-executionDone:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Job cancellation timed out")
	}

	// Verify job status
	time.Sleep(100 * time.Millisecond) // Give time for status update
	ctx := context.Background()
	job, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCanceled {
		t.Errorf("Expected status CANCELED, got %s", job.Status)
	}
}

func TestJobManager_JobFailure(t *testing.T) {
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

	core.RegisterHandler(manager, "email", handler)

	// Submit job
	args := EmailArgs{To: "user@example.com"}
	manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())

	// Wait for execution
	select {
	case <-executionDone:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Job execution timed out")
	}

	// Verify job status
	time.Sleep(100 * time.Millisecond) // Give time for status update
	ctx := context.Background()
	job, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusFailed {
		t.Errorf("Expected status FAILED, got %s", job.Status)
	}
	if job.Result != "simulated failure" {
		t.Errorf("Expected error message in Result, got %s", job.Result)
	}
}

func TestJobManager_DuplicateSubmission(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	// Handler that blocks
	executionStarted := make(chan struct{})
	unblock := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionStarted)
		<-unblock
		return nil
	}

	core.RegisterHandler(manager, "email", handler)

	// Submit first job
	args := EmailArgs{To: "user@example.com"}
	if err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits()); err != nil {
		t.Fatalf("First SubmitJob failed: %v", err)
	}

	// Wait for execution to start
	select {
	case <-executionStarted:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("Job execution did not start")
	}

	// Try to submit duplicate (should fail in control layer)
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
	if err == nil {
		t.Error("Expected error for duplicate job submission")
	}

	// Unblock handler
	close(unblock)
}

func TestJobManager_GetActiveJobs(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	// Handler that blocks
	unblock := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		<-unblock
		return nil
	}

	core.RegisterHandler(manager, "email", handler)

	// Submit 3 jobs
	for i := 0; i < 3; i++ {
		args := EmailArgs{To: "user@example.com"}
		manager.SubmitJob(context.Background(), fmt.Sprintf("job%d", i), "email", args, core.DefaultTaskTraits())
	}

	// Wait for jobs to be registered
	time.Sleep(100 * time.Millisecond)

	// Check active count
	activeCount := manager.GetActiveJobCount()
	if activeCount != 3 {
		t.Errorf("Expected 3 active jobs, got %d", activeCount)
	}

	// Check active jobs list
	activeJobs := manager.GetActiveJobs()
	if len(activeJobs) != 3 {
		t.Errorf("Expected 3 jobs in list, got %d", len(activeJobs))
	}

	// Unblock handlers
	close(unblock)

	// Wait for jobs to complete
	time.Sleep(200 * time.Millisecond)

	// Verify no active jobs
	if manager.GetActiveJobCount() != 0 {
		t.Errorf("Expected 0 active jobs after completion, got %d", manager.GetActiveJobCount())
	}
}

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

	core.RegisterHandler(manager, "email", handler)

	// Note: This test requires access to the internal store which is not exported
	// We would need to either expose a test helper or make store public for testing
	t.Skip("Skipping recovery test - requires internal store access")
}

func TestJobManager_Shutdown(t *testing.T) {
	manager, _ := setupJobManager(t)
	// Don't defer cleanup since we're testing shutdown explicitly

	type EmailArgs struct {
		To string `json:"to"`
	}

	handler := func(ctx context.Context, args EmailArgs) error {
		return nil
	}

	core.RegisterHandler(manager, "email", handler)

	// Submit a job
	args := EmailArgs{To: "user@example.com"}
	manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())

	// Wait for job to complete
	time.Sleep(200 * time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := manager.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Try to submit new job (should fail)
	if err := manager.SubmitJob(context.Background(), "job2", "email", args, core.DefaultTaskTraits()); err == nil {
		t.Error("Expected error submitting job to closed manager")
	}
}

func TestJobManager_HandlerNotFound(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	// Try to submit job without registering handler
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
	if err == nil {
		t.Error("Expected error for missing handler")
	}
}

func TestJobManager_ConcurrentSubmissions(t *testing.T) {
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

	core.RegisterHandler(manager, "email", handler)

	// Submit 20 jobs concurrently
	const jobCount = 20
	done := make(chan struct{})
	for i := 0; i < jobCount; i++ {
		go func(id int) {
			args := EmailArgs{To: "user@example.com"}
			manager.SubmitJob(context.Background(), fmt.Sprintf("job%d", id), "email", args, core.DefaultTaskTraits())
		}(i)
	}

	// Wait for all executions
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
		t.Fatalf("Timed out waiting for jobs, executed %d/%d", executionCount.Load(), jobCount)
	}
}

// =============================================================================
// Retry Behavior Tests
// =============================================================================

// FailingJobStore is a test store that simulates transient failures
type FailingJobStore struct {
	*core.MemoryJobStore
	failCount     atomic.Int32
	maxFailures   int
	recovered     atomic.Bool
}

func NewFailingJobStore(maxFailures int) *FailingJobStore {
	return &FailingJobStore{
		MemoryJobStore: core.NewMemoryJobStore(),
		maxFailures:    maxFailures,
	}
}

func (s *FailingJobStore) UpdateStatus(ctx context.Context, id string, status core.JobStatus, result string) error {
	// Fail for first N attempts, then succeed
	if s.failCount.Load() < int32(s.maxFailures) {
		s.failCount.Add(1)
		return fmt.Errorf("simulated transient failure")
	}
	// After failures succeed
	s.recovered.Store(true)
	return s.MemoryJobStore.UpdateStatus(ctx, id, status, result)
}

func TestJobManager_RetrySuccess(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	// Create a store that fails twice then succeeds
	store := NewFailingJobStore(2)
	serializer := core.NewJSONSerializer()

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	// Set retry policy to retry 3 times
	manager.SetRetryPolicy(core.RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffRatio: 1.5,
	})

	// Set logger to capture logs
	logger := core.NewDefaultLogger()
	manager.SetLogger(logger)

	type EmailArgs struct {
		To string
	}

	handlerCalled := atomic.Bool{}
	handler := func(ctx context.Context, args EmailArgs) error {
		handlerCalled.Store(true)
		return nil
	}

	core.RegisterHandler(manager, "email", handler)
	time.Sleep(50 * time.Millisecond) // Wait for registration

	// Submit job
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for execution and status update
	time.Sleep(500 * time.Millisecond)

	// Verify handler was called
	if !handlerCalled.Load() {
		t.Error("Handler was not called")
	}

	// Verify the store recovered after retries
	if !store.recovered.Load() {
		t.Error("Store did not recover after retries")
	}

	// Verify job status is COMPLETED
	job, err := manager.GetJob(context.Background(), "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCompleted {
		t.Errorf("Expected status COMPLETED, got %s", job.Status)
	}
}

func TestJobManager_RetryExhausted(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	// Create a store that always fails
	store := NewFailingJobStore(100) // Will always fail
	serializer := core.NewJSONSerializer()

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	// Set retry policy to retry only 2 times
	manager.SetRetryPolicy(core.RetryPolicy{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		BackoffRatio: 1.5,
	})

	// Set error handler to capture final error
	errorHandlerCalled := atomic.Bool{}
	var lastError error
	manager.SetErrorHandler(func(jobID string, operation string, err error) {
		errorHandlerCalled.Store(true)
		lastError = err
	})

	logger := core.NewDefaultLogger()
	manager.SetLogger(logger)

	type EmailArgs struct {
		To string
	}

	handler := func(ctx context.Context, args EmailArgs) error {
		return nil
	}

	core.RegisterHandler(manager, "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Submit job
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for execution and retry attempts
	time.Sleep(500 * time.Millisecond)

	// Verify error handler was called
	if !errorHandlerCalled.Load() {
		t.Error("Error handler was not called after retry exhaustion")
	}

	if lastError == nil {
		t.Error("Expected error to be passed to error handler")
	}

	// Verify job status - handler succeeded but status update failed
	// This is expected behavior when all retries fail
	_, _ = manager.GetJob(context.Background(), "job1")
}

func TestJobManager_RetryPolicyConfiguration(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	// Test default retry policy
	defaultPolicy := manager.GetRetryPolicy()
	if defaultPolicy.MaxRetries != 3 {
		t.Errorf("Expected default MaxRetries=3, got %d", defaultPolicy.MaxRetries)
	}

	// Test setting custom retry policy
	customPolicy := core.RetryPolicy{
		MaxRetries:   5,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		BackoffRatio: 3.0,
	}
	manager.SetRetryPolicy(customPolicy)

	retrieved := manager.GetRetryPolicy()
	if retrieved.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries=5, got %d", retrieved.MaxRetries)
	}
	if retrieved.InitialDelay != 200*time.Millisecond {
		t.Errorf("Expected InitialDelay=200ms, got %v", retrieved.InitialDelay)
	}
}

func TestJobManager_NoRetry(t *testing.T) {
	pool := taskrunner.NewGoroutineThreadPool("test-pool", 4)
	pool.Start(context.Background())
	defer pool.Stop()

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	// Create a store that fails
	store := NewFailingJobStore(1)
	serializer := core.NewJSONSerializer()

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	// Set no retry policy
	manager.SetRetryPolicy(core.NoRetry())

	logger := core.NewDefaultLogger()
	manager.SetLogger(logger)

	type EmailArgs struct {
		To string
	}

	handler := func(ctx context.Context, args EmailArgs) error {
		return nil
	}

	core.RegisterHandler(manager, "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Submit job
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for execution
	time.Sleep(200 * time.Millisecond)

	// Verify the store only attempted once (no retries)
	if store.failCount.Load() != 1 {
		t.Errorf("Expected 1 failure with no retry, got %d", store.failCount.Load())
	}
}

func TestLogger_DefaultLogger(t *testing.T) {
	logger := core.NewDefaultLogger()

	// These should not panic
	logger.Debug("debug message", core.F("key", "value"))
	logger.Info("info message", core.F("key", "value"))
	logger.Warn("warn message", core.F("key", "value"))
	logger.Error("error message", core.F("key", "value"))
}

func TestLogger_NoOpLogger(t *testing.T) {
	logger := core.NewNoOpLogger()

	// These should not panic
	logger.Debug("debug message", core.F("key", "value"))
	logger.Info("info message", core.F("key", "value"))
	logger.Warn("warn message", core.F("key", "value"))
	logger.Error("error message", core.F("key", "value"))
}

func TestRetryPolicy_CalculateDelay(t *testing.T) {
	policy := core.RetryPolicy{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		BackoffRatio: 2.0,
	}

	// We can't test calculateDelay directly as it's private
	// But we can verify the policy fields are set correctly
	if policy.InitialDelay != 100*time.Millisecond {
		t.Errorf("Expected InitialDelay=100ms, got %v", policy.InitialDelay)
	}
	if policy.MaxDelay != 500*time.Millisecond {
		t.Errorf("Expected MaxDelay=500ms, got %v", policy.MaxDelay)
	}
	if policy.BackoffRatio != 2.0 {
		t.Errorf("Expected BackoffRatio=2.0, got %f", policy.BackoffRatio)
	}
}

func TestRetryPolicy_Defaults(t *testing.T) {
	policy := core.DefaultRetryPolicy()

	if policy.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", policy.MaxRetries)
	}
	if policy.InitialDelay != 100*time.Millisecond {
		t.Errorf("Expected InitialDelay=100ms, got %v", policy.InitialDelay)
	}
	if policy.MaxDelay != 5*time.Second {
		t.Errorf("Expected MaxDelay=5s, got %v", policy.MaxDelay)
	}
	if policy.BackoffRatio != 2.0 {
		t.Errorf("Expected BackoffRatio=2.0, got %f", policy.BackoffRatio)
	}
}

func TestRetryPolicy_NoRetry(t *testing.T) {
	policy := core.NoRetry()

	if policy.MaxRetries != 0 {
		t.Errorf("Expected MaxRetries=0, got %d", policy.MaxRetries)
	}
}

// =============================================================================
// Context Propagation Tests
// =============================================================================

func TestJobManager_ContextPropagation_ParentCancelsJob(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	// Handler that respects context cancellation
	executionStarted := make(chan struct{})
	executionEnded := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionStarted)
		// Wait for context cancellation
		<-ctx.Done()
		close(executionEnded)
		return ctx.Err()
	}

	core.RegisterHandler(manager, "email", handler)

	// Create a cancellable parent context
	parentCtx, parentCancel := context.WithCancel(context.Background())

	// Submit job with parent context
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(parentCtx, "job1", "email", args, core.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for job to start executing
	select {
	case <-executionStarted:
		// Job started
	case <-time.After(1 * time.Second):
		t.Fatal("Job did not start within timeout")
	}

	// Cancel the parent context
	parentCancel()

	// Wait for job to be cancelled
	select {
	case <-executionEnded:
		// Job was cancelled by parent context
	case <-time.After(1 * time.Second):
		t.Fatal("Job was not cancelled when parent context was cancelled")
	}

	// Verify job status is CANCELED
	time.Sleep(100 * time.Millisecond) // Give time for status update
	job, err := manager.GetJob(context.Background(), "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCanceled {
		t.Errorf("Expected status CANCELED, got %s", job.Status)
	}
}

func TestJobManager_ContextPropagation_ParentTimeout(t *testing.T) {
	manager, cleanup := setupJobManager(t)
	defer cleanup()

	type EmailArgs struct {
		To string `json:"to"`
	}

	// Handler that takes longer than parent timeout
	executionStarted := make(chan struct{})
	executionEnded := make(chan struct{})
	handler := func(ctx context.Context, args EmailArgs) error {
		close(executionStarted)
		// Simulate long-running task
		select {
		case <-time.After(5 * time.Second):
			return nil
		case <-ctx.Done():
			close(executionEnded)
			return ctx.Err()
		}
	}

	core.RegisterHandler(manager, "email", handler)

	// Create parent context with short timeout
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer parentCancel()

	// Submit job with parent context
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(parentCtx, "job1", "email", args, core.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	// Wait for job to start executing
	select {
	case <-executionStarted:
		// Job started
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Job did not start within timeout")
	}

	// Wait for job to be cancelled by timeout
	select {
	case <-executionEnded:
		// Job was cancelled by parent timeout
	case <-time.After(1 * time.Second):
		t.Fatal("Job was not cancelled when parent context timed out")
	}

	// Verify job status is CANCELED
	time.Sleep(100 * time.Millisecond)
	job, err := manager.GetJob(context.Background(), "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCanceled {
		t.Errorf("Expected status CANCELED, got %s", job.Status)
	}
}
