.PHONY: clean test
export SHELL := /bin/bash

trabbits: gen go.* *.go
	go build -o $@ ./cmd/trabbits

clean:
	rm -rf trabbits dist/

test:
	go test -v ./... -count=1

install:
	go install github.com/fujiwara/trabbits/cmd/trabbits

dist:
	goreleaser build --snapshot --clean

gen:
	go run spec/gen.go < spec/amqp0-9-1.stripped.extended.xml | gofmt > amqp091/spec091.go

run-bench-servers: _run-bench-rabbitmq _run-bench-trabbits

run-bench-pprof:
	go tool pprof -seconds 50 -http=localhost:1080 http://localhost:6060/debug/pprof/profile

run-bench-trabbits:
	docker run -i --rm --network=host --cpus=1 pivotalrabbitmq/perf-test:latest --uri amqp://admin:admin@127.0.0.1:5673 -z 30 $(BENCH_ARGS)

run-bench-rabbitmq:
	docker run -i --rm --network=host --cpus=1 pivotalrabbitmq/perf-test:latest --uri amqp://admin:admin@127.0.0.1:5672 -z 30 $(BENCH_ARGS)
