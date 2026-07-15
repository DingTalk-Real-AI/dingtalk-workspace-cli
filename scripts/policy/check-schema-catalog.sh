#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/search.sh"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

if [ -e internal/cli/schema_native_contracts.go ] ||
	[ -e internal/cli/schema_native_contracts_generated.go ] ||
	policy_search_go 'ApplyNativeRuntimeSchemaContracts|nativeRuntimeSchemaContracts|runtimeSchemaIdentityCandidate' internal/cli; then
	printf '%s\n' 'native Schema identity materialization must not be reintroduced' >&2
	exit 1
fi

# The Go delivery gates compare final content against the reviewed
# CommandRegistry. This shell check intentionally treats Catalog as output and
# does not decode the registry into a competing identity model.
registry_count="$(jq -r '.tools | length' internal/cli/schema_catalog.json)"
catalog_count="$registry_count"
catalog_product_count="$(jq -r '.catalog.count' internal/cli/schema_catalog.json)"
mcp_snapshot_registry_count="$(jq -r '.coverage.surface_tools' internal/cli/schema_mcp_metadata.json)"
agent_registry_count="$(jq -r '.coverage.surface_tools' internal/cli/schema_agent_metadata/index.json)"
agent_product_count="$(jq -r '.coverage.products_with_metadata' internal/cli/schema_agent_metadata/index.json)"
agent_selection_count="$(jq -r '[.coverage.tools_with_use_when, .coverage.tools_with_avoid_when, .coverage.tools_with_examples, .coverage.tools_with_interface_mode] | min' internal/cli/schema_agent_metadata/index.json)"
if [ "$agent_registry_count" != "$registry_count" ] ||
	[ "$agent_product_count" != "$catalog_product_count" ] ||
	[ "$agent_selection_count" != "$registry_count" ]; then
	printf 'generated schema counts disagree: registry=%s catalog=%s products=%s agent=%s/%s\n' \
		"$registry_count" "$catalog_count" "$catalog_product_count" \
		"$agent_product_count" "$agent_registry_count" >&2
	exit 1
fi

