GO ?= go
DWS_PACKAGE_VERSION ?= 0.0.0-test
REMOTE ?=
PUBLISH ?= 0
YES ?= 0
DWS_POLICY_TMPDIR ?= $(CURDIR)/.worktrees/policy-tmp
POLICY_GOTMPDIR ?= $(DWS_POLICY_TMPDIR)/go
SCHEMA_CATALOG_OUTPUT ?= artifacts/schema_catalog
SCHEMA_META_INDEX_OUTPUT ?= artifacts/schema_meta_index.gob
POLICY_ENV = DWS_POLICY_TMPDIR="$(DWS_POLICY_TMPDIR)" GOTMPDIR="$(POLICY_GOTMPDIR)"
GO_SOURCE_LIST = git ls-files -z --cached --others --exclude-standard -- '*.go'

.PHONY: all help build rebuild test test-plan test-auth-legacy-compat shortcut-public-e2e-proof lint format-check fmt policy edition-test interface-integrity authoritative-interface-integrity coverage-gate coverage-gate-platform update-interface-baseline reset-interface-baseline schema-compatibility skill-command-integrity skill-context-budget multi-im-skill-chain-integrity cli-smoke mock-mcp-smoke test-schema-agent-examples generate-schema fetch-mcp-metadata generate-schema-catalog package release release-pre release-stable changelog-pre changelog-stable publish-homebrew-formula setup-hooks

all: setup-hooks fmt lint build test rebuild

help:
	@printf "Available targets:\n"
	@printf "  make build         - Build the dws CLI binary\n"
	@printf "  make test          - Run the Go test suite\n"
	@printf "  make test-plan     - Verify CI test and full-suite coverage package plans cover their scopes exactly once\n"
	@printf "  make test-auth-legacy-compat - Run stable legacy authentication compatibility regressions\n"
	@printf "  make shortcut-public-e2e-proof - Prove every reviewed Devdoc/HRbrain/PAT public Shortcut through exact and owning raw execution\n"
	@printf "  make lint          - Run formatting checks, go vet, and staticcheck\n"
	@printf "  make format-check  - Check all repository Go source files with gofmt\n"
	@printf "  make fmt           - Format all repository Go source files\n"
	@printf "  make policy        - Check the built dws plus open-source and Schema policies\n"
	@printf "  make interface-integrity [BASE_REF=<ref>] [STABLE_REF=<tag>] [CANDIDATE_REF=<ref>] - Check authoritative CLI history\n"
	@printf "  make authoritative-interface-integrity BASE_REF=<ref> [STABLE_REF=<tag>] [CANDIDATE_REF=<ref>] - Check Git-owned CLI history\n"
	@printf "  make coverage-gate BASE_REF=<ref> - Enforce overall non-regression and 100%% changed-code coverage\n"
	@printf "  make coverage-gate-platform BASE_REF=<ref> PROFILE=<file> - Enforce 100%% native changed-code coverage\n"
	@printf "  make update-interface-baseline - Update the non-authoritative CLI smoke fixture\n"
	@printf "  make reset-interface-baseline - DANGEROUS: replace the non-authoritative CLI smoke fixture\n"
	@printf "  make schema-compatibility BASE_REF=<ref> [STABLE_REF=<tag>] [CANDIDATE_REF=<ref>] - Check the authoritative Schema history\n"
	@printf "  make skill-command-integrity - Check dws commands referenced by skills exist\n"
	@printf "  make skill-context-budget - Check generated Skill drift and common-path context budgets\n"
	@printf "  make multi-im-skill-chain-integrity - Check reviewed IM intents keep one default Skill route\n"
	@printf "  make cli-smoke     - Verify help for every public top-level command\n"
	@printf "  make mock-mcp-smoke - Verify HTTP and stdio MCP request/response transport\n"
	@printf "  make test-schema-agent-examples - Contract-check all Agent examples and dry-run the eligible subset\n"
	@printf "  make generate-schema - Refresh param_aliases + verify Schema assembly determinism\n"
	@printf "  make generate-schema-catalog - Optional assembled Catalog dump under artifacts/ (not a delivery step)\n"
	@printf "  make package       - Build all release artifacts locally\n"
	@printf "  make changelog-pre VERSION=vX.Y.Z-beta.N - Prepare prerelease notes\n"
	@printf "  make changelog-stable VERSION=vX.Y.Z FROM_BETA=vX.Y.Z-beta.N - Prepare stable notes\n"
	@printf "  make release-pre VERSION=vX.Y.Z-beta.N - Validate prerelease; publish official releases from Actions\n"
	@printf "  make release-stable VERSION=vX.Y.Z FROM_BETA=vX.Y.Z-beta.N - Validate stable; publish official releases from Actions\n"
	@printf "  make publish-homebrew-formula - Push dist/homebrew/dingtalk-workspace-cli.rb to a tap repo\n"

