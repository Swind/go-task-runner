---
name: golangci-lint
description: Go lint workflow and common fix patterns, aligned with this repository's CI lint scope.
tools: Read, Write, Edit, Bash, Glob, Grep
---

# golangci-lint Code Quality Guide

## Overview
golangci-lint aggregates multiple Go linters. This skill provides common issues and fixes.

## Running the Linter
```bash
# Run all linters
golangci-lint run

# Run with this repository's CI scope (matches .github/workflows/ci.yml)
golangci-lint run core/... .

# Run on specific files
golangci-lint run path/to/file.go

# Show all issues (not just new ones)
golangci-lint run --new=false

# Fix auto-fixable issues
golangci-lint run --fix

# Output as JSON for parsing
golangci-lint run --out-format=json
```

## Common Issues & Fixes

### errcheck - Unchecked errors
**Issue:** Not checking function return errors
```go
// ❌ Bad
file.Close()
defer file.Close()

// ✅ Good
if err := file.Close(); err != nil {
    log.Printf("failed to close file: %v", err)
}

// ✅ Good for defer
defer func() {
    if err := file.Close(); err != nil {
        log.Printf("failed to close file: %v", err)
    }
}()
```

### gosec - Security issues
**Issue:** Use of weak random number generator
```go
// ❌ Bad
import "math/rand"
rand.Intn(100)

// ✅ Good
import "crypto/rand"
import "math/big"
n, _ := rand.Int(rand.Reader, big.NewInt(100))
```

### ineffassign - Ineffectual assignments
**Issue:** Variable assigned but never used
```go
// ❌ Bad
func example() error {
    err := doSomething()
    err = doAnotherThing()  // previous err ignored
    return err
}

// ✅ Good
func example() error {
    if err := doSomething(); err != nil {
        return err
    }
    return doAnotherThing()
}
```

### staticcheck - Various static analysis
**Issue:** Deprecated function usage
```go
// ❌ Bad
import "io/ioutil"
ioutil.ReadFile("file.txt")

// ✅ Good
import "os"
os.ReadFile("file.txt")
```

### revive - Code style
**Issue:** Exported function without comment
```go
// ❌ Bad
func ParseConfig() {}

// ✅ Good
// ParseConfig reads and parses the application configuration
// from the default config file location.
func ParseConfig() {}
```

### govet - Suspicious constructs
**Issue:** Printf format string mismatch
```go
// ❌ Bad
fmt.Printf("value: %d", "string")

// ✅ Good
fmt.Printf("value: %s", "string")
```

## Understanding Output Format
```
path/to/file.go:10:2: Error message (linter-name)
```
- Line 10, column 2 has an issue
- Check the linter name to find fix pattern above

## Workflow

1. Run lint using repo CI scope first: `golangci-lint run core/... .`
2. Try `golangci-lint run --fix` first for auto-fixable ones
3. For manual fixes, identify linter name and apply pattern
4. Run `go test ./...` after each fix
5. Re-run linter until clean
