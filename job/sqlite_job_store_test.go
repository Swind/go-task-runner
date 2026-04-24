package job_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Swind/go-task-runner/job"
)

func TestSQLiteJobStore(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close: %v", err)
		}
	})

	store, err := job.NewSQLiteJobStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteJobStore: %v", err)
	}

	runJobStoreSuite(t, store)
}

func TestSQLiteJobStore_DurableCreate(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close: %v", err)
		}
	})

	store, err := job.NewSQLiteJobStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteJobStore: %v", err)
	}

	ctx := context.Background()
	entity := &job.JobEntity{ID: "dup-job", Type: "email", Status: job.JobStatusPending}

	if err := store.CreateJob(ctx, entity); err != nil {
		t.Fatalf("first CreateJob: %v", err)
	}

	err = store.CreateJob(ctx, entity)
	if !errors.Is(err, job.ErrJobAlreadyExists) {
		t.Errorf("second CreateJob err = %v, want ErrJobAlreadyExists", err)
	}
}