if ! jq -e '
  .version == 1 and
  ((.source_revision // "") | length) > 0 and
  .coverage.surface_scope == "source_revision" and
  .coverage.source_services == (.coverage.snapshot_services + (.coverage.missing_services | length)) and
  .coverage.surface_tools == (.coverage.matched_tools + .coverage.unmatched_tools) and
  .coverage.source_tools >= .coverage.surface_tools and
  .coverage.matched_tools == (.tools | length) and
  .coverage.aliased_tools <= .coverage.matched_tools
' internal/cli/schema_mcp_metadata.json >/dev/null; then
	printf 'MCP source-revision snapshot coverage is inconsistent: snapshot_registry=%s\n' \
		"$mcp_snapshot_registry_count" >&2
	exit 1
fi

if ! jq -e '
  .coverage.source_tools == (.tools | length)
' internal/cli/schema_hints/selection-review.json >/dev/null; then
	printf '%s\n' 'reviewed Agent selection coverage is stale' >&2
	exit 1
fi

if ! jq -e '
  .coverage.source_tools == (.tools | length) and
  .coverage.matched_tools == (.tools | length)
' internal/cli/schema_hints/sibling-disambiguation.json >/dev/null; then
	printf '%s\n' 'reviewed sibling-disambiguation source coverage is stale' >&2
	exit 1
fi

if ! jq -e --arg registry_count "$registry_count" '
  .coverage.source_tools == ($registry_count | tonumber) and
  .coverage.matched_tools == (.tools | length) and
  all(.tools[];
    .reviewed == false and
    (has("interface_mode") | not) and
    (has("availability") | not) and
    (has("interface_ref") | not) and
    (has("interface_reason") | not)
  )
' internal/cli/schema_hints/runtime-surface-completeness.json >/dev/null; then
	printf '%s\n' 'runtime-surface completeness source must remain unreviewed and interface-free' >&2
	exit 1
fi

if ! jq -e '
  .source.name == "reviewed-interface-disposition-v2" and
  .source.reviewed == true and
  (.tools | length) > 0 and
  all(.tools[];
    ((keys - ["availability", "interface_mode", "interface_reason", "interface_ref"]) | length) == 0 and
    .availability == "available" and
    (if .interface_mode == "mcp" then
      ((.interface_ref.product_id // "") | length) > 0 and
      ((.interface_ref.rpc_name // "") | length) > 0 and
      ((.interface_reason // "") | length) == 0
    elif .interface_mode == "composite" then
      .interface_ref == null and
      ((.interface_reason // "") | length) > 0
    else
      false
    end)
  )
' internal/cli/schema_hints/zz-interface-disposition-review.json >/dev/null; then
	printf '%s\n' 'reviewed interface-only disposition source is invalid' >&2
	exit 1
fi

catalog_registry_hash="$(jq -r '.surface_hash' internal/cli/schema_catalog.json)"
agent_registry_hash="$(jq -r '.surface_hash' internal/cli/schema_agent_metadata/index.json)"
if [ "$catalog_registry_hash" != "$agent_registry_hash" ]; then
	printf 'schema CommandRegistry hashes disagree: catalog=%s agent=%s\n' \
		"$catalog_registry_hash" "$agent_registry_hash" >&2
	exit 1
fi

if ! jq -e --arg registry_count "$registry_count" '
  (.tools | length) == ($registry_count | tonumber) and
  all(.catalog.products[]; ((.agent_summary // "") | length) > 0) and
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
	(((.agent_source_refs // []) | map(test("schema_hints/selection/")) | any))
  )
' internal/cli/schema_catalog.json >/dev/null; then
	printf '%s\n' 'schema tools must have complete Agent summary/effect/safety metadata' >&2
	exit 1
fi

if ! jq -e 'all(.tools[];
  if .availability == "unavailable" then .interface_ref == null
  elif .interface_mode == "mcp" then .interface_ref != null
  else .interface_ref == null
  end
)' internal/cli/schema_catalog.json >/dev/null; then
	printf '%s
' 'schema interface disposition is inconsistent with interface_ref presence' >&2
	exit 1
fi

mcp_source_hash="$(jq -r '.source_hash' internal/cli/schema_mcp_metadata.json)"
if ! jq -e --arg source_hash "$mcp_source_hash" '
  .version == 1 and
  .snapshot_source_hash == $source_hash and
  (.missing_services | keys) == ["notify"] and
  .missing_services.notify.status == "out_of_surface" and
  ((.missing_services.notify.reason // "") | length) > 0
' internal/cli/schema_mcp_service_review.json >/dev/null; then
	printf '%s\n' 'missing MCP service review is stale or incomplete' >&2
	exit 1
fi

if ! jq -e '
  .tools["chat.send_personal_message"].primary_cli_path == "chat message send" and
  .tools["chat.reply_personal_message"].primary_cli_path == "chat message reply" and
  .tools["chat.reply_personal_message"].interface_ref == {
    "product_id": "chat",
    "rpc_name": "send_personal_message"
  } and
  (.tools | has("chat.upload_conversation_file") | not)
' internal/cli/schema_catalog.json >/dev/null; then
	printf '%s\n' 'chat send/reply schema identities are inconsistent' >&2
	exit 1
fi

if ! jq -e '
  [.tools[] | select(.constraints != null)] as $tools |
  ($tools | length) >= 21 and
  all($tools[];
    (((.parameters // {}) | keys) + ((.positionals // []) | map(.name))) as $names |
    all((.constraints.mutually_exclusive // [])[]; length > 1 and all(.[]; IN($names[]))) and
    all((.constraints.require_one_of // [])[]; length > 1 and all(.[]; IN($names[]))) and
    all((.constraints.require_together // [])[]; length > 1 and all(.[]; IN($names[])))
  )
' internal/cli/schema_catalog.json >/dev/null; then
	printf '%s\n' 'schema command constraints are incomplete or reference unknown parameters' >&2
	exit 1
fi

binding_count="$(jq '[.bindings[] | length] | add' internal/cli/schema_parameter_bindings.json)"
if ! jq -e --slurpfile bindings internal/cli/schema_parameter_bindings.json '
  . as $catalog |
  $bindings[0].version == 3 and
  $bindings[0].baseline.manifest == "schema-parameter-bindings-v3" and
  ($bindings[0].baseline.sha256 | test("^sha256:[0-9a-f]{64}$")) and
  ($bindings[0].baseline.reason | length) > 0 and
  $bindings[0].baseline.reviewed == true and
  all(($bindings[0].removals // {} | to_entries)[];
    (.key | length) > 0 and
    (.value.reason | length) > 0 and
    .value.reviewed == true and
    ((.value.replaced_by // "") | type) == "string"
  ) and
  all(($bindings[0].corrections // {} | to_entries)[];
    (.key | length) > 0 and
    (.value.old_property | length) > 0 and
    (.value.new_property | length) > 0 and
    .value.old_property != .value.new_property and
    (.value.reason | length) > 0 and
    .value.reviewed == true
  ) and
  all(($bindings[0].mapping_exclusions // {} | to_entries)[];
    (.key | length) > 0 and (.value | length) > 0
  ) and
  ([$bindings[0].bindings | to_entries[] |
    .key as $tool | .value | to_entries[] |
    {tool: $tool, flag: .key, property: .value}
  ]) as $expected |
  all($expected[];
    . as $binding |
    $catalog.tools[$binding.tool].parameters[$binding.flag].property == $binding.property
  )
' internal/cli/schema_catalog.json >/dev/null; then
	printf 'schema parameter bindings are incomplete or differ from generated catalog: count=%s\n' "$binding_count" >&2
	exit 1
fi

if ! jq -e '
  [.. | objects | select(
    has("endpoint") or has("auth_headers") or has("authorization") or
    has("access_token") or has("client_secret")
  )] | length == 0
' internal/cli/schema_catalog.json >/dev/null; then
	printf '%s\n' 'schema catalog contains runtime endpoint or credential fields' >&2
	exit 1
fi

if policy_search_paths 'mcp-gw\.dingtalk\.com|mcp\.dingtalk\.com/server|Authorization[^[:alnum:]]*:|Bearer [A-Za-z0-9]|access[_-]?token|client[_-]?secret' \
	internal/cli/schema_catalog.json \
	internal/cli/schema_mcp_metadata.json \
	internal/cli/schema_mcp_service_review.json \
	internal/cli/schema_agent_metadata \
	internal/cli/schema_parameter_bindings.json \
	internal/cli/schema_hints; then
	printf '%s\n' 'schema assets contain endpoint or credential material' >&2
	exit 1
fi

if policy_search_go '\.ListTools\(' internal/app internal/cli; then
	printf '%s\n' 'startup/schema packages must not call MCP tools/list' >&2
	exit 1
fi

./scripts/policy/check-runtime-confirmation-truth.sh

# Run the typed content gates as policy, rather than treating non-empty
# correction/exclusion maps as proof that their exact keys and winners are
# valid against the shipped Catalog and pinned MCP metadata.
go test ./internal/cli \
	-run '^(TestEmbeddedSchemaCatalog.*|TestEmbeddedSchemaAllPayload.*|TestRuntimeSchemaAllPayload.*|TestSchemaAllReturnsCompleteEmbeddedLeafSchemas|TestSchemaCatalogDeliveryCompleteness.*|TestValidateSchemaDeliveryInvariants.*|TestSchemaAliasViewProblem.*|TestSchemaDeliveryToolsByCanonical.*|TestSchemaUsesEmbeddedCatalogWithoutRuntimeLoad|TestWalkLeafCommandsTraversesAnnotatedHiddenSubtree|TestSchemaParameterBindingsMatchReviewedBaselineAndEmbeddedCatalog|TestDecodeSchemaParameterBindingsFailsClosed|TestSchemaParameterBindingManifestHashIsExactContentNotCount|TestBuildEffectiveCommandRegistryFailsClosedOnInvalidParameterBindingSource|TestValidateSchemaParameterBindingDeliveryRejectsStaleReviewedKeys|TestEmbeddedCatalogMCPParameterMappingsAreComplete|TestSchemaParameterMappingAuditExclusionRules|TestRuntimeSchemaReviewedMappingExclusionSelectsEmptyProperty|TestRuntimeCommandParameterSpecsPreserveReviewedEmptyPropertyProvenance|TestSchemaParameterBindingCorrectionsAreReviewed)$' \
	-count=1
go test ./internal/helpers \
	-run '^TestSheetConfirmationGuardCoversEveryProtectedLeaf$' \
	-count=1
go test ./internal/app \
	-run '^(TestEmbeddedSchemaContractMapsToExecutableTree|TestFinalSchemaParametersMatchExecutableHelpFlags|TestEmbeddedSchemaParametersMatchExecutableHelpFlags|TestSchemaHelpFlagCompletenessRejects.*|TestRuntimeSchemaCompletenessCoversPublicCommandTree|TestReviewedRoutedInterfacesReachFinalSchema|TestViewGetWrappersUsePinnedGetViewsInterface|TestReviewedInterfaceDispositionSourceOwnsRuntimeSurface|TestSheetFinalSchemaConfirmationMatchesRuntimeGuards|TestRegisterPluginHTTPServerDoesNotProbeEndpoint|TestRegisterStdioServerFromManifestDoesNotStartProcess)$' \
	-count=1

printf 'schema catalog check: ok (%s products, %s tools)\n' "$catalog_product_count" "$registry_count"
