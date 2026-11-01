.PHONY: build install dist test test-integration fmt lint clean release

VERSION ?=
LDFLAGS := -s -w -buildid= -X github.com/mcnull/sshex/internal/assets.version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

build:
	CGO_ENABLED=0 go build -trimpath -tags osusergo,netgo -ldflags "$(LDFLAGS)" -o ./dist/sshex .

# dist cross-compiles sshex for every platform it can install itself on. The
# binary installer caches platform builds under the sshex data directory.
dist:
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		echo "building sshex-$$os-$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -tags osusergo,netgo -ldflags "$(LDFLAGS)" -o "./dist/sshex-$$os-$$arch" . || exit 1; \
	done

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
	rm -rf ./dist

release:
	@test -n "$(VERSION)" || { echo "usage: make release VERSION=x.y.z"; exit 1; }
	@echo "$(VERSION)" > internal/assets/VERSION.txt
	git add internal/assets/VERSION.txt
	git commit -m "chore(release): v$(VERSION)"
	git tag -a "v$(VERSION)" -m "v$(VERSION)"
	git push origin main
	git push origin "v$(VERSION)"
