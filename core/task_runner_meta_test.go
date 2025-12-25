package core_test

import (
	"context"
	"testing"

	taskrunner "github.com/Swind/go-task-runner"
)

// TestTaskRunner_NameAndMetadata tests TaskRunner name and metadata methods
// Given: a newly created SequencedTaskRunner or SingleThreadTaskRunner
// When: SetName and SetMetadata are called
// Then: Name() and Metadata() return the set values, and Metadata() returns a copy
func TestTaskRunner_NameAndMetadata(t *testing.T) {
	// Arrange & Act & Assert - Test SequencedTaskRunner
	t.Run("SequencedTaskRunner", func(t *testing.T) {
		// Arrange - Create pool and runner
		pool := taskrunner.NewGoroutineThreadPool("test-pool", 1)
		pool.Start(context.Background())
		defer pool.Stop()

		runner := taskrunner.NewSequencedTaskRunner(pool)

		// Assert - Verify defaults
		gotName := runner.Name()
		wantName := ""
		if gotName != wantName {
			t.Errorf("Name default: got = %q, want = %q", gotName, wantName)
		}

		gotMetaLen := len(runner.Metadata())
		wantMetaLen := 0
		if gotMetaLen != wantMetaLen {
			t.Errorf("Metadata default length: got = %d, want = %d", gotMetaLen, wantMetaLen)
		}

		// Act - Set name
		expectedName := "MySequencedRunner"
		runner.SetName(expectedName)

		// Assert - Verify name was set
		gotName = runner.Name()
		if gotName != expectedName {
			t.Errorf("Name after SetName: got = %q, want = %q", gotName, expectedName)
		}

		// Act - Set metadata
		runner.SetMetadata("key1", "value1")
		runner.SetMetadata("key2", 123)

		meta := runner.Metadata()

		// Assert - Verify metadata
		gotMetaLen = len(meta)
		wantMetaLen = 2
		if gotMetaLen != wantMetaLen {
			t.Errorf("Metadata length: got = %d, want = %d", gotMetaLen, wantMetaLen)
		}

		if meta["key1"] != "value1" {
			t.Errorf("metadata['key1']: got = %v, want = value1", meta["key1"])
		}

		if meta["key2"] != 123 {
			t.Errorf("metadata['key2']: got = %v, want = 123", meta["key2"])
		}

		// Act - Mutate returned metadata
		meta["key1"] = "mutated"

		// Assert - Verify Metadata() returns a copy (original unchanged)
		originalMeta := runner.Metadata()
		if originalMeta["key1"] == "mutated" {
			t.Error("Metadata() returns a copy: got = mutated map, want = original unchanged")
		}
	})

	// Arrange & Act & Assert - Test SingleThreadTaskRunner
	t.Run("SingleThreadTaskRunner", func(t *testing.T) {
		// Arrange - Create runner
		runner := taskrunner.NewSingleThreadTaskRunner()
		defer runner.Stop()

		// Assert - Verify defaults
		gotName := runner.Name()
		wantName := ""
		if gotName != wantName {
			t.Errorf("Name default: got = %q, want = %q", gotName, wantName)
		}

		gotMetaLen := len(runner.Metadata())
		wantMetaLen := 0
		if gotMetaLen != wantMetaLen {
			t.Errorf("Metadata default length: got = %d, want = %d", gotMetaLen, wantMetaLen)
		}

		// Act - Set name
		expectedName := "MySingleThreadRunner"
		runner.SetName(expectedName)

		// Assert - Verify name was set
		gotName = runner.Name()
		if gotName != expectedName {
			t.Errorf("Name after SetName: got = %q, want = %q", gotName, expectedName)
		}

		// Act - Set metadata
		runner.SetMetadata("type", "worker")
		runner.SetMetadata("id", 99)

		meta := runner.Metadata()

		// Assert - Verify metadata
		gotMetaLen = len(meta)
		wantMetaLen = 2
		if gotMetaLen != wantMetaLen {
			t.Errorf("Metadata length: got = %d, want = %d", gotMetaLen, wantMetaLen)
		}

		if meta["type"] != "worker" {
			t.Errorf("metadata['type']: got = %v, want = worker", meta["type"])
		}

		if meta["id"] != 99 {
			t.Errorf("metadata['id']: got = %v, want = 99", meta["id"])
		}
	})
}
