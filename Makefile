VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-X main.version=$(VERSION)" -o licgate ./cmd/licgate

test:
	go vet ./...
	go test ./...

check: build
	./licgate -l MIT --no-tui

.PHONY: build test check
