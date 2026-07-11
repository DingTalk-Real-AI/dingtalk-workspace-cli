GO ?= go

.PHONY: all help build rebuild test lint fmt policy edition-test interface-integrity update-interface-baseline reset-interface-baseline schema-compatibility update-schema-baseline skill-command-integrity cli-smoke mock-mcp-smoke package release publish-homebrew-formula setup-hooks

all: setup-hooks fmt lint build test rebuild

help:
	@printf "Available targets:\n"
	@printf "  make build         - Build the dws CLI binary\n"
	@printf "  make test          - Run the Go test suite\n"
	@printf "  make lint          - Run formatting checks and golangci-lint when available\n"
	@printf "  make fmt           - Format Go source files\n"
	@printf "  make policy        - Run open-source asset and command-surface checks\n"
	@printf "  make interface-integrity - Check historical commands and help contracts still work\n"
	@printf "  make update-interface-baseline - Add new CLI contracts without removing history\n"
	@printf "  make reset-interface-baseline - DANGEROUS: replace all CLI compatibility history\n"
	@printf "  make schema-compatibility - Check schema list remains backwards compatible\n"
	@printf "  make update-schema-baseline - Add new schema contracts without removing history\n"
	@printf "  make skill-command-integrity - Check dws commands referenced by skills exist\n"
	@printf "  make cli-smoke     - Verify help for every public top-level command\n"
	@printf "  make mock-mcp-smoke - Verify HTTP and stdio MCP request/response transport\n"
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

interface-integrity:
	@./scripts/policy/check-interface-baseline.sh

update-interface-baseline:
	@./scripts/policy/check-interface-baseline.sh --update

reset-interface-baseline:
	@./scripts/policy/check-interface-baseline.sh --reset

schema-compatibility:
	@./scripts/policy/check-schema-list-compat.sh

update-schema-baseline:
	@./scripts/policy/check-schema-list-compat.sh --update

skill-command-integrity:
	@./scripts/policy/check-skill-commands.sh

cli-smoke:
	@./scripts/policy/check-cli-smoke.sh

mock-mcp-smoke:
	$(GO) test -v -count=1 -run '^(TestHTTPClientEndToEnd|TestStdioClientEndToEnd)$$' ./internal/transport

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
