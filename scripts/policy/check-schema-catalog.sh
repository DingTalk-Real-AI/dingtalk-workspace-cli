#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/search.sh"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

# Assemble a fresh runtime Catalog dump (ResolveSchemaBuild via with-catalog.sh)
# into the single-document shape (version + surface_hash + source_hash +
# catalog + tools) that the jq queries below consume. No committed
# schema_catalog/ split is consulted.
catalog="$(mktemp)"
trap 'rm -f "$catalog"' EXIT HUP INT TERM
scripts/policy/with-catalog.sh >"$catalog"

if [ -e internal/cli/schema_native_contracts.go ] ||
	[ -e internal/cli/schema_native_contracts_generated.go ] ||
	policy_search_go 'ApplyNativeRuntimeSchemaContracts|nativeRuntimeSchemaContracts|runtimeSchemaIdentityCandidate' internal/cli; then
	printf '%s\n' 'native Schema identity materialization must not be reintroduced' >&2
	exit 1
fi

# Legacy hint maps used to select primary CLI paths and discover helper roots.
# They are identity/navigation sources, so bringing any of them back would
# silently reintroduce a second source beside the identity collector.
if policy_search_go '(schemaPrimaryCLIPath|RuntimeSchemaRootHint|RegisterRuntimeSchemaRoot|PrimaryCLIPaths|RegisterSchemaProductVisibility|SchemaProductVisibilityFor|productVisibility)' \
	internal/cli; then
	printf '%s\n' 'legacy Schema hint navigation or visibility sources must not be reintroduced' >&2
	exit 1
fi

# The Go delivery gates compare final content against the reviewed
# CommandRegistry. This shell check intentionally treats Catalog as output and
# does not decode the registry into a competing identity model.
registry_count="$(jq -r '.tools | length' "$catalog")"
catalog_count="$registry_count"
catalog_product_count="$(jq -r '.catalog.count' "$catalog")"
# Agent metadata is no longer a shipped intermediate. Selection completeness is
# proven on the Catalog itself (every tool/product carries Agent prose).
agent_registry_count="$registry_count"
agent_product_count="$catalog_product_count"
agent_selection_count="$(jq -r '[.tools[] | select((.use_when|type)=="array" and (.avoid_when|type)=="array" and (.examples|type)=="array" and ((.interface_mode//"")|length)>0)] | length' "$catalog")"
if [ "$agent_registry_count" != "$registry_count" ] ||
	[ "$agent_product_count" != "$catalog_product_count" ] ||
	[ "$agent_selection_count" != "$registry_count" ]; then
	printf 'generated schema counts disagree: registry=%s catalog=%s products=%s selection=%s\n' \
		"$registry_count" "$catalog_count" "$catalog_product_count" \
		"$agent_selection_count" >&2
	exit 1
fi

# schema_mcp_metadata.json (pinned MCP baseline) is retired. Schema assembly
# is Contract/ParamDecl/Interface + Cobra only; the pin must not reappear.
if [ -e internal/cli/schema_mcp_metadata.json ]; then
	printf '%s\n' 'retired schema_mcp_metadata.json must not be present' >&2
	exit 1
fi

# schema_hints/ audit JSON is retired. Selection completeness, sibling
# routing, and interface disposition are proven on the Catalog itself
# (ContractFinal / ProductDecl provenance) by the jq gates below and the
# focused Go tests invoked at the end of this script.
if [ -e internal/cli/schema_hints ]; then
	printf '%s\n' 'retired schema_hints/ must not be present' >&2
	exit 1
fi

catalog_registry_hash="$(jq -r '.surface_hash' "$catalog")"
if [ -z "$catalog_registry_hash" ] || [ "$catalog_registry_hash" = "null" ]; then
	printf 'schema Catalog is missing surface_hash\n' >&2
	exit 1
fi

