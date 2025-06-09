# Go Streaming Server with JSON-RPC Support

A comprehensive Go-based streaming server that processes JSON-RPC requests over multiple transport protocols including HTTP/HTTPS, TCP/TLS, and WebSocket/Secure WebSocket connections.

## Features

- **Multiple Transport Protocols**: HTTP, HTTPS, TCP, TLS, WebSocket, Secure WebSocket
- **JSON-RPC 2.0 Compliance**: Full support for JSON-RPC 2.0 specification
- **Middleware System**: Functional programming approach with configurable middleware chain
- **Request Dispatching**: Efficient routing of requests to handler functions
- **Observability**: Comprehensive tracing and monitoring capabilities
- **Kafka Integration**: Middleware for logging successful requests to Kafka topics
- **Extensible Design**: Easy addition of new middleware components
- **Comprehensive Testing**: Full unit test coverage

## Architecture

### Core Components

1. **Types Package** (`pkg/types/`): Core data structures and interfaces
2. **Middleware Package** (`pkg/middleware/`): Middleware system and implementations
3. **Dispatcher Package** (`pkg/dispatcher/`): Request routing and handler management
4. **Server Package** (`pkg/server/`): Multi-protocol server implementation
5. **Handlers Package** (`pkg/handlers/`): Example JSON-RPC method handlers

### Middleware System

The middleware system uses a functional programming approach where each middleware is a function that:
- Accepts a deserialized JSON-RPC request
- Receives a context object for passing request-specific data
- Can modify the request/context before passing to the next middleware
- Can process the response after the handler executes

```go
type Middleware func(*JSONRPCRequest, *RequestContext, Handler) (*JSONRPCResponse, error)
```

### Request Context

The `RequestContext` provides:
- Request metadata (ID, transport, remote address, timing)
- Key-value storage for middleware communication
- HTTP request details (when applicable)
- Tracing span information
- Handler selection information

## Quick Start

### Prerequisites

- Go 1.21 or later
- Kafka (optional, for logging middleware)

### Installation

\\\`bash
git clone <repository-url>
cd streaming-server
go mod download
```

### Running the Server

```bash
go run cmd/server/main.go
```

The server will start on multiple ports: 
- HTTP: :8080
- HTTPS: :8443  
- TCP: :8081
- TLS: :8444
- WebSocket: :8082
- Secure WebSocket: :8445

### Testing with Example Client

```bash
go run examples/client/main.go
```

## Available JSON-RPC Methods

### echo
Echoes back the received parameters with additional metadata.

```json
{
  "jsonrpc": "2.0",
  "method": "echo",
  "params": {"message": "Hello, World!"},
  "id": 1
}
```

### time
Returns the current server time.

```json
{
  "jsonrpc": "2.0",
  "method": "time",
  "id": 2
}
```

### status
Returns server status information.

```json
{
  "jsonrpc": "2.0",
  "method": "status",
  "id": 3
}
```

### calculate
Performs basic arithmetic operations.

```json
{
  "jsonrpc": "2.0",
  "method": "calculate",
  "params": {"operation": "add", "a": 10, "b": 5},
  "id": 4
}
```

## Middleware Examples

### Logging Middleware

Logs successful HTTP requests to a Kafka topic:

```go
kafkaConfig := middleware.LoggingConfig{
    KafkaBrokers: []string{"localhost:9092"},
    Topic:        "rpc-requests",
    Enabled:      true,
}

kafkaLogger := middleware.NewKafkaLogger(kafkaConfig)
loggingMiddleware := middleware.LoggingMiddleware(kafkaLogger)
```

### Custom Middleware

```go
func CustomMiddleware() types.Middleware {
    return func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
        // Pre-processing
        ctx.WithValue("custom_data", "processed")
        
        // Call next middleware/handler
        response, err := next(req, ctx)
        
        // Post-processing
        if err == nil {
            // Log successful request
        }
        
        return response, err
    }
}
```

## Configuration

### Server Configuration

```go
config := server.Config{
    HTTPAddr:     ":8080",
    HTTPSAddr:    ":8443",
    TCPAddr:      ":8081",
    TLSAddr:      ":8444",
    WSAddr:       ":8082",
    WSSAddr:      ":8445",
    ReadTimeout:  30 * time.Second,
    WriteTimeout: 30 * time.Second,
    IdleTimeout:  60 * time.Second,
    TLSConfig:    &tls.Config{...},
}
```

### Kafka Configuration

```go
kafkaConfig := middleware.LoggingConfig{
    KafkaBrokers: []string{"localhost:9092"},
    Topic:        "rpc-requests",
    Enabled:      true,
}
```

## Testing

Run all tests:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -cover ./...
```

Run specific package tests:

```bash
go test ./pkg/middleware/
go test ./pkg/dispatcher/
go test ./pkg/types/
```

## Extending the Server

### Adding New Handlers

```go
func MyCustomHandler(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
    // Implementation
    return &types.JSONRPCResponse{
        JSONRPC: "2.0",
        Result:  "custom result",
        ID:      req.ID,
    }, nil
}

// Register the handler
server.RegisterHandler("my_method", MyCustomHandler)
```

### Adding New Middleware

```go
func RateLimitingMiddleware(limit int) types.Middleware {
    return func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
        // Rate limiting logic
        if !checkRateLimit(ctx.RemoteAddr, limit) {
            return &types.JSONRPCResponse{
                JSONRPC: "2.0",
                Error: &types.RPCError{
                    Code:    -32000,
                    Message: "Rate limit exceeded",
                },
                ID: req.ID,
            }, nil
        }
        
        return next(req, ctx)
    }
}
```

## Performance Considerations

- The server uses goroutines for handling concurrent connections
- Middleware execution is sequential within a request
- Kafka logging is asynchronous to avoid blocking request processing
- Connection pooling is handled by the underlying HTTP and WebSocket libraries
- TCP/TLS connections maintain persistent connections for multiple requests

## Security Considerations

- TLS configuration should use proper certificates in production
- Authentication middleware should be implemented for production use
- Rate limiting middleware is recommended for public-facing deployments
- Input validation should be implemented in handlers
- CORS configuration should be restricted in production

## Monitoring and Observability

The server includes built-in support for:
- Request tracing with OpenTelemetry integration points
- Metrics collection through middleware
- Structured logging to Kafka
- Request timing and metadata collection
- Error tracking and reporting
