package core

import (
	"fmt"
	"log"
	"time"
)

// Logger interface for structured logging
// Implementations can provide custom logging behavior (e.g., integration with logrus, zap, etc.)
type Logger interface {
	// Debug logs a debug message with optional fields
	Debug(msg string, fields ...Field)

	// Info logs an info message with optional fields
	Info(msg string, fields ...Field)

	// Warn logs a warning message with optional fields
	Warn(msg string, fields ...Field)

	// Error logs an error message with optional fields
	Error(msg string, fields ...Field)
}

// Field represents a key-value pair for structured logging
type Field struct {
	Key   string
	Value any
}

// F creates a new Field with the given key and value
func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}

// DefaultLogger is a simple logger implementation using the standard log package
type DefaultLogger struct{}

// NewDefaultLogger creates a new DefaultLogger
func NewDefaultLogger() *DefaultLogger {
	return &DefaultLogger{}
}

// Debug logs a debug message
func (l *DefaultLogger) Debug(msg string, fields ...Field) {
	l.log("DEBUG", msg, fields...)
}

// Info logs an info message
func (l *DefaultLogger) Info(msg string, fields ...Field) {
	l.log("INFO", msg, fields...)
}

// Warn logs a warning message
func (l *DefaultLogger) Warn(msg string, fields ...Field) {
	l.log("WARN", msg, fields...)
}

// Error logs an error message
func (l *DefaultLogger) Error(msg string, fields ...Field) {
	l.log("ERROR", msg, fields...)
}

// log is the internal logging method
func (l *DefaultLogger) log(level, msg string, fields ...Field) {
	// Build the log message
	logMsg := fmt.Sprintf("[%s] %s", level, msg)
	if len(fields) > 0 {
		logMsg += " {"
		for i, f := range fields {
			if i > 0 {
				logMsg += ", "
			}
			logMsg += fmt.Sprintf("%s: %v", f.Key, f.Value)
		}
		logMsg += "}"
	}
	log.Println(logMsg)
}

// NoOpLogger is a logger that discards all log messages
// Useful for tests or when logging is not desired
type NoOpLogger struct{}

// NewNoOpLogger creates a new NoOpLogger
func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

func (l *NoOpLogger) Debug(msg string, fields ...Field) {}
func (l *NoOpLogger) Info(msg string, fields ...Field)  {}
func (l *NoOpLogger) Warn(msg string, fields ...Field)  {}
func (l *NoOpLogger) Error(msg string, fields ...Field) {}

// =============================================================================
// Retry Policy
// =============================================================================

// RetryPolicy defines retry behavior for IO operations
type RetryPolicy struct {
	// MaxRetries is the maximum number of retry attempts (0 = no retry, 1 = one retry)
	MaxRetries int

	// InitialDelay is the delay before the first retry
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration

	// BackoffRatio is the multiplier for delay after each retry (e.g., 2.0 for exponential)
	// For example, with InitialDelay=100ms and BackoffRatio=2.0:
	// - Retry 1 delay: 100ms
	// - Retry 2 delay: 200ms
	// - Retry 3 delay: 400ms (capped by MaxDelay)
	BackoffRatio float64
}

// DefaultRetryPolicy returns a sensible default retry policy
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		BackoffRatio: 2.0,
	}
}

// NoRetry returns a retry policy with no retries
func NoRetry() RetryPolicy {
	return RetryPolicy{
		MaxRetries:   0,
		InitialDelay: 0,
		MaxDelay:     0,
		BackoffRatio: 1.0,
	}
}

// calculateDelay calculates the delay for the given retry attempt
// attempt is 0-indexed (0 = first retry, 1 = second retry, etc.)
func (p RetryPolicy) calculateDelay(attempt int) time.Duration {
	if p.InitialDelay == 0 {
		return 0
	}

	// Calculate exponential backoff
	delay := float64(p.InitialDelay)
	for i := 0; i < attempt; i++ {
		delay *= p.BackoffRatio
	}

	// Cap at MaxDelay
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}

	return time.Duration(delay)
}
