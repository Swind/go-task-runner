package job_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Swind/go-task-runner/job"
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
	store := job.NewMemoryJobStore()
	ctx := context.Background()

	entity := &job.JobEntity{
		ID:       "job1",
		Type:     "email",
		ArgsData: []byte(`{"to":"user@example.com"}`),
		Status:   job.JobStatusPending,
		Priority: 1,
	}

	// Act - Save job
	if err := store.SaveJob(ctx, entity); err != nil {
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
	if retrieved.Status != job.JobStatusPending {
		t.Errorf("Status = %s, want PENDING", retrieved.Status)
	}
}

// TestMemoryJobStore_UpdateStatus verifies job status updates
// Given: A job store with a PENDING job
// When: Status is updated to RUNNING
// Then: Retrieved job has RUNNING status
func TestMemoryJobStore_UpdateStatus(t *testing.T) {
	// Arrange
	store := job.NewMemoryJobStore()
	ctx := context.Background()

	entity := &job.JobEntity{
		ID:     "job1",
		Type:   "email",
		Status: job.JobStatusPending,
	}
	_ = store.SaveJob(ctx, entity)

	// Act - Update status
	if err := store.UpdateStatus(ctx, "job1", job.JobStatusRunning, ""); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Assert
	retrieved, _ := store.GetJob(ctx, "job1")
	if retrieved.Status != job.JobStatusRunning {
		t.Errorf("Status = %s, want RUNNING", retrieved.Status)
	}
}

// TestMemoryJobStore_ListJobs verifies job listing with filters
// Given: A store with 5 jobs of mixed statuses
// When: Jobs are listed with various filters
// Then: Correct jobs are returned based on filter criteria
func TestMemoryJobStore_ListJobs(t *testing.T) {
	// Arrange
	store := job.NewMemoryJobStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		status := job.JobStatusPending
		if i%2 == 0 {
			status = job.JobStatusCompleted
		}
		job := &job.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "email",
			Status: status,
		}
		_ = store.SaveJob(ctx, job)
	}

	// Act - List all jobs
	allJobs, err := store.ListJobs(ctx, job.JobFilter{})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}

	// Assert
	if len(allJobs) != 5 {
		t.Errorf("len(allJobs) = %d, want 5", len(allJobs))
	}

	// Act - List pending jobs only
	pendingJobs, err := store.ListJobs(ctx, job.JobFilter{Status: job.JobStatusPending})
	if err != nil {
		t.Fatalf("ListJobs with filter failed: %v", err)
	}

	// Assert
	if len(pendingJobs) != 2 {
		t.Errorf("len(pendingJobs) = %d, want 2", len(pendingJobs))
	}

	// Act - Test limit
	limitedJobs, err := store.ListJobs(ctx, job.JobFilter{Limit: 3})
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
	store := job.NewMemoryJobStore()
	ctx := context.Background()

	statuses := []job.JobStatus{job.JobStatusPending, job.JobStatusRunning, job.JobStatusCompleted, job.JobStatusPending}
	for i, status := range statuses {
		job := &job.JobEntity{
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

	for _, j := range recoverable {
		if j.Status != job.JobStatusPending {
			t.Errorf("Recoverable job has status %s, want PENDING", j.Status)
		}
	}
}

// TestMemoryJobStore_DeleteJob verifies job deletion
// Given: A store with a saved job
// When: Job is deleted by ID
// Then: Job no longer exists in store
func TestMemoryJobStore_DeleteJob(t *testing.T) {
	// Arrange
	store := job.NewMemoryJobStore()
	ctx := context.Background()

	job := &job.JobEntity{ID: "job1", Type: "email", Status: job.JobStatusPending}
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
	store := job.NewMemoryJobStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		job := &job.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "email",
			Status: job.JobStatusPending,
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
	store := job.NewMemoryJobStore()
	ctx := context.Background()

	// Assert - Initially empty
	if count := store.Count(); count != 0 {
		t.Errorf("Count() = %d initially, want 0", count)
	}

	// Act - Add jobs one by one
	for i := 1; i <= 10; i++ {
		job := &job.JobEntity{
			ID:     fmt.Sprintf("job%d", i),
			Type:   "email",
			Status: job.JobStatusPending,
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
