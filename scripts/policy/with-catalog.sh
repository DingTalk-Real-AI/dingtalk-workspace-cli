#!/bin/sh
# Reassemble the split release Catalog shards back into the single JSON
# document shape (version + surface_hash + source_hash + catalog + tools) that
# the policy jq queries consume.
#
# The Catalog is a committed global file (schema_catalog/catalog.json); each
# product's leaf ToolSpecs live in their own shard (schema_catalog/tools/*.json)
# so concurrent feature PRs only rewrite one shard. This helper merges the
# shards back into one document so existing jq queries keep working unchanged.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
dir="$ROOT/internal/cli/schema_catalog"

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
