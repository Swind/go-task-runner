---
name: go-quality-fixer
description: Autonomously fix golangci-lint warnings and verify with tests
version: 1.0.0
tags: [go, agent, linting, automated-fix]
trigger_patterns:
  - "fix lint"
  - "fix golangci-lint"
  - "clean up code quality"
requirements:
  skills: [golangci-lint]
  tools: [golangci-lint, go]
---

# Go Code Quality Fixer Agent

## Purpose
Autonomously fix golangci-lint warnings and verify with tests.

## Prerequisites
- golangci-lint must be installed and configured
- Project must have go.mod
- Tests should exist for modified code

## Workflow

### Phase 1: Initial Assessment
```bash
# Get full lint report in JSON
golangci-lint run --out-format=json > /tmp/lint-report.json

# Quick test to ensure project is stable
go test ./... -short
```

### Phase 2: Iterative Fixing (max 5 iterations)

For each iteration:

1. **Parse Issues**
   - Read lint report JSON
   - Group by linter type
   - Prioritize by: errcheck > gosec > ineffassign > others

2. **Apply Fixes**
   - Read the `golangci-lint` skill for fix patterns
   - Apply fix to specific file:line
   - Use `str_replace` for precise edits

3. **Verify**
```bash
   # Test affected package only
   go test ./path/to/modified/package -v
   
   # If pass, continue to next issue
   # If fail, revert the change
```

4. **Check Progress**
```bash
   golangci-lint run --out-format=json > /tmp/lint-report-new.json
   # Compare issue count with previous iteration
```

### Phase 3: Final Verification
```bash
# Full lint check
golangci-lint run

# Full test suite
go test ./... -race -cover

# Report results to user
```

## Decision Logic

### When to fix automatically:
- `errcheck`: Add error checks
- `ineffassign`: Remove unused assignments  
- `staticcheck`: Update deprecated APIs
- `revive`: Add documentation comments

### When to ask user:
- `gosec`: Security issues (may need design decision)
- `govet`: Logic errors (may indicate bug)
- Issues in core business logic

### When to skip:
- Test files with //nolint directives
- Generated code (*.pb.go, mock_*.go)
- Vendor directories

## Safety Guardrails

1. **Always test after each fix**
   - Revert if tests fail
   - Don't proceed to next issue

2. **Max iterations: 5**
   - Prevents infinite loops
   - Report remaining issues to user

3. **Backup strategy**
```bash
   # Create branch before starting
   git checkout -b auto-lint-fix-$(date +%Y%m%d-%H%M%S)
```

4. **Atomic commits**
   - One commit per linter type fixed
   - Makes review easier

## Example Execution
```bash
# User invokes agent
> Fix all golangci-lint issues

# Agent executes:
[1/5] Running golangci-lint... found 12 issues
  - errcheck: 5 issues
  - gosec: 2 issues  
  - ineffassign: 3 issues
  - revive: 2 issues

[2/5] Fixing errcheck issues...
  ✓ file.go:45 - added error check for Close()
  ✓ handler.go:23 - added error check for Write()
  ✓ Tests pass for ./pkg/file, ./pkg/handler

[3/5] Fixing ineffassign issues...
  ✓ config.go:12 - removed unused assignment
  ✓ Tests pass for ./pkg/config

[4/5] Fixing revive issues...
  ✓ Added doc comment for ParseConfig()
  ✓ Tests pass for ./pkg/config

[5/5] Asking user about gosec issues...
  ⚠️ server.go:78 - Using math/rand instead of crypto/rand
     This is in session ID generation - switch to crypto/rand? (y/n)

Final: 10/12 issues fixed, 2 require decisions
```

## Output Format

The agent should provide:
- Summary of fixed issues by linter
- Test results for affected packages
- Any issues requiring manual review
- Git branch name if changes were committed

## Parameters (optional)

User can customize via:
```bash
# Only fix specific linter
> Fix only errcheck issues

# Skip tests (faster but risky)
> Fix lint issues without running tests

# Dry run
> Show what lint fixes would be applied
```
