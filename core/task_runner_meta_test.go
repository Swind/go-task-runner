package core_test

import (
	"context"
	"testing"

	taskrunner "github.com/Swind/go-task-runner"
)

func TestTaskRunner_NameAndMetadata(t *testing.T) {
	t.Run("SequencedTaskRunner", func(t *testing.T) {
		pool := taskrunner.NewGoroutineThreadPool("test-pool", 1)
		pool.Start(context.Background())
		defer pool.Stop()

		runner := taskrunner.NewSequencedTaskRunner(pool)

		// Defaults
		if runner.Name() != "" {
			t.Errorf("Expected empty name, got %q", runner.Name())
		}
		if len(runner.Metadata()) != 0 {
			t.Errorf("Expected empty metadata, got %v", runner.Metadata())
		}

		// Set Name
		expectedName := "MySequencedRunner"
		runner.SetName(expectedName)
		if runner.Name() != expectedName {
			t.Errorf("Expected name %q, got %q", expectedName, runner.Name())
		}

		// Set Metadata
		runner.SetMetadata("key1", "value1")
		runner.SetMetadata("key2", 123)

		meta := runner.Metadata()
		if len(meta) != 2 {
			t.Errorf("Expected 2 metadata entries, got %d", len(meta))
		}
		if meta["key1"] != "value1" {
			t.Errorf("Expected key1=value1, got %v", meta["key1"])
		}
		if meta["key2"] != 123 {
			t.Errorf("Expected key2=123, got %v", meta["key2"])
		}

		// Verify copy behavior
		meta["key1"] = "mutated"
		if runner.Metadata()["key1"] == "mutated" {
			t.Error("Metadata() should return a copy")
		}
	})

	t.Run("SingleThreadTaskRunner", func(t *testing.T) {
		runner := taskrunner.NewSingleThreadTaskRunner()
		defer runner.Stop()

		// Defaults
		if runner.Name() != "" {
			t.Errorf("Expected empty name, got %q", runner.Name())
		}
		if len(runner.Metadata()) != 0 {
			t.Errorf("Expected empty metadata, got %v", runner.Metadata())
		}

		// Set Name
		expectedName := "MySingleThreadRunner"
		runner.SetName(expectedName)
		if runner.Name() != expectedName {
			t.Errorf("Expected name %q, got %q", expectedName, runner.Name())
		}

		// Set Metadata
		runner.SetMetadata("type", "worker")
		runner.SetMetadata("id", 99)

		meta := runner.Metadata()
		if len(meta) != 2 {
			t.Errorf("Expected 2 metadata entries, got %d", len(meta))
		}
		if meta["type"] != "worker" {
			t.Errorf("Expected type=worker, got %v", meta["type"])
		}
		if meta["id"] != 99 {
			t.Errorf("Expected id=99, got %v", meta["id"])
		}
	})
}
