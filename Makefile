# KubeWhy — read-only Kubernetes troubleshooting CLI.

BINARY      := kubectl-why
PKG         := github.com/xavimf87/kubewhy
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE        ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w \
	-X $(PKG)/internal/version.Version=$(VERSION) \
	-X $(PKG)/internal/version.Commit=$(COMMIT) \
	-X $(PKG)/internal/version.Date=$(DATE)
GOBIN       ?= $(shell go env GOPATH)/bin

.DEFAULT_GOAL := build

.PHONY: build
build: ## Build the plugin binary into ./bin
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/$(BINARY)

.PHONY: install
install: ## Install the plugin into GOBIN so `kubectl why` works
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/$(BINARY)
	@echo "installed $(GOBIN)/$(BINARY); make sure $(GOBIN) is in your PATH"

.PHONY: test
test: ## Run the unit tests
	go test ./...

.PHONY: test-race
test-race: ## Run the unit tests with the race detector
	go test -race ./...

.PHONY: cover
cover: ## Run the tests and report coverage
	go test -coverprofile=coverage.txt ./...
	go tool cover -func=coverage.txt | tail -1

.PHONY: golden
golden: ## Rewrite the golden files for the text renderer
	go test ./internal/output -update

.PHONY: fmt
fmt: ## Format the code
	gofmt -s -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint when it is installed
	@command -v golangci-lint >/dev/null 2>&1 \
		&& golangci-lint run \
		|| echo "golangci-lint is not installed; see https://golangci-lint.run"

.PHONY: check
check: fmt vet test scripts ## Everything CI runs on a pull request

.PHONY: scripts
scripts: ## Check the shell that decides releases
	@for script in hack/*.sh test/e2e/*.sh; do bash -n "$$script" || exit 1; done
	@hack/next-version.test.sh

.PHONY: test-e2e
test-e2e: build ## Run the end-to-end scenarios against the current context
	./test/e2e/run.sh

.PHONY: clean
clean: ## Remove build output
	rm -rf bin dist coverage.txt

.PHONY: help
help: ## List the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
