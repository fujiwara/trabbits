# trabbits

This project is currently in ALPHA status and should NOT be used in production environments.

The API and behavior may change significantly before reaching stable release.

---

trabbits is a proxy server for sending and receiving messages using the AMQP protocol. This project supports RabbitMQ's AMQP 0-9-1 protocol.

trabbits can have multiple upstreams, which can be single RabbitMQ servers or RabbitMQ clusters that it connects to. It can also route messages to different upstreams based on the routing key.

## Propose of this project

trabbits is helpful for migrating from one RabbitMQ server to another. You can use trabbits to proxy messages from the old RabbitMQ server to the new RabbitMQ server. This allows you to migrate your RabbitMQ server without any downtime.

## Architecture

```
+-----------------+     +-----------------+
| RabbitMQ Server |     | RabbitMQ Server |
+-----------------+     +-----------------+
                  \     /
                   \   /  (routed by routing key)
                    \ /
            +-----------------+
            |     trabbits    |
            +-----------------+
                     |
                     | Publish/Consume
                     |
           +-------------------+
           | AMQP 0.9.1 client |
           +-------------------+
```

1. The AMQP 0.9.1 client connects to trabbits.
2. trabbits connects to multiple RabbitMQ servers (upstreams).
3. trabbits routes messages to different upstreams based on the routing key with `Basic.Publish` method.
4. trabbits consumes messages from all upstreams and sends them to the AMQP 0.9.1 client.

Your clients can connect to trabbits and send and receive messages without knowing the actual RabbitMQ server.

## Features

- Support for AMQP 0-9-1 protocol
- Proxy functionality between client and upstream
- Support for both single RabbitMQ servers and RabbitMQ clusters as upstreams
- Automatic failover within RabbitMQ clusters
- Health checking for cluster nodes with automatic node isolation and recovery
- Routing publishing messages to different upstreams based on the routing key
- Consuming messages from multiple upstreams
- Prometheus exporter for monitoring
- Dynamic configuration reloading
- CLI and API for managing configuration

## Limitations

