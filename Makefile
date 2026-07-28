SHELL := bash

.DEFAULT_GOAL := help
.PHONY: help check lint format test build docs-serve docs-build docs-deploy docs-clean

DOCS := docs
DOCS_PORT := 10000

# Everything in $(DOCS) is published, so the Python environment uv would
# otherwise create in $(DOCS)/.venv is kept out of it.
export UV_PROJECT_ENVIRONMENT := $(CURDIR)/.venv-docs

# The generator runs from the root, where zensical.toml is, so that the site
# and the build cache are written outside $(DOCS).
ZENSICAL := uv run --project $(DOCS)

help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "} {printf "  \033[1m%-12s\033[0m %s\n", $$1, $$2}'
	@echo
	@echo "Setup and prerequisites: docs/contribute/index.md"

check: lint test ## Run every quality gate (what CI runs, and what to run before pushing)

lint: ## Verify formatting, vet the code, and check the docs indexes
	@scripts/check-docs-index.sh
	@scripts/check-gofmt.sh
	@go vet ./...

format: ## Rewrite Go files into their canonical formatting
	@gofmt -l -w .

test: ## Run the Go test suite
	@go test ./...

build: ## Build the airfs command into bin/
	@go build -o bin/airfs ./cmd/airfs

docs-serve: ## Open the documentation with live reload (port: DOCS_PORT, default 10000)
	$(ZENSICAL) zensical serve --open -a 127.0.0.1:$(DOCS_PORT)

docs-build: ## Build the documentation site into site/
	$(ZENSICAL) zensical build --clean --strict

docs-deploy: ## Publish VERSION of the docs to gh-pages as the new "latest" (CI does this on release)
	@test -n "$(VERSION)" || { echo "docs-deploy needs VERSION, e.g. make docs-deploy VERSION=0.1"; exit 2; }
	$(ZENSICAL) mike deploy --push --update-aliases "$(VERSION)" latest
	# Point the bare site URL at the alias, so it keeps working when "latest"
	# moves on to the next release.
	$(ZENSICAL) mike set-default --push latest

docs-clean: ## Remove the built documentation site and its build cache
	rm -rf site .cache
