GO ?= go
VERSION ?= 0.0.0-SNAPSHOT
SCHEMA_REGISTRY ?= $(HOME)/.dws/cache/default_default/market/servers.json
SCHEMA_TOOLS_DIR ?= $(patsubst %/market/servers.json,%/tools,$(SCHEMA_REGISTRY))
SCHEMA_INTERFACE_HINTS ?= skills/mono/schema-hints
SCHEMA_SOURCE_REVISION ?=
WUKONG_ENVELOPE_DIR ?=
WUKONG_REVISION ?=

.PHONY: all help build rebuild test lint fmt policy edition-test generate-schema-command-surface generate-schema-agent-metadata generate-schema-interface-metadata generate-schema-catalog generate-schema-wukong-agent-hints package release publish-homebrew-formula setup-hooks

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
	@printf "  make generate-schema-catalog - Freeze the reviewed release Schema catalog\n"
	@printf "  make generate-schema-wukong-agent-hints - Import sanitized Wukong envelope descriptions at WUKONG_REVISION\n"
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
	@./scripts/policy/check-schema-catalog.sh

edition-test:
	$(GO) test -v -count=1 ./pkg/editiontest/...

generate-schema-command-surface:
	$(GO) run ./internal/generator/cmd_schema_agent_metadata -root . -write-surface internal/cli/schema_command_surface.json

generate-schema-agent-metadata:
	$(GO) generate ./internal/cli

generate-schema-interface-metadata:
	@test -f "$(SCHEMA_REGISTRY)" || (printf 'missing SCHEMA_REGISTRY: %s\n' "$(SCHEMA_REGISTRY)" >&2; exit 1)
	@test -d "$(SCHEMA_TOOLS_DIR)" || (printf 'missing SCHEMA_TOOLS_DIR: %s\n' "$(SCHEMA_TOOLS_DIR)" >&2; exit 1)
	$(GO) run ./internal/generator/cmd_schema_metadata \
		-registry "$(SCHEMA_REGISTRY)" \
		-tools-dir "$(SCHEMA_TOOLS_DIR)" \
		-surface internal/cli/schema_command_surface.json \
		-hints "$(SCHEMA_INTERFACE_HINTS)" \
		-source-revision "$(SCHEMA_SOURCE_REVISION)" \
		-output internal/cli/schema_mcp_metadata.json

generate-schema-catalog:
	$(GO) run ./internal/generator/cmd_schema_catalog \
		-surface internal/cli/schema_command_surface.json \
		-output internal/cli/schema_catalog.json

generate-schema-wukong-agent-hints:
	@test -d "$(WUKONG_ENVELOPE_DIR)" || (printf 'missing WUKONG_ENVELOPE_DIR: %s\n' "$(WUKONG_ENVELOPE_DIR)" >&2; exit 1)
	@test -n "$(WUKONG_REVISION)" || (printf '%s\n' 'missing WUKONG_REVISION' >&2; exit 1)
	$(GO) run ./internal/generator/cmd_wukong_agent_hints \
		-envelope-dir "$(WUKONG_ENVELOPE_DIR)" \
		-surface internal/cli/schema_command_surface.json \
		-revision "$(WUKONG_REVISION)" \
		-output skills/mono/schema-hints/imported/wukong.json \
		-audit-output internal/cli/schema_wukong_agent_hints_audit.json

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
