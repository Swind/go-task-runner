---
name: go-clean-architecture
description: Reference guide for applying Clean Architecture/DDD to service-oriented Go apps; not a mandatory target for library repositories.
tools: Read, Write, Edit, Bash, Glob, Grep
---

## Applicability

Use this skill for application/service codebases with clear business domains.

Do NOT apply this structure blindly to:
- Framework/library repositories
- Small utility packages
- Repos where public API stability is more important than layered decomposition

For this repository (`go-task-runner`), treat this skill as reference only, not a mandatory migration target.

## Architecture Layers

### 1. Domain Layer (Entities)
- Pure business logic
- No external dependencies
- Location: `internal/domain/`

Checklist:
- [ ] Entities defined with business rules
- [ ] Value objects created for domain concepts
- [ ] Domain services for complex logic
- [ ] Repository interfaces defined

### 2. Application Layer (Use Cases)
- Application business rules
- Orchestrates domain objects
- Location: `internal/usecase/`

Checklist:
- [ ] Use cases implement single responsibility
- [ ] Input/output ports defined
- [ ] Transaction boundaries clear
- [ ] Error handling comprehensive

### 3. Interface Adapters Layer
- Controllers, presenters, gateways
- Location: `internal/handler/`, `internal/repository/`

### 4. Infrastructure Layer
- External dependencies
- Location: `internal/infrastructure/`

## DDD Patterns

### Aggregates
Design principles:
- Small aggregates preferred
- Consistency boundary explicit
- Reference by ID only

### Domain Events
Implementation:
- Event naming: past tense
- Payload: domain objects
- Location: `internal/domain/event/`

## Workflow

When invoked:
1. Analyze requirements for domain boundaries
2. Identify aggregates and entities
3. Design use case flow
4. Implement layer by layer
5. Verify dependency rules

## Dependency Rules

Critical: Dependencies only point inward
- Domain ← Application ← Interface ← Infrastructure
- Use dependency injection
- Interfaces in inner layers

## Code Organization
```
internal/
├── domain/          # Entities, value objects, domain services
├── usecase/         # Application business rules
├── handler/         # HTTP/gRPC handlers
├── repository/      # Data access implementations
└── infrastructure/  # External services, DB, etc.
```

## Quality Checks

Architecture validation:
- [ ] No domain dependencies on outer layers
- [ ] Repository interfaces in domain
- [ ] Use cases testable without infrastructure
- [ ] Clear bounded contexts