build:
	@./scripts/dev/build.sh

rebuild:
	@./scripts/dev/build.sh

test:
	@DWS_PACKAGE_VERSION="$(DWS_PACKAGE_VERSION)" $(GO) test -count=1 -timeout=10m ./...

test-plan:
	@./scripts/ci/test-packages.sh verify

test-auth-legacy-compat:
	@mkdir -p "$(POLICY_GOTMPDIR)"
	@GO="$(GO)" $(POLICY_ENV) ./scripts/policy/check-auth-legacy-compat.sh

shortcut-public-e2e-proof: build
	@GO="$(GO)" DWS_PACKAGE_VERSION="$(DWS_PACKAGE_VERSION)" ./scripts/policy/check-shortcut-public-e2e-proof.sh

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

policy: test-auth-legacy-compat shortcut-public-e2e-proof
	@mkdir -p "$(POLICY_GOTMPDIR)"
	@$(POLICY_ENV) ./scripts/policy/check-open-source-assets.sh
	@$(POLICY_ENV) ./scripts/policy/check-skill-context-budget.sh
	@$(POLICY_ENV) ./scripts/policy/check-multi-im-skill-chain.sh
	@$(POLICY_ENV) ./scripts/policy/check-multi-doc-skill-chain.sh
	@python3 scripts/run_chat_shortcut_live_audit_test.py
	@$(POLICY_ENV) ./scripts/policy/check-command-surface.sh --strict
	@$(POLICY_ENV) ./scripts/policy/check-generated-drift.sh
	@$(POLICY_ENV) ./scripts/policy/check-param-concepts.sh
	@$(POLICY_ENV) ./scripts/policy/check-param-alias-cooccurrence.sh
	@$(POLICY_ENV) $(GO) test -count=1 ./internal/app -run '^(TestParamAlias(FixtureThroughEmbeddedDeliveryPath|ReadCommandFinalPayload|WriteCommandFinalPayload|CanonicalConflictFailsBeforeRunE|BlockedFlagReachesReviewedFinalError)|TestFlagConflictErrorFormattingIsDeterministic)$$'
	@$(POLICY_ENV) ./scripts/policy/check-schema-catalog.sh
	@$(POLICY_ENV) ./scripts/policy/check-schema-binary.sh
	@$(POLICY_ENV) $(MAKE) test-schema-agent-examples

edition-test:
	$(GO) test -v -count=1 ./pkg/editiontest/...

interface-integrity:
	@base_ref="$(BASE_REF)"; \
	candidate_ref="$(CANDIDATE_REF)"; \
	if [ -z "$$base_ref" ]; then base_ref="origin/main"; fi; \
	if [ -z "$$candidate_ref" ]; then candidate_ref="HEAD"; fi; \
	./scripts/policy/check-authoritative-interface-baselines.sh \
		--base-ref "$$base_ref" \
		--stable-ref "$(STABLE_REF)" \
		--candidate-ref "$$candidate_ref"

authoritative-interface-integrity:
	@candidate_ref="$(CANDIDATE_REF)"; \
	if [ -z "$$candidate_ref" ]; then candidate_ref="HEAD"; fi; \
	./scripts/policy/check-authoritative-interface-baselines.sh \
		--base-ref "$(BASE_REF)" \
		--stable-ref "$(STABLE_REF)" \
		--candidate-ref "$$candidate_ref"

coverage-gate:
	@./scripts/policy/check-coverage-gate.sh --base-ref "$(BASE_REF)" --scope-buildable

coverage-gate-platform:
	@./scripts/policy/run-platform-coverage-gate.sh --base-ref "$(BASE_REF)" --profile "$(PROFILE)"

update-interface-baseline:
	@./scripts/policy/check-interface-baseline.sh --update

reset-interface-baseline:
	@./scripts/policy/check-interface-baseline.sh --reset

schema-compatibility:
	@candidate_ref="$(CANDIDATE_REF)"; \
	if [ -z "$$candidate_ref" ]; then candidate_ref="HEAD"; fi; \
	./scripts/policy/check-authoritative-schema-compatibility.sh \
		--base-ref "$(BASE_REF)" \
		--stable-ref "$(STABLE_REF)" \
		--candidate-ref "$$candidate_ref"

skill-command-integrity:
	@./scripts/policy/check-skill-commands.sh

