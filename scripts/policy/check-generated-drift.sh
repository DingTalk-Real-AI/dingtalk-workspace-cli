#!/bin/sh
set -eu

# Regenerate deterministic downstream release assets from the reviewed
# CommandRegistry into a temporary directory and compare them with the
# committed files.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

tmp="$(mktemp -d)"
exec_tmp="$(policy_runtime_mktemp_dir dws-generated-drift)"
metadata_generator="$exec_tmp/schema-agent-metadata"
catalog_generator="$exec_tmp/schema-catalog"
trap 'rm -rf "$tmp" "$exec_tmp"' EXIT HUP INT TERM

go build -o "$metadata_generator" ./internal/generator/cmd_schema_agent_metadata
go build -a -o "$catalog_generator" ./internal/generator/cmd_schema_catalog

# CommandRegistry is a reviewed input, never a generated artifact. Keep an
# independent byte-for-byte guard around the ordinary downstream generators.
registry_guard="$tmp/schema_command_registry"
cp -R internal/cli/schema_command_registry "$registry_guard"
# Also guard human-authored metadata + selection hint trees.
metadata_guard="$tmp/metadata-hints"
selection_guard="$tmp/selection-hints"
cp -R internal/cli/schema_hints/metadata "$metadata_guard"
cp -R internal/cli/schema_hints/selection "$selection_guard"

metadata_tmp="$tmp/metadata"
audit_tmp="$tmp/audit.json"
catalog_tmp="$tmp/schema_catalog"
catalog_tmp_second="$tmp/schema_catalog-second"

"$metadata_generator" \
  -root . \
  -registry internal/cli/schema_command_registry \
  -output-dir "$metadata_tmp" \
  -audit-output "$audit_tmp"

if ! diff -qr internal/cli/schema_agent_metadata "$metadata_tmp" >/dev/null; then
	printf '%s\n' 'generated drift: internal/cli/schema_agent_metadata is stale' >&2
	printf '%s\n' 'run: make generate-schema' >&2
	diff -ru internal/cli/schema_agent_metadata "$metadata_tmp" || true
	exit 1
fi

if ! cmp -s internal/cli/schema_agent_metadata_audit.json "$audit_tmp"; then
	printf '%s\n' 'generated drift: internal/cli/schema_agent_metadata_audit.json is stale' >&2
	printf '%s\n' 'run: make generate-schema' >&2
	diff -u internal/cli/schema_agent_metadata_audit.json "$audit_tmp" || true
	exit 1
fi

"$catalog_generator" \
	-root . \
	-output "$catalog_tmp"

"$catalog_generator" \
	-root . \
	-output "$catalog_tmp_second"

if ! diff -qr internal/cli/schema_command_registry "$registry_guard" >/dev/null; then
	printf '%s\n' 'generation modified reviewed input internal/cli/schema_command_registry/' >&2
	exit 1
fi

if ! diff -qr internal/cli/schema_hints/metadata "$metadata_guard" >/dev/null; then
	printf '%s\n' 'generation modified reviewed input internal/cli/schema_hints/metadata' >&2
	exit 1
fi

if ! diff -qr internal/cli/schema_hints/selection "$selection_guard" >/dev/null; then
	printf '%s\n' 'generation modified reviewed input internal/cli/schema_hints/selection' >&2
	exit 1
fi

if ! diff -qr "$catalog_tmp" "$catalog_tmp_second" >/dev/null; then
	printf '%s\n' 'generated drift: consecutive Catalog generations are not byte-identical' >&2
	diff -ru "$catalog_tmp" "$catalog_tmp_second" || true
	exit 1
fi

if ! diff -qr internal/cli/schema_catalog "$catalog_tmp" >/dev/null; then
	printf '%s\n' 'generated drift: internal/cli/schema_catalog is stale' >&2
	printf '%s\n' 'run: make generate-schema' >&2
	diff -ru internal/cli/schema_catalog "$catalog_tmp" || true
	exit 1
fi

printf 'generated drift check: ok\n'
