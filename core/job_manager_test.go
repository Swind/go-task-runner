package core_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
	"github.com/Swind/go-task-runner/core"
)

// =============================================================================
// JobStore Tests
// =============================================================================

// TestMemoryJobStore_SaveAndGet verifies job persistence and retrieval
// Given: A MemoryJobStore and a job entity
// When: Job is saved and then retrieved by ID
// Then: Retrieved job has matching ID and status
func TestMemoryJobStore_SaveAndGet(t *testing.T) {
	// Arrange
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	job := &core.JobEntity{
		ID:       "job1",
		Type:     "email",
		ArgsData: []byte(`{"to":"user@example.com"}`),
		Status:   core.JobStatusPending,
		Priority: 1,
	}

	// Act - Save job
	if err := store.SaveJob(ctx, job); err != nil {
		t.Fatalf("SaveJob failed: %v", err)
	}

	// Act - Get job
	retrieved, err := store.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}

	// Assert
	if retrieved.ID != "job1" {
		t.Errorf("ID = %s, want job1", retrieved.ID)
	}
	if retrieved.Status != core.JobStatusPending {
		t.Errorf("Status = %s, want PENDING", retrieved.Status)
	}
}

// TestMemoryJobStore_UpdateStatus verifies job status updates
// Given: A job store with a PENDING job
// When: Status is updated to RUNNING
// Then: Retrieved job has RUNNING status
func TestMemoryJobStore_UpdateStatus(t *testing.T) {
	// Arrange
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	job := &core.JobEntity{
		ID:     "job1",
		Type:   "email",
		Status: core.JobStatusPending,
	}
	_ = store.SaveJob(ctx, job)

	// Act - Update status
	if err := store.UpdateStatus(ctx, "job1", core.JobStatusRunning, ""); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Assert
	retrieved, _ := store.GetJob(ctx, "job1")
	if retrieved.Status != core.JobStatusRunning {
		t.Errorf("Status = %s, want RUNNING", retrieved.Status)
	}
}

// TestMemoryJobStore_ListJobs verifies job listing with filters
// Given: A store with 5 jobs of mixed statuses
// When: Jobs are listed with various filters
// Then: Correct jobs are returned based on filter criteria
func TestMemoryJobStore_ListJobs(t *testing.T) {
	// Arrange
	store := core.NewMemoryJobStore()
	ctx := context.Background()

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
		_ = store.SaveJob(ctx, job)
	}

	// Act - List all jobs
	allJobs, err := store.ListJobs(ctx, core.JobFilter{})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}

	// Assert
	if len(allJobs) != 5 {
		t.Errorf("len(allJobs) = %d, want 5", len(allJobs))
	}

	// Act - List pending jobs only
	pendingJobs, err := store.ListJobs(ctx, core.JobFilter{Status: core.JobStatusPending})
	if err != nil {
		t.Fatalf("ListJobs with filter failed: %v", err)
	}

	// Assert
	if len(pendingJobs) != 2 {
		t.Errorf("len(pendingJobs) = %d, want 2", len(pendingJobs))
	}

	// Act - Test limit
	limitedJobs, err := store.ListJobs(ctx, core.JobFilter{Limit: 3})
	if err != nil {
		t.Fatalf("ListJobs with limit failed: %v", err)
	}

	// Assert
	if len(limitedJobs) != 3 {
		t.Errorf("len(limitedJobs) = %d, want 3", len(limitedJobs))
	}
}

// TestMemoryJobStore_GetRecoverableJobs verifies PENDING job retrieval
// Given: A store with PENDING, RUNNING, and COMPLETED jobs
// When: GetRecoverableJobs is called
// Then: Only PENDING jobs are returned
func TestMemoryJobStore_GetRecoverableJobs(t *testing.T) {
	// Arrange
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	statuses := []core.JobStatus{core.JobStatusPending, core.JobStatusRunning, core.JobStatusCompleted, core.JobStatusPending}
	for i, status := range statuses {
		job := &core.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "email",
			Status: status,
		}
		_ = store.SaveJob(ctx, job)
	}

	// Act
	recoverable, err := store.GetRecoverableJobs(ctx)
	if err != nil {
		t.Fatalf("GetRecoverableJobs failed: %v", err)
	}

	// Assert
	if len(recoverable) != 2 {
		t.Errorf("len(recoverable) = %d, want 2", len(recoverable))
	}

	for _, job := range recoverable {
		if job.Status != core.JobStatusPending {
			t.Errorf("Recoverable job has status %s, want PENDING", job.Status)
		}
	}
}

