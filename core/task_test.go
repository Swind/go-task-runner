package core

import (
	"context"
	"testing"
)

// TestTaskID_StringAndIsZero verifies TaskID zero-state and string behavior
// Given: A zero TaskID and a generated TaskID
// When: IsZero and String are called
// Then: Zero ID reports true and generated ID is non-zero with non-empty string
func TestTaskID_StringAndIsZero(t *testing.T) {
	// Arrange
	var zero TaskID

	// Act and Assert
	if !zero.IsZero() {
		t.Fatal("zero TaskID should report IsZero() == true")
	}

	// Act
	id := GenerateTaskID()

	// Assert
	if id.IsZero() {
		t.Fatal("generated TaskID should not be zero")
	}
	if id.String() == "" {
		t.Fatal("TaskID.String() should not be empty")
	}
}

// TestGetCurrentTaskRunner verifies extracting task runner from context
// Given: A plain context and a context containing task runner value
// When: GetCurrentTaskRunner is called
// Then: It returns nil for plain context and the stored runner for annotated context
func TestGetCurrentTaskRunner(t *testing.T) {
	// Arrange, Act and Assert - plain context
	if got := GetCurrentTaskRunner(context.Background()); got != nil {
		t.Fatalf("GetCurrentTaskRunner(background) = %#v, want nil", got)
	}

	// Arrange
	runner := &MockTaskRunner{}
	ctx := context.WithValue(context.Background(), taskRunnerKey, runner)

	// Act and Assert
	if got := GetCurrentTaskRunner(ctx); got != runner {
		t.Fatal("GetCurrentTaskRunner(ctx) did not return the runner from context")
	}
}
