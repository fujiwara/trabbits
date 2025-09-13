# trabbits Project Instructions for Claude Code

## Project Overview
This is trabbits, an AMQP proxy server for RabbitMQ written in Go. The project is currently in ALPHA status.

## Development Guidelines

### Code Standards
- Follow standard Go formatting with `go fmt`
- Always run `go fmt ./...` before committing code
- Add tests for new functionality in `*_test.go` files
- Use structured logging with `log/slog`
- Maintain AMQP 0.9.1 protocol compliance
- **Error handling**: Always log errors before returning, use structured logging with context
- **Goroutine management**: Use proper synchronization with `sync.WaitGroup` and timeout patterns to prevent hanging
- **Resource cleanup**: Implement proper cleanup in defer functions, especially for connections and channels
- **Git operations**: Use `git add <file>` for individual files, never use `git add -A` or `git add .`

### Testing
- Run tests with: `go test ./...`
- Test files are located alongside source files as `*_test.go`
- Test data is in `testdata/` directory
- Use table-driven tests where appropriate
- Use `t.Context()` in test functions instead of manually creating contexts with `context.Background()`
- Use `mustTestConn(t)` for connecting to test proxy server
- Test files should use `package trabbits_test` (not `package trabbits`) for consistency with other tests
- Export functions needed for testing via `export_test.go`
- **Test isolation**: Always clear active proxies at the beginning of tests that use proxy management to ensure clean test state (use server instance methods or check TestServer != nil)
- **Mock connections**: When testing with `net.Pipe()` connections, be aware that AMQP protocol operations will fail - design tests accordingly
- **Async testing**: Use channel-based interfaces for testing asynchronous operations like `DisconnectOutdatedProxies()` to verify completion
- **Test timeouts**: Configure shorter timeouts for test environment using `export_test.go` to speed up test execution

### Build and Run
- Build: `go build -o trabbits ./cmd/trabbits`
- Run: `./trabbits run --config config.json`
- Development config examples in `testdata/`

### Key Architecture Components
- `server.go` - Main proxy server implementation
- `upstream.go` - RabbitMQ upstream connection management
- `proxy.go` - Client-server proxy logic
- `amqp091/` - AMQP 0.9.1 protocol implementation
- `routing.go` - Message routing based on patterns
- `error.go` - AMQP error handling and propagation

### Configuration
- JSON-based configuration in `config.json`
- Dynamic reloading via API server
- Supports multiple upstream RabbitMQ servers
- Routing rules based on topic exchange patterns

### API and Monitoring
- Unix socket API for configuration management
- Prometheus metrics on port 16692
- CLI management via `trabbits manage config`

### Protocol Support
- AMQP 0.9.1 methods (see README for supported list)
- PLAIN and AMQPLAIN authentication only
- Server-named queue emulation with `trabbits.gen-` prefix

### Common Tasks
- Add new AMQP method support in `methods.go` and `amqp091/`
- Extend routing patterns in `pattern.go`
- Add metrics in `metrics.go`
- Configuration changes require `conf.go` updates

### Design Patterns and Best Practices
- **Configuration hashing**: Use simplified JSON marshalling with password unmasking for reliable change detection
- **Rate limiting for mass operations**: Prevent reconnection storms by implementing controlled disconnection rates
- **Environment-specific configuration**: Use `export_test.go` to configure different values for test vs production environments
- **Early returns for optimization**: Implement zero-work shortcuts to avoid unnecessary processing overhead
- **Separate test files by functionality**: Split large test suites into focused files (e.g., `proxy_disconnect_*.go` series)

### CLI Command Development
- When adding new CLI commands, consider if they should be optional arguments or required
- Interactive modes should be avoided - commands should complete with provided arguments
- Commands that test/validate should return appropriate exit codes (0 for success, 1 for failure)
- Keep utility functions simple - avoid creating separate functions for trivial operations

### Important Notes
- This is ALPHA software - not for production use
- Protocol compliance with RabbitMQ AMQP 0.9.1 is critical
- Maintain backward compatibility where possible
- SO_REUSEPORT support for multiple instances

### Upstream Connection Management
- Each upstream connection is monitored via `rabbitmq.Connection.NotifyClose()`
- Upstream disconnections trigger `Connection.Close` with error code 320 (connection-forced) to clients
- Monitoring goroutines are started for each upstream in `ConnectToUpstreams()`
- `MonitorUpstreamConnection()` handles upstream disconnection events
- Client disconnection on upstream failure ensures proper error propagation

### Proxy Management and Configuration Updates
- **Config versioning**: Use SHA256 hash of configuration (with unmasked passwords) to detect changes
- **Graceful disconnection**: When config changes, outdated proxies are disconnected with `amqp091.ConnectionForced` error
- **Rate limiting**: Use `golang.org/x/time/rate` for controlled disconnection (100/sec default, 1000/sec for small counts)
- **Worker pool pattern**: Parallel processing with controlled concurrency (10 workers) and proper synchronization
- **Dynamic timeouts**: Calculate timeouts based on proxy count and rate limits to avoid premature failures
- **Global proxy registry**: Use `sync.Map` for thread-safe proxy tracking across the application
- **Channel-based async interfaces**: Return completion channels for async operations to enable proper testing and monitoring

### Timeout Configuration Management
- **Configuration-based timeouts**: ReadTimeout and ConnectionCloseTimeout are configured in the main Config struct
- **No circular dependencies**: Proxy must not hold Server reference - use dependency injection patterns instead
- **Default values**: Use SetDefaults() method to apply default timeout values if not specified in config
- **Package constants**: DefaultReadTimeout and DefaultConnectionCloseTimeout constants define default values
- **Timeout passing**: Pass timeout values to Proxy during creation rather than accessing them later
- **Thread-safe access**: Use Server methods with proper locking to access timeout configuration
- **ConnectionCloseTimeout**: Timeout for waiting Connection.Close-Ok during graceful connection shutdown
