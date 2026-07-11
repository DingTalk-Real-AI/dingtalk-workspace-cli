GO ?= go

.PHONY: all help build rebuild test lint fmt policy edition-test generate-schema generate-schema-command-surface generate-schema-agent-metadata generate-schema-catalog package release publish-homebrew-formula setup-hooks

all: setup-hooks fmt lint build test rebuild

help:
	@printf "Available targets:\n"
	@printf "  make build         - Build the dws CLI binary\n"
	@printf "  make test          - Run the Go test suite\n"
	@printf "  make lint          - Run formatting checks and golangci-lint when available\n"
	@printf "  make fmt           - Format Go source files\n"
	@printf "  make policy        - Run open-source asset and command-surface checks\n"
	@printf "  make generate-schema - Regenerate embedded Agent metadata and the release Catalog\n"
	@printf "  make generate-schema-command-surface - Refresh the reviewed command surface\n"
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
	@./scripts/policy/check-command-surface.sh --strict
	@./scripts/policy/check-generated-drift.sh
	@./scripts/policy/check-schema-catalog.sh

edition-test:
	$(GO) test -v -count=1 ./pkg/editiontest/...

generate-schema:
	$(GO) generate ./internal/cli

generate-schema-command-surface:
	$(GO) run ./internal/generator/cmd_schema_agent_metadata \
		-root . \
		-write-surface internal/cli/schema_command_surface.json

generate-schema-agent-metadata:
	$(GO) run ./internal/generator/cmd_schema_agent_metadata \
		-root . \
		-surface internal/cli/schema_command_surface.json \
		-output-dir internal/cli/schema_agent_metadata \
		-audit-output internal/cli/schema_agent_metadata_audit.json

generate-schema-catalog:
	$(GO) run ./internal/generator/cmd_schema_catalog \
		-surface internal/cli/schema_command_surface.json \
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
