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

run-bench-upstream:
	docker run -it --network=host --env RABBITMQ_DEFAULT_USER=admin --env RABBITMQ_DEFAULT_PASS=admin --cpus=1 rabbitmq:3.12-management

run-bench-trabbits: trabbits
	ENABLE_PPROF=true ./trabbits run --config <(echo '{"upstreams":[{"host":"127.0.0.1","port":5672}]}') --port 5673

run-bench-servers: run-bench-upstream run-bench-trabbits

run-bench-client-direct:
	docker run -it --rm --network=host pivotalrabbitmq/perf-test:latest --uri amqp://admin:admin@127.0.0.1:5672 -z 60

run-bench-client-proxy:
	docker run -it --rm --network=host pivotalrabbitmq/perf-test:latest --uri amqp://admin:admin@127.0.0.1:5673 -z 60
