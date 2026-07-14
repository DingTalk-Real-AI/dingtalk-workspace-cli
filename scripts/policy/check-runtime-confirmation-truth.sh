#!/bin/sh
set -eu

# Ensure catalog confirmation=user_required exactly matches metadata
# runtime_gate != none across schema_hints/metadata/*.json.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

metadata_dir="internal/cli/schema_hints/metadata"
catalog="internal/cli/schema_catalog.json"

if [ ! -d "$metadata_dir" ]; then
	printf '%s\n' "missing agent metadata directory: $metadata_dir" >&2
	exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

jq -r '
  .tools
  | to_entries[]
  | select((.value.runtime_gate // "none") != "none")
  | .key
' "$metadata_dir"/*.json | sort -u >"$tmp/truth_gated"

jq -r '
  .tools
  | to_entries[]
  | select(.value.confirmation == "user_required")
  | .key
' "$catalog" | sort >"$tmp/catalog_required"

if ! cmp -s "$tmp/truth_gated" "$tmp/catalog_required"; then
	printf '%s\n' 'catalog confirmation=user_required differs from schema_hints/metadata runtime_gate!=none set' >&2
	printf '%s\n' 'update internal/cli/schema_hints/metadata/<product>.json runtime_gate/confirmation, then regenerate schema' >&2
	diff -u "$tmp/truth_gated" "$tmp/catalog_required" || true
	exit 1
fi

jq -s '
  reduce .[] as $file ({};
    . * (
      $file.tools
      | to_entries
      | map({key: .key, value: (.value.runtime_gate // "none")})
      | from_entries
    )
  )
' "$metadata_dir"/*.json >"$tmp/gates.json"

jq -r --slurpfile catalog "$catalog" '
  . as $gate_map |
  ($catalog[0].tools) as $tools |
  $gate_map
  | to_entries[]
  | .key as $canonical
  | .value as $gate
  | $tools[$canonical] as $tool
  | select($tool != null)
  | select(
      if $gate == "none" then
        $tool.confirmation != "not_required" or $tool.risk == "high" or $tool.effect == "destructive"
      else
        $tool.confirmation != "user_required"
      end
    )
  | "\(.key)\tgate=\(.value)\teffect=\($tool.effect // "MISSING")\trisk=\($tool.risk // "MISSING")\tconfirmation=\($tool.confirmation // "MISSING")"
' "$tmp/gates.json" >"$tmp/gate_problems" || true

if [ -s "$tmp/gate_problems" ]; then
	printf '%s\n' 'schema_hints/metadata runtime_gate disagree with catalog effect/risk/confirmation' >&2
	cat "$tmp/gate_problems" >&2
	exit 1
fi

printf '%s\n' "runtime confirmation truth ok ($(wc -l <"$tmp/truth_gated" | tr -d ' ') gated)"
