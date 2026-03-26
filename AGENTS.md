# Repository Guidelines

## Project Overview

Experimental Go library implementing a multi-threaded programming architecture inspired by Chromium's Threading and Tasks. Post tasks to virtual threads (Task Runners) instead of managing raw goroutines or channels. Educational/testing purposes only, not production use.

## Project Structure

- Root package (`pool.go`, `types.go`, `doc.go`) — public API; re-exports core types via type aliases
- `core/` — internal scheduler, queue, runner, job-manager implementations
- `observability/prometheus/` — Prometheus metrics exporter and snapshot poller
- `examples/` — runnable demos (each subdirectory has a `main.go`)
- `docs/` — architecture and subsystem docs
- Tests live beside code as `*_test.go` in root, `core/`, and `observability/`

## Build, Test, and Lint Commands

```bash
# Install dependencies
go mod download

# Build (library, no binary)
go build ./...

# Run all tests
go test ./...

# Run all tests (CI mode, excludes scalable stress tests, verbose, coverage)
go test -tags=ci -v -coverprofile=coverage.out ./...

# Run tests for a specific package
go test ./core/...
go test ./observability/prometheus/...

# Run a single test by name
go test -v -run TestSequencedTaskRunner_SequentialExecution ./core/
go test -v -run TestPostTaskAndReply_BasicExecution ./core/

# Run a single test with race detector
go test -race -v -run TestSequencedTaskRunner_ShutdownDrainsQueue ./core/

# Run tests with verbose output
go test -v ./...

# Coverage report
go test -tags=ci -v -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Lint (matches CI scope; lints core/ and root, excludes examples/)
golangci-lint run --timeout=5m core/... .

# Run all examples
find examples -name "main.go" | sort | xargs -n1 go run
```

## Coding Style & Formatting

- Run `gofmt` on changed files before committing (standard Go formatting)
- Use `go vet` and `golangci-lint` to catch issues
- No code comments unless explaining non-obvious concurrency contracts or design decisions

## Imports

- Group imports into three blocks: stdlib, external, internal (this project)
- Use `core "github.com/Swind/go-task-runner/core"` when importing from root package tests
- Use `taskrunner "github.com/Swind/go-task-runner"` when importing the root package from tests

## Types & Naming

- Exported identifiers: `PascalCase`; unexported: `camelCase`
- Prefer clear, domain-specific names: `TaskRunner`, `JobManager`, `DelayManager`, `TaskScheduler`
- Use type aliases (`type X = core.X`) in root package for re-exports
- Interfaces: name with `-er` suffix (`TaskRunner`, `PanicHandler`, `Metrics`, `ThreadPool`)
- Constructor functions: `NewXxx()` or `NewXxxWithConfig()`
- Constants: `PascalCase` for exported, `camelCase` for unexported

## Error Handling

- Return `error` values; do not swallow errors
- Use `fmt.Errorf("context: %w", err)` for wrapping
- Panic for programmer errors (nil args to constructors, invariant violations)
- Recover panics in task execution; use `PanicHandler` interface for custom handling
- In `PostTaskAndReply`: task panic prevents reply from executing

## Concurrency Patterns

- Use `sync.Mutex` for queue and metadata protection; use `sync.RWMutex` for read-heavy fields
- Use `atomic.Bool`, `atomic.Int32`, `atomic.CompareAndSwapInt32` for simple state flags
- Use `sync.Once` for one-time initialization (e.g., `Shutdown()`)
- Use `sync.WaitGroup` for goroutine lifecycle tracking
- Use `sync.Map` for concurrent maps with dynamic keys
- Use `context.Context` propagation throughout task APIs
- Tasks are closures: `type Task func(ctx context.Context)`
- SequencedTaskRunner uses lock-free `runningCount` CAS pattern (0=idle, 1=running)
- Zero-out popped slice elements (`q.tasks[0] = TaskItem{}`) to prevent memory leaks
- Drain timer channels properly with `select { case <-timer.C: default: }`

## Code Organization

- Keep packages focused: public API at root, mechanics in `core/`
- Use section separators (`// ===...===`) to divide logical sections within files
- Unexported helper functions: `ensureRunning()`, `scheduleNextIfNeeded()`, `maybeCompactLocked()`
- Short, composable functions; avoid deep nesting
- Reuse patterns across runner types (Sequenced, Parallel, SingleThread all follow same API surface)

## Testing Guidelines

- Use Go's built-in testing framework with table-driven tests where useful
- Tests in `core/` use `package core` (whitebox) or `package core_test` (blackbox with mocks)
- Use AAA pattern with BDD-style comments: `// Arrange`, `// Act`, `// Assert`
- Name tests with behavior intent: `TestSequencedTaskRunner_ShutdownDrainsQueue`
- Use `t.Helper()` in test helpers; use `t.Fatalf` / `t.Errorf` for assertions
- Use `atomic.Bool` or channels for synchronizing async test assertions
- Add regression tests for race-prone or scheduling-edge behavior when changing concurrency logic
- GC test files use `_gc_test.go` suffix for garbage collection specific tests
- Scalable stress tests are gated behind `//go:build ci` build tag
- Run `go test ./...` and `go test -tags=ci -v -coverprofile=coverage.out ./...` before opening a PR

## Commit & Pull Request Guidelines

- Follow Conventional-Commit prefixes: `feat(...)`, `fix(...)`, `test(...)`, `docs(...)`, `chore(...)`
- Keep commit subjects imperative and scoped: `feat(job-manager): add durable-ack fallback`
- PRs should include: purpose, key design/behavior changes, test evidence, linked issue(s)
- Update `README.md`, `docs/`, and relevant `examples/` in the same PR as behavior changes
