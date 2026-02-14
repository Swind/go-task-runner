---
name: go-cli-architecture
description: For Go CLI applications using urfave/cli with command/service separation; not intended for non-CLI library repositories.
tools: Read, Write, Edit, Bash, Grep
---

# Go CLI Architecture

Build Go CLI with proper layer separation.

## Core Rules
```
CLI Layer (cmd/)     → Flags, validation, output ONLY
    ↓
Service Layer        → ALL business logic
```

**3 Golden Rules:**
1. CLI has ZERO business logic
2. Service contains ALL implementation  
3. CLI: parse → validate → call service → format

## Directory Structure
```
project/
├── cmd/
│   ├── app.go              # CLI app setup
│   │
│   ├── create/             # Create subcommand
│   │   └── create.go
│   │
│   ├── list/               # List subcommand
│   │   └── list.go
│   │
│   ├── delete/             # Delete subcommand
│   │   └── delete.go
│   │
│   └── resource/           # Resource command group
│       ├── resource.go     # Parent command
│       ├── create.go       # resource create
│       ├── list.go         # resource list
│       └── update.go       # resource update
│
├── internal/
│   ├── service/            # Business logic
│   │   ├── creator.go
│   │   ├── lister.go
│   │   └── deleter.go
│   │
│   ├── domain/             # Domain models
│   │   └── resource.go
│   │
│   └── repository/         # Data access
│       └── repository.go
│
└── main.go
```

**Why this structure?**
- Each subcommand isolated in own folder
- Easy to find related files
- Clean separation when commands grow
- Support nested command groups

## Quick Start Templates

### App Setup
```go
// cmd/app.go
package cmd

import (
    "myapp/cmd/create"
    "myapp/cmd/list"
    "myapp/cmd/delete"
    "github.com/urfave/cli/v2"
)

func NewApp() *cli.App {
    return &cli.App{
        Name:  "myapp",
        Usage: "My application",
        Commands: []*cli.Command{
            create.Command(),
            list.Command(),
            delete.Command(),
        },
    }
}
```

### Minimal Command
```go
// cmd/create/create.go
package create

import (
    "fmt"
    "github.com/urfave/cli/v2"
    "myapp/internal/service"
)

func Command() *cli.Command {
    return &cli.Command{
        Name:  "create",
        Usage: "Create resource",
        Flags: []cli.Flag{
            &cli.StringFlag{
                Name:     "name",
                Required: true,
            },
        },
        Action: action,
    }
}

func action(c *cli.Context) error {
    // 1. Get flags
    name := c.String("name")
    
    // 2. Validate (format only)
    if len(name) < 3 {
        return cli.Exit("name must be 3+ chars", 1)
    }
    
    // 3. Call service
    svc := service.NewCreatorService()
    result, err := svc.Create(name)
    if err != nil {
        return cli.Exit(fmt.Sprintf("Failed: %v", err), 1)
    }
    
    // 4. Format output
    fmt.Printf("✓ Created: %s\n", result.ID)
    return nil
}
```

### Command Group (Nested)
```go
// cmd/resource/resource.go
package resource

import (
    "github.com/urfave/cli/v2"
)

func Command() *cli.Command {
    return &cli.Command{
        Name:  "resource",
        Usage: "Manage resources",
        Subcommands: []*cli.Command{
            CreateCommand(),
            ListCommand(),
            UpdateCommand(),
        },
    }
}

// cmd/resource/create.go
package resource

func CreateCommand() *cli.Command {
    return &cli.Command{
        Name:   "create",
        Usage:  "Create a resource",
        Action: createAction,
    }
}

// Usage: myapp resource create
```

### Main Entry
```go
// main.go
package main

import (
    "log"
    "os"
    "myapp/cmd"
)

func main() {
    app := cmd.NewApp()
    
    if err := app.Run(os.Args); err != nil {
        log.Fatal(err)
    }
}
```

## When to Read More

**Simple CRUD?** → Use templates above ✓

**Need more?** → Read specific doc (paths are relative to this skill directory):
```bash
# Multiple flags, validation
docs/cli-layer.md

# Complex business logic
docs/service-layer.md

# Testing
docs/testing.md

# Patterns (DI, formatting, config)
docs/patterns.md
```

## Decision Tree
```
Task?
├─ Simple command          → Use templates above
├─ Command with subcommands → See "Command Group" template
├─ Multiple flags          → Read cli-layer.md
├─ Output formats         → Read cli-layer.md
├─ Complex logic          → Read service-layer.md
├─ Testing                → Read testing.md
└─ DI / Config            → Read patterns.md
```

## Quick Checklist

Before commit:
- [ ] CLI has no business logic (no if/else decisions)
- [ ] Service doesn't import `urfave/cli`
- [ ] Flags validated (format only)
- [ ] Service testable independently
- [ ] Each command in own folder

## Common Mistakes

**❌ Bad: Logic in CLI**
```go
func action(c *cli.Context) error {
    if resourceType == "premium" {
        quota = 1000  // Business logic!
    }
}
```

**✅ Good: Logic in Service**
```go
func action(c *cli.Context) error {
    svc.Create(resourceType)  // Service decides quota
}
```

**❌ Bad: All commands in one file**
```
cmd/
└── commands.go  // 1000+ lines!
```

**✅ Good: Each command in own folder**
```
cmd/
├── create/
│   └── create.go
└── list/
    └── list.go
```

## Integration

- Works with `backend-developer` for services
- Works with `go-clean-architecture` for structure
- Works with `test-automator` for tests

---

**Remember:** 80% tasks use templates. Read docs only when needed.
