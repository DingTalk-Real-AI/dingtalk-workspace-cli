GO ?= go
VERSION ?= 0.0.0-SNAPSHOT
SCHEMA_REGISTRY ?= $(HOME)/.dws/cache/default_default/market/servers.json

.PHONY: all help build rebuild test lint fmt policy edition-test generate-schema-command-surface generate-schema-agent-metadata generate-schema-interface-metadata package release publish-homebrew-formula setup-hooks

all: setup-hooks fmt lint build test rebuild

help:
	@printf "Available targets:\n"
	@printf "  make build         - Build the dws CLI binary\n"
	@printf "  make test          - Run the Go test suite\n"
	@printf "  make lint          - Run formatting checks and golangci-lint when available\n"
	@printf "  make fmt           - Format Go source files\n"
	@printf "  make policy        - Run open-source asset and command-surface checks\n"
	@printf "  make generate-schema-command-surface - Refresh the versioned Agent command-surface snapshot\n"
	@printf "  make generate-schema-agent-metadata - Regenerate embedded Agent schema metadata from skills\n"
	@printf "  make generate-schema-interface-metadata - Snapshot sanitized CLI/MCP metadata from SCHEMA_REGISTRY\n"
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

edition-test:
	$(GO) test -v -count=1 ./pkg/editiontest/...

generate-schema-command-surface:
	$(GO) run ./internal/generator/cmd_schema_agent_metadata -root . -write-surface internal/cli/schema_command_surface.json

generate-schema-agent-metadata:
	$(GO) generate ./internal/cli

generate-schema-interface-metadata:
	@test -f "$(SCHEMA_REGISTRY)" || (printf 'missing SCHEMA_REGISTRY: %s\n' "$(SCHEMA_REGISTRY)" >&2; exit 1)
	$(GO) run ./internal/generator/cmd_schema_metadata -registry "$(SCHEMA_REGISTRY)" -output internal/cli/schema_mcp_metadata.json

package:
	@VERSION="$(VERSION)" ./scripts/dev/build-all.sh
	@DWS_PACKAGE_VERSION="$(VERSION)" ./scripts/release/post-goreleaser.sh

publish-homebrew-formula:
	@./scripts/release/publish-homebrew-formula.sh

setup-hooks:
	@git config core.hooksPath scripts/hooks 2>/dev/null || true

release:
	goreleaser release --clean
	@./scripts/release/post-goreleaser.sh
