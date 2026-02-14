# Service Layer Guide

Read when implementing business logic.

## Core Patterns

### Simple Service
```go
type CreatorService struct {
    repo repository.Repository
}

func (s *CreatorService) Create(name string) (*Resource, error) {
    // 1. Validate business rules
    if s.nameExists(name) {
        return nil, ErrAlreadyExists
    }
    
    // 2. Create entity
    resource := &Resource{
        ID:   generateID(),
        Name: name,
    }
    
    // 3. Save
    if err := s.repo.Save(resource); err != nil {
        return nil, err
    }
    
    return resource, nil
}
```

### Service with Dependencies
```go
type OrderService struct {
    orderRepo    OrderRepository
    paymentSvc   PaymentService
    inventorySvc InventoryService
}

func (s *OrderService) CreateOrder(items []Item) (*Order, error) {
    // Orchestrate multiple services
    
    // 1. Reserve inventory
    reservation, err := s.inventorySvc.Reserve(items)
    if err != nil {
        return nil, err
    }
    
    // 2. Process payment
    payment, err := s.paymentSvc.Charge(total)
    if err != nil {
        s.inventorySvc.Release(reservation)  // Rollback
        return nil, err
    }
    
    // 3. Create order
    order := &Order{...}
    return s.orderRepo.Save(order)
}
```

## Error Handling
```go
var (
    ErrNotFound      = errors.New("resource not found")
    ErrAlreadyExists = errors.New("resource already exists")
    ErrPermission    = errors.New("permission denied")
)

func (s *Service) Get(id string) (*Resource, error) {
    resource := s.repo.Find(id)
    if resource == nil {
        return nil, ErrNotFound  // Use sentinel errors
    }
    return resource, nil
}
```

## Testability

Service must be testable without CLI:
```go
func TestCreatorService_Create(t *testing.T) {
    repo := &mockRepository{}
    svc := &CreatorService{repo: repo}
    
    result, err := svc.Create("test")
    
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

**More patterns:** See [patterns.md](patterns.md)