- Authentication mechanism support is limited to PLAIN and AMQPLAIN.
- Not all AMQP 0-9-1 methods are supported. See [Supported Methods](#supported-methods) section for more details.

## Installation

TODO

## Usage

```
Usage: trabbits <command> [flags]

Flags:
  -h, --help                    Show context-sensitive help.
      --config="config.json"    Path to the configuration file ($TRABBITS_CONFIG).
      --port=6672               Port to listen on ($TRABBITS_PORT).
      --metrics-port=16692      Port to listen on for metrics ($TRABBITS_METRICS_PORT)
      --api-socket="/tmp/trabbits.sock"
                                Path to the API socket ($TRABBITS_API_SOCKET).
      --debug                   Enable debug mode ($DEBUG).
      --enable-pprof            Enable pprof ($ENABLE_PPROF).
      --version                 Show version.

Commands:
  run [flags]
    Run the trabbits server.

  manage config <command>
    Manage the configuration.

  test match-routing <pattern> <key>
    Test routing pattern matching.

Run "trabbits <command> --help" for more information on a command.
```

To start trabbits server, `run` the following command:

```sh
trabbits run --config config.json
```

By default, trabbits listens on port 6672 and connects to an upstream RabbitMQ server.

## Configuration

trabbit's configuration file is located at `config.json`. The configuration file contains the following fields:

```json
{
    "read_timeout": "5s",
    "connection_close_timeout": "1s",
    "upstreams": [
        {
            "name": "primary",
            "address": "localhost:5672"
        },
        {
            "name": "secondary-cluster",
            "cluster": {
                "nodes": [
                    "localhost:5673",
                    "localhost:5674",
                    "localhost:5675"
                ]
            },
            "timeout": "10s",
            "health_check": {
                "enabled": true,
                "interval": "30s",
                "timeout": "5s",
                "unhealthy_threshold": 3,
                "recovery_interval": "60s"
            },
            "routing": {
                "key_patterns": [
                    "test.queue.another.*"
                ]
            },
            "queue_attributes": {
                "durable": true,
                "auto_delete": false,
                "exclusive": false,
                "arguments": {
                    "x-queue-type": "quorum"
                }
            }
        }
    ]
}
```

trabbits supports dynamic configuration reloading. You can put a new configuration via HTTP PUT request to `/config` endpoint. See [API server](#api-server) section for more details.

### Upstreams section

The `upstreams` section contains an array of upstreams.

The first upstream is used as the default. If the routing key does not match any patterns, trabbits will use the default upstream to publish messages.

Each `upstream` has the following fields:

- `name`: (Required) A unique name for the upstream.
- `address`: The address of the RabbitMQ server in `host:port` format (required for single server configuration). Supports IPv6 addresses (e.g., `[::1]:5672`).
- `cluster`: Configuration for RabbitMQ cluster connection (alternative to `address`).
  - `nodes`: An array of cluster node addresses in `host:port` format.
- `timeout`: Connection timeout duration (optional, default: 5s). Accepts Go duration format (e.g., "10s", "1m").
- `health_check`: Health check configuration for cluster upstreams (optional).
  - `interval`: Health check interval (default: 30s). Accepts Go duration format.
  - `timeout`: Health check timeout (default: 5s). Accepts Go duration format.
  - `unhealthy_threshold`: Number of consecutive failures before marking node as unhealthy (default: 3).
  - `recovery_interval`: Interval for checking unhealthy nodes for recovery (default: 60s).
  - `username`: Username for health check authentication (required).
  - `password`: Password for health check authentication (required).
- `routing`: The routing rules for this upstream.
  - `key_patterns`: An array of routing key patterns. If the routing key matches any of these patterns, trabbits will use this upstream to publish.
    The patterns are the same as the RabbitMQ's topic exchange routing key patterns, including wildcard characters `*` and `#`.
- `queue_attributes`: The attributes of the queue that will be declared on this upstream.
   All of the attributes are optional.
   The defined attributes will override the request attributes from the client.
   - `arguments`: A map of arguments for the queue.
      The keys are strings and the values are any type. If the value is `null`, the argument will be removed.

### Cluster Connection Behavior

When connecting to a cluster upstream, trabbits will:

1. **Health-based Selection**: Prioritize healthy nodes for connections
2. **Random Selection**: Randomly shuffle available nodes to distribute connection load
3. **Failover**: Try each node in the shuffled order until a successful connection is established
4. **Connection Reuse**: Once connected to a cluster node, that connection is used for all operations
5. **Timeout**: Use the configured timeout (default: 5s) for each connection attempt

### Health Check Behavior

For cluster upstreams with health checking enabled:

1. **Background Monitoring**: Run health checks in a separate goroutine at configured intervals
2. **Node Isolation**: Mark nodes as unhealthy after consecutive failures exceed the threshold
3. **Automatic Recovery**: Periodically check unhealthy nodes and restore them when they recover
4. **Graceful Degradation**: Fall back to all nodes if no healthy nodes are available
5. **Metrics Export**: Expose healthy/unhealthy node counts via Prometheus metrics

Health checks use simple AMQP connection attempts with immediate disconnection to minimize overhead.

### Routing Algorithm

You can specify routing rules in the configuration file. The routing rules are based on the routing key. If the routing key matches the specified pattern, trabbits will route the message to the corresponding upstream.

Supported patterns are equivalent to the RabbitMQ's topic exchange routing key patterns:
- `*` matches a single word
- `#` matches zero or more words

trabbits tries to match the routing key with the specified pattern in the order they are defined in the configuration file. If the routing key matches a pattern, trabbits will use the corresponding upstream immediately (will not check other patterns).

If the routing key does not match any patterns, trabbits will use the first upstream as the default.

### Global Configuration Options

#### Timeout Settings (Advanced)

These timeout settings control internal connection behavior and typically do not need to be modified:

- `read_timeout`: (Optional) Maximum time to wait for reading data from client connections (default: 5s). Accepts Go duration format (e.g., "5s", "10s").
- `connection_close_timeout`: (Optional) Maximum time to wait for Connection.Close-Ok response during graceful connection shutdown (default: 1s). Accepts Go duration format.

**Note:** These are advanced settings that should only be adjusted if you experience specific timeout-related issues. The default values are suitable for most use cases.

#### Graceful Shutdown Settings

trabbits implements graceful shutdown when receiving termination or reload signals. On SIGTERM, the server first closes the listener to prevent new connections, then gracefully disconnects all existing connections using the AMQP Connection.Close protocol. On SIGHUP (configuration reload), connections using outdated configuration are gracefully disconnected with rate limiting.

The default configuration can handle approximately 1000 connections within a 10-second timeout, which covers most typical deployments. For larger-scale environments with thousands of connections, you may need to adjust the timeout and rate limiting parameters based on your requirements.

**Important:** If graceful shutdown cannot complete within the configured timeout (e.g., due to unresponsive clients or too many connections), remaining connections will be forcibly terminated.

- `graceful_shutdown`: Configuration for graceful shutdown behavior
  - `shutdown_timeout`: Maximum time to wait for graceful shutdown on SIGTERM (default: 10s). Accepts Go duration format.
  - `reload_timeout`: Maximum time to wait for graceful disconnection during configuration reload on SIGHUP (default: 30s). Accepts Go duration format.
  - `rate_limit`: Maximum number of connections to disconnect per second (default: 100). Controls disconnection rate to prevent connection storms.
  - `burst_size`: Initial burst size for rate limiting (default: 10). Allows quick disconnection of small connection pools.

Example configuration for large-scale deployments:

```json
{
  "graceful_shutdown": {
    "shutdown_timeout": "30s",
    "reload_timeout": "60s",
    "rate_limit": 300,
    "burst_size": 50
  },
  "upstreams": [...]
}
```

## Configuration File Formats

trabbits supports two configuration file formats:

### JSON Configuration

Standard JSON format configuration files are supported with environment variable expansion using the `${VAR}` syntax.

### Jsonnet Configuration

trabbits also supports [Jsonnet](https://jsonnet.org/) configuration files (`.jsonnet` extension), which provides:
- Dynamic configuration generation
- Functions and conditionals
- Imports and includes
- Environment variable access via `std.native('env')`
- Additional utility functions from [jsonnet-armed](https://github.com/fujiwara/jsonnet-armed) such as file reading, JSON/YAML parsing, and more

#### Jsonnet Example

```jsonnet
local env = std.native('env');
{
  upstreams: [
    {
      name: 'cluster',
      cluster: {
        nodes: [
          'localhost:5672',
        ],
      },
      health_check: {
        username: env('RABBITMQ_HEALTH_USER', 'guest'),
        password: env('RABBITMQ_HEALTH_PASS', 'guest'),
        interval: '30s',
        timeout: '5s',
        unhealthy_threshold: 3,
        recovery_interval: '60s',
      },
    },
  ],
}
```

To use a Jsonnet configuration:

```sh
export RABBITMQ_HEALTH_USER=healthcheck
export RABBITMQ_HEALTH_PASS=secretpassword
trabbits run --config config.jsonnet
```

## Environment Variable Expansion

For JSON configuration files, trabbits supports environment variable expansion using the `${VAR}` syntax. This is particularly useful for sensitive information like passwords that should not be stored in plain text in configuration files.

### Example

Instead of storing credentials directly in the configuration file:

```json
{
    "upstreams": [
        {
            "name": "cluster",
            "cluster": {
                "nodes": [
                    "localhost:5672"
                ]
            },
            "health_check": {
                "username": "${RABBITMQ_HEALTH_USER}",
                "password": "${RABBITMQ_HEALTH_PASS}",
                "interval": "30s",
                "timeout": "5s",
                "unhealthy_threshold": 3,
                "recovery_interval": "60s"
            }
        }
    ]
}
```

Set the environment variables before running trabbits:

```sh
export RABBITMQ_HEALTH_USER=healthcheck
export RABBITMQ_HEALTH_PASS=secretpassword
trabbits run --config config.json
```

### Notes

- If an environment variable is not set, it expands to an empty string in JSON files
- For Jsonnet files, use `std.native('env')` function with default values
- Environment variable expansion works for any string field in the configuration
- Variable names are case-sensitive
- Use double quotes around the `${VAR}` syntax in JSON

## Setting Log Level

By default, the log level is set to info. You can change the log level to debug by setting the `DEBUG` environment variable to `true`.

```sh
DEBUG=true trabbits run
```

## Supported Methods

trabbits currently supports the following AMQP methods:

- ChannelOpen
- ChannelClose
- QueueDeclare
- QueueDelete
- QueueBind
- QueueUnbind
- QueuePurge
- ExchangeDeclare
- BasicPublish
- BasicConsume
- BasicGet
- BasicAck
- BasicNack
- BasicCancel
- BasicQos

## Server-named queues emulation

trabbits can emulate server-named queues. If you declare a queue with an empty name, trabbits will generate a unique name for the queue.

This is not a feature of the AMQP 0.9.1 protocol, but a feature in RabbitMQ. See [Server-named queues](https://www.rabbitmq.com/queues.html#server-named-queues).

RabbitMQ generates a unique name for example `amq.gen-(random string)`. trabbits generates a unique name in the format `trabbits.gen-(random string)` because `amq.gen-` is reserved by RabbitMQ.

The generated queue by trabbis is not a temporary queue on the upstream RabbitMQ server. It is created as a normal queue with the specified attributes. The queue will not be deleted when the client disconnects by RabbitMQ, So trabbits emulates the server-named queue behavior.

trabbits will delete the queue when the connection that declared the queue is closed (=exclusive).

## Monitoring server

trabbits provides a monitoring server that exposes metrics about the proxy server. You can access the metrics at `http://localhost:16692/metrics`.

These metrics format is compatible with Prometheus.

The monitoring server is enabled by default and listens on port 16692. You can change the port by using the `--metrics-port` option.


## API Server

trabbits provides an HTTP API server that allows you to manage the configuration.

`trabbits` listens on unix domain socket by default. You can change the socket path by using the `--api-socket` option.

### Configuration API / CLI

You can update the configuration of trabbits via the API server. You can get the current configuration and update with a new configuration via HTTP request to `/config` endpoint.

trabbits cli also supports the configuration management. You can use `trabbits manage config` command to manage
the configuration. The cli access to the API server on the localhost.

```
Usage: trabbits manage config <command> [<file>]

Manage the configuration.

Arguments:
  <command>    Command to run (get, diff, put, reload).
  <file>       Configuration file (required for diff/put commands).
```

You can also manage connected clients using the `trabbits manage clients` command. This provides both CLI commands for scripting and a Terminal User Interface (TUI) for interactive management:

```
Usage: trabbits manage clients <command>

Manage connected clients.

Commands:
  list                     Get connected clients information
  info <proxy-id>          Get detailed information for a specific proxy
  shutdown <proxy-id>      Shutdown a specific proxy
  tui                      Interactive terminal interface for managing clients
```

#### Interactive Client Management (TUI)

For interactive client management, trabbits provides a modern Terminal User Interface built with Bubble Tea:

```console
$ trabbits manage clients
# or
$ trabbits manage clients tui
```

The TUI provides a real-time, top-like interface with the following features:

**Main Interface:**
- **Fixed Header**: Server statistics (active clients, total clients, last update time) always visible at top
- **Client List**: Scrollable table showing connected clients with key information:
  - Client ID (shortened for display)
  - Username and Virtual Host
  - Client address
  - Connection status (active/shutting_down)
  - Connected time (relative)
  - Method and frame statistics
- **Real-time Updates**: All information refreshes automatically every 2 seconds
- **Pagination**: Handles large numbers of clients with proper scrolling and pagination indicators

**Navigation:**
- `↑↓` or `k/j`: Navigate through client list
- `Page Up/Down`: Jump by pages for large client lists
- `Home/End`: Jump to first/last client
- `Enter`: View detailed client information
- `Shift+K`: Shutdown selected client (with confirmation)
- `r`: Force refresh
- `q`: Quit

**Client Detail View:**
- Complete client information including properties and capabilities
- Method-level statistics breakdown showing usage patterns
- Frame counters and connection duration
- Scrollable content for clients with many properties
- `Shift+K`: Shutdown client directly from detail view
- Real-time updates of statistics and status

**Client Shutdown:**
- Confirmation dialog showing client details before shutdown
- Optional reason (defaults to "TUI shutdown")
- Immediate feedback on success/failure
- Graceful disconnection with proper AMQP close protocol

**Benefits:**
- **No Terminal Disruption**: Unlike `curl` commands, the TUI doesn't clutter your terminal with JSON output
- **Real-time Monitoring**: See client connections, disconnections, and activity as it happens
- **Efficient Navigation**: Quickly browse through many clients without manual ID copying
- **Comprehensive Information**: All client details in an easy-to-read format
- **Safe Operations**: Confirmation dialogs prevent accidental shutdowns

#### Configuration Versioning and Graceful Disconnection

When a new configuration is loaded (via API PUT request or SIGHUP signal), trabbits implements a graceful proxy management system:

- Each active proxy connection maintains a hash of the configuration it was created with
- Upon configuration update, trabbits calculates a new configuration hash
- Proxy connections using outdated configurations are gracefully disconnected with `connection-forced` (320) error code
- Clients receive a "Configuration updated, please reconnect" message and can immediately reconnect with the new configuration
- This ensures that configuration changes are reflected quickly across all connections while maintaining service availability

This behavior helps ensure that configuration updates (such as upstream changes, routing rule modifications, or credential updates) are applied consistently across all active connections without requiring a full server restart.

#### Use curl to access the API server

You can use `curl` to access the API server. The API server listens on the unix domain socket by default. You can change the socket path by using the `--api-socket` option.

```console
$ curl --unix-socket /tmp/trabbits.sock http://localhost/config
```

#### Note

Reloading the configuration will not affect the existing connections. The new configuration will be applied to new connections only.

#### Get the current configuration

You can get the current configuration by sending a GET request to the `/config` endpoint.

```console
$ curl --unix-socket /tmp/trabbits.sock http://localhost/config
```

trabbits returns the current configuration in JSON format.

You can also use the `trabbits` cli to get the current configuration.

```console
$ trabbits manage config get
```

#### Update the configuration

You can update the configuration by sending a PUT request with a new configuration in JSON format.

```console
$ curl --unix-socket /tmp/trabbits.sock -X PUT -d @new_config.json \
    -H "Content-Type: application/json" http://localhost/config
```

trabbits will reload the configuration and apply the new configuration.

You can also use the `trabbits` cli to update the configuration.

```console
$ trabbits manage config put new_config.json
```

#### Diff the configuration

You can diff the current configuration and a new configuration using trabbits cli.

```console
$ trabbits manage config diff new_config.json
```

#### Reload the configuration

You can reload the configuration from the original configuration file using trabbits cli.

```console
$ trabbits manage config reload
```

This command reloads the configuration from the file specified by `--config` option (default: `config.json`).

### Clients API

trabbits provides an API to get information about connected clients. You can get the list of all connected clients by sending a GET request to the `/clients` endpoint.

#### Get connected clients information

You can get information about all connected clients by sending a GET request to the `/clients` endpoint.

```console
$ curl --unix-socket /tmp/trabbits.sock http://localhost/clients
```

The response includes the following information for each client:

```json
[
  {
    "id": "proxy-12345678",
    "client_address": "127.0.0.1:54321",
    "user": "guest",
    "virtual_host": "/",
    "client_banner": "Platform/Product/Version",
    "connected_at": "2025-01-01T12:00:00Z",
    "status": "active",
    "shutdown_reason": ""
  },
  {
    "id": "proxy-87654321",
    "client_address": "127.0.0.1:54322",
    "user": "guest",
    "virtual_host": "/",
    "client_banner": "Platform/Product/Version",
    "connected_at": "2025-01-01T12:01:00Z",
    "status": "shutting_down",
    "shutdown_reason": "Configuration updated, please reconnect"
  }
]
```

Fields description:

- `id`: Unique proxy identifier
- `client_address`: Client's IP address and port
- `user`: Authenticated username
- `virtual_host`: Virtual host the client is connected to
- `client_banner`: Client platform/product/version information
- `connected_at`: Timestamp when the client connected
- `status`: Connection status - either `active` or `shutting_down`
- `shutdown_reason`: Reason for shutdown (only present when status is `shutting_down`)

You can also use the `trabbits` CLI to get clients information:

```console
$ trabbits manage clients list
```

#### Get detailed client information

You can get comprehensive information about a specific client by sending a GET request to the `/clients/{proxy_id}` endpoint.

```console
$ curl --unix-socket /tmp/trabbits.sock http://localhost/clients/proxy-12345678
```

This returns complete client information including:

```json
{
  "id": "proxy-12345678",
  "client_address": "127.0.0.1:54321",
  "user": "admin",
  "virtual_host": "/",
  "client_banner": "golang/golang/AMQP 0.9.1 Client/1.10.0",
  "client_properties": {
    "product": "my-app",
    "version": "1.0.0",
    "capabilities": {
      "publisher_confirms": true,
      "consumer_cancel_notify": true
    }
  },
  "connected_at": "2023-11-20T10:30:00Z",
  "status": "active",
  "stats": {
    "started_at": "2023-11-20T10:30:00Z",
    "methods": {
      "Basic.Publish": 150,
      "Basic.Consume": 5,
      "Queue.Declare": 3
    },
    "total_methods": 158,
    "received_frames": 320,
    "sent_frames": 285,
    "total_frames": 605,
    "duration": "45m30s"
  }
}
```

Using the CLI:

```console
$ trabbits manage clients info proxy-12345678
```

The response includes:
- Complete client properties and capabilities
- Detailed statistics with method-level breakdown
- Frame counters (received/sent frames)
- Connection duration and timestamps

#### Shutdown a specific proxy

You can gracefully shutdown a specific proxy by sending a DELETE request to the `/clients/{proxy_id}` endpoint.

```console
$ curl -X DELETE --unix-socket /tmp/trabbits.sock http://localhost/clients/proxy-12345678
```

You can also provide a custom shutdown reason using the `reason` query parameter:

```console
$ curl -X DELETE --unix-socket /tmp/trabbits.sock "http://localhost/clients/proxy-12345678?reason=Maintenance"
```

The response includes the shutdown status:

```json
{
  "status": "shutdown_initiated",
  "proxy_id": "proxy-12345678",
  "reason": "Maintenance"
}
```

Using the CLI:

```console
# Shutdown a proxy with default reason
$ trabbits manage clients shutdown proxy-12345678

# Shutdown a proxy with custom reason
$ trabbits manage clients shutdown proxy-12345678 --reason "Scheduled maintenance"
```

The proxy will be gracefully disconnected, allowing it to properly close ongoing operations before termination.

#### Reload configuration with SIGHUP

You can also reload the configuration by sending a SIGHUP signal to the trabbits process.

For better reliability, use a PID file to ensure you target the correct process:

```console
$ trabbits run --pid-file /var/run/trabbits.pid --config config.json
$ kill -HUP $(cat /var/run/trabbits.pid)
```

Alternatively, you can use process discovery (less reliable):

```console
$ kill -HUP $(pidof trabbits)
```

or

```console
$ pkill -HUP trabbits
```

This will reload the configuration from the original configuration file specified when trabbits was started.
```diff
--- http://localhost:16692/config
+++ new_config.json
@@ -7,10 +7,10 @@
     },
     {
       "address": "localhost:5673",
+      "address": "localhost:5674",
       "routing": {
         "key_patterns": [
-          "#"
+          "test.queue.example.*"
         ]
       },
       "queue_attributes": {
```

### Test utilities

#### Test routing pattern matching

The `test match-routing` command allows you to test whether a routing key matches a given binding pattern. This is useful for debugging routing rules before applying them in your configuration.

```console
# Test pattern matching
$ trabbits test match-routing "logs.*.error" "logs.app.error"
✓ MATCHED: pattern 'logs.*.error' matches key 'logs.app.error'

$ trabbits test match-routing "logs.*.error" "metrics.app.error"
✗ NOT MATCHED: pattern 'logs.*.error' does not match key 'metrics.app.error'

# Exit code is 0 for match, 1 for no match
$ trabbits test match-routing "logs.#" "logs.app.error.critical"
✓ MATCHED: pattern 'logs.#' matches key 'logs.app.error.critical'
$ echo $?
0

$ trabbits test match-routing "metrics.*" "metrics.cpu.usage"  
✗ NOT MATCHED: pattern 'metrics.*' does not match key 'metrics.cpu.usage'
$ echo $?
1
```

Pattern matching follows RabbitMQ's topic exchange rules with extensions:
- `*` (star) matches exactly one word
- `#` (hash) matches zero or more words  
- `%` (percent) matches zero or more characters within a single word (substring matching)
- Words are delimited by dots (`.`)

Examples with `%` wildcard:
- `app.%service` matches `app.webservice`, `app.apiservice`, `app.service`
- `foo.*.a%` matches `foo.xxx.aaa`, `foo.yyy.app` 
- `%.server.*` matches `web.server.01`, `api.server.prod`

## Support for multiple instances

trabbits can run multiple instances on the same server. You can use same port (`--port`) for multiple instances.

This feature uses `SO_REUSEPORT` socket option. This allows multiple processes to bind to the same port. The kernel will distribute incoming connections to the processes.

This feature is useful for deploying a new version without downtime. You can start a new instance with a new version and configuration and stop the old instance.

### Note

The `--metrics-port` and `--api-socket` options must be different for each instance because they are individual for each instance.

## License

This project is licensed under the BSD-style license. See the `LICENSE` file for details.

## Contributing

Bug reports and pull requests are welcome. For contribution guidelines, see `CONTRIBUTING.md`.

## Copyright

- 2021 VMware, Inc. or its affiliates
- 2012-2021 Sean Treadway, SoundCloud Ltd.
- 2025 fujiwara

All rights reserved.
