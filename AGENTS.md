# AGENTS.md

## Build / Test Commands
- `go test ./...` – run all tests.
- `go test -run TestName ./...` – run a single test.
- `go test -v ./...` – verbose output.
- `go build ./...` – compile the module.
- `go vet ./...` – static analysis.
- `go fmt ./...` – format source.

## Code Style Guidelines
- Use `gofmt`‑styled indentation and spacing.
- Group imports: standard, blank line, third‑party.
- Exported identifiers start with a capital letter and have a comment.
- Prefer `camelCase` for locals, `PascalCase` for exported names.
- Return errors, wrap with `fmt.Errorf("msg: %w", err)`.
- Pass `context.Context` as the first argument to functions that block.
- Prefer interfaces for abstractions; avoid globals.
- Use zero‑allocation patterns (e.g., slice reuse in queues).
- Concurrency: use atomic operations as shown in `sequenced_task_runner.go`.
