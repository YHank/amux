BINARY := amux
PKG    := ./cmd/amux
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PREFIX ?= /usr/local
BUILD_INSTALL_PATH ?= $(HOME)/.local/bin/$(BINARY)

.PHONY: build install test vet fmt run clean

build: ## Build ./amux and copy it to $(BUILD_INSTALL_PATH)
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)
	install -d "$$(dirname "$(BUILD_INSTALL_PATH)")"
	install -m 0755 $(BINARY) "$(BUILD_INSTALL_PATH)"

install: build ## Build and install to $(PREFIX)/bin
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
