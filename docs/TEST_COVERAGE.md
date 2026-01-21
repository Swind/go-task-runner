# Test Coverage Report

**Date**: 2026-01-21

## Overall Coverage

- **Root package**: 82.4%
- **Core package**: 89.8%
- **Combined (excluding examples)**: 88.1% (216 functions)
  - Total functions: 216
  - Partially uncovered functions: 60

## Per-File Breakdown

| File | Coverage | Functions | Status |
|------|----------|-----------|--------|
| delay_manager.go | 98.6% | 13 | ✓ Excellent |
| interfaces.go | 38.1% | 7 | ✗ Low |
| job_manager.go | 94.4% | 25 | ✓ Excellent |
| job_serializer.go | 92.2% | 4 | ✓ Excellent |
| job_store.go | 97.0% | 9 | ✓ Excellent |
| logger.go | 71.7% | 15 | ! Fair |
| parallel_task_runner.go | 72.9% | 31 | ! Fair |
| queue.go | 93.6% | 26 | ✓ Excellent |
| sequenced_task_runner.go | 97.4% | 26 | ✓ Excellent |
| single_thread_task_runner.go | 95.5% | 29 | ✓ Excellent |
| task.go | 70.8% | 8 | ! Fair |
| task_and_reply.go | 99.0% | 6 | ✓ Excellent |
| task_scheduler.go | 96.4% | 17 | ✓ Excellent |

## Low Coverage Areas (<80%)

| File | Coverage | Functions |
|------|----------|-----------|
| interfaces.go | 38.1% | 7 |
| logger.go | 71.7% | 15 |
| parallel_task_runner.go | 72.9% | 31 |
| task.go | 70.8% | 8 |

## Analysis

### Areas with Lower Coverage

1. **interfaces.go (38.1%)** - Metrics recording functions not covered:
   - `RecordTaskDuration`, `RecordTaskPanic`, `RecordQueueDepth`, `RecordTaskRejected`
   - These are optional hooks for custom metrics

2. **logger.go (71.7%)** - `NoOpLogger` methods not covered:
   - Debug, Info, Warn, Error methods

3. **parallel_task_runner.go (72.9%)** - Some methods not covered:
   - Getter/setter methods: Name, SetName, Metadata, SetMetadata, GetThreadPool
   - PostTaskAndReplyWithTraits, WaitShutdown

4. **task.go (70.8%)** - Utility methods not covered:
   - String() - Debug string representation
   - IsZero() - Zero value check

### Summary

The project has excellent overall test coverage (~88%).

The uncovered functions are primarily:
- Optional callback interfaces (metrics hooks)
- Getter/setter methods
- Utility functions like `String()` for debugging

All tests pass with `go test -tags=ci`. The failing test without tags is
a scalable stress test (`TestDelayManager_ConcurrentAdd_Scalable`) that's
intentionally excluded from CI.

## How to Regenerate This Report

```bash
# Generate coverage profile
go test -tags=ci -coverprofile=coverage.out ./...

# View coverage by function
go tool cover -func=coverage.out

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html
```
