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
