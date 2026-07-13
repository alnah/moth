SHELL := /bin/sh

GO ?= go
GORELEASER ?= goreleaser

GOIMPORTS_LOCAL := github.com/alnah/moth
BROWSER_TEST_ENV := MOTH_BROWSER_REQUIRED=1

.PHONY: help
help: ## Show available targets.
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "%-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: quick
quick: fmt-check lint test build ## Run fast local checks.

.PHONY: check
check: actionlint mod-check fmt-check fix-check lint vet build goreleaser-check test test-race cover govulncheck ## Run CI-equivalent checks.

.PHONY: ci
ci: check test-browser cover-browser ## Run all required CI checks, including browser-tag checks.

.PHONY: fmt
fmt: ## Format Go files.
	$(GO)fmt -w .
	$(GO) tool goimports -w -local $(GOIMPORTS_LOCAL) .

.PHONY: fmt-check
fmt-check: ## Check Go formatting and imports.
	@test -z "$$($(GO)fmt -l .)"
	@output="$$($(GO) tool goimports -l -local $(GOIMPORTS_LOCAL) .)"; \
	if [ -n "$$output" ]; then printf '%s\n' "$$output"; exit 1; fi

.PHONY: fix
fix: ## Apply Go modernizer fixes.
	$(GO) fix ./...
	$(GO)fmt -w .
	$(GO) tool goimports -w -local $(GOIMPORTS_LOCAL) .

.PHONY: fix-check
fix-check: ## Check whether Go modernizer fixes are pending.
	$(GO) fix -diff ./...

.PHONY: mod-check
mod-check: ## Check go.mod and go.sum are tidy.
	$(GO) mod tidy
	git diff --exit-code go.mod go.sum

.PHONY: lint
lint: ## Run golangci-lint.
	$(GO) tool golangci-lint run

.PHONY: vet
vet: ## Run go vet.
	$(GO) vet ./...

.PHONY: test
test: ## Run tests.
	$(GO) test ./...

.PHONY: test-race
test-race: ## Run race tests.
	$(GO) test -race ./...

.PHONY: test-browser
test-browser: ## Run browser-tag integration tests with a required browser.
	$(BROWSER_TEST_ENV) $(GO) test -tags browser ./internal/browser

.PHONY: test-integration
test-integration: ## Run optional integration tests that require external tools.
	$(GO) test -tags integration ./...

.PHONY: cover
cover: ## Run coverage and print summary.
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

.PHONY: cover-browser
cover-browser: ## Run browser-tag coverage and print summary.
	$(GO) test -tags browser -coverprofile=coverage-browser.out ./...
	$(GO) tool cover -func=coverage-browser.out

.PHONY: build
build: ## Build all packages.
	$(GO) build ./...

.PHONY: install
install: ## Install moth into GOPATH/bin or GOBIN.
	$(GO) install ./cmd/moth

.PHONY: actionlint
actionlint: ## Lint GitHub Actions workflows.
	$(GO) tool actionlint .github/workflows/ci.yml .github/workflows/release.yml

.PHONY: govulncheck
govulncheck: ## Check reachable Go vulnerabilities.
	$(GO) tool govulncheck ./...

.PHONY: goreleaser-check
goreleaser-check: ## Check GoReleaser config.
	$(GORELEASER) check

.PHONY: snapshot
snapshot: ## Build local GoReleaser snapshot artifacts.
	$(GORELEASER) release --snapshot --clean

.PHONY: clean
clean: ## Remove generated local artifacts.
	rm -rf dist coverage.out coverage-browser.out
