BINARY  := tea-dash
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

PKG     := github.com/gbarany/tea-dash/internal/build
LDFLAGS := -s -w \
	-X $(PKG).Version=$(VERSION) \
	-X $(PKG).Commit=$(COMMIT) \
	-X $(PKG).Date=$(DATE)

.DEFAULT_GOAL := build

.PHONY: build
build: ## Build the binary into ./bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

.PHONY: run
run: ## Run the app
	go run .

.PHONY: install
install: ## Install into $GOBIN
	go install -ldflags "$(LDFLAGS)" .

.PHONY: test
test: ## Run tests with the race detector
	go test -race ./...

.PHONY: fmt
fmt: ## Format the code
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if the code is not gofmt-clean
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: public-hygiene
public-hygiene: ## Fail on local/private examples that should not be in the public repo
	bash scripts/check-public-hygiene.sh

.PHONY: lint
lint: ## Run golangci-lint (requires golangci-lint >= v2)
	golangci-lint run

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	go mod tidy

.PHONY: check
check: fmt-check vet test public-hygiene ## Run the full local check suite

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf bin dist

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
