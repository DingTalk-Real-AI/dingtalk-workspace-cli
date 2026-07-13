#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

jq -r '.products[].tools[].canonical_path' internal/cli/schema_command_surface.json | sort >"$tmp/surface"
jq -r '.tools | keys[]' internal/cli/schema_catalog.json | sort >"$tmp/catalog"
if ! cmp -s "$tmp/surface" "$tmp/catalog"; then
	printf '%s\n' 'schema catalog paths differ from the reviewed command surface' >&2
	diff -u "$tmp/surface" "$tmp/catalog" || true
	exit 1
fi

surface_count="$(wc -l <"$tmp/surface" | tr -d ' ')"
catalog_count="$(jq -r '.tools | length' internal/cli/schema_catalog.json)"
catalog_product_count="$(jq -r '.catalog.count' internal/cli/schema_catalog.json)"
mcp_snapshot_surface_count="$(jq -r '.coverage.surface_tools' internal/cli/schema_mcp_metadata.json)"
agent_surface_count="$(jq -r '.coverage.surface_tools' internal/cli/schema_agent_metadata/index.json)"
agent_product_count="$(jq -r '.coverage.products_with_metadata' internal/cli/schema_agent_metadata/index.json)"
agent_selection_count="$(jq -r '[.coverage.tools_with_use_when, .coverage.tools_with_avoid_when, .coverage.tools_with_examples, .coverage.tools_with_interface_mode] | min' internal/cli/schema_agent_metadata/index.json)"
if [ "$catalog_count" != "$surface_count" ] ||
	[ "$agent_surface_count" != "$surface_count" ] ||
	[ "$agent_product_count" != "$catalog_product_count" ] ||
	[ "$agent_selection_count" != "$surface_count" ]; then
	printf 'current schema counts disagree: surface=%s catalog=%s products=%s agent=%s/%s\n' \
		"$surface_count" "$catalog_count" "$catalog_product_count" \
		"$agent_product_count" "$agent_surface_count" >&2
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
	printf 'MCP source-revision snapshot coverage is inconsistent: snapshot_surface=%s\n' \
		"$mcp_snapshot_surface_count" >&2
	exit 1
fi

if ! jq -e '
  .coverage.source_tools == (.tools | length) and
  .coverage.eligible_tools == (.tools | length) and
  .coverage.matched_tools == (.tools | length) and
  .coverage.unmatched_tools == 0
' skills/mono/schema-hints/selection-review.json >/dev/null; then
	printf '%s\n' 'reviewed Agent selection coverage is stale' >&2
	exit 1
fi

if ! jq -e --arg surface_count "$surface_count" '
  .coverage.source_tools == ($surface_count | tonumber) and
  .coverage.matched_tools == (.tools | length) and
  all(.tools[]; ((.interface_mode // "") | length) > 0)
' skills/mono/schema-hints/runtime-surface-completeness.json >/dev/null; then
	printf '%s\n' 'runtime-surface interface disposition coverage is stale' >&2
	exit 1
fi

catalog_surface_hash="$(jq -r '.surface_hash' internal/cli/schema_catalog.json)"
agent_surface_hash="$(jq -r '.surface_hash' internal/cli/schema_agent_metadata/index.json)"
if [ "$catalog_surface_hash" != "$agent_surface_hash" ]; then
	printf 'schema surface hashes disagree: catalog=%s agent=%s\n' \
		"$catalog_surface_hash" "$agent_surface_hash" >&2
	exit 1
fi

if ! jq -e --arg surface_count "$surface_count" '
  (.tools | length) == ($surface_count | tonumber) and
  all(.catalog.products[]; ((.agent_summary // "") | length) > 0) and
  all(.tools[];
    ((.agent_summary // "") | length) > 0 and
    (.effect == "read" or .effect == "write" or .effect == "destructive") and
    (.risk == "low" or .risk == "medium" or .risk == "high") and
    (.confirmation == "not_required" or .confirmation == "user_required") and
    (.idempotency == "idempotent" or .idempotency == "non_idempotent" or .idempotency == "unknown") and
	((.use_when // []) | length) > 0 and
	((.avoid_when // []) | length) > 0 and
	((.examples // []) | length) > 0 and
	(.interface_mode == "mcp" or .interface_mode == "composite" or .interface_mode == "local" or .interface_mode == "unavailable") and
	(.availability == "available" or .availability == "unavailable") and
	(. as $tool | all(.examples[];
	  startswith("dws " + $tool.primary_cli_path) and
	  (test("(^|\\s)--yes(\\s|$)") | not)
	)) and
	(if .interface_mode == "mcp" then .interface_ref != null and .availability == "available" else true end) and
	(if .interface_mode == "composite" or .interface_mode == "local" then .interface_ref == null and .availability == "available" and ((.interface_reason // "") | length) > 0 else true end) and
	(if .interface_mode == "unavailable" then .interface_ref == null and .availability == "unavailable" and ((.interface_reason // "") | length) > 0 else true end) and
	((((.agent_source_refs // []) | index("skills/mono/schema-hints/selection-review.json")) != null) or
	 (((.agent_source_refs // []) | index("skills/mono/schema-hints/runtime-surface-completeness.json")) != null)) and
    (if .effect == "destructive" then .risk == "high" and .confirmation == "user_required" else true end) and
    (if .risk == "high" then .confirmation == "user_required" else true end)
  )
' internal/cli/schema_catalog.json >/dev/null; then
	printf '%s\n' 'schema tools must have complete Agent summary/effect/safety metadata' >&2
	exit 1
fi

if ! jq -e 'all(.tools[]; (.interface_mode == "mcp") == (.interface_ref != null))' internal/cli/schema_catalog.json >/dev/null; then
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
  $bindings[0].version == 2 and
  $bindings[0].historical_binding_count == 311 and
  ($bindings[0].migrations | length) == 5 and
  ($bindings[0].excluded | length) == 3 and
  ($bindings[0].added | length) == 8 and
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

if rg -n 'mcp-gw\.dingtalk\.com|mcp\.dingtalk\.com/server|Authorization[^[:alnum:]]*:|Bearer [A-Za-z0-9]|access[_-]?token|client[_-]?secret' \
	internal/cli/schema_catalog.json \
	internal/cli/schema_mcp_metadata.json \
	internal/cli/schema_mcp_service_review.json \
	internal/cli/schema_agent_metadata \
	internal/cli/schema_parameter_bindings.json \
	skills/mono/schema-hints; then
	printf '%s\n' 'schema assets contain endpoint or credential material' >&2
	exit 1
fi

if rg -n '\.ListTools\(' internal/app internal/cli --glob '*.go'; then
	printf '%s\n' 'startup/schema packages must not call MCP tools/list' >&2
	exit 1
fi

go test ./internal/cli \
	-run '^(TestEmbeddedSchemaCatalog.*|TestSchemaCatalogDeliveryCompleteness.*|TestSchemaUsesEmbeddedCatalogWithoutRuntimeLoad|TestWalkLeafCommandsTraversesAnnotatedHiddenSubtree)$' \
	-count=1
go test ./internal/app \
	-run '^(TestEmbeddedSchemaContractMapsToExecutableTree|TestRuntimeSchemaCompletenessCoversPublicCommandTree|TestRegisterPluginHTTPServerDoesNotProbeEndpoint|TestRegisterStdioServerFromManifestDoesNotStartProcess)$' \
	-count=1

printf 'schema catalog check: ok (%s products, %s tools)\n' "$catalog_product_count" "$surface_count"
