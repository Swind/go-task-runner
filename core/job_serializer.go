package core

import (
	"encoding/json"
	"fmt"
)

// =============================================================================
// JobSerializer Interface
// =============================================================================

// JobSerializer defines the interface for serializing and deserializing job arguments.
type JobSerializer interface {
	// Serialize converts a Go value to bytes
	Serialize(args any) ([]byte, error)

	// Deserialize converts bytes back to a Go value
	Deserialize(data []byte, target any) error

	// Name returns the serializer name (for debugging/logging)
	Name() string
}

// =============================================================================
// JSONSerializer Implementation
// =============================================================================

// JSONSerializer uses JSON encoding for serialization.
type JSONSerializer struct{}

// NewJSONSerializer creates a new JSON serializer
func NewJSONSerializer() *JSONSerializer {
	return &JSONSerializer{}
}

func (s *JSONSerializer) Serialize(args any) ([]byte, error) {
	if args == nil {
		return []byte("null"), nil
	}

	data, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("json marshal failed: %w", err)
	}

	return data, nil
}

func (s *JSONSerializer) Deserialize(data []byte, target any) error {
	if target == nil {
		return fmt.Errorf("deserialize target cannot be nil")
	}

	if len(data) == 0 {
		return fmt.Errorf("data is empty")
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("json unmarshal failed: %w", err)
	}

	return nil
}

func (s *JSONSerializer) Name() string {
	return "json"
}
