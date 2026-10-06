.PHONY: build install test test-integration fmt lint clean

build:
	go build -o ./dist/sshex .

install: build
	sudo cp ./dist/sshex /usr/bin/sshex

test:
	go test -race ./...

test-integration:
	go test -race -tags=integration ./test/integration/...

fmt:
	gofmt -w .

lint:
	gofmt -l .
	go vet ./...

clean:
	go clean
