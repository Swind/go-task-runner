package job_test

import (
	"testing"

	"github.com/Swind/go-task-runner/job"
)

// TestJSONSerializer_Serialize verifies struct serialization to JSON
// Given: A struct with JSON tags
// When: Struct is serialized
// Then: Valid JSON bytes are produced
func TestJSONSerializer_Serialize(t *testing.T) {
	// Arrange
	serializer := job.NewJSONSerializer()

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
	serializer := job.NewJSONSerializer()

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
	serializer := job.NewJSONSerializer()

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
