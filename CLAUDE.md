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

### Testing
- Run tests with: `go test ./...`
- Test files are located alongside source files as `*_test.go`
- Test data is in `testdata/` directory
- Use table-driven tests where appropriate

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

### Important Notes
- This is ALPHA software - not for production use
- Protocol compliance with RabbitMQ AMQP 0.9.1 is critical
- Maintain backward compatibility where possible
- SO_REUSEPORT support for multiple instances
