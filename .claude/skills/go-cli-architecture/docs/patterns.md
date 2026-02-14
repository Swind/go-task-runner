# Advanced Patterns

## Dependency Injection
```go
// internal/app/app.go
type App struct {
    Repo   repository.Repository
    Logger logger.Logger
    Config *config.Config
}

func New() *App {
    return &App{
        Repo:   repository.NewPostgres(),
        Logger: logger.New(),
        Config: config.Load(),
    }
}

// cmd/create.go
var app *app.App

func createAction(c *cli.Context) error {
    svc := service.NewCreator(app.Repo, app.Logger)
    return svc.Create(c.String("name"))
}

// main.go
func main() {
    app := app.New()
    cmd.SetApp(app)  // Inject
    
    cli := cmd.NewApp()
    cli.Run(os.Args)
}
```

## Configuration
```go
type Config struct {
    DatabaseURL string
    APIKey      string
    LogLevel    string
}

func LoadConfig(path string) (*Config, error) {
    // Load from file/env
}

// Use in service
func NewService(cfg *Config) *Service {
    return &Service{
        db: connectDB(cfg.DatabaseURL),
    }
}
```

## Output Formatters
```go
type Formatter interface {
    Format(data interface{}) error
}

type JSONFormatter struct{}
type TableFormatter struct{}

func GetFormatter(format string) Formatter {
    switch format {
    case "json":
        return &JSONFormatter{}
    default:
        return &TableFormatter{}
    }
}

// In command
func listAction(c *cli.Context) error {
    data, _ := svc.List()
    
    formatter := GetFormatter(c.String("format"))
    return formatter.Format(data)
}
```
