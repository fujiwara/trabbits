# trabbits

This project is currently in ALPHA status and should NOT be used in production environments.

The API and behavior may change significantly before reaching stable release.

---

trabbits is a proxy server for sending and receiving messages using the AMQP protocol. This project supports RabbitMQ's AMQP 0-9-1 protocol.

trabbits can have multiple upstreams, which are RabbitMQ servers that it connects to. It can also route messages to different upstreams based on the routing key.

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
    "upstreams": [
        {
            "host": "localhost",
            "port": 5672,
            "default": true
        },
        {
            "host": "localhost",
            "port": 5673,
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

- `host`: The hostname of the RabbitMQ server.
- `port`: The port number of the RabbitMQ server.
- `routing`: The routing rules for this upstream.
  - `key_patterns`: An array of routing key patterns. If the routing key matches any of these patterns, trabbits will use this upstream to publish.
    The patterns are the same as the RabbitMQ's topic exchange routing key patterns, including wildcard characters `*` and `#`.
- `queue_attributes`: The attributes of the queue that will be declared on this upstream.
   All of the attributes are optional.
   The defined attributes will override the request attributes from the client.
   - `arguments`: A map of arguments for the queue.
      The keys are strings and the values are any type. If the value is `null`, the argument will be removed.

### Routing Algorithm

You can specify routing rules in the configuration file. The routing rules are based on the routing key. If the routing key matches the specified pattern, trabbits will route the message to the corresponding upstream.

Supported patterns are equivalent to the RabbitMQ's topic exchange routing key patterns:
- `*` matches a single word
- `#` matches zero or more words

trabbits tries to match the routing key with the specified pattern in the order they are defined in the configuration file. If the routing key matches a pattern, trabbits will use the corresponding upstream immediately (will not check other patterns).

If the routing key does not match any patterns, trabbits will use the first upstream as the default.

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
Usage: trabbits manage config <command>

Manage the configuration.

Arguments:
  <command>    Command to run (get, diff, put).
```

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
$ trabbits manage config put --config new_config.json
```

#### Diff the configuration

You can diff the current configuration and a new configuration using trabbits cli.

```console
$ trabbits manage config diff --config new_config.json
```
```diff
--- http://localhost:16692/config
+++ new_config.json
@@ -7,10 +7,10 @@
     },
     {
       "host": "localhost",
-      "port": 5673,
+      "port": 5674,
       "routing": {
         "key_patterns": [
-          "#"
+          "test.queue.example.*"
         ]
       },
       "queue_attributes": {
```

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
