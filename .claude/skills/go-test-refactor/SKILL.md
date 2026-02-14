# Go Test Refactoring Expert

## Description
Professional Golang test code refactoring skill that transforms existing tests into clean, maintainable, and reliable test suites following industry best practices and TDD principles.

## When to Use This Skill
Claude should automatically apply this skill when:
- User asks to "refactor", "improve", or "review" Go test files
- User mentions "add comments to tests" or "document tests"
- User asks to "fix flaky tests" or "remove time.Sleep"
- User requests "clean up test code" or "make tests more readable"
- Working with files ending in `_test.go` and the user asks for test quality improvements
- User mentions "AAA pattern", "BDD", or "test structure"

## Core Refactoring Standards

### 1. Intent-Driven Comments (BDD Style)

Every test function MUST start with a clear intent comment using Given-When-Then format:
```go
// TestUserRegistration verifies the complete user registration workflow
// Given: A new user with valid email and password
// When: The registration endpoint is called
// Then: User account is created and confirmation email is sent
func TestUserRegistration(t *testing.T) {
    // implementation
}
```

**Format Template:**
```go
// Test{FunctionName} verifies {what behavior is being tested}
// Given: {initial conditions or context}
// When: {the action or trigger}
// Then: {expected outcome or behavior}
```

### 2. AAA Pattern (Arrange-Act-Assert)

Organize ALL test code into three clearly marked sections:
```go
func TestCalculateTotal(t *testing.T) {
    // Arrange - Setup test data and dependencies
    items := []Item{
        {Price: 10.0, Quantity: 2},
        {Price: 5.0, Quantity: 3},
    }
    calculator := NewPriceCalculator()
    
    // Act - Execute the function under test
    total := calculator.CalculateTotal(items)
    
    // Assert - Verify expected outcomes
    want := 35.0
    if total != want {
        t.Errorf("CalculateTotal() = %v, want %v", total, want)
    }
}
```

**Section Guidelines:**
- **Arrange**: Setup inputs, mocks, dependencies, test data
- **Act**: Call the function/method being tested (ideally ONE action)
- **Assert**: Verify results, check expectations

### 3. Eliminate Flaky Tests

Prefer not to use `time.Sleep` for synchronization when a deterministic primitive is available.
Use `WaitGroup`, channels, or context-based signals first.

Exception: timing/scheduler behavior tests may use bounded `time.Sleep` with:
- clear rationale in comments
- timeout guard (`select` + `time.After` or `context.WithTimeout`)
- stable upper bounds to avoid flaky CI behavior

#### ❌ Bad (Flaky):
```go
func TestAsyncOperation(t *testing.T) {
    go processAsync()
    time.Sleep(100 * time.Millisecond) // Unreliable!
    // check results
}
```

#### ✅ Good (Reliable):

**Using WaitGroup:**
```go
func TestAsyncOperation(t *testing.T) {
    // Arrange
    var wg sync.WaitGroup
    wg.Add(1)
    
    // Act
    go func() {
        defer wg.Done()
        processAsync()
    }()
    
    // Assert
    wg.Wait()
    // verify results
}
```

**Using Channels:**
```go
func TestAsyncOperation(t *testing.T) {
    // Arrange
    done := make(chan bool)
    
    // Act
    go func() {
        processAsync()
        done <- true
    }()
    
    // Assert
    <-done
    // verify results
}
```

**Using Context:**
```go
func TestAsyncOperation(t *testing.T) {
    // Arrange
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // Act
    result := processWithContext(ctx)
    
    // Assert
    // verify results
}
```

### 4. Robust Concurrency with Timeouts

ALL asynchronous tests MUST have timeout protection:
```go
func TestAsyncWithTimeout(t *testing.T) {
    // Arrange
    resultChan := make(chan Result)
    
    // Act
    go func() {
        result := expensiveOperation()
        resultChan <- result
    }()
    
    // Assert
    select {
    case result := <-resultChan:
        if result.Value != expected {
            t.Errorf("got %v, want %v", result.Value, expected)
        }
    case <-time.After(5 * time.Second):
        t.Fatal("Test timed out after 5 seconds - possible deadlock")
    }
}
```

**Timeout Best Practices:**
- Use reasonable timeouts (typically 1-10 seconds)
- Always explain WHY a timeout occurred in error messages
- Consider using `context.WithTimeout` for complex scenarios

### 5. Descriptive Assertions

Every assertion MUST clearly state what was expected vs. what was received:

#### ❌ Bad (Unclear):
```go
if result != expected {
    t.Error("wrong result")
}
```

#### ✅ Good (Clear):
```go
if result != expected {
    t.Errorf("ProcessOrder() returned %v, want %v", result, expected)
}
```

**Assertion Templates:**
```go
// For simple values
t.Errorf("%s() = %v, want %v", funcName, got, want)

// For errors
t.Errorf("%s() error = %v, wantErr %v", funcName, err, wantErr)

// For complex structures
t.Errorf("%s() = %+v, want %+v", funcName, got, want)

// For fatal errors (stops test immediately)
t.Fatalf("Setup failed: %v", err)
```

