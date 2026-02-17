# Repository Guidelines

## Project Structure & Module Organization
- Root package (`pool.go`, `types.go`, `doc.go`) exposes the public task-runner API.
- `core/` contains internal scheduler, queue, runner, and job-manager implementations.
- `examples/` includes runnable demos (each folder has a `main.go`) for patterns like sequencing, delayed tasks, and task/reply.
- `docs/` holds architecture and subsystem docs (`DESIGN.md`, `JOB_MANAGER.md`).
- Tests live beside code as `*_test.go` in both root and `core/`.

## Build, Test, and Development Commands
- `go mod download`: install/update module dependencies.
- `go test ./...`: run all unit tests locally.
- `go test -tags=ci -v -coverprofile=coverage.out ./...`: match CI behavior and generate coverage data.
- `go tool cover -func=coverage.out`: print per-package and total coverage.
- `golangci-lint run --timeout=5m core/... .`: run the same lint scope as CI.
- `find examples -name "main.go" | sort | xargs -n1 go run`: execute all examples.

## Coding Style & Naming Conventions
- Follow standard Go formatting: run `gofmt` on changed files before committing.
- Keep exported identifiers in `PascalCase`, unexported in `camelCase`; prefer clear, domain-specific names (`TaskRunner`, `JobManager`, `DelayManager`).
- Keep packages focused; put reusable API at root package and lower-level mechanics in `core/`.
- Favor small, composable functions and explicit context propagation (`context.Context`) in task APIs.

## Testing Guidelines
- Use Go’s built-in testing framework with table-driven tests where useful.
- Name tests with behavior intent (e.g., `TestSequencedTaskRunner_ShutdownDrainsQueue`).
- Add regression tests for race-prone or scheduling-edge behavior when changing concurrency logic.
- Run `go test ./...` and CI-aligned `go test -tags=ci -v -coverprofile=coverage.out ./...` before opening a PR.

## Commit & Pull Request Guidelines
- Follow Conventional-Commit-like prefixes seen in history: `feat(...)`, `fix(...)`, `test(...)`, `docs(...)`, `chore(...)`.
- Keep commit subjects imperative and scoped (example: `feat(job-manager): add durable-ack fallback`).
- PRs should include: purpose, key design/behavior changes, test evidence (commands run), and linked issue(s).
- If behavior changes affect examples or docs, update `README.md`, `docs/`, and relevant `examples/` in the same PR.
