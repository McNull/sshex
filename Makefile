.PHONY: build install test test-integration fmt lint clean release

VERSION ?=
LDFLAGS := -s -w -buildid= -X github.com/mcnull/sshex/internal/assets.version=$(VERSION)

build:
	CGO_ENABLED=0 go build -trimpath -tags osusergo,netgo -ldflags "$(LDFLAGS)" -o ./dist/sshex .

install: build
	sudo cp ./dist/sshex /usr/local/bin/sshex

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

release:
	@test -n "$(VERSION)" || { echo "usage: make release VERSION=x.y.z"; exit 1; }
	@echo "$(VERSION)" > internal/assets/VERSION.txt
	git add internal/assets/VERSION.txt
	git commit -m "chore(release): v$(VERSION)"
	git tag -a "v$(VERSION)" -m "v$(VERSION)"
	git push origin main
	git push origin "v$(VERSION)"
