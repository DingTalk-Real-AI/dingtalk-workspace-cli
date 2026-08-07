#!/bin/sh
set -eu

# Drift policy after Catalog generator retirement as a committed delivery step:
#   1. committed parameter-alias and command-path-fallback Go tables match
#      fresh generations
#   2. Schema assembly is deterministic (check-schema-assembly.sh)
#   3. Reviewed inputs are not mutated; retired delivery artifacts stay absent
#
# Production delivers Schema exclusively through runtime assembly:
# RegisterSchemaSourceRoot → ResolveSchemaBuild. Catalog and meta-index dumps
# are CI/local artifacts and must never be committed under internal/cli.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

tmp="$(mktemp -d)"
exec_tmp="$(policy_runtime_mktemp_dir dws-generated-drift)"
param_aliases_generator="$exec_tmp/param-aliases"
command_fallbacks_generator="$exec_tmp/command-path-fallbacks"
trap 'rm -rf "$tmp" "$exec_tmp"' EXIT HUP INT TERM

go build -o "$param_aliases_generator" ./internal/generator/cmd_param_aliases
go build -o "$command_fallbacks_generator" ./internal/generator/cmd_command_path_fallbacks

concepts_guard="$tmp/param_concepts.json"
concepts_schema_guard="$tmp/param_concepts.schema.json"
cp internal/cli/param_concepts.json "$concepts_guard"
cp internal/cli/param_concepts.schema.json "$concepts_schema_guard"
# Command path fallbacks and their editor schema are reviewed inputs too.
command_fallbacks_guard="$tmp/command_path_fallbacks.json"
command_fallbacks_schema_guard="$tmp/command_path_fallbacks.schema.json"
cp internal/cli/command_path_fallbacks.json "$command_fallbacks_guard"
cp internal/cli/command_path_fallbacks.schema.json "$command_fallbacks_schema_guard"
param_aliases_tmp="$tmp/param_aliases_generated.go"
param_aliases_tmp_second="$tmp/param_aliases_generated-second.go"
command_fallbacks_tmp="$tmp/command_path_fallbacks_generated.go"
command_fallbacks_tmp_second="$tmp/command_path_fallbacks_generated-second.go"

if [ -e internal/cli/schema_agent_metadata ] || [ -e internal/cli/schema_agent_metadata_audit.json ]; then
	printf '%s\n' 'generated drift: retired schema_agent_metadata delivery artifact is present' >&2
	printf '%s\n' 'remove internal/cli/schema_agent_metadata/ and schema_agent_metadata_audit.json' >&2
	exit 1
fi

"$param_aliases_generator" -root . -output "$param_aliases_tmp"
"$param_aliases_generator" -root . -output "$param_aliases_tmp_second"
"$command_fallbacks_generator" -root . -output "$command_fallbacks_tmp"
"$command_fallbacks_generator" -root . -output "$command_fallbacks_tmp_second"

if [ -e internal/cli/schema_command_registry ]; then
	printf '%s\n' 'generated drift: retired schema_command_registry/ must not be present' >&2
	exit 1
fi

if ! cmp -s internal/cli/param_concepts.json "$concepts_guard"; then
	printf '%s\n' 'generation modified reviewed input internal/cli/param_concepts.json' >&2
	exit 1
fi

if ! cmp -s internal/cli/param_concepts.schema.json "$concepts_schema_guard"; then
	printf '%s\n' 'generation modified reviewed input internal/cli/param_concepts.schema.json' >&2
	exit 1
fi

if ! cmp -s internal/cli/command_path_fallbacks.json "$command_fallbacks_guard"; then
	printf '%s\n' 'generation modified reviewed input internal/cli/command_path_fallbacks.json' >&2
	exit 1
fi

if ! cmp -s internal/cli/command_path_fallbacks.schema.json "$command_fallbacks_schema_guard"; then
	printf '%s\n' 'generation modified reviewed input internal/cli/command_path_fallbacks.schema.json' >&2
	exit 1
fi

if [ -e internal/cli/schema_hints ]; then
	printf '%s\n' 'generated drift: retired schema_hints/ must not be present' >&2
	exit 1
fi

if [ -e internal/cli/schema_catalog ] ||
	[ -e internal/cli/schema_meta_index.gob ] ||
	[ -e internal/cli/schema_meta_index.json ]; then
	printf '%s\n' 'generated drift: committed Schema Catalog/meta-index fixtures must not be present' >&2
	printf '%s\n' 'remove internal/cli/schema_catalog, schema_meta_index.gob, and schema_meta_index.json (ResolveMeta projects from runtime assembly)' >&2
	exit 1
fi

if ! cmp -s "$param_aliases_tmp" "$param_aliases_tmp_second"; then
	printf '%s\n' 'generated drift: consecutive parameter-alias generations are not byte-identical' >&2
	diff -u "$param_aliases_tmp" "$param_aliases_tmp_second" || true
	exit 1
fi

if ! cmp -s internal/cli/param_aliases_generated.go "$param_aliases_tmp"; then
	printf '%s\n' 'generated drift: internal/cli/param_aliases_generated.go is stale' >&2
	printf '%s\n' 'run: make generate-schema' >&2
	diff -u internal/cli/param_aliases_generated.go "$param_aliases_tmp" || true
	exit 1
fi

if ! cmp -s "$command_fallbacks_tmp" "$command_fallbacks_tmp_second"; then
	printf '%s\n' 'generated drift: consecutive command-path fallback generations are not byte-identical' >&2
	diff -u "$command_fallbacks_tmp" "$command_fallbacks_tmp_second" || true
	exit 1
fi

if ! cmp -s internal/cli/command_path_fallbacks_generated.go "$command_fallbacks_tmp"; then
	printf '%s\n' 'generated drift: internal/cli/command_path_fallbacks_generated.go is stale' >&2
	printf '%s\n' 'run: make generate-schema' >&2
	diff -u internal/cli/command_path_fallbacks_generated.go "$command_fallbacks_tmp" || true
	exit 1
fi

# Assembly determinism validates fresh CI/local Catalog dumps.
"$ROOT/scripts/policy/check-schema-assembly.sh"

printf 'generated drift check: ok\n'
