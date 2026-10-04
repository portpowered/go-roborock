GO ?= go
GOLANGCI_LINT_VERSION := v2.14.0
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
export GOWORK := $(abspath go.work)

.DEFAULT_GOAL := check
.PHONY: check lint build test coverage generate generated contracts modules cli formatting inventory integration-skip

check: formatting lint generated contracts inventory modules build test coverage cli integration-skip

integration-skip:
	$(GO) run ./tools/integrationcheck

inventory:
	$(GO) run ./tools/inventory -check

formatting:
	$(GO) run ./tools/formatcheck

lint:
	$(GO) vet ./...
	$(GO) vet -tags=integration ./tests/integration/...
	$(GOLANGCI_LINT) run --build-tags integration --allow-parallel-runners --config .golangci.yml --timeout=10m ./...
	$(GO) -C cmd/go-roborock vet ./...
	$(GO) -C cmd/go-roborock run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --allow-parallel-runners --config ../../.golangci.yml --timeout=10m ./...

generate:
	$(GO) run ./tools/generate
	$(GO) run ./tools/publicdocs

generated:
	$(GO) run ./tools/generate -check
	$(GO) run ./tools/publicdocs -check
	git diff --exit-code -- "*.gen.go" "api/*.openapi.yaml"
	$(GO) run ./tools/generatedfiles

contracts:
	$(GO) run ./tools/contracts

modules:
	$(GO) mod tidy -diff
	$(GO) run ./tools/modules

build:
	$(GO) build ./...

test:
	$(GO) test -race ./...

coverage:
	$(GO) test -race -coverpkg=./pkg/... -coverprofile=coverage.out ./pkg/... ./tests/replay/...
	$(GO) run ./tools/coverage -profile coverage.out -min 80 -filtered-profile coverage.filtered.out

cli:
	$(GO) -C cmd/go-roborock build ./...
	$(GO) -C cmd/go-roborock test -race ./...
