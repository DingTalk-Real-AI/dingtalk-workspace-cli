#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

go build -o "$tmp/dws" ./cmd
go run ./internal/generator/cmd_schema_registry_smoke >"$tmp/smoke-vector.json"

"$tmp/dws" schema list --format json >"$tmp/list.json"
"$tmp/dws" schema --all --format json >"$tmp/all.json"

if ! jq -e '
  .version == 1 and
  (.registry_hash | type) == "string" and
  (.registry_hash | test("^sha256:[0-9a-f]{64}$")) and
  (.commands | type) == "array" and
  (.commands | length) > 0 and
  ((.commands | map(.canonical_path) | unique | length) == (.commands | length)) and
  ((.commands | map(.primary_cli_path) | unique | length) == (.commands | length)) and
  all(.commands[];
    (.canonical_path | type) == "string" and (.canonical_path | length) > 0 and
    (.primary_cli_path | type) == "string" and (.primary_cli_path | length) > 0 and
    (.alias_cli_paths | type) == "array" and
    ((.alias_cli_paths | unique | length) == (.alias_cli_paths | length)))
' "$tmp/smoke-vector.json" >/dev/null; then
	printf '%s\n' 'EffectiveCommandRegistry full smoke vector is malformed or empty' >&2
	exit 1
fi

