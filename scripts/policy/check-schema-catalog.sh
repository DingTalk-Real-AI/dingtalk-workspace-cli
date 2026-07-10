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
interface_surface_count="$(jq -r '.coverage.surface_tools' internal/cli/schema_mcp_metadata.json)"
agent_surface_count="$(jq -r '.coverage.surface_tools' internal/cli/schema_agent_metadata/index.json)"
if [ "$surface_count" != "$catalog_count" ] ||
	[ "$surface_count" != "$interface_surface_count" ] ||
	[ "$surface_count" != "$agent_surface_count" ]; then
	printf 'schema surface counts disagree: surface=%s catalog=%s interface=%s agent=%s\n' \
		"$surface_count" "$catalog_count" "$interface_surface_count" "$agent_surface_count" >&2
	exit 1
fi

catalog_surface_hash="$(jq -r '.surface_hash' internal/cli/schema_catalog.json)"
agent_surface_hash="$(jq -r '.surface_hash' internal/cli/schema_agent_metadata/index.json)"
if [ "$catalog_surface_hash" != "$agent_surface_hash" ]; then
	printf 'schema surface hashes disagree: catalog=%s agent=%s\n' \
		"$catalog_surface_hash" "$agent_surface_hash" >&2
	exit 1
fi

catalog_agent_count="$(jq -r '[.tools[] | select(.agent_metadata_source != null)] | length' internal/cli/schema_catalog.json)"
catalog_summary_count="$(jq -r '[.tools[] | select((.agent_summary // "") != "")] | length' internal/cli/schema_catalog.json)"
agent_count="$(jq -r '.coverage.tools_with_metadata' internal/cli/schema_agent_metadata/index.json)"
agent_summary_count="$(jq -r '.coverage.tools_with_agent_summary' internal/cli/schema_agent_metadata/index.json)"
if [ "$catalog_agent_count" != "$agent_count" ] ||
	[ "$catalog_summary_count" != "$agent_summary_count" ]; then
	printf 'Agent metadata counts disagree: catalog=%s/%s index=%s/%s\n' \
		"$catalog_agent_count" "$catalog_summary_count" "$agent_count" "$agent_summary_count" >&2
	exit 1
fi

interface_applied="$(jq -r '.interface_metadata.applied_summaries // 0' internal/cli/schema_agent_metadata_audit.json)"
catalog_interface_summaries="$(jq -r '[.tools[] | select((.agent_summary_source // "") | startswith("mcp-tools-list+cli-registry@"))] | length' internal/cli/schema_catalog.json)"
if [ "$interface_applied" != "$catalog_interface_summaries" ]; then
	printf 'MCP Agent summary counts disagree: audit=%s catalog=%s\n' \
		"$interface_applied" "$catalog_interface_summaries" >&2
	exit 1
fi
if ! jq -e 'all(.tools[];
	if ((.agent_summary_source // "") | startswith("mcp-tools-list+cli-registry@"))
	then .reviewed == false and ((.agent_summary | length) <= 120)
	else true end)' internal/cli/schema_catalog.json >/dev/null; then
	printf '%s\n' 'MCP-derived Agent summaries must be unreviewed and at most 120 characters' >&2
	exit 1
fi

if rg -n 'mcp-gw\.dingtalk\.com|mcp\.dingtalk\.com/server|Authorization|Bearer [A-Za-z0-9]|access[_-]?token' \
	internal/cli/schema_catalog.json \
	internal/cli/schema_mcp_metadata.json \
	skills/mono/schema-hints; then
	printf '%s\n' 'schema assets contain endpoint or credential material' >&2
	exit 1
fi

go test ./internal/cli -run '^TestEmbeddedSchemaCatalog' -count=1
printf 'schema catalog check: ok (%s tools)\n' "$surface_count"