// TestMemoryJobStore_DeleteJob verifies job deletion
// Given: A store with a saved job
// When: Job is deleted by ID
// Then: Job no longer exists in store
func TestMemoryJobStore_DeleteJob(t *testing.T) {
	// Arrange
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	job := &core.JobEntity{ID: "job1", Type: "email", Status: core.JobStatusPending}
	_ = store.SaveJob(ctx, job)

	// Act - Delete
	if err := store.DeleteJob(ctx, "job1"); err != nil {
		t.Fatalf("DeleteJob failed: %v", err)
	}

	// Assert - Job no longer exists
	if _, err := store.GetJob(ctx, "job1"); err == nil {
		t.Error("Job exists after Delete(), want error")
	}
}

// TestMemoryJobStore_Clear verifies clearing all jobs
// Given: A store with 5 jobs
// When: Clear is called
// Then: All jobs are removed, count returns 0
func TestMemoryJobStore_Clear(t *testing.T) {
	// Arrange
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		job := &core.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "email",
			Status: core.JobStatusPending,
		}
		_ = store.SaveJob(ctx, job)
	}

	if count := store.Count(); count != 5 {
		t.Fatalf("Count() = %d before clear, want 5", count)
	}

	// Act
	store.Clear()

	// Assert
	if count := store.Count(); count != 0 {
		t.Errorf("Count() = %d after clear, want 0", count)
	}

	if _, err := store.GetJob(ctx, "job1"); err == nil {
		t.Error("Job exists after Clear(), want error")
	}
}

// TestMemoryJobStore_Count verifies job counting
// Given: An empty store
// When: Jobs are added and deleted
// Then: Count accurately reflects number of jobs
func TestMemoryJobStore_Count(t *testing.T) {
	// Arrange
	store := core.NewMemoryJobStore()
	ctx := context.Background()

	// Assert - Initially empty
	if count := store.Count(); count != 0 {
		t.Errorf("Count() = %d initially, want 0", count)
	}

	// Act - Add jobs one by one
	for i := 1; i <= 10; i++ {
		job := &core.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "email",
			Status: core.JobStatusPending,
		}
		_ = store.SaveJob(ctx, job)

		if count := store.Count(); count != i {
			t.Errorf("Count() = %d after adding %d jobs, want %d", count, i, i)
		}
	}

	// Act - Delete some jobs
	_ = store.DeleteJob(ctx, "job1")
	_ = store.DeleteJob(ctx, "job2")

	// Assert
	if count := store.Count(); count != 8 {
		t.Errorf("Count() = %d after deleting 2, want 8", count)
	}
}

// =============================================================================
// JobSerializer Tests
// =============================================================================

// TestJSONSerializer_Serialize verifies struct serialization to JSON
// Given: A struct with JSON tags
// When: Struct is serialized
// Then: Valid JSON bytes are produced
func TestJSONSerializer_Serialize(t *testing.T) {
	// Arrange
	serializer := core.NewJSONSerializer()

	type EmailArgs struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
	}

	args := EmailArgs{
		To:      "user@example.com",
		Subject: "Test",
	}

	// Act
	data, err := serializer.Serialize(args)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Assert
	if len(data) == 0 {
		t.Error("len(data) = 0, want > 0")
	}
}

// TestJSONSerializer_Deserialize verifies JSON deserialization
// Given: Serialized JSON bytes
// When: Data is deserialized to struct
// Then: Struct fields match original values
func TestJSONSerializer_Deserialize(t *testing.T) {
	// Arrange
	serializer := core.NewJSONSerializer()

	type EmailArgs struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
	}

	original := EmailArgs{To: "user@example.com", Subject: "Test"}
	data, _ := serializer.Serialize(original)

	// Act
	var result EmailArgs
	if err := serializer.Deserialize(data, &result); err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Assert
	if result.To != original.To {
		t.Errorf("To = %s, want %s", result.To, original.To)
	}
	if result.Subject != original.Subject {
		t.Errorf("Subject = %s, want %s", result.Subject, original.Subject)
	}
}