if ! jq -e -n \
	--slurpfile vector "$tmp/smoke-vector.json" \
	--slurpfile catalog internal/cli/schema_catalog.json \
	--slurpfile list "$tmp/list.json" \
	--slurpfile all "$tmp/all.json" '
  $vector[0] as $vector |
  $catalog[0] as $catalog |
  $list[0] as $list |
  $all[0] as $all |
  $vector.registry_hash == $catalog.surface_hash and
  $list.surface_hash == $vector.registry_hash and
  $all.surface_hash == $vector.registry_hash and
  $list.catalog_hash == $catalog.source_hash and
  $all.catalog_hash == $catalog.source_hash and
  ($vector.commands | map(.canonical_path) | sort) == ($catalog.tools | keys | sort) and
  ($vector.commands | map(.canonical_path) | sort) ==
  ([$all.products[].tools[].canonical_path] | sort) and
	  ([$vector.commands[] as $command |
	    ($catalog.tools[$command.canonical_path].primary_cli_path == $command.primary_cli_path and
	    (($catalog.tools[$command.canonical_path].aliases // []) | sort) ==
	      ($command.alias_cli_paths | sort))] | all) and
  all($all.products[].tools[]; . == $catalog.tools[.canonical_path])
' >/dev/null; then
	printf '%s\n' 'built dws Schema navigation/hash/--all content differs from EffectiveCommandRegistry or embedded Catalog' >&2
	exit 1
fi

# schema list is an intentionally compact product overview, but every field
# still has to be the exact typed projection of the same embedded Catalog.
# Derive the expected overview without invoking another Schema code path so a
# release binary cannot hide loader/query drift behind two identical helpers.
jq -S '
  def nonempty: type == "string" and (gsub("^\\s+|\\s+$"; "") | length) > 0;
  . as $snapshot |
  $snapshot.catalog as $catalog |
  {
    kind: (if ($catalog.kind | nonempty) then $catalog.kind else "schema" end),
    level: "products",
    count: ($catalog.products | length),
    tool_count: ($catalog.products | map(.tool_count) | add // 0),
    products: [
      $catalog.products[] |
      . as $product |
      {
        id: $product.id,
        tool_count: $product.tool_count,
        schema_path: $product.id
      } +
      if (($product.agent_summary // "") | nonempty) then
        {agent_summary: $product.agent_summary}
      elif (($product.use_when // []) | length) > 0 then
        {use_when: [$product.use_when[0]]}
      elif (($product.description // "") | type == "string" and length > 0) then
        {description: $product.description}
      else {} end
    ]
  } +
  (if (($catalog.source // "") | type == "string" and length > 0) then {source: $catalog.source} else {} end) +
  (if $catalog | has("interface_metadata") then {interface_metadata: $catalog.interface_metadata} else {} end) +
  (if $catalog | has("agent_metadata") then {agent_metadata: $catalog.agent_metadata} else {} end) +
  {catalog_hash: $snapshot.source_hash} +
  (if (($snapshot.surface_hash // "") | nonempty) then {surface_hash: $snapshot.surface_hash} else {} end)
' internal/cli/schema_catalog.json >"$tmp/expected-list.json"
jq -S . "$tmp/list.json" >"$tmp/actual-list.json"
if ! cmp -s "$tmp/expected-list.json" "$tmp/actual-list.json"; then
	printf '%s\n' 'built dws schema list differs from the typed Catalog overview projection' >&2
	diff -u "$tmp/expected-list.json" "$tmp/actual-list.json" || true
	exit 1
fi

# The exact public EffectiveCommandRegistry set and every alias mapping were compared above using the
# release binary's --all payload. Exercise representative canonical/primary
# query routes here; exhaustive per-path resolution runs in the in-process Go
# delivery gate, avoiding more than one thousand cold binary startups in CI.
jq -c '.commands as $commands | [$commands[0], $commands[($commands|length)/2|floor], $commands[-1]] | unique_by(.canonical_path)[]' \
	"$tmp/smoke-vector.json" >"$tmp/commands.jsonl"
while IFS= read -r command; do
	canonical="$(printf '%s\n' "$command" | jq -r '.canonical_path')"
	primary="$(printf '%s\n' "$command" | jq -r '.primary_cli_path')"

	"$tmp/dws" schema "$canonical" --format json >"$tmp/canonical.json"
	"$tmp/dws" schema --cli-path "$primary" --format json >"$tmp/primary.json"
	jq -S --arg canonical "$canonical" '
      [.products[].tools[] | select(.canonical_path == $canonical)] |
      if length == 1 then .[0] else null end
    ' "$tmp/all.json" >"$tmp/all-tool.json"
	jq -S . "$tmp/canonical.json" >"$tmp/canonical-sorted.json"
	jq -S . "$tmp/primary.json" >"$tmp/primary-sorted.json"

	if ! cmp -s "$tmp/canonical-sorted.json" "$tmp/primary-sorted.json" ||
		! cmp -s "$tmp/canonical-sorted.json" "$tmp/all-tool.json"; then
		printf 'built dws schema canonical/primary/--all payloads differ: %s\n' "$canonical" >&2
		exit 1
	fi

done <"$tmp/commands.jsonl"

alias_command="$(jq -c 'first(.commands[] | select((.alias_cli_paths | length) > 0)) // empty' "$tmp/smoke-vector.json")"
if [ -n "$alias_command" ]; then
	canonical="$(printf '%s\n' "$alias_command" | jq -r '.canonical_path')"
	alias_path="$(printf '%s\n' "$alias_command" | jq -r '.alias_cli_paths[0]')"
	"$tmp/dws" schema "$canonical" --format json >"$tmp/canonical.json"
	"$tmp/dws" schema --cli-path "$alias_path" --format json >"$tmp/alias.json"
	jq -S 'del(.cli_path, .is_alias)' "$tmp/canonical.json" >"$tmp/canonical-content.json"
	jq -S 'del(.cli_path, .is_alias)' "$tmp/alias.json" >"$tmp/alias-content.json"
	if ! cmp -s "$tmp/canonical-content.json" "$tmp/alias-content.json"; then
		printf 'built dws schema alias changes fields other than cli_path/is_alias: %s\n' "$alias_path" >&2
		exit 1
	fi
	if ! jq -e --arg canonical "$canonical" --arg alias_path "$alias_path" '
	  .canonical_path == $canonical and
	  .cli_path == $alias_path and
	  .is_alias == true
	' "$tmp/alias.json" >/dev/null; then
		printf 'built dws schema alias view fields are invalid: %s\n' "$alias_path" >&2
		exit 1
	fi
fi

printf '%s\n' 'schema binary smoke: ok'
