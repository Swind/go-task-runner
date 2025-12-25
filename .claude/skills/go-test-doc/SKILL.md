---
name: go-test-doc
description: Add professional documentation to Go test files using BDD-style (Given-When-Then) comments and AAA pattern organization. Use when adding comments to tests, documenting test cases, improving test readability, or working with *_test.go files that need better documentation.
---

# Go Test Documentation Expert

Add clear, professional documentation to Golang test cases using BDD-style (Given-When-Then) comments and AAA pattern (Arrange-Act-Assert) organization. This Skill focuses on making tests readable and maintainable through excellent documentation.

## When This Skill Activates

I'll use this Skill when you:
- Ask to "add comments" or "document" test files
- Mention "Given-When-Then", "BDD", or "AAA pattern"
- Request to "improve readability" or "explain" tests
- Want to "organize" or "structure" test code
- Say "make tests clearer" or "add test descriptions"
- Work with `*_test.go` files and need documentation improvements

## Core Documentation Standards

### 1. Intent-Driven Comments (BDD Style)

Every test function starts with a clear three-part comment:
```go
// TestUserRegistration verifies the user registration workflow
// Given: A new user with valid email and password
// When: User submits registration form
// Then: Account is created and stored in database
func TestUserRegistration(t *testing.T) {
    // implementation
}
```

**Template Format:**
```go
// Test{FunctionName} verifies {what behavior is being tested}
// Given: {initial conditions or context}
// When: {the action or event that triggers behavior}
// Then: {expected outcome or result}
```

**Good Examples:**
```go
// TestCalculateDiscount verifies discount calculation logic
// Given: A shopping cart with items totaling $100
// When: A 10% discount code is applied
// Then: Final price is reduced to $90

// TestUserLogin verifies authentication with valid credentials
// Given: An existing user account in the database
// When: User provides correct username and password
// Then: Authentication succeeds and session token is returned

// TestEmailValidation verifies email format checking
// Given: Various email string formats
// When: ValidateEmail is called
// Then: Returns true for valid formats, false for invalid
```

### 2. AAA Pattern Organization

