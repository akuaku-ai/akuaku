# Akuaku developer tasks. Run `make help` to list available targets.

GO           ?= go
BINARY       := akuaku
PKG          := ./...
COVERPROFILE := coverage.out
# Stamp the binary with the current tag (or commit) so `akuaku version` is real.
VERSION      := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS      := -X main.version=$(VERSION)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "} {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the akuaku binary into bin/.
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

.PHONY: run
run: ## Run akuaku from source.
	$(GO) run -ldflags "$(LDFLAGS)" ./cmd/$(BINARY)

.PHONY: fmt
fmt: ## Format the code with gofmt.
	gofmt -s -w .

.PHONY: vet
vet: ## Run go vet.
	$(GO) vet $(PKG)

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run

.PHONY: test
test: ## Run tests with the race detector.
	$(GO) test -race $(PKG)

.PHONY: cover
cover: ## Run tests and print total coverage.
	$(GO) test -race -covermode=atomic -coverprofile=$(COVERPROFILE) $(PKG)
	$(GO) tool cover -func=$(COVERPROFILE) | tail -n 1

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum.
	$(GO) mod tidy

.PHONY: check
check: fmt vet lint test ## Run fmt, vet, lint, and test.
