GO ?= go
DWS_PACKAGE_VERSION ?= 0.0.0-test
REMOTE ?=
PUBLISH ?= 0
YES ?= 0
DWS_POLICY_TMPDIR ?= $(CURDIR)/.worktrees/policy-tmp
POLICY_GOTMPDIR ?= $(DWS_POLICY_TMPDIR)/go
POLICY_ENV = DWS_POLICY_TMPDIR="$(DWS_POLICY_TMPDIR)" GOTMPDIR="$(POLICY_GOTMPDIR)"
GO_SOURCE_LIST = git ls-files -z --cached --others --exclude-standard -- '*.go'

.PHONY: all help build rebuild test test-plan lint format-check fmt policy edition-test interface-integrity authoritative-interface-integrity coverage-gate coverage-gate-platform update-interface-baseline reset-interface-baseline schema-compatibility skill-command-integrity cli-smoke mock-mcp-smoke test-schema-agent-examples generate-schema generate-schema-agent-metadata generate-schema-catalog package release release-pre release-stable changelog-pre changelog-stable publish-homebrew-formula setup-hooks

all: setup-hooks fmt lint build test rebuild

help:
	@printf "Available targets:\n"
	@printf "  make build         - Build the dws CLI binary\n"
	@printf "  make test          - Run the Go test suite\n"
	@printf "  make test-plan     - Verify every default Go package belongs to one CI test shard\n"
	@printf "  make lint          - Run formatting checks, go vet, and staticcheck\n"
	@printf "  make format-check  - Check all repository Go source files with gofmt\n"
	@printf "  make fmt           - Format all repository Go source files\n"
	@printf "  make policy        - Check the built dws plus open-source and Schema policies\n"
	@printf "  make interface-integrity - Check historical commands and help contracts still work\n"
	@printf "  make authoritative-interface-integrity BASE_REF=<ref> - Check the Git-owned PR merge-base\n"
	@printf "  make coverage-gate BASE_REF=<ref> - Enforce overall non-regression and 100%% changed-code coverage\n"
	@printf "  make coverage-gate-platform BASE_REF=<ref> PROFILE=<file> - Enforce 100%% native changed-code coverage\n"
	@printf "  make update-interface-baseline - Add new CLI contracts without removing history\n"
	@printf "  make reset-interface-baseline - DANGEROUS: replace all CLI compatibility history\n"
	@printf "  make schema-compatibility BASE_REF=<ref> - Check the complete Schema contract against the PR merge-base\n"
	@printf "  make skill-command-integrity - Check dws commands referenced by skills exist\n"
	@printf "  make cli-smoke     - Verify help for every public top-level command\n"
	@printf "  make mock-mcp-smoke - Verify HTTP and stdio MCP request/response transport\n"
	@printf "  make test-schema-agent-examples - Contract-check all Agent examples and dry-run the eligible subset\n"
	@printf "  make generate-schema - Regenerate embedded Agent metadata and the release Catalog\n"
	@printf "  make generate-schema-agent-metadata - Regenerate versioned Agent metadata\n"
	@printf "  make generate-schema-catalog - Regenerate the embedded release Catalog\n"
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
	@DWS_PACKAGE_VERSION="$(DWS_PACKAGE_VERSION)" $(GO) test -count=1 -timeout=10m ./...

test-plan:
	@./scripts/ci/test-packages.sh verify

lint:
	@./scripts/dev/lint.sh

format-check:
	@set -eu; \
	go_files="$$(mktemp "$${TMPDIR:-/tmp}/dws-go-files.XXXXXX")"; \
	trap 'rm -f "$$go_files"' EXIT HUP INT TERM; \
	$(GO_SOURCE_LIST) > "$$go_files"; \
	unformatted="$$(xargs -0 sh -c 'if [ "$$#" -gt 0 ]; then exec gofmt -l -- "$$@"; fi' sh < "$$go_files")"; \
	if [ -n "$$unformatted" ]; then \
		printf '%s\n' "$$unformatted"; \
		printf '%s\n' "Go files are not formatted. Run 'make fmt'." >&2; \
		exit 1; \
	fi

fmt:
	@set -eu; \
	go_files="$$(mktemp "$${TMPDIR:-/tmp}/dws-go-files.XXXXXX")"; \
	trap 'rm -f "$$go_files"' EXIT HUP INT TERM; \
	$(GO_SOURCE_LIST) > "$$go_files"; \
	xargs -0 sh -c 'if [ "$$#" -gt 0 ]; then exec gofmt -w -- "$$@"; fi' sh < "$$go_files"

policy:
	@mkdir -p "$(POLICY_GOTMPDIR)"
	@$(POLICY_ENV) ./scripts/policy/check-open-source-assets.sh
	@$(POLICY_ENV) ./scripts/policy/check-schema-command-registry.sh
	@$(POLICY_ENV) ./scripts/policy/check-command-surface.sh --strict
	@$(POLICY_ENV) ./scripts/policy/check-generated-drift.sh
	@$(POLICY_ENV) ./scripts/policy/check-schema-catalog.sh
	@$(POLICY_ENV) ./scripts/policy/check-schema-binary.sh
	@$(POLICY_ENV) $(MAKE) test-schema-agent-examples

edition-test:
	$(GO) test -v -count=1 ./pkg/editiontest/...

interface-integrity:
	@./scripts/policy/check-interface-baseline.sh

authoritative-interface-integrity:
	@./scripts/policy/check-authoritative-interface-baselines.sh --base-ref "$(BASE_REF)"

coverage-gate:
	@./scripts/policy/check-coverage-gate.sh --base-ref "$(BASE_REF)" --scope-buildable

coverage-gate-platform:
	@./scripts/policy/run-platform-coverage-gate.sh --base-ref "$(BASE_REF)" --profile "$(PROFILE)"

update-interface-baseline:
	@./scripts/policy/check-interface-baseline.sh --update

reset-interface-baseline:
	@./scripts/policy/check-interface-baseline.sh --reset

schema-compatibility:
	@./scripts/policy/check-authoritative-schema-compatibility.sh --base-ref "$(BASE_REF)"

skill-command-integrity:
	@./scripts/policy/check-skill-commands.sh

cli-smoke:
	@./scripts/policy/check-cli-smoke.sh

mock-mcp-smoke:
	$(GO) test -v -count=1 -run '^(TestHTTPClientEndToEnd|TestStdioClientEndToEnd)$$' ./internal/transport

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