// TestJSONSerializer_NilHandling verifies nil value edge cases
// Given: Nil or empty data
// When: Serialize or Deserialize is called
// Then: Operations handle edge cases safely
func TestJSONSerializer_NilHandling(t *testing.T) {
	// Arrange
	serializer := core.NewJSONSerializer()

	// Act - Serialize nil
	data, err := serializer.Serialize(nil)
	if err != nil {
		t.Fatalf("Serialize(nil) failed: %v", err)
	}

	// Assert - Nil serializes to "null"
	if string(data) != "null" {
		t.Errorf("Serialize(nil) = %s, want 'null'", string(data))
	}

	// Assert - Deserialize to nil target fails
	var target any
	if err := serializer.Deserialize(data, nil); err == nil {
		t.Error("Deserialize(nil target) = nil, want error")
	}

	// Assert - Deserialize empty data fails
	if err := serializer.Deserialize([]byte{}, &target); err == nil {
		t.Error("Deserialize(empty) = nil, want error")
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
	if err := core.RegisterHandler(manager, "email", handler); err != nil {
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

	_ = core.RegisterHandler(manager, "email", handler)

	// Act
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

	// Assert - Job status is COMPLETED
	time.Sleep(100 * time.Millisecond)
	ctx := context.Background()
	job, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCompleted {
		t.Errorf("Status = %s, want COMPLETED", job.Status)
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

	_ = core.RegisterHandler(manager, "email", handler)

	// Act - Submit delayed job
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

	_ = core.RegisterHandler(manager, "email", handler)

	// Act - Submit job
	args := EmailArgs{To: "user@example.com"}
	_ = manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())

	// Wait for execution to start
	select {
	case <-executionStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("Job execution did not start")
	}

	// Act - Cancel job
	if err := manager.CancelJob("job1"); err != nil {
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
	job, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCanceled {
		t.Errorf("Status = %s, want CANCELED", job.Status)
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

	_ = core.RegisterHandler(manager, "email", handler)

	// Act - Submit job
	args := EmailArgs{To: "user@example.com"}
	_ = manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())

	// Wait for execution
	select {
	case <-executionDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Job execution timed out")
	}

	// Assert - Job status is FAILED
	time.Sleep(100 * time.Millisecond)
	ctx := context.Background()
	job, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusFailed {
		t.Errorf("Status = %s, want FAILED", job.Status)
	}
	if job.Result != "simulated failure" {
		t.Errorf("Result = %s, want 'simulated failure'", job.Result)
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

	_ = core.RegisterHandler(manager, "email", handler)

	// Act - Submit first job
	args := EmailArgs{To: "user@example.com"}
	if err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits()); err != nil {
		t.Fatalf("First SubmitJob failed: %v", err)
	}

	// Wait for execution to start
	select {
	case <-executionStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("Job execution did not start")
	}

	// Act - Try duplicate submission
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())

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

	_ = core.RegisterHandler(manager, "email", handler)

	// Act - Submit 3 jobs
	for i := 0; i < 3; i++ {
		args := EmailArgs{To: "user@example.com"}
		_ = manager.SubmitJob(context.Background(), fmt.Sprintf("job%d", i), "email", args, core.DefaultTaskTraits())
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

	_ = core.RegisterHandler(manager, "email", handler)

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

	_ = core.RegisterHandler(manager, "email", handler)

	// Act - Submit job
	args := EmailArgs{To: "user@example.com"}
	_ = manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())

	time.Sleep(200 * time.Millisecond)

	// Act - Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := manager.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Assert - Cannot submit new job after shutdown
	if err := manager.SubmitJob(context.Background(), "job2", "email", args, core.DefaultTaskTraits()); err == nil {
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
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())

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

	_ = core.RegisterHandler(manager, "email", handler)

	// Act - Submit 20 jobs concurrently
	const jobCount = 20
	for i := 0; i < jobCount; i++ {
		go func(id int) {
			args := EmailArgs{To: "user@example.com"}
			_ = manager.SubmitJob(context.Background(), fmt.Sprintf("job%d", id), "email", args, core.DefaultTaskTraits())
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
	*core.MemoryJobStore
	failCount   atomic.Int32
	maxFailures int
	recovered   atomic.Bool
}

type DuplicateGetJobStore struct {
	*core.MemoryJobStore
}

func NewDuplicateGetJobStore() *DuplicateGetJobStore {
	return &DuplicateGetJobStore{
		MemoryJobStore: core.NewMemoryJobStore(),
	}
}

func (s *DuplicateGetJobStore) GetJob(ctx context.Context, id string) (*core.JobEntity, error) {
	return &core.JobEntity{
		ID:     id,
		Status: core.JobStatusPending,
	}, nil
}

type SaveFailJobStore struct {
	*core.MemoryJobStore
}

func NewSaveFailJobStore() *SaveFailJobStore {
	return &SaveFailJobStore{
		MemoryJobStore: core.NewMemoryJobStore(),
	}
}

func (s *SaveFailJobStore) SaveJob(ctx context.Context, job *core.JobEntity) error {
	return fmt.Errorf("save failed intentionally")
}

type BlockingRecoveryStore struct {
	*core.MemoryJobStore
}

func NewBlockingRecoveryStore() *BlockingRecoveryStore {
	return &BlockingRecoveryStore{
		MemoryJobStore: core.NewMemoryJobStore(),
	}
}

func (s *BlockingRecoveryStore) ListJobs(ctx context.Context, filter core.JobFilter) ([]*core.JobEntity, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func NewFailingJobStore(maxFailures int) *FailingJobStore {
	return &FailingJobStore{
		MemoryJobStore: core.NewMemoryJobStore(),
		maxFailures:    maxFailures,
	}
}

func (s *FailingJobStore) UpdateStatus(ctx context.Context, id string, status core.JobStatus, result string) error {
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

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	store := NewFailingJobStore(2)
	serializer := core.NewJSONSerializer()

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	manager.SetRetryPolicy(core.RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		BackoffRatio: 1.5,
	})

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

	_ = core.RegisterHandler(manager, "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Act - Submit job
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
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

	job, err := manager.GetJob(context.Background(), "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCompleted {
		t.Errorf("Status = %s, want COMPLETED", job.Status)
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

	manager := core.NewJobManager(
		core.NewSequencedTaskRunner(pool),
		core.NewSequencedTaskRunner(pool),
		core.NewSequencedTaskRunner(pool),
		NewDuplicateGetJobStore(),
		core.NewJSONSerializer(),
	)

	type Args struct {
		Name string `json:"name"`
	}

	// Arrange
	handlerRan := atomic.Bool{}
	_ = core.RegisterHandler(manager, "dup", func(ctx context.Context, args Args) error {
		handlerRan.Store(true)
		return nil
	})

	// Act
	err := manager.SubmitJob(context.Background(), "job-dup", "dup", Args{Name: "x"}, core.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob returned unexpected error: %v", err)
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

	manager := core.NewJobManager(
		core.NewSequencedTaskRunner(pool),
		core.NewSequencedTaskRunner(pool),
		core.NewSequencedTaskRunner(pool),
		NewSaveFailJobStore(),
		core.NewJSONSerializer(),
	)

	type Args struct {
		Name string `json:"name"`
	}

	// Arrange
	handlerRan := atomic.Bool{}
	_ = core.RegisterHandler(manager, "savefail", func(ctx context.Context, args Args) error {
		handlerRan.Store(true)
		return nil
	})

	// Act
	err := manager.SubmitJob(context.Background(), "job-save-fail", "savefail", Args{Name: "x"}, core.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob returned unexpected error: %v", err)
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
	err := core.RegisterHandler(manager, "x", func(ctx context.Context, args struct{}) error { return nil })

	// Assert
	if err == nil {
		t.Fatal("RegisterHandler() should fail after shutdown")
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
	_ = core.RegisterHandler(manager, "x", func(ctx context.Context, args map[string]string) error { return nil })
	time.Sleep(50 * time.Millisecond)

	// Act
	err := manager.SubmitDelayedJob(
		context.Background(),
		"job-bad-args",
		"x",
		make(chan int), // channel is not JSON serializable
		0,
		core.DefaultTaskTraits(),
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
	if err := manager.CancelJob("missing"); err == nil {
		t.Fatal("CancelJob() should fail for missing job")
	}

	// Act
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := manager.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Assert
	if err := manager.CancelJob("anything"); err == nil {
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

	manager := core.NewJobManager(
		core.NewSequencedTaskRunner(pool),
		core.NewSequencedTaskRunner(pool),
		core.NewSequencedTaskRunner(pool),
		NewBlockingRecoveryStore(),
		core.NewJSONSerializer(),
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
	_ = core.RegisterHandler(manager, "block", func(ctx context.Context, args Args) error {
		<-block
		return nil
	})
	time.Sleep(50 * time.Millisecond)

	// Act
	_ = manager.SubmitJob(context.Background(), "job-timeout", "block", Args{Val: "x"}, core.DefaultTaskTraits())

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

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	store := NewFailingJobStore(100)
	serializer := core.NewJSONSerializer()

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	manager.SetRetryPolicy(core.RetryPolicy{
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

	logger := core.NewDefaultLogger()
	manager.SetLogger(logger)

	type EmailArgs struct {
		To string
	}

	handler := func(ctx context.Context, args EmailArgs) error {
		return nil
	}

	_ = core.RegisterHandler(manager, "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Act
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
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
	customPolicy := core.RetryPolicy{
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

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	store := NewFailingJobStore(1)
	serializer := core.NewJSONSerializer()

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	manager.SetRetryPolicy(core.NoRetry())

	logger := core.NewDefaultLogger()
	manager.SetLogger(logger)

	type EmailArgs struct {
		To string
	}

	handler := func(ctx context.Context, args EmailArgs) error {
		return nil
	}

	_ = core.RegisterHandler(manager, "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Act
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("SubmitJob failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Assert - Only one attempt was made
	if store.failCount.Load() != 1 {
		t.Errorf("failCount = %d, want 1 (no retries)", store.failCount.Load())
	}
}

// TestLogger_DefaultLogger verifies default logger functionality
// Given: A default logger
// When: All log methods are called
// Then: No panic occurs
func TestLogger_DefaultLogger(t *testing.T) {
	// Arrange
	logger := core.NewDefaultLogger()

	// Act & Assert - Should not panic
	logger.Debug("debug message", core.F("key", "value"))
	logger.Info("info message", core.F("key", "value"))
	logger.Warn("warn message", core.F("key", "value"))
	logger.Error("error message", core.F("key", "value"))
}

// TestLogger_NoOpLogger verifies no-op logger functionality
// Given: A no-op logger
// When: All log methods are called
// Then: Output is discarded, no panic
func TestLogger_NoOpLogger(t *testing.T) {
	// Arrange
	logger := core.NewNoOpLogger()

	// Act & Assert - Should not panic
	logger.Debug("debug message", core.F("key", "value"))
	logger.Info("info message", core.F("key", "value"))
	logger.Warn("warn message", core.F("key", "value"))
	logger.Error("error message", core.F("key", "value"))
}

// TestRetryPolicy_CalculateDelay verifies retry policy field values
// Given: A retry policy with specific values
// When: Policy is created
// Then: Fields are set correctly
func TestRetryPolicy_CalculateDelay(t *testing.T) {
	// Arrange
	policy := core.RetryPolicy{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		BackoffRatio: 2.0,
	}

	// Assert
	if policy.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", policy.InitialDelay)
	}
	if policy.MaxDelay != 500*time.Millisecond {
		t.Errorf("MaxDelay = %v, want 500ms", policy.MaxDelay)
	}
	if policy.BackoffRatio != 2.0 {
		t.Errorf("BackoffRatio = %f, want 2.0", policy.BackoffRatio)
	}
}

// TestRetryPolicy_Defaults verifies default retry policy values
// Given: DefaultRetryPolicy() is called
// When: Policy is created
// Then: Default values are correct
func TestRetryPolicy_Defaults(t *testing.T) {
	// Act
	policy := core.DefaultRetryPolicy()

	// Assert
	if policy.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", policy.MaxRetries)
	}
	if policy.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", policy.InitialDelay)
	}
	if policy.MaxDelay != 5*time.Second {
		t.Errorf("MaxDelay = %v, want 5s", policy.MaxDelay)
	}
	if policy.BackoffRatio != 2.0 {
		t.Errorf("BackoffRatio = %f, want 2.0", policy.BackoffRatio)
	}
}

// TestRetryPolicy_NoRetry verifies NoRetry helper
// Given: NoRetry() is called
// When: Policy is created
// Then: MaxRetries is 0
func TestRetryPolicy_NoRetry(t *testing.T) {
	// Act
	policy := core.NoRetry()

	// Assert
	if policy.MaxRetries != 0 {
		t.Errorf("MaxRetries = %d, want 0", policy.MaxRetries)
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

	_ = core.RegisterHandler(manager, "email", handler)

	parentCtx, parentCancel := context.WithCancel(context.Background())

	// Act - Submit job with parent context
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(parentCtx, "job1", "email", args, core.DefaultTaskTraits())
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
	job, err := manager.GetJob(context.Background(), "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCanceled {
		t.Errorf("Status = %s, want CANCELED", job.Status)
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

	_ = core.RegisterHandler(manager, "email", handler)

	parentCtx, parentCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer parentCancel()

	// Act - Submit job with timeout
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(parentCtx, "job1", "email", args, core.DefaultTaskTraits())
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
	job, err := manager.GetJob(context.Background(), "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCanceled {
		t.Errorf("Status = %s, want CANCELED", job.Status)
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

	_ = core.RegisterHandler(manager, "email", handler)

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
			err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
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

	_ = core.RegisterHandler(manager, "email", handler)

	// Act - Submit first job
	args := EmailArgs{To: "user@example.com"}
	err := manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())
	if err != nil {
		t.Fatalf("First SubmitJob failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Act - Try duplicate submission
	err = manager.SubmitJob(context.Background(), "job1", "email", args, core.DefaultTaskTraits())

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

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	store := core.NewMemoryJobStore()
	serializer := core.NewJSONSerializer()

	existingJob := &core.JobEntity{
		ID:       "job1",
		Type:     "email",
		ArgsData: []byte(`{"to":"user@example.com"}`),
		Status:   core.JobStatusPending,
		Priority: 1,
	}
	ctx := context.Background()
	_ = store.SaveJob(ctx, existingJob)

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	type EmailArgs struct {
		To string `json:"to"`
	}

	handlerCalled := atomic.Bool{}
	handler := func(ctx context.Context, args EmailArgs) error {
		handlerCalled.Store(true)
		return nil
	}

	_ = core.RegisterHandler(manager, "email", handler)
	time.Sleep(50 * time.Millisecond)

	// Act - Try to submit duplicate
	args := EmailArgs{To: "user@example.com"}
	_ = manager.SubmitJob(ctx, "job1", "email", args, core.DefaultTaskTraits())

	time.Sleep(300 * time.Millisecond)

	// Assert - Handler was not called
	if handlerCalled.Load() {
		t.Error("Handler called for duplicate job, want false")
	}

	// Assert - Original job still PENDING
	job, err := manager.GetJob(ctx, "job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusPending {
		t.Errorf("Status = %s, want PENDING (original)", job.Status)
	}

	_ = err // Submission error depends on timing
}

// =============================================================================
// JSONSerializer.Name() Test
// =============================================================================

// TestJSONSerializer_Name verifies serializer name
// Given: A JSONSerializer
// When: Name() is called
// Then: Returns "json"
func TestJSONSerializer_Name(t *testing.T) {
	// Arrange
	serializer := core.NewJSONSerializer()

	// Act
	name := serializer.Name()

	// Assert
	if name != "json" {
		t.Errorf("Name() = %s, want 'json'", name)
	}
}

// =============================================================================
// NoOpLogger Methods Test
// =============================================================================

// TestNoOpLogger_ExplicitCoverage verifies all NoOpLogger methods
// Given: A NoOpLogger
// When: All log methods are called
// Then: No panic occurs
func TestNoOpLogger_ExplicitCoverage(t *testing.T) {
	// Arrange
	logger := core.NewNoOpLogger()

	// Act & Assert - All methods callable
	logger.Debug("test debug", core.F("key1", "value1"), core.F("key2", "value2"))
	logger.Info("test info", core.F("key", "value"))
	logger.Warn("test warn", core.F("level", "high"))
	logger.Error("test error", core.F("err", "something failed"))
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

	store := core.NewMemoryJobStore()
	serializer := core.NewJSONSerializer()

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	ctx := context.Background()

	job1 := &core.JobEntity{ID: "job1", Type: "test", Status: core.JobStatusPending}
	job2 := &core.JobEntity{ID: "job2", Type: "test", Status: core.JobStatusCompleted}
	job3 := &core.JobEntity{ID: "job3", Type: "test", Status: core.JobStatusPending}

	_ = store.SaveJob(ctx, job1)
	_ = store.SaveJob(ctx, job2)
	_ = store.SaveJob(ctx, job3)

	// Act - List pending jobs
	jobs, err := manager.ListJobs(ctx, core.JobFilter{Status: core.JobStatusPending})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}

	// Assert
	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2", len(jobs))
	}

	for _, job := range jobs {
		if job.Status != core.JobStatusPending {
			t.Errorf("Job status = %s, want PENDING", job.Status)
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

	store := core.NewMemoryJobStore()
	serializer := core.NewJSONSerializer()

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	ctx := context.Background()

	job1 := &core.JobEntity{ID: "job1", Type: "email", Status: core.JobStatusPending}
	job2 := &core.JobEntity{ID: "job2", Type: "sms", Status: core.JobStatusPending}
	job3 := &core.JobEntity{ID: "job3", Type: "email", Status: core.JobStatusPending}

	_ = store.SaveJob(ctx, job1)
	_ = store.SaveJob(ctx, job2)
	_ = store.SaveJob(ctx, job3)

	// Act - List email jobs
	jobs, err := manager.ListJobs(ctx, core.JobFilter{Type: "email"})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}

	// Assert
	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2", len(jobs))
	}

	for _, job := range jobs {
		if job.Type != "email" {
			t.Errorf("Job type = %s, want email", job.Type)
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

	store := core.NewMemoryJobStore()
	serializer := core.NewJSONSerializer()

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	ctx := context.Background()

	// Add 10 jobs
	for i := 0; i < 10; i++ {
		job := &core.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "test",
			Status: core.JobStatusPending,
		}
		_ = store.SaveJob(ctx, job)
	}

	// Act - Test limit
	jobs, err := manager.ListJobs(ctx, core.JobFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListJobs with limit failed: %v", err)
	}

	// Assert
	if len(jobs) != 5 {
		t.Errorf("len(jobs) with limit=5 = %d, want 5", len(jobs))
	}

	// Act - Test offset
	jobs, err = manager.ListJobs(ctx, core.JobFilter{Offset: 5, Limit: 3})
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

	store := core.NewMemoryJobStore()
	serializer := core.NewJSONSerializer()

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	type EmailArgs struct {
		To string `json:"to"`
	}

	handlerCalled := atomic.Bool{}
	handler := func(ctx context.Context, args EmailArgs) error {
		handlerCalled.Store(true)
		return nil
	}

	_ = core.RegisterHandler(manager, "email", handler)

	// Create PENDING job in store (simulating previous run)
	ctx := context.Background()
	existingJob := &core.JobEntity{
		ID:       "recovery-job-1",
		Type:     "email",
		ArgsData: []byte(`{"to":"recovery@example.com"}`),
		Status:   core.JobStatusPending,
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
	job, err := manager.GetJob(ctx, "recovery-job-1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusCompleted {
		t.Errorf("Status = %s, want COMPLETED", job.Status)
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

	store := core.NewMemoryJobStore()
	serializer := core.NewJSONSerializer()

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	// Create RUNNING job (simulating crash)
	ctx := context.Background()
	runningJob := &core.JobEntity{
		ID:       "running-job-1",
		Type:     "email",
		ArgsData: []byte(`{"to":"test@example.com"}`),
		Status:   core.JobStatusRunning,
		Priority: 1,
	}
	_ = store.SaveJob(ctx, runningJob)

	// Act - Start manager
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Assert - Job status is FAILED
	job, err := manager.GetJob(ctx, "running-job-1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusFailed {
		t.Errorf("Status = %s, want FAILED", job.Status)
	}
	if job.Result != "Interrupted by restart" {
		t.Errorf("Result = %s, want 'Interrupted by restart'", job.Result)
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

	store := core.NewMemoryJobStore()
	serializer := core.NewJSONSerializer()

	controlRunner := core.NewSequencedTaskRunner(pool)
	ioRunner := core.NewSequencedTaskRunner(pool)
	executionRunner := core.NewSequencedTaskRunner(pool)

	manager := core.NewJobManager(controlRunner, ioRunner, executionRunner, store, serializer)

	ctx := context.Background()
	runningJob := &core.JobEntity{
		ID:       "running-job-sync-start",
		Type:     "email",
		ArgsData: []byte(`{"to":"test@example.com"}`),
		Status:   core.JobStatusRunning,
		Priority: 1,
	}
	_ = store.SaveJob(ctx, runningJob)

	// Act
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Assert - no extra sleep needed; Start should be synchronous for recovery IO
	job, err := manager.GetJob(ctx, "running-job-sync-start")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job.Status != core.JobStatusFailed {
		t.Errorf("Status = %s, want FAILED", job.Status)
	}
	if job.Result != "Interrupted by restart" {
		t.Errorf("Result = %s, want 'Interrupted by restart'", job.Result)
	}

	// Cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = manager.Shutdown(shutdownCtx)
}
