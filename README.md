# trabbits

trabbits is a proxy server for sending and receiving messages using the AMQP protocol. This project supports RabbitMQ's AMQP 0-9-1 protocol.

## Features

- Support for AMQP 0-9-1 protocol
- Proxy functionality between client and upstream
- Debugging capabilities with log output

## Installation

TODO

## Usage

To start trabbits, run the following command:

```sh
trabbits
```

By default, trabbits listens on port 5673 and connects to an upstream RabbitMQ server.

## Configuration

You can configure the upstream URL using the `config` variable.

```go
var config = &Config{
    Upstream: UpstreamConfig{
        URL: "amqp://127.0.0.1:5672/",
    },
}
```

## Setting Log Level

By default, the log level is set to debug. You can change the log level in the `init` function if needed.

```go
func init() {
    handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
    slog.SetDefault(slog.New(handler))
}
```

## Supported Methods

trabbits currently supports the following AMQP methods:

- `ChannelOpen`
- `ChannelClose`
- `ConnectionClose`
- `QueueDeclare`
- `BasicPublish`
- `BasicConsume`

## License

This project is licensed under the BSD-style license. See the `LICENSE` file for details.

## Contributing

Bug reports and pull requests are welcome. For contribution guidelines, see `CONTRIBUTING.md`.

## Copyright

- 2021 VMware, Inc. or its affiliates
- 2012-2021 Sean Treadway, SoundCloud Ltd.
- 2025 fujiwara

All rights reserved.
