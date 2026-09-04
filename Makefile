BINARY  := macrarcli
BIN_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build test vet fmt run clean release-snapshot

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Usage: make run RAR=path/to/archive.rar
run: build
	./$(BIN_DIR)/$(BINARY) $(RAR)

# Dry-run release (no publish) — requires goreleaser in PATH.
release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf $(BIN_DIR)
