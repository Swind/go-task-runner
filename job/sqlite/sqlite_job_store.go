package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sqdriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/Swind/go-task-runner/job"
	jobdb "github.com/Swind/go-task-runner/job/sqlite/db"
)

// Compile-time interface checks.
var _ job.JobStore = (*SQLiteJobStore)(nil)
var _ job.DurableJobStore = (*SQLiteJobStore)(nil)

// SQLiteJobStore is a SQLite-backed implementation of JobStore and DurableJobStore.
type SQLiteJobStore struct {
	q *jobdb.Queries
}

// NewSQLiteJobStore initialises the jobs table and returns a ready store.
// Pass an *sql.DB opened with driver "sqlite" (modernc.org/sqlite).
func NewSQLiteJobStore(sqlDB *sql.DB) (*SQLiteJobStore, error) {
	if _, err := sqlDB.Exec(sqliteSchema); err != nil {
		return nil, fmt.Errorf("create jobs table: %w", err)
	}
	return &SQLiteJobStore{q: jobdb.New(sqlDB)}, nil
}

// sqliteSchema is the DDL used to initialise a new database.
const sqliteSchema = `
CREATE TABLE IF NOT EXISTS jobs (
    id         TEXT     NOT NULL PRIMARY KEY,
    type       TEXT     NOT NULL,
    args_data  BLOB,
    status     TEXT     NOT NULL DEFAULT 'PENDING',
    result     TEXT     NOT NULL DEFAULT '',
    priority   INTEGER  NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs (status);
CREATE INDEX IF NOT EXISTS idx_jobs_type   ON jobs (type);
`

// CreateJob implements DurableJobStore — fails if the ID already exists.
func (s *SQLiteJobStore) CreateJob(ctx context.Context, j *job.JobEntity) error {
	if j.ID == "" {
		return fmt.Errorf("job ID cannot be empty")
	}
	now := time.Now()
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	j.UpdatedAt = now

	err := s.q.InsertJob(ctx, jobdb.InsertJobParams{
		ID:        j.ID,
		Type:      j.Type,
		ArgsData:  j.ArgsData,
		Status:    string(j.Status),
		Result:    j.Result,
		Priority:  int64(j.Priority),
		CreatedAt: j.CreatedAt,
		UpdatedAt: j.UpdatedAt,
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return job.ErrJobAlreadyExists
		}
		return err
	}
	return nil
}

// SaveJob implements JobStore — upserts (insert or replace).
func (s *SQLiteJobStore) SaveJob(ctx context.Context, j *job.JobEntity) error {
	if j.ID == "" {
		return fmt.Errorf("job ID cannot be empty")
	}
	now := time.Now()
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	j.UpdatedAt = now

	return s.q.UpsertJob(ctx, jobdb.UpsertJobParams{
		ID:        j.ID,
		Type:      j.Type,
		ArgsData:  j.ArgsData,
		Status:    string(j.Status),
		Result:    j.Result,
		Priority:  int64(j.Priority),
		CreatedAt: j.CreatedAt,
		UpdatedAt: j.UpdatedAt,
	})
}

// UpdateStatus implements JobStore.
func (s *SQLiteJobStore) UpdateStatus(ctx context.Context, id string, status job.JobStatus, result string) error {
	res, err := s.q.UpdateJobStatus(ctx, jobdb.UpdateJobStatusParams{
		Status:    string(status),
		Result:    result,
		UpdatedAt: time.Now(),
		ID:        id,
	})
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("job %s: %w", id, job.ErrJobNotFound)
	}
	return nil
}

// GetJob implements JobStore.
func (s *SQLiteJobStore) GetJob(ctx context.Context, id string) (*job.JobEntity, error) {
	row, err := s.q.GetJob(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("job %s: %w", id, job.ErrJobNotFound)
		}
		return nil, err
	}
	return rowToEntity(row), nil
}

// ListJobs implements JobStore with filter dispatching.
func (s *SQLiteJobStore) ListJobs(ctx context.Context, filter job.JobFilter) ([]*job.JobEntity, error) {
	limit := int64(filter.Limit)
	offset := int64(filter.Offset)

	hasStatus := filter.Status != ""
	hasType := filter.Type != ""

	// limit appears twice in each param struct because the SQL uses
	// CASE WHEN ? = 0 THEN -1 ELSE ? END to implement "no limit".
	switch {
	case hasStatus && hasType:
		rows, err := s.q.ListJobsByStatusAndType(ctx, jobdb.ListJobsByStatusAndTypeParams{
			Status:  string(filter.Status),
			Type:    filter.Type,
			Column3: limit,
			Column4: limit,
			Offset:  offset,
		})
		if err != nil {
			return nil, err
		}
		return rowsToEntities(rows), nil

	case hasStatus:
		rows, err := s.q.ListJobsByStatus(ctx, jobdb.ListJobsByStatusParams{
			Status:  string(filter.Status),
			Column2: limit,
			Column3: limit,
			Offset:  offset,
		})
		if err != nil {
			return nil, err
		}
		return rowsToEntities(rows), nil

	case hasType:
		rows, err := s.q.ListJobsByType(ctx, jobdb.ListJobsByTypeParams{
			Type:    filter.Type,
			Column2: limit,
			Column3: limit,
			Offset:  offset,
		})
		if err != nil {
			return nil, err
		}
		return rowsToEntities(rows), nil

	default:
		rows, err := s.q.ListAllJobs(ctx, jobdb.ListAllJobsParams{
			Column1: limit,
			Column2: limit,
			Offset:  offset,
		})
		if err != nil {
			return nil, err
		}
		return rowsToEntities(rows), nil
	}
}

// GetRecoverableJobs implements JobStore — returns only PENDING jobs.
func (s *SQLiteJobStore) GetRecoverableJobs(ctx context.Context) ([]*job.JobEntity, error) {
	rows, err := s.q.GetPendingJobs(ctx)
	if err != nil {
		return nil, err
	}
	return rowsToEntities(rows), nil
}

// DeleteJob implements JobStore.
func (s *SQLiteJobStore) DeleteJob(ctx context.Context, id string) error {
	return s.q.DeleteJob(ctx, id)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func rowToEntity(r jobdb.Job) *job.JobEntity {
	return &job.JobEntity{
		ID:        r.ID,
		Type:      r.Type,
		ArgsData:  r.ArgsData,
		Status:    job.JobStatus(r.Status),
		Result:    r.Result,
		Priority:  int(r.Priority),
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func rowsToEntities(rows []jobdb.Job) []*job.JobEntity {
	out := make([]*job.JobEntity, len(rows))
	for i, r := range rows {
		out[i] = rowToEntity(r)
	}
	return out
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr *sqdriver.Error
	if errors.As(err, &sqliteErr) {
		// SQLite extended result codes encode the primary code in the low 8 bits.
		// SQLITE_CONSTRAINT (19) covers UNIQUE, PRIMARY KEY, and other uniqueness violations.
		return sqliteErr.Code()&0xFF == sqlite3.SQLITE_CONSTRAINT
	}
	return false
}
