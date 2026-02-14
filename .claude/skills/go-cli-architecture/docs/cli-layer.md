# CLI Layer Guide

Read when you need beyond basic commands.

## Table of Contents

1. [Multiple Flags](#multiple-flags)
2. [Subcommands](#subcommands)
3. [Output Formats](#output-formats)
4. [Flag Validation](#flag-validation)
5. [Arguments](#arguments)
6. [Confirmation Prompts](#confirmation-prompts)
7. [Exit Codes](#exit-codes)

## Multiple Flags

### Basic Flags
```go
Flags: []cli.Flag{
    &cli.StringFlag{
        Name:     "name",
        Aliases:  []string{"n"},
        Required: true,
    },
    &cli.StringFlag{
        Name:    "type",
        Value:   "default",  // Default value
    },
    &cli.BoolFlag{
        Name:    "force",
        Aliases: []string{"f"},
    },
    &cli.IntFlag{
        Name:  "count",
        Value: 10,
    },
}
```

### Flag Types
```go
// Duration
&cli.DurationFlag{
    Name:  "timeout",
    Value: 30 * time.Second,
}
// Usage: c.Duration("timeout")

// StringSlice (repeatable)
&cli.StringSliceFlag{
    Name: "tags",
}
// Usage: myapp create --tags=a --tags=b
// Get: c.StringSlice("tags") → []string{"a", "b"}

// Int64Slice
&cli.Int64SliceFlag{
    Name: "ids",
}
```

### Environment Variables
```go
&cli.StringFlag{
    Name:    "api-key",
    EnvVars: []string{"API_KEY", "MYAPP_API_KEY"},
}
// Reads from $API_KEY or $MYAPP_API_KEY if flag not provided
```

## Subcommands

### Nested Commands
```go
func ResourceCommand() *cli.Command {
    return &cli.Command{
        Name:  "resource",
        Usage: "Manage resources",
        Subcommands: []*cli.Command{
            {
                Name:   "create",
                Usage:  "Create resource",
                Action: createAction,
            },
            {
                Name:   "list",
                Usage:  "List resources",
                Action: listAction,
            },
        },
    }
}

// Usage:
// myapp resource create
// myapp resource list
```

### Global Flags
```go
// cmd/cli.go
func NewApp() *cli.App {
    return &cli.App{
        Name: "myapp",
        Flags: []cli.Flag{
            &cli.BoolFlag{
                Name: "verbose",  // Available to all commands
            },
        },
        Commands: []*cli.Command{
            CreateCommand(),
        },
    }
}

// Access in any command
func action(c *cli.Context) error {
    verbose := c.Bool("verbose")  // Global flag
}
```

## Output Formats

### Multiple Formats
```go
Flags: []cli.Flag{
    &cli.StringFlag{
        Name:  "format",
        Value: "table",
        Usage: "Output format (table|json|yaml)",
    },
}

func listAction(c *cli.Context) error {
    svc := service.NewLister()
    data, _ := svc.List()
    
    // CLI: Format based on flag
    switch c.String("format") {
    case "json":
        return printJSON(data)
    case "yaml":
        return printYAML(data)
    default:
        return printTable(data)
    }
}

func printJSON(data interface{}) error {
    enc := json.NewEncoder(os.Stdout)
    enc.SetIndent("", "  ")
    return enc.Encode(data)
}

func printTable(items []*Item) error {
    w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "ID\tNAME\tSTATUS")
    for _, item := range items {
        fmt.Fprintf(w, "%s\t%s\t%s\n", item.ID, item.Name, item.Status)
    }
    return w.Flush()
}
```

## Flag Validation

### Format Validation
```go
func validateAction(c *cli.Context) error {
    email := c.String("email")
    
    // Email format
    if _, err := mail.ParseAddress(email); err != nil {
        return cli.Exit("Invalid email format", 1)
    }
    
    // URL format
    url := c.String("url")
    if _, err := url.Parse(url); err != nil {
        return cli.Exit("Invalid URL", 1)
    }
    
    // Range validation
    count := c.Int("count")
    if count < 1 || count > 100 {
        return cli.Exit("count must be 1-100", 1)
    }
    
    return nil
}
```

### Flag Dependencies
```go
Before: func(c *cli.Context) error {
    // Mutually exclusive
    if c.IsSet("source") && c.IsSet("stdin") {
        return fmt.Errorf("cannot use both --source and --stdin")
    }
    
    // Required together
    if c.IsSet("username") && !c.IsSet("password") {
        return fmt.Errorf("--password required with --username")
    }
    
    // Conditional required
    if c.String("format") == "csv" && !c.IsSet("delimiter") {
        return fmt.Errorf("--delimiter required for CSV format")
    }
    
    return nil
}
```

## Arguments

### Positional Arguments
```go
func DeleteCommand() *cli.Command {
    return &cli.Command{
        Name:      "delete",
        Usage:     "Delete resource",
        ArgsUsage: "<id>",  // Show in help
        
        Action: func(c *cli.Context) error {
            // Validate arg count
            if c.NArg() != 1 {
                return cli.Exit("resource ID required", 1)
            }
            
            // Get argument
            id := c.Args().Get(0)
            
            // Call service
            return service.Delete(id)
        },
    }
}
```

### Multiple Arguments
```go
ArgsUsage: "<source> <target>"

func action(c *cli.Context) error {
    if c.NArg() != 2 {
        return cli.Exit("source and target required", 1)
    }
    
    source := c.Args().Get(0)
    target := c.Args().Get(1)
    
    // Or get all
    args := c.Args().Slice()
}
```

## Confirmation Prompts
```go
func deleteAction(c *cli.Context) error {
    id := c.Args().Get(0)
    
    // Skip if --force
    if !c.Bool("force") {
        if !confirm(fmt.Sprintf("Delete %s?", id)) {
            fmt.Println("Cancelled")
            return nil
        }
    }
    
    return service.Delete(id)
}

func confirm(msg string) bool {
    reader := bufio.NewReader(os.Stdin)
    fmt.Printf("%s (y/N): ", msg)
    
    response, _ := reader.ReadString('\n')
    response = strings.ToLower(strings.TrimSpace(response))
    
    return response == "y" || response == "yes"
}
```

## Exit Codes
```go
const (
    ExitSuccess      = 0
    ExitError        = 1
    ExitInvalidInput = 2
    ExitNotFound     = 3
    ExitPermission   = 4
)

func getAction(c *cli.Context) error {
    resource, err := svc.Get(id)
    
    if errors.Is(err, service.ErrNotFound) {
        return cli.Exit("Not found", ExitNotFound)
    }
    
    if errors.Is(err, service.ErrPermission) {
        return cli.Exit("Permission denied", ExitPermission)
    }
    
    if err != nil {
        return cli.Exit(err.Error(), ExitError)
    }
    
    fmt.Println(resource)
    return nil
}
```

## Command Hooks

### Before Hook
```go
func BackupCommand() *cli.Command {
    return &cli.Command{
        Name: "backup",
        
        Before: func(c *cli.Context) error {
            // Setup, validation
            if !checkPrerequisites() {
                return fmt.Errorf("prerequisites not met")
            }
            return nil
        },
        
        Action: backupAction,
        
        After: func(c *cli.Context) error {
            // Cleanup
            cleanup()
            return nil
        },
    }
}
```

## Best Practices

### Flag Naming
```go
// ✅ Good
&cli.StringFlag{Name: "output", Aliases: []string{"o"}}
&cli.BoolFlag{Name: "force", Aliases: []string{"f"}}
&cli.StringFlag{Name: "batch-size"}  // kebab-case

// ❌ Bad
&cli.StringFlag{Name: "o"}           // No full name
&cli.StringFlag{Name: "batchSize"}   // camelCase
```

### Error Messages
```go
// ✅ Good: Specific and actionable
return cli.Exit("Config file not found at ~/.myapp/config.yml. Run 'myapp init' to create.", 1)

// ❌ Bad: Vague
return cli.Exit("Error", 1)

// ✅ Good: Include context
return cli.Exit(fmt.Sprintf("Failed to connect to %s: %v", host, err), 1)
```

### Help Text
```go
&cli.Command{
    Name:  "create",
    Usage: "Create a new resource",  // Short description
    Description: `Longer description with details.

Examples:
    myapp create --name test
    myapp create --name prod --type premium`,
    
    Flags: []cli.Flag{
        &cli.StringFlag{
            Name:  "name",
            Usage: "Resource name (alphanumeric, 3-50 chars)",  // Helpful
        },
    },
}
```