skill-context-budget:
	@./scripts/policy/check-skill-context-budget.sh

multi-im-skill-chain-integrity:
	@./scripts/policy/check-multi-im-skill-chain.sh

multi-doc-skill-chain-integrity:
	@./scripts/policy/check-multi-doc-skill-chain.sh

skill-mono-multi-content:
	@./scripts/policy/check-mono-multi-skill-content.sh

cli-smoke:
	@./scripts/policy/check-cli-smoke.sh

mock-mcp-smoke:
	$(GO) test -v -count=1 -run '^(TestHTTPClientEndToEnd|TestStdioClientEndToEnd)$$' ./internal/transport

test-schema-agent-examples:
	DWS_AGENT_EXAMPLES_DRY_RUN=1 $(GO) test -v -count=1 ./internal/app -run '^TestAgentExamplesDryRun$$'

# generate-schema refreshes param_aliases_generated.go and verifies that
# ResolveSchemaBuild assembly is deterministic. Catalog is runtime-assembled
# (声明即 Catalog); cmd_schema_catalog is not a committed delivery step.
# schema_agent_metadata/ and schema_hints/ must stay absent.
generate-schema:
	@set -e; \
	concepts_guard=$$(mktemp); \
	concepts_schema_guard=$$(mktemp); \
	command_fallbacks_guard=$$(mktemp); \
	command_fallbacks_schema_guard=$$(mktemp); \
	trap 'rm -f "$$concepts_guard" "$$concepts_schema_guard" "$$command_fallbacks_guard" "$$command_fallbacks_schema_guard"' EXIT HUP INT TERM; \
	cp internal/cli/param_concepts.json "$$concepts_guard"; \
	cp internal/cli/param_concepts.schema.json "$$concepts_schema_guard"; \
	cp internal/cli/command_path_fallbacks.json "$$command_fallbacks_guard"; \
	cp internal/cli/command_path_fallbacks.schema.json "$$command_fallbacks_schema_guard"; \
	$(GO) generate ./internal/cli; \
	rm -rf internal/cli/schema_agent_metadata internal/cli/schema_agent_metadata_audit.json; \
	rm -f internal/cli/schema_meta_index.json; \
	if [ -e internal/cli/schema_command_registry ]; then \
		printf '%s\n' 'retired schema_command_registry/ must not reappear after generation' >&2; \
		exit 1; \
	fi; \
	cmp -s internal/cli/param_concepts.json "$$concepts_guard" || { \
		printf '%s\n' 'generation modified reviewed input internal/cli/param_concepts.json' >&2; \
		exit 1; \
	}; \
	cmp -s internal/cli/param_concepts.schema.json "$$concepts_schema_guard" || { \
		printf '%s\n' 'generation modified reviewed input internal/cli/param_concepts.schema.json' >&2; \
		exit 1; \
	}; \
	cmp -s internal/cli/command_path_fallbacks.json "$$command_fallbacks_guard" || { \
		printf '%s\n' 'generation modified reviewed input internal/cli/command_path_fallbacks.json' >&2; \
		exit 1; \
	}; \
	cmp -s internal/cli/command_path_fallbacks.schema.json "$$command_fallbacks_schema_guard" || { \
		printf '%s\n' 'generation modified reviewed input internal/cli/command_path_fallbacks.schema.json' >&2; \
		exit 1; \
	}; \
	if [ -e internal/cli/schema_hints ]; then \
		printf '%s\n' 'retired schema_hints/ must not reappear after generation' >&2; \
		exit 1; \
	fi; \
	if [ -e internal/cli/schema_meta_index.json ]; then \
		printf '%s\n' 'retired schema_meta_index.json must not remain after generation' >&2; \
		exit 1; \
	fi; \
	./scripts/policy/check-schema-assembly.sh

# Optional local/CI dump of an assembled Catalog under artifacts/ by default.
# Override SCHEMA_CATALOG_OUTPUT and SCHEMA_META_INDEX_OUTPUT as needed. This
# is not a go:generate or production delivery step.
generate-schema-catalog:
	$(GO) run -a ./internal/generator/cmd_schema_catalog \
		-root . \
		-output "$(SCHEMA_CATALOG_OUTPUT)" \
		-meta-index "$(SCHEMA_META_INDEX_OUTPUT)"

fetch-mcp-metadata:
	@printf '  %sFetching diagnostic MCP dump (not a Schema pin)%s\n' "$(COLOR_RUN)" "$(COLOR_RESET)"
	@./scripts/dev/fetch_mcp_metadata.sh

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