if ! jq -e --arg registry_count "$registry_count" '
  (.tools | length) == ($registry_count | tonumber) and
  all(.catalog.products[];
    ((.agent_summary // "") | length) > 0 and
    (has("use_when") and (.use_when | type) == "array" and (.use_when | length) > 0) and
    (has("avoid_when") and (.avoid_when | type) == "array" and (.avoid_when | length) > 0) and
    (
      (.field_provenance.agent_summary.precedence // "") == "contract_final" or
      (.field_provenance.agent_summary.source // "") == "corecmd.contract" or
      (.field_provenance.agent_summary.source // "") == "cli.product_decl"
    )
  ) and
  all(.tools[];
    ((.agent_summary // "") | length) > 0 and
    (.effect == "read" or .effect == "write" or .effect == "destructive") and
    (.risk == "low" or .risk == "medium" or .risk == "high") and
    (.confirmation == "not_required" or .confirmation == "user_required") and
    (.idempotency == "idempotent" or .idempotency == "non_idempotent" or .idempotency == "unknown") and
	(has("use_when") and (.use_when | type) == "array") and
	(has("avoid_when") and (.avoid_when | type) == "array") and
	(has("examples") and (.examples | type) == "array") and
	(.interface_mode == "mcp" or .interface_mode == "composite" or .interface_mode == "local") and
	(.availability == "available" or .availability == "unavailable") and
	(. as $tool | all(.examples[];
	  startswith("dws " + $tool.primary_cli_path) and
	  (test("(^|\\s)--yes(\\s|$)") | not)
	)) and
	(if .availability == "unavailable" then
	  .interface_ref == null and ((.interface_reason // "") | length) > 0
	 elif .interface_mode == "mcp" then
	  .interface_ref != null
	 elif .interface_mode == "local" then
	  .interface_ref == null
	 elif .interface_mode == "composite" then
	  .interface_ref == null and ((.interface_reason // "") | length) > 0
	 else false end) and
	(
	  .field_provenance.agent_summary.source == "corecmd.contract" or
	  .field_provenance.agent_summary.source == "cli.product_decl" or
	  (.field_provenance.agent_summary.precedence // "") == "contract_final"
	) and
	(
	  (.field_provenance.interface_mode.precedence // "") == "contract_final" or
	  (.field_provenance.interface_mode.source // "") == "corecmd.contract"
	)
  )
' "$catalog" >/dev/null; then
	printf '%s\n' 'schema tools must have complete Agent summary/effect/safety metadata' >&2
	exit 1
fi

if ! jq -e 'all(.tools[];
  if .availability == "unavailable" then .interface_ref == null
  elif .interface_mode == "mcp" then .interface_ref != null
  else .interface_ref == null
  end
)' "$catalog" >/dev/null; then
	printf '%s
' 'schema interface disposition is inconsistent with interface_ref presence' >&2
	exit 1
fi

# MCP service review (schema_mcp_service_review.json and any ledger) is
# retired: no snapshot_source_hash / missing_services disposition gate.
if [ -e internal/cli/schema_mcp_service_review.json ] ||
	[ -e internal/cli/schema_mcp_service_review_ledger.go ]; then
	printf '%s\n' 'retired MCP service review artifact must not be present' >&2
	exit 1
fi

if ! jq -e '
  .tools["chat.send_personal_message"].primary_cli_path == "chat message send" and
  .tools["chat.reply_personal_message"].primary_cli_path == "chat message reply" and
  .tools["chat.reply_personal_message"].interface_ref == {
    "product_id": "chat",
    "rpc_name": "send_personal_message"
  } and
  (.tools | has("chat.upload_conversation_file") | not) and
  .tools["chat.upload_local_conversation_file"].primary_cli_path == "chat conversation-file upload" and
  .tools["chat.upload_local_conversation_file"].interface_mode == "composite"
' "$catalog" >/dev/null; then
	printf '%s\n' 'chat send/reply/upload schema identities are inconsistent' >&2
	exit 1
fi

# declare≡execute may project hidden execute-side siblings into constraint
# groups. Require every group to touch at least one published parameter/positional;
# unpublished members are allowed as hidden companions.
if ! jq -e '
  [.tools[] | select(.constraints != null)] as $tools |
  ($tools | length) >= 21 and
  all($tools[];
    (((.parameters // {}) | keys) + ((.positionals // []) | map(.name))) as $names |
    def ok_group:
      length > 1 and
      all(.[]; type == "string" and length > 0) and
      any(.[]; IN($names[]));
    all((.constraints.mutually_exclusive // [])[]; ok_group) and
    all((.constraints.require_one_of // [])[]; ok_group) and
    all((.constraints.require_together // [])[]; ok_group)
  )
' "$catalog" >/dev/null; then
	printf '%s\n' 'schema command constraints are incomplete or reference unknown parameters' >&2
	exit 1
fi

# Track 1 Phase 2: active bindings JSON is retired. ParamDecl.Property owns
# property delivery; mapping exclusions / removals live in the Go ledger.
if [ -e internal/cli/schema_parameter_bindings.json ]; then
	printf '%s\n' 'retired schema_parameter_bindings.json must not be present; use schema_parameter_mapping_ledger.go' >&2
	exit 1
fi
if [ ! -f internal/cli/schema_parameter_mapping_ledger.go ]; then
	printf '%s\n' 'missing reviewed parameter mapping ledger internal/cli/schema_parameter_mapping_ledger.go' >&2
	exit 1
fi
if ! grep -q 'var reviewedSchemaParameterMappingExclusions = map' internal/cli/schema_parameter_mapping_ledger.go; then
	printf '%s\n' 'schema_parameter_mapping_ledger.go must declare reviewedSchemaParameterMappingExclusions' >&2
	exit 1
fi
if ! grep -q 'var reviewedSchemaParameterBindingRemovals = map' internal/cli/schema_parameter_mapping_ledger.go; then
	printf '%s\n' 'schema_parameter_mapping_ledger.go must declare reviewedSchemaParameterBindingRemovals' >&2
	exit 1
fi

if ! jq -e '
  [.. | objects | select(
    has("endpoint") or has("auth_headers") or has("authorization") or
    has("access_token") or has("client_secret")
  )] | length == 0
' "$catalog" >/dev/null; then
	printf '%s\n' 'schema catalog contains runtime endpoint or credential fields' >&2
	exit 1
fi

if policy_search_paths 'mcp-gw\.dingtalk\.com|mcp\.dingtalk\.com/server|Authorization[^[:alnum:]]*:|Bearer [A-Za-z0-9]|access[_-]?token|client[_-]?secret' \
	"$catalog" \
	internal/cli/schema_parameter_mapping_ledger.go; then
	printf '%s\n' 'schema assets contain endpoint or credential material' >&2
	exit 1
fi

if [ -e internal/cli/schema_agent_metadata ] || [ -e internal/cli/schema_agent_metadata_audit.json ]; then
	printf '%s\n' 'retired schema_agent_metadata delivery artifact must not be present' >&2
	exit 1
fi

if policy_search_go '\.ListTools\(' internal/app internal/cli; then
	printf '%s\n' 'startup/schema packages must not call MCP tools/list' >&2
	exit 1
fi

# Single-track delivery (声明即 Catalog): go:generate only refreshes
# param_aliases. Catalog assembly is runtime ResolveSchemaBuild; CI proves
# determinism via check-schema-assembly.sh. Reject committed Catalog generate.
if ! grep -Eq '^//go:generate .*cmd_param_aliases' internal/cli/gen.go; then
	printf '%s\n' 'go generate must register the param_aliases generator' >&2
	exit 1
fi
if grep -E '^//go:generate' internal/cli/gen.go | grep -Eq 'cmd_schema_catalog'; then
	printf '%s\n' 'go generate must not register committed Catalog delivery (cmd_schema_catalog)' >&2
	exit 1
fi
if ! grep -Eq 'check-schema-assembly\.sh' Makefile; then
	printf '%s\n' 'Makefile must invoke check-schema-assembly.sh for assembly determinism' >&2
	exit 1
fi
if grep -E '^//go:generate' internal/cli/gen.go | grep -Eq 'cmd_schema_agent_metadata|schema_agent_metadata'; then
	printf '%s\n' 'go generate must not regenerate retired schema_agent_metadata/' >&2
	exit 1
fi

# Embedded MCP/parameter metadata is intentionally expensive and must be
# parsed only through its sync.Once accessor. Each raw loader is allowed at
# exactly two production locations: its declaration and the assignment inside
# that accessor. Any third reference is an eager initializer or an accessor
# bypass and fails this static check.
# Agent metadata JSON embed/loader is retired; production must not reopen it.
if policy_search_production_go 'go:embed schema_agent_metadata' internal/cli; then
	printf '%s\n' 'schema_agent_metadata must not be re-embedded' >&2
	exit 1
fi
if policy_search_production_go 'loadEmbeddedAgentMetadata\(' internal/cli; then
	printf '%s\n' 'retired loadEmbeddedAgentMetadata must not remain in production code' >&2
	exit 1
fi
if policy_search_production_go 'loadAgentMetadataFixtureFrom\(' internal/cli; then
	printf '%s\n' 'Agent metadata fixture loader must stay test-only' >&2
	exit 1
fi

# The lazy loaders for the retired MCP pin (loadPinnedMCPMetadata) and the
# retired bindings snapshot (loadSchemaParameterBindingSnapshot) are gone with
# their JSON inputs; the bans below keep them from reappearing.

# Injection-seam swaps in tests must use internal/testseam.Swap/Protect: the
# manual save/assign/t.Cleanup-restore trio can forget the restore and leak
# process-global state into sibling tests. The testseam package implements
# the mechanism and is exempt.
manual_seam_restores="$(policy_search_go \
	't\.Cleanup\(func\(\) *\{ *[A-Za-z_][A-Za-z0-9_.]* *= *prev(ious)? *\}\)' \
	internal | grep '_test\.go:' | grep -v '^internal/testseam/' || true)"
if [ -n "$manual_seam_restores" ]; then
	printf '%s\n' 'manual seam restore found; use testseam.Swap or testseam.Protect instead:' >&2
	printf '%s\n' "$manual_seam_restores" >&2
	exit 1
fi

# Catch the common direct eager form statically; the fresh-process tests below
# additionally catch indirect or multi-line package initializers.
if policy_search_production_go '^[[:space:]]*var .*=[[:space:]]*(runtimeAgentMetadata|runtimeMCPMetadata|runtimeSchemaParameterBindingData)\(' \
	internal/cli; then
	printf '%s\n' 'Schema metadata accessors must not be called from package-scope variable initializers' >&2
	exit 1
fi

# Root construction may register the schema command, but app production code
# must never parse or inspect generation metadata. The schema command reads the
# already embedded Catalog only when it is actually executed.
if policy_search_production_go '(loadEmbeddedAgentMetadata|loadPinnedMCPMetadata|loadSchemaParameterBindingSnapshot|runtimeAgentMetadata|runtimeMCPMetadata|runtimeSchemaParameterBindingData|EmbeddedSchemaParameterBindings)\(' \
	internal/app; then
	printf '%s\n' 'root/app production code must not access Schema generation metadata loaders or accessors' >&2
	exit 1
fi

# Gated confirmation truth: live Contract SafetySpec ↔ ToolSpec ↔ runtime gate
# (not Catalog-vs-Catalog provenance). Invokes
# TestUserRequiredSafetyHomologyWithRuntimeGate; also listed in the focused
# ./internal/cli -run whitelist below so a direct schema-catalog run stays
# complete if the helper script is skipped.
./scripts/policy/check-runtime-confirmation-truth.sh

# Run the typed content gates as policy, rather than treating non-empty
# correction/exclusion maps as proof that their exact keys and winners are
# valid against the shipped Catalog (declaration + Cobra; no MCP pin).
# Homology (HOM-*) CI entrypoints: Safety confirmation truth + vocabulary /
# whitelist pins live in ./internal/cli/homology; parameter/help set equality
# is in ./internal/app below; bindings/mapping subset remains in ./internal/cli.
# Keep TestHomologyCIEntrypointsPinned in sync when adding gate IDs to policy.
go test ./internal/cli \
	-run '^(TestDeliverySchemaCatalog.*|TestDeliverySchemaAllPayload.*|TestRuntimeSchemaAllPayload.*|TestSchemaAllReturnsCompleteDeliveryLeafSchemas|TestSchemaCatalogDeliveryCompleteness.*|TestValidateSchemaDeliveryInvariants.*|TestSchemaAliasViewProblem.*|TestSchemaDeliveryToolsByCanonical.*|TestSchemaUsesDeliveryCatalogWithoutRuntimeLoad|TestWalkLeafCommandsTraversesAnnotatedHiddenSubtree|TestSchemaParameterBindingsMatchReviewedBaselineAndDeliveryCatalog|TestBindEffectiveCommandRegistryFailsClosedOnInvalidParameterBindingSource|TestValidateSchemaParameterBindingDeliveryRejectsStaleReviewedKeys|TestDeliveryCatalogMCPParameterMappingsAreComplete|TestSchemaParameterMappingAuditExclusionRules|TestRuntimeSchemaReviewedMappingExclusionSelectsEmptyProperty|TestRuntimeCommandParameterSpecsPreserveReviewedEmptyPropertyProvenance|TestSchemaParameterBindingActiveBindingsRemainEmpty|TestResolveMetaFailsClosedOnUnusableMetaIndex|TestRuntimeSchemaMetadataLoadsOnlyOnDemand|TestSchemaParameterBindingsPhase2.*)$' \
	-count=1
go test ./internal/cli/homology \
	-run '^(TestUserRequiredSafetyHomologyWithRuntimeGate|TestHomologyDecisionDocPinsPathAAndGateIDs|TestMCPPassthroughAdmissionExcludesLeafAndShortcut|TestHomologyCIEntrypointsPinned)$' \
	-count=1
go test ./internal/helpers \
	-run '^(TestSheetConfirmationGuardCoversEveryProtectedLeaf|TestCrossPlatformCoverageDeclareLeafMetadataDeferredConfirmAfterRunEWithoutCallTool)$' \
	-count=1
go test ./internal/app \
	-run '^(TestDeliverySchemaContractMapsToExecutableTree|TestFinalSchemaParametersMatchExecutableHelpFlags|TestDeliverySchemaParametersMatchExecutableHelpFlags|TestSchemaHelpFlagCompletenessRejects.*|TestRuntimeSchemaCompletenessCoversPublicCommandTree|TestReviewedRoutedInterfacesReachFinalSchema|TestViewGetWrappersUsePinnedGetViewsInterface|TestReviewedInterfaceDispositionSourceOwnsRuntimeSurface|TestSheetFinalSchemaConfirmationMatchesRuntimeGuards|TestRegisterPluginHTTPServerDoesNotProbeEndpoint|TestRegisterStdioServerFromManifestDoesNotStartProcess|TestOrdinaryRootCommandsDoNotLoadSchemaMetadata)$' \
	-count=1

printf 'schema catalog check: ok (%s products, %s tools)\n' "$catalog_product_count" "$registry_count"
