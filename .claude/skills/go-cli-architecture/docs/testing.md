# Testing Guide

## CLI Tests

Test flag validation and output formatting:
```go
func TestCreateCommand(t *testing.T) {
    app := cmd.NewApp()
    
    err := app.Run([]string{"myapp", "create", "--name", "test"})
    
    assert.NoError(t, err)
}

func TestValidation(t *testing.T) {
    tests := []struct {
        name    string
        args    []string
        wantErr bool
    }{
        {"valid", []string{"--name", "test"}, false},
        {"too short", []string{"--name", "ab"}, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := app.Run(append([]string{"myapp", "create"}, tt.args...))
            if (err != nil) != tt.wantErr {
                t.Errorf("unexpected error: %v", err)
            }
        })
    }
}
```

## Service Tests

Test business logic only:
```go
func TestCreatorService_Create(t *testing.T) {
    repo := &mockRepository{}
    repo.On("FindByName", "test").Return(nil, nil)
    repo.On("Save", mock.Anything).Return(nil)
    
    svc := &CreatorService{repo: repo}
    
    result, err := svc.Create("test")
    
    assert.NoError(t, err)
    assert.NotNil(t, result)
    repo.AssertExpectations(t)
}
```

## Mock Repository
```go
type mockRepository struct {
    mock.Mock
}

func (m *mockRepository) Save(r *Resource) error {
    args := m.Called(r)
    return args.Error(0)
}

func (m *mockRepository) FindByName(name string) (*Resource, error) {
    args := m.Called(name)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*Resource), args.Error(1)
}
```
