package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	jobdb "github.com/Swind/go-task-runner/job/db"
)

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
func (s *SQLiteJobStore) CreateJob(ctx context.Context, job *JobEntity) error {
	if job.ID == "" {
		return fmt.Errorf("job ID cannot be empty")
	}
	now := time.Now()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now

	err := s.q.InsertJob(ctx, jobdb.InsertJobParams{
		ID:        job.ID,
		Type:      job.Type,
		ArgsData:  job.ArgsData,
		Status:    string(job.Status),
		Result:    job.Result,
		Priority:  int64(job.Priority),
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrJobAlreadyExists
		}
		return err
	}
	return nil
}

// SaveJob implements JobStore — upserts (insert or replace).
func (s *SQLiteJobStore) SaveJob(ctx context.Context, job *JobEntity) error {
	if job.ID == "" {
		return fmt.Errorf("job ID cannot be empty")
	}
	now := time.Now()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now

	return s.q.UpsertJob(ctx, jobdb.UpsertJobParams{
		ID:        job.ID,
		Type:      job.Type,
		ArgsData:  job.ArgsData,
		Status:    string(job.Status),
		Result:    job.Result,
		Priority:  int64(job.Priority),
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
	})
}

// UpdateStatus implements JobStore.
func (s *SQLiteJobStore) UpdateStatus(ctx context.Context, id string, status JobStatus, result string) error {
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
		return fmt.Errorf("job %s: %w", id, ErrJobNotFound)
	}
	return nil
}

// GetJob implements JobStore.
func (s *SQLiteJobStore) GetJob(ctx context.Context, id string) (*JobEntity, error) {
	row, err := s.q.GetJob(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("job %s: %w", id, ErrJobNotFound)
		}
		return nil, err
	}
	return rowToEntity(row), nil
}

// ListJobs implements JobStore with filter dispatching.
func (s *SQLiteJobStore) ListJobs(ctx context.Context, filter JobFilter) ([]*JobEntity, error) {
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
func (s *SQLiteJobStore) GetRecoverableJobs(ctx context.Context) ([]*JobEntity, error) {
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

func rowToEntity(r jobdb.Job) *JobEntity {
	return &JobEntity{
		ID:        r.ID,
		Type:      r.Type,
		ArgsData:  r.ArgsData,
		Status:    JobStatus(r.Status),
		Result:    r.Result,
		Priority:  int(r.Priority),
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func rowsToEntities(rows []jobdb.Job) []*JobEntity {
	out := make([]*JobEntity, len(rows))
	for i, r := range rows {
		out[i] = rowToEntity(r)
	}
	return out
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		// SQLite extended result codes encode the primary code in the low 8 bits.
		// SQLITE_CONSTRAINT (19) covers UNIQUE, PRIMARY KEY, and other uniqueness
		// violations. We check the primary code so this works whether or not
		// extended result codes are enabled on the connection.
		code := sqliteErr.Code()
		return code&0xFF == sqlite3.SQLITE_CONSTRAINT
	}
	// Fallback: string match for drivers that wrap the error differently.
	return false
}