Organize test code into three clearly labeled sections:
```go
func TestCalculateTotal(t *testing.T) {
    // Arrange - Setup test data and dependencies
    items := []Item{
        {Name: "Book", Price: 10.0, Quantity: 2},
        {Name: "Pen", Price: 5.0, Quantity: 3},
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
- **Arrange**: Setup test data, create mocks, initialize dependencies
- **Act**: Call the function/method being tested (ONE clear action)
- **Assert**: Verify the results match expectations

### 3. Table-Driven Tests

Each test case needs a descriptive name:
```go
// TestEmailValidation verifies email format validation
// Given: Various email string inputs
// When: ValidateEmail is called with each input
// Then: Returns appropriate validation result for each case
func TestEmailValidation(t *testing.T) {
    tests := []struct {
        name    string  // Descriptive name explaining this test case
        input   string
        want    bool
        wantErr bool
    }{
        {
            name:    "valid email with common domain",
            input:   "user@example.com",
            want:    true,
            wantErr: false,
        },
        {
            name:    "missing @ symbol returns error",
            input:   "userexample.com",
            want:    false,
            wantErr: true,
        },
        {
            name:    "empty string is invalid",
            input:   "",
            want:    false,
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Arrange
            validator := NewEmailValidator()
            
            // Act
            got, err := validator.ValidateEmail(tt.input)
            
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

### 4. Descriptive Assertion Messages

Every assertion clearly states expected vs actual:
```go
// ✅ Good - Clear what was expected and received
if got != want {
    t.Errorf("CalculateTotal() = %v, want %v", got, want)
}

if err != nil {
    t.Errorf("CalculateTotal() unexpected error: %v", err)
}

if len(results) != expectedCount {
    t.Errorf("got %d results, want %d", len(results), expectedCount)
}

// ❌ Bad - Unclear what went wrong
if got != want {
    t.Error("wrong result")
}
```

**Assertion Templates:**
```go
// For value comparisons
t.Errorf("%s() = %v, want %v", functionName, got, want)

// For errors
t.Errorf("%s() unexpected error: %v", functionName, err)

// For nil checks
t.Errorf("%s() returned nil, want non-nil value", functionName)

// For collections
t.Errorf("got %d items, want %d", len(got), len(want))
```

## Complete Example

### Before Documentation:
```go
func TestProcess(t *testing.T) {
    result := process(5)
    if result != 25 {
        t.Error("wrong")
    }
}

func TestValidate(t *testing.T) {
    v := NewValidator()
    ok, _ := v.Validate("test@example.com")
    if !ok {
        t.Error("failed")
    }
}
```

### After Documentation:
```go
// TestProcess verifies the square calculation
// Given: An integer input value
// When: process() is called with the value
// Then: Returns the square of the input
func TestProcess(t *testing.T) {
    // Arrange
    input := 5
    want := 25
    
    // Act
    got := process(input)
    
    // Assert
    if got != want {
        t.Errorf("process(%d) = %d, want %d", input, got, want)
    }
}

// TestValidate verifies email validation with valid input
// Given: A validator instance
// When: Validate is called with a valid email address
// Then: Returns true without error
func TestValidate(t *testing.T) {
    // Arrange
    validator := NewValidator()
    validEmail := "test@example.com"
    
    // Act
    got, err := validator.Validate(validEmail)
    
    // Assert
    if err != nil {
        t.Errorf("Validate() unexpected error: %v", err)
    }
    if !got {
        t.Errorf("Validate(%q) = false, want true", validEmail)
    }
}
```

## Output Format

When documenting test files, I'll provide:

1. **Summary of Changes**
```
   Documentation Added:
   - Added Given-When-Then comments to X test functions
   - Organized code with AAA pattern labels
   - Improved Y assertion messages
   - Added descriptive names to Z table test cases
```

2. **Refactored Code**
   - Complete, properly formatted code with all documentation

3. **Verification Command**
```bash
   go test -v ./...
```

## Documentation Checklist

- [ ] Every test has Given-When-Then comment block
- [ ] AAA sections (Arrange-Act-Assert) clearly marked
- [ ] Table-driven test cases have descriptive names
- [ ] Assertion messages explain expected vs received
- [ ] Test function names indicate what is tested
- [ ] Comments explain "why" not just "what"

## Best Practices

### Simple Tests
Keep it straightforward with clear structure:
```go
// TestAdd verifies basic addition operation
// Given: Two positive integers
// When: Add function is called
// Then: Returns the sum of both integers
func TestAdd(t *testing.T) {
    // Arrange
    a, b := 2, 3
    want := 5
    
    // Act
    got := Add(a, b)
    
    // Assert
    if got != want {
        t.Errorf("Add(%d, %d) = %d, want %d", a, b, got, want)
    }
}
```

### Complex Tests with Setup
Document setup steps clearly:
```go
// TestDatabaseQuery verifies user retrieval from database
// Given: A database with existing user records
// When: FindUserByEmail is called with a valid email
// Then: Returns the correct user object
func TestDatabaseQuery(t *testing.T) {
    // Arrange - Setup database and test data
    db := setupTestDB(t)
    testEmail := "user@example.com"
    testUser := User{Email: testEmail, Name: "Test User"}
    insertTestUser(t, db, testUser)
    repo := NewUserRepository(db)
    
    // Act - Execute the query
    got, err := repo.FindUserByEmail(testEmail)
    
    // Assert - Verify results
    if err != nil {
        t.Fatalf("FindUserByEmail() unexpected error: %v", err)
    }
    if got.Email != testUser.Email {
        t.Errorf("got email %q, want %q", got.Email, testUser.Email)
    }
}
```

### Subtests
Organize related tests:
```go
// TestUserOperations verifies user management operations
// Given: A user repository with test data
// When: Different operations are performed
// Then: Each operation produces expected results
func TestUserOperations(t *testing.T) {
    // Arrange - Common setup
    db := setupTestDB(t)
    repo := NewUserRepository(db)
    
    t.Run("create new user succeeds", func(t *testing.T) {
        // Arrange
        newUser := User{Email: "new@example.com"}
        
        // Act
        err := repo.Create(newUser)
        
        // Assert
        if err != nil {
            t.Errorf("Create() unexpected error: %v", err)
        }
    })
}
```

## Notes

- Focus on **readability** - tests document code for future developers
- Comments explain **intent**, not just repeat code
- Keep assertions **informative** for debugging failed tests
- Use **consistent formatting** across all test files
- AAA pattern helps readers understand test structure quickly
- Given-When-Then provides context without reading implementation
