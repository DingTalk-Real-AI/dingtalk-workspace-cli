#!/bin/sh
# Assemble a fresh Schema Catalog via ResolveSchemaBuild (cmd_schema_catalog)
# into a temp directory, then emit the single-document JSON shape
# (version + surface_hash + source_hash + catalog + tools) that policy jq
# queries consume.
#
# Production reads runtime assembly only; this helper creates an ephemeral
# Catalog dump exclusively for CI/policy queries.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

tmp="$(mktemp -d)"
exec_tmp="$(policy_runtime_mktemp_dir dws-with-catalog)"
catalog_generator="$exec_tmp/schema-catalog"
trap 'rm -rf "$tmp" "$exec_tmp"' EXIT HUP INT TERM

go build -a -o "$catalog_generator" ./internal/generator/cmd_schema_catalog
dir="$tmp/schema_catalog"
"$catalog_generator" -root . -output "$dir" -meta-index "$tmp/schema_meta_index.gob" >/dev/null

jq -s '
  .[0] as $envelope |
  ($envelope.surface_hash // null) as $surface |
  {
    version: $envelope.version,
    source_hash: $envelope.source_hash,
    catalog: $envelope.catalog,
    tools: (reduce .[1:][] as $shard ({}; . + ($shard.tools // {})))
  } +
  (if $surface then {surface_hash: $surface} else {} end)
' "$dir/catalog.json" "$dir"/tools/*.json
