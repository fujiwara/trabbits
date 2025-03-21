# trabbits

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
- Debugging capabilities with log output

## Installation

TODO

## Usage

To start trabbits, run the following command:

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

## Monitoring

trabbits provides a Prometheus exporter that exposes metrics about the proxy server. You can access the metrics at `http://localhost:16692/metrics`.


## Server-named queues emulation

trabbits can emulate server-named queues. If you declare a queue with an empty name, trabbits will generate a unique name for the queue.

This is not a feature of the AMQP 0.9.1 protocol, but a feature in RabbitMQ. See [Server-named queues](https://www.rabbitmq.com/queues.html#server-named-queues).

RabbitMQ generates a unique name for example `amq.gen-(random string)`. trabbits generates a unique name in the format `trabbits.gen-(random string)` because `amq.gen-` is reserved by RabbitMQ.

The generated queue by trabbis is not a temporary queue on the upstream RabbitMQ server. It is created as a normal queue with the specified attributes. The queue will not be deleted when the client disconnects by RabbitMQ, So trabbits emulates the server-named queue behavior.

trabbits will delete the queue when the connection that declared the queue is closed (=exclusive).

## License

This project is licensed under the BSD-style license. See the `LICENSE` file for details.

## Contributing

Bug reports and pull requests are welcome. For contribution guidelines, see `CONTRIBUTING.md`.

## Copyright

- 2021 VMware, Inc. or its affiliates
- 2012-2021 Sean Treadway, SoundCloud Ltd.
- 2025 fujiwara

All rights reserved.
