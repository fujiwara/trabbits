.PHONY: clean test
export SHELL := /bin/bash

trabbits: gen go.* *.go
	go build -o $@ ./cmd/trabbits

clean:
	rm -rf trabbits dist/

test:
	RABBITMQ_HEALTH_PASS=healthpass go test ./... -count=1 --timeout=90s

test-coverage:
	RABBITMQ_HEALTH_PASS=healthpass go test -coverprofile=coverage.out -coverpkg=./... ./... -count=1 --timeout=90s
	go tool cover -html=coverage.out -o coverage.html
	./scripts/coverage-summary.sh

install:
	go install github.com/fujiwara/trabbits/cmd/trabbits

dist:
	goreleaser build --snapshot --clean

gen:
	go run spec/gen.go < spec/amqp0-9-1.stripped.extended.xml | gofmt > amqp091/spec091.go

_run-bench-rabbitmq:
	docker run -t --network=host --env RABBITMQ_DEFAULT_USER=admin --env RABBITMQ_DEFAULT_PASS=admin --cpus=1 mirror.gcr.io/rabbitmq:3.12-management

_run-bench-trabbits: trabbits
	ENABLE_PPROF=true ./trabbits run --config <(echo '{"upstreams":[{"name":"single","address":"127.0.0.1:5672"}]}') --port 6672

run-bench-servers: _run-bench-rabbitmq _run-bench-trabbits

run-bench-pprof:
	go tool pprof -seconds 30 -http=localhost:1080 http://localhost:6060/debug/pprof/profile

run-bench-trabbits:
	docker run -i --rm --network=host --cpus=1 mirror.gcr.io/pivotalrabbitmq/perf-test:latest --uri amqp://admin:admin@127.0.0.1:6672 -z 30 $(BENCH_ARGS)

run-bench-rabbitmq:
	docker run -i --rm --network=host --cpus=1 mirror.gcr.io/pivotalrabbitmq/perf-test:latest --uri amqp://admin:admin@127.0.0.1:5672 -z 30 $(BENCH_ARGS)

setup-compose-cluster:
	docker compose -f compose.yml exec rabbitmq1 sh -c "rabbitmqctl wait --pid 1 --timeout 60"
	docker compose -f compose.yml exec rabbitmq2 sh -c "rabbitmqctl wait --pid 1 --timeout 60"
	docker compose -f compose.yml exec rabbitmq3 sh -c "rabbitmqctl wait --pid 1 --timeout 60"
	docker compose -f compose.yml exec rabbitmq4 sh -c "rabbitmqctl wait --pid 1 --timeout 60"
	docker compose -f compose.yml exec rabbitmq3 sh -c "rabbitmqctl stop_app && rabbitmqctl reset && rabbitmqctl join_cluster rabbit@rabbitmq2 && rabbitmqctl start_app"
	docker compose -f compose.yml exec rabbitmq4 sh -c "rabbitmqctl stop_app && rabbitmqctl reset && rabbitmqctl join_cluster rabbit@rabbitmq2 && rabbitmqctl start_app"
	docker compose -f compose.yml exec rabbitmq2 sh -c "rabbitmqctl cluster_status"
	# Create health check user on standalone instance
	docker compose -f compose.yml exec rabbitmq1 sh -c "rabbitmqctl add_user healthcheck healthpass || true"
	docker compose -f compose.yml exec rabbitmq1 sh -c "rabbitmqctl set_user_tags healthcheck monitoring"
	docker compose -f compose.yml exec rabbitmq1 sh -c "rabbitmqctl set_permissions -p / healthcheck '.*' '.*' '.*'"
	# Create health check user on cluster (only need to add to one node, will replicate to others)
	docker compose -f compose.yml exec rabbitmq2 sh -c "rabbitmqctl add_user healthcheck healthpass || true"
	docker compose -f compose.yml exec rabbitmq2 sh -c "rabbitmqctl set_user_tags healthcheck monitoring"
	docker compose -f compose.yml exec rabbitmq2 sh -c "rabbitmqctl set_permissions -p / healthcheck '.*' '.*' '.*'"
