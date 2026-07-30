BINARY := amux
PKG    := ./cmd/amux
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PREFIX ?= /usr/local

.PHONY: build install test vet fmt run clean

build: ## Build the amux binary into ./amux
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install: ## Build and install to $(PREFIX)/bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

test: ## Run unit tests
	go test ./...

vet: ## Static analysis
	go vet ./...

fmt: ## Format
	gofmt -w .

run: build ## Build and run
	./$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf dist
