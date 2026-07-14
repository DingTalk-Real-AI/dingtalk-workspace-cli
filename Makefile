GO ?= go

.PHONY: all help build rebuild test lint fmt policy edition-test test-schema-agent-examples generate-schema generate-schema-agent-metadata generate-schema-catalog package release publish-homebrew-formula setup-hooks

all: setup-hooks fmt lint build test rebuild

help:
	@printf "Available targets:\n"
	@printf "  make build         - Build the dws CLI binary\n"
	@printf "  make test          - Run the Go test suite\n"
	@printf "  make lint          - Run formatting checks and golangci-lint when available\n"
	@printf "  make fmt           - Format Go source files\n"
	@printf "  make policy        - Run open-source asset and Schema registry checks\n"
	@printf "  make test-schema-agent-examples - Contract-check all Agent examples and dry-run the eligible subset\n"
	@printf "  make generate-schema - Regenerate embedded Agent metadata and the release Catalog\n"
	@printf "  make generate-schema-agent-metadata - Regenerate versioned Agent metadata\n"
	@printf "  make generate-schema-catalog - Regenerate the embedded release Catalog\n"
	@printf "  make package       - Build all release artifacts locally (goreleaser snapshot)\n"
	@printf "  make release       - Build and publish a release via goreleaser\n"
	@printf "  make publish-homebrew-formula - Push dist/homebrew/dingtalk-workspace-cli.rb to a tap repo\n"

build:
	@./scripts/dev/build.sh

rebuild:
	@./scripts/dev/build.sh

test:
	@./test/scripts/run_all_tests.sh

lint:
	@./scripts/dev/lint.sh

fmt:
	@find cmd internal test -name '*.go' -print0 2>/dev/null | xargs -0r gofmt -w

policy:
	@./scripts/policy/check-open-source-assets.sh
	@./scripts/policy/check-schema-command-registry.sh
	@./scripts/policy/check-command-surface.sh --strict
	@./scripts/policy/check-generated-drift.sh
	@./scripts/policy/check-schema-catalog.sh
	@./scripts/policy/check-schema-binary.sh
	@$(MAKE) test-schema-agent-examples

edition-test:
	$(GO) test -v -count=1 ./pkg/editiontest/...

test-schema-agent-examples:
	DWS_AGENT_EXAMPLES_DRY_RUN=1 $(GO) test -v -count=1 ./internal/app -run '^TestManualAgentExamplesDryRun$$'

generate-schema:
	@set -e; \
	registry_guard=$$(mktemp); \
	metadata_guard=$$(mktemp -d); \
	selection_guard=$$(mktemp -d); \
	trap 'rm -rf "$$registry_guard" "$$metadata_guard" "$$selection_guard"' EXIT HUP INT TERM; \
	cp internal/cli/schema_command_registry.json "$$registry_guard"; \
	cp -R internal/cli/schema_hints/metadata/. "$$metadata_guard/"; \
	cp -R internal/cli/schema_hints/selection/. "$$selection_guard/"; \
	$(GO) generate ./internal/cli; \
	cmp -s internal/cli/schema_command_registry.json "$$registry_guard" || { \
		printf '%s\n' 'generation modified reviewed input internal/cli/schema_command_registry.json' >&2; \
		exit 1; \
	}; \
	diff -qr internal/cli/schema_hints/metadata "$$metadata_guard" >/dev/null || { \
		printf '%s\n' 'generation modified reviewed input internal/cli/schema_hints/metadata' >&2; \
		exit 1; \
	}; \
	diff -qr internal/cli/schema_hints/selection "$$selection_guard" >/dev/null || { \
		printf '%s\n' 'generation modified reviewed input internal/cli/schema_hints/selection' >&2; \
		exit 1; \
	}

generate-schema-agent-metadata:
	$(GO) run ./internal/generator/cmd_schema_agent_metadata \
		-root . \
		-registry internal/cli/schema_command_registry.json \
		-output-dir internal/cli/schema_agent_metadata \
		-audit-output internal/cli/schema_agent_metadata_audit.json

generate-schema-catalog:
	$(GO) run -a ./internal/generator/cmd_schema_catalog \
		-root . \
		-output internal/cli/schema_catalog.json

package:
	@./scripts/dev/build-all.sh
	@./scripts/release/post-goreleaser.sh

publish-homebrew-formula:
	@./scripts/release/publish-homebrew-formula.sh

setup-hooks:
	@git config core.hooksPath scripts/hooks 2>/dev/null || true

release:
	goreleaser release --clean
	@./scripts/release/post-goreleaser.sh
