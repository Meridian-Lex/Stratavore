# Go Architecture

## Ports and Adapters

All external integrations implement a Go interface. New backends require no core changes.

```go
// Define interface in package (backends/interface.go pattern)
type ModelBackend interface {
    Name() string
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    ListModels(ctx context.Context) ([]ModelInfo, error)
    Ping(ctx context.Context) error
}
```

- Interface lives in the package it belongs to (`backends/interface.go`)
- Each implementation in its own file (`messages_api.go`, `ollama.go`)
- Registry maps name to implementation

## Constructor Options Structs

Use options structs for constructors with 3+ parameters. Never long positional arg lists.

```go
type HTTPServerOptions struct {
    Port         int
    Handler      *GRPCServer
    Logger       *zap.Logger
    Fleet        *FleetHandler
    KnowledgeSvc *knowledge.Service // optional; nil disables feature
}

func NewHTTPServer(opts HTTPServerOptions) *HTTPServer { ... }
```

- Optional fields documented with inline comment `// optional; nil disables X`

## Error Wrapping

Always wrap errors with context using `%w`. Never discard errors silently.

```go
return nil, fmt.Errorf("get sprint: %w", err)
return nil, fmt.Errorf("create pool: %w", err)
```

- Prefix describes the operation, not the error type
- Chain readable: `"parse connection string: %w"` not `"error: %w"`

## Logger Injection

`*zap.Logger` injected into every struct via constructor. Never use global logger.

```go
type SprintExecutor struct {
    db     *storage.PostgresClient
    router *dispatch.TierRouter
    logger *zap.Logger
}
```
