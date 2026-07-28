SHELL := bash

.DEFAULT_GOAL := help
.PHONY: help check lint docs-serve docs-build docs-clean

DOCS := docs
DOCS_PORT := 10000

# The site generator treats all of $(DOCS) as content, so the Python
# environment must live outside it or it ends up in the published site.
export UV_PROJECT_ENVIRONMENT := $(CURDIR)/.venv-docs

help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "} {printf "  \033[1m%-12s\033[0m %s\n", $$1, $$2}'
	@echo
	@echo "Setup and prerequisites: docs/contribute/index.md"

check: lint ## Run every quality gate (what CI runs, and what to run before pushing)

lint: ## Verify the docs indexes list every spec and how-to document
	@scripts/check-docs-index.sh

docs-serve: ## Open the documentation with live reload (port: DOCS_PORT, default 10000)
	cd $(DOCS) && uv run zensical serve --open -a 127.0.0.1:$(DOCS_PORT)

docs-build: ## Build the documentation site into docs/build
	cd $(DOCS) && uv run zensical build --clean --strict

docs-clean: ## Remove the built documentation site and its build cache
	rm -rf $(DOCS)/build $(DOCS)/.cache
