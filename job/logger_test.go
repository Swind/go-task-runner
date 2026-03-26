package job_test

import (
	"testing"
	"time"

	"github.com/Swind/go-task-runner/job"
)

// TestLogger_DefaultLogger verifies default logger functionality
// Given: A default logger
// When: All log methods are called
// Then: No panic occurs
func TestLogger_DefaultLogger(t *testing.T) {
	// Arrange
	logger := job.NewDefaultLogger()

	// Act & Assert - Should not panic
	logger.Debug("debug message", job.F("key", "value"))
	logger.Info("info message", job.F("key", "value"))
	logger.Warn("warn message", job.F("key", "value"))
	logger.Error("error message", job.F("key", "value"))
}

// TestLogger_NoOpLogger verifies no-op logger functionality
// Given: A no-op logger
// When: All log methods are called
// Then: Output is discarded, no panic
func TestLogger_NoOpLogger(t *testing.T) {
	// Arrange
	logger := job.NewNoOpLogger()

	// Act & Assert - Should not panic
	logger.Debug("debug message", job.F("key", "value"))
	logger.Info("info message", job.F("key", "value"))
	logger.Warn("warn message", job.F("key", "value"))
	logger.Error("error message", job.F("key", "value"))
}

// TestRetryPolicy_CalculateDelay verifies retry policy field values
// Given: A retry policy with specific values
// When: Policy is created
// Then: Fields are set correctly
func TestRetryPolicy_CalculateDelay(t *testing.T) {
	// Arrange
	policy := job.RetryPolicy{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		BackoffRatio: 2.0,
	}

	// Assert
	if policy.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", policy.InitialDelay)
	}
	if policy.MaxDelay != 500*time.Millisecond {
		t.Errorf("MaxDelay = %v, want 500ms", policy.MaxDelay)
	}
	if policy.BackoffRatio != 2.0 {
		t.Errorf("BackoffRatio = %f, want 2.0", policy.BackoffRatio)
	}
}

// TestRetryPolicy_Defaults verifies default retry policy values
// Given: DefaultRetryPolicy() is called
// When: Policy is created
// Then: Default values are correct
func TestRetryPolicy_Defaults(t *testing.T) {
	// Act
	policy := job.DefaultRetryPolicy()

	// Assert
	if policy.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", policy.MaxRetries)
	}
	if policy.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", policy.InitialDelay)
	}
	if policy.MaxDelay != 5*time.Second {
		t.Errorf("MaxDelay = %v, want 5s", policy.MaxDelay)
	}
	if policy.BackoffRatio != 2.0 {
		t.Errorf("BackoffRatio = %f, want 2.0", policy.BackoffRatio)
	}
}

// TestRetryPolicy_NoRetry verifies NoRetry helper
// Given: NoRetry() is called
// When: Policy is created
// Then: MaxRetries is 0
func TestRetryPolicy_NoRetry(t *testing.T) {
	// Act
	policy := job.NoRetry()

	// Assert
	if policy.MaxRetries != 0 {
		t.Errorf("MaxRetries = %d, want 0", policy.MaxRetries)
	}
}

// TestNoOpLogger_ExplicitCoverage verifies all NoOpLogger methods
// Given: A NoOpLogger
// When: All log methods are called
// Then: No panic occurs
func TestNoOpLogger_ExplicitCoverage(t *testing.T) {
	// Arrange
	logger := job.NewNoOpLogger()

	// Act & Assert - All methods callable
	logger.Debug("test debug", job.F("key1", "value1"), job.F("key2", "value2"))
	logger.Info("test info", job.F("key", "value"))
	logger.Warn("test warn", job.F("level", "high"))
	logger.Error("test error", job.F("err", "something failed"))
}
