package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Job Data Models
// =============================================================================

type JobStatus string

const (
	JobStatusPending   JobStatus = "PENDING"
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
	JobStatusFailed    JobStatus = "FAILED"
	JobStatusCanceled  JobStatus = "CANCELED"
)

type JobEntity struct {
	ID        string
	Type      string
	ArgsData  []byte
	Status    JobStatus
	Result    string
	Priority  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type JobFilter struct {
	Status JobStatus // Empty means all
	Type   string    // Empty means all
	Limit  int       // 0 means no limit
	Offset int       // Default 0
}

// =============================================================================
// JobStore Interface
// =============================================================================

// JobStore defines the interface for job persistence.
// Implementations can use in-memory storage, databases, or other backends.
type JobStore interface {
	// SaveJob saves a new job or updates an existing one
	SaveJob(ctx context.Context, job *JobEntity) error

	// UpdateStatus updates the status and result of a job
	UpdateStatus(ctx context.Context, id string, status JobStatus, result string) error

	// GetJob retrieves a job by ID
	GetJob(ctx context.Context, id string) (*JobEntity, error)

	// ListJobs returns jobs matching the filter
	ListJobs(ctx context.Context, filter JobFilter) ([]*JobEntity, error)

	// GetRecoverableJobs returns jobs that should be recovered on startup
	// (typically PENDING jobs)
	GetRecoverableJobs(ctx context.Context) ([]*JobEntity, error)

	// DeleteJob removes a job from storage
	DeleteJob(ctx context.Context, id string) error
}

// ErrJobAlreadyExists indicates the job ID already exists in persistent storage.
var ErrJobAlreadyExists = errors.New("job already exists")

// DurableJobStore provides atomic create semantics for durable-ack submission.
// Implement this interface to guarantee CreateJob fails when a job ID already exists.
type DurableJobStore interface {
	CreateJob(ctx context.Context, job *JobEntity) error
}

// =============================================================================
// MemoryJobStore Implementation
// =============================================================================

// MemoryJobStore is an in-memory implementation of JobStore.
// It uses sync.Map for concurrent-safe storage.
type MemoryJobStore struct {
	data sync.Map // map[string]*JobEntity
}

// NewMemoryJobStore creates a new in-memory job store
func NewMemoryJobStore() *MemoryJobStore {
	return &MemoryJobStore{}
}

func cloneJobEntity(job *JobEntity) *JobEntity {
	return &JobEntity{
		ID:        job.ID,
		Type:      job.Type,
		ArgsData:  append([]byte(nil), job.ArgsData...),
		Status:    job.Status,
		Result:    job.Result,
		Priority:  job.Priority,
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
	}
}

// CreateJob inserts a new job atomically.
// Returns ErrJobAlreadyExists if the ID already exists.
func (s *MemoryJobStore) CreateJob(ctx context.Context, job *JobEntity) error {
	if job.ID == "" {
		return fmt.Errorf("job ID cannot be empty")
	}

	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	job.UpdatedAt = time.Now()

	jobCopy := cloneJobEntity(job)
	if _, loaded := s.data.LoadOrStore(job.ID, jobCopy); loaded {
		return ErrJobAlreadyExists
	}

	return nil
}

func (s *MemoryJobStore) SaveJob(ctx context.Context, job *JobEntity) error {
	if job.ID == "" {
		return fmt.Errorf("job ID cannot be empty")
	}

	// Set timestamps
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	job.UpdatedAt = time.Now()

	// Create a copy to avoid external modifications
	jobCopy := cloneJobEntity(job)

	s.data.Store(job.ID, jobCopy)
	return nil
}

func (s *MemoryJobStore) UpdateStatus(ctx context.Context, id string, status JobStatus, result string) error {
	raw, ok := s.data.Load(id)
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}

	job := raw.(*JobEntity)

	// Create updated copy
	updatedJob := &JobEntity{
		ID:        job.ID,
		Type:      job.Type,
		ArgsData:  job.ArgsData,
		Status:    status,
		Result:    result,
		Priority:  job.Priority,
		CreatedAt: job.CreatedAt,
		UpdatedAt: time.Now(),
	}

	s.data.Store(id, updatedJob)
	return nil
}

func (s *MemoryJobStore) GetJob(ctx context.Context, id string) (*JobEntity, error) {
	raw, ok := s.data.Load(id)
	if !ok {
		return nil, fmt.Errorf("job %s not found", id)
	}

	job := raw.(*JobEntity)

	// Return a copy to prevent external modifications
	return &JobEntity{
		ID:        job.ID,
		Type:      job.Type,
		ArgsData:  append([]byte(nil), job.ArgsData...),
		Status:    job.Status,
		Result:    job.Result,
		Priority:  job.Priority,
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
	}, nil
}

func (s *MemoryJobStore) ListJobs(ctx context.Context, filter JobFilter) ([]*JobEntity, error) {
	var jobs []*JobEntity
	count := 0
	skipped := 0

	s.data.Range(func(key, value any) bool {
		job := value.(*JobEntity)

		// Apply filters
		if filter.Status != "" && job.Status != filter.Status {
			return true
		}
		if filter.Type != "" && job.Type != filter.Type {
			return true
		}

		// Apply offset
		if skipped < filter.Offset {
			skipped++
			return true
		}

		// Apply limit
		if filter.Limit > 0 && count >= filter.Limit {
			return false
		}

		// Create a copy
		jobCopy := &JobEntity{
			ID:        job.ID,
			Type:      job.Type,
			ArgsData:  append([]byte(nil), job.ArgsData...),
			Status:    job.Status,
			Result:    job.Result,
			Priority:  job.Priority,
			CreatedAt: job.CreatedAt,
			UpdatedAt: job.UpdatedAt,
		}

		jobs = append(jobs, jobCopy)
		count++
		return true
	})

	return jobs, nil
}

func (s *MemoryJobStore) GetRecoverableJobs(ctx context.Context) ([]*JobEntity, error) {
	// Only PENDING jobs are recoverable
	return s.ListJobs(ctx, JobFilter{Status: JobStatusPending})
}

func (s *MemoryJobStore) DeleteJob(ctx context.Context, id string) error {
	s.data.Delete(id)
	return nil
}

// Clear removes all jobs from the store (useful for testing)
func (s *MemoryJobStore) Clear() {
	s.data = sync.Map{}
}

// Count returns the total number of jobs in the store
func (s *MemoryJobStore) Count() int {
	count := 0
	s.data.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}
