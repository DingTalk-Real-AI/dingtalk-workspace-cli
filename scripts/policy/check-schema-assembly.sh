#!/bin/sh
set -eu

# Prove ResolveSchemaBuild → Catalog assembly is deterministic across two
# consecutive CI/local dump runs. Production consumes the assembled registry,
# not a committed Catalog or meta-index fixture.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

tmp="$(mktemp -d)"
exec_tmp="$(policy_runtime_mktemp_dir dws-schema-assembly)"
catalog_generator="$exec_tmp/schema-catalog"
trap 'rm -rf "$tmp" "$exec_tmp"' EXIT HUP INT TERM

go build -a -o "$catalog_generator" ./internal/generator/cmd_schema_catalog

catalog_a="$tmp/schema_catalog_a"
catalog_b="$tmp/schema_catalog_b"
meta_a="$tmp/schema_meta_index_a.gob"
meta_b="$tmp/schema_meta_index_b.gob"

"$catalog_generator" -root . -output "$catalog_a" -meta-index "$meta_a"
"$catalog_generator" -root . -output "$catalog_b" -meta-index "$meta_b"

if ! diff -qr "$catalog_a" "$catalog_b" >/dev/null; then
	printf '%s\n' 'schema assembly: consecutive Catalog assemblies are not byte-identical' >&2
	diff -ru "$catalog_a" "$catalog_b" || true
	exit 1
fi

if ! cmp -s "$meta_a" "$meta_b"; then
	printf '%s\n' 'schema assembly: consecutive CommandMeta index assemblies are not byte-identical' >&2
	cmp -l "$meta_a" "$meta_b" | head -20 || true
	exit 1
fi

if [ -e internal/cli/schema_agent_metadata ] || [ -e internal/cli/schema_agent_metadata_audit.json ]; then
	printf '%s\n' 'schema assembly: retired schema_agent_metadata delivery artifact is present' >&2
	exit 1
fi

if [ -e internal/cli/schema_hints ]; then
	printf '%s\n' 'schema assembly: retired schema_hints/ must not be present' >&2
	exit 1
fi

if [ -e internal/cli/schema_catalog ] ||
	[ -e internal/cli/schema_meta_index.gob ] ||
	[ -e internal/cli/schema_meta_index.json ]; then
	printf '%s\n' 'schema assembly: committed Schema Catalog/meta-index fixtures must not be present' >&2
	exit 1
fi

printf 'schema assembly determinism check: ok\n'
