GO ?= go
REMOTE ?=
PUBLISH ?= 0
YES ?= 0

.PHONY: all help build rebuild test lint fmt policy edition-test package release release-pre release-stable changelog-pre changelog-stable publish-homebrew-formula setup-hooks

all: setup-hooks fmt lint build test rebuild

help:
	@printf "Available targets:\n"
	@printf "  make build         - Build the dws CLI binary\n"
	@printf "  make test          - Run the Go test suite\n"
	@printf "  make lint          - Run formatting checks and golangci-lint when available\n"
	@printf "  make fmt           - Format Go source files\n"
	@printf "  make policy        - Run open-source asset and command-surface checks\n"
	@printf "  make package       - Build all release artifacts locally\n"
	@printf "  make changelog-pre VERSION=vX.Y.Z-beta.N - Prepare prerelease notes\n"
	@printf "  make changelog-stable VERSION=vX.Y.Z FROM_BETA=vX.Y.Z-beta.N - Prepare stable notes\n"
	@printf "  make release-pre VERSION=vX.Y.Z-beta.N [PUBLISH=1] - Validate or publish prerelease\n"
	@printf "  make release-stable VERSION=vX.Y.Z FROM_BETA=vX.Y.Z-beta.N [PUBLISH=1] - Validate or publish stable\n"
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

package:
	@version="$(if $(VERSION),$(VERSION),v0.0.0-SNAPSHOT)"; VERSION="$${version#v}" ./scripts/dev/build-all.sh
	@version="$(if $(VERSION),$(VERSION),v0.0.0-SNAPSHOT)"; DWS_PACKAGE_VERSION="$$version" ./scripts/release/post-goreleaser.sh

publish-homebrew-formula:
	@./scripts/release/publish-homebrew-formula.sh

setup-hooks:
	@git config core.hooksPath scripts/hooks 2>/dev/null || true

changelog-pre:
	@test -n "$(VERSION)" || (printf 'VERSION is required, e.g. v1.2.3-beta.1\n' >&2; exit 2)
	@./scripts/release/prepare-changelog.sh prerelease "$(VERSION)"

changelog-stable:
	@test -n "$(VERSION)" || (printf 'VERSION is required, e.g. v1.2.3\n' >&2; exit 2)
	@test -n "$(FROM_BETA)" || (printf 'FROM_BETA is required, e.g. v1.2.3-beta.2\n' >&2; exit 2)
	@./scripts/release/prepare-changelog.sh stable "$(VERSION)" --from-beta "$(FROM_BETA)"

release-pre:
	@test -n "$(VERSION)" || (printf 'VERSION is required, e.g. v1.2.3-beta.1\n' >&2; exit 2)
	@test -n "$(REMOTE)" || (printf 'REMOTE is required, e.g. origin\n' >&2; exit 2)
	@args=""; \
	  if [ "$(PUBLISH)" = "1" ]; then args="$$args --publish"; fi; \
	  if [ "$(YES)" = "1" ]; then args="$$args --yes"; fi; \
	  ./scripts/release/release.sh prerelease "$(VERSION)" --remote "$(REMOTE)" $$args

release-stable:
	@test -n "$(VERSION)" || (printf 'VERSION is required, e.g. v1.2.3\n' >&2; exit 2)
	@test -n "$(FROM_BETA)" || (printf 'FROM_BETA is required, e.g. v1.2.3-beta.2\n' >&2; exit 2)
	@test -n "$(REMOTE)" || (printf 'REMOTE is required, e.g. origin\n' >&2; exit 2)
	@args=""; \
	  if [ "$(PUBLISH)" = "1" ]; then args="$$args --publish"; fi; \
	  if [ "$(YES)" = "1" ]; then args="$$args --yes"; fi; \
	  ./scripts/release/release.sh stable "$(VERSION)" --from-beta "$(FROM_BETA)" --remote "$(REMOTE)" $$args

release:
	@printf 'Use make release-pre or make release-stable; direct goreleaser publishing is disabled.\n' >&2
	@exit 2