## Advanced Patterns

### Table-Driven Tests

Use for testing multiple scenarios of the same function:
```go
// TestValidateEmail verifies email validation logic
// Given: Various email format inputs
// When: ValidateEmail is called
// Then: Returns appropriate validation result
func TestValidateEmail(t *testing.T) {
    tests := []struct {
        name    string
        email   string
        want    bool
        wantErr bool
    }{
        {
            name:    "valid email",
            email:   "user@example.com",
            want:    true,
            wantErr: false,
        },
        {
            name:    "missing @ symbol",
            email:   "userexample.com",
            want:    false,
            wantErr: true,
        },
        {
            name:    "empty email",
            email:   "",
            want:    false,
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Arrange
            validator := NewEmailValidator()
            
            // Act
            got, err := validator.ValidateEmail(tt.email)
            
            // Assert
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateEmail() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("ValidateEmail() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Test Helpers

Extract common setup/teardown logic:
```go
// setupTestDB creates a test database connection
func setupTestDB(t *testing.T) *sql.DB {
    t.Helper() // Mark as helper to improve error reporting
    
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("Failed to create test database: %v", err)
    }
    
    t.Cleanup(func() {
        db.Close()
    })
    
    return db
}

func TestDatabaseOperation(t *testing.T) {
    // Arrange
    db := setupTestDB(t)
    repo := NewRepository(db)
    
    // Act & Assert
    // ... test implementation
}
```

### Cleanup Management

Use `t.Cleanup()` instead of `defer` for better test isolation:
```go
func TestFileOperation(t *testing.T) {
    // Arrange
    tempFile, err := os.CreateTemp("", "test-*.txt")
    if err != nil {
        t.Fatalf("Failed to create temp file: %v", err)
    }
    
    // Register cleanup (runs even if test fails)
    t.Cleanup(func() {
        os.Remove(tempFile.Name())
    })
    
    // Act
    err = writeToFile(tempFile.Name(), "test data")
    
    // Assert
    if err != nil {
        t.Errorf("writeToFile() error = %v", err)
    }
}
```

## Refactoring Checklist

When refactoring a test file, ensure ALL of these items are addressed:

- [ ] **Intent Comments**: Every test has Given-When-Then comment
- [ ] **AAA Sections**: Arrange, Act, Assert clearly marked
- [ ] **Sleep usage justified**: Replaced with deterministic sync OR documented as timing-test necessity
- [ ] **Timeout Protection**: Async operations have timeout guards
- [ ] **Descriptive Errors**: All assertions explain expected vs. actual
- [ ] **Table-Driven**: Multiple scenarios use table-driven approach
- [ ] **Test Helpers**: Common setup extracted and marked with `t.Helper()`
- [ ] **Cleanup**: Resources cleaned with `t.Cleanup()` not `defer`
- [ ] **Subtests**: Related tests grouped with `t.Run()`
- [ ] **Race Detection**: Code is safe for `go test -race`

## Output Format

When refactoring tests, provide:

1. **Summary of Improvements**: Brief bullet points of key changes
```
   Improvements Made:
   - Added BDD-style intent comments to all 5 test functions
   - Replaced 3 instances of time.Sleep with WaitGroup
   - Added timeout protection to async tests
   - Converted TestValidation into table-driven test (8 cases)
   - Improved assertion messages for clarity
```

2. **Refactored Code**: Complete, formatted code with proper spacing

3. **Testing Recommendation**: Suggest running tests
```
   Run the following to verify:
   go test -v -race ./...
```

## Examples

### Before Refactoring:
```go
func TestProcess(t *testing.T) {
    ch := make(chan int)
    go func() {
        ch <- process()
    }()
    time.Sleep(100 * time.Millisecond)
    result := <-ch
    if result != 42 {
        t.Error("wrong")
    }
}
```

### After Refactoring:
```go
// TestProcess verifies the async processing workflow
// Given: A processing task is initiated
// When: Process() is called asynchronously
// Then: Returns expected result within timeout period
func TestProcess(t *testing.T) {
    // Arrange
    resultChan := make(chan int, 1)
    expected := 42
    
    // Act
    go func() {
        result := process()
        resultChan <- result
    }()
    
    // Assert
    select {
    case got := <-resultChan:
        if got != expected {
            t.Errorf("process() = %d, want %d", got, expected)
        }
    case <-time.After(2 * time.Second):
        t.Fatal("process() timed out after 2 seconds - possible deadlock or performance issue")
    }
}
```

## Common Anti-Patterns to Fix

1. **Magic Numbers**: Replace with named constants
2. **God Tests**: Split large tests into focused subtests
3. **Hidden Dependencies**: Make all dependencies explicit in Arrange
4. **Assertion Roulette**: Multiple assertions without clear context
5. **Test Interdependence**: Tests that rely on execution order

## Notes

- This skill focuses on TEST CODE quality, not production code
- Always maintain the original test's intent and coverage
- Suggest additional test cases if obvious gaps are found
- Prioritize readability over brevity
- Ensure refactored tests are race-detector safe (`go test -race`)
