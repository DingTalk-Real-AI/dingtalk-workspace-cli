GO ?= go
export GOCACHE ?= $(CURDIR)/.cache/go-build
export XDG_CACHE_HOME ?= $(CURDIR)/.cache/xdg
export STATICCHECK_CACHE ?= $(CURDIR)/.cache/staticcheck

.PHONY: all help build rebuild test lint fmt policy package release publish-homebrew-formula build-all setup-hooks generate-skills

all: setup-hooks fmt lint build test rebuild

help:
	@printf "Available targets:\n"
	@printf "  make build         - Build the dws CLI binary\n"
	@printf "  make test          - Run the Go test suite\n"
	@printf "  make lint          - Run formatting checks and golangci-lint when available\n"
	@printf "  make fmt           - Format Go source files\n"
	@printf "  make build-all     - Build cross-platform archives without goreleaser\n"
	@printf "  make policy        - Run open-source asset and command-surface checks\n"
	@printf "  make package       - Build all release artifacts locally (goreleaser snapshot)\n"
	@printf "  make release       - Build and publish a release via goreleaser\n"
	@printf "  make publish-homebrew-formula - Push dist/homebrew/dingtalk-workspace-cli.rb to a tap repo\n"
	@printf "  make generate-skills - Regenerate skills definitions to keep them up-to-date\n"

build:
	@./scripts/dev/build.sh

rebuild:
	@./scripts/dev/build.sh

test:
	@./test/scripts/run_all_tests.sh --jobs 1

lint:
	@./scripts/dev/lint.sh

fmt:
	@find cmd internal test -name '*.go' -print0 2>/dev/null | xargs -0r gofmt -w

build-all:
	@./scripts/dev/build-all.sh

policy:
	@./scripts/policy/check-open-source-assets.sh
	@./scripts/policy/check-command-surface.sh --strict

package:
	goreleaser release --snapshot --clean
	@./scripts/release/post-goreleaser.sh

publish-homebrew-formula:
	@./scripts/release/publish-homebrew-formula.sh

setup-hooks:
	@git config core.hooksPath scripts/hooks 2>/dev/null || true

generate-skills: build
	@./dws generate-skills

release:
	goreleaser release --clean
	@./scripts/release/post-goreleaser.sh
