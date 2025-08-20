.PHONY: clean test
export SHELL := /bin/bash

trabbits: gen go.* *.go
	go build -o $@ ./cmd/trabbits

clean:
	rm -rf trabbits dist/

test:
	go test ./... -count=1

install:
	go install github.com/fujiwara/trabbits/cmd/trabbits

dist:
	goreleaser build --snapshot --clean

gen:
	go run spec/gen.go < spec/amqp0-9-1.stripped.extended.xml | gofmt > amqp091/spec091.go

_run-bench-rabbitmq:
	docker run -t --network=host --env RABBITMQ_DEFAULT_USER=admin --env RABBITMQ_DEFAULT_PASS=admin --cpus=1 rabbitmq:3.12-management

_run-bench-trabbits: trabbits
	ENABLE_PPROF=true ./trabbits run --config <(echo '{"upstreams":[{"host":"127.0.0.1","port":5672}]}') --port 6672

run-bench-servers: _run-bench-rabbitmq _run-bench-trabbits

run-bench-pprof:
	go tool pprof -seconds 30 -http=localhost:1080 http://localhost:6060/debug/pprof/profile

run-bench-trabbits:
	docker run -i --rm --network=host --cpus=1 pivotalrabbitmq/perf-test:latest --uri amqp://admin:admin@127.0.0.1:6672 -z 30 $(BENCH_ARGS)

run-bench-rabbitmq:
	docker run -i --rm --network=host --cpus=1 pivotalrabbitmq/perf-test:latest --uri amqp://admin:admin@127.0.0.1:5672 -z 30 $(BENCH_ARGS)
