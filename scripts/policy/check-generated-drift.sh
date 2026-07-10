#!/bin/sh
set -eu

# Check committed version metadata against deterministic Skill and Hint sources.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

metadata_tmp="$tmp/metadata"
audit_tmp="$tmp/audit.json"

go run ./internal/generator/cmd_schema_agent_metadata \
  -root . \
  -surface internal/cli/schema_command_surface.json \
  -output-dir "$metadata_tmp" \
  -audit-output "$audit_tmp"

if ! diff -qr internal/cli/schema_agent_metadata "$metadata_tmp" >/dev/null; then
	printf '%s\n' 'generated drift: internal/cli/schema_agent_metadata is stale' >&2
	printf '%s\n' 'run: make generate-schema-agent-metadata' >&2
	diff -ru internal/cli/schema_agent_metadata "$metadata_tmp" || true
	exit 1
fi

if ! cmp -s internal/cli/schema_agent_metadata_audit.json "$audit_tmp"; then
	printf '%s\n' 'generated drift: internal/cli/schema_agent_metadata_audit.json is stale' >&2
	printf '%s\n' 'run: make generate-schema-agent-metadata' >&2
	diff -u internal/cli/schema_agent_metadata_audit.json "$audit_tmp" || true
	exit 1
fi

printf 'generated drift check: ok\n'
