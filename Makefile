.PHONY: clean test

trabbits: gen go.* *.go
	go build -o $@ ./cmd/trabbits

clean:
	rm -rf trabbits dist/

test:
	go test -v ./...

install:
	go install github.com/fujiwara/trabbits/cmd/trabbits

dist:
	goreleaser build --snapshot --clean

gen:
	go run spec/gen.go < spec/amqp0-9-1.stripped.extended.xml | gofmt > amqp091/spec091.go
