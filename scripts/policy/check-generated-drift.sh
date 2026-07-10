#!/bin/sh
set -eu

# Check committed version metadata against its deterministic Skill sources.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

go run ./internal/generator/cmd_schema_agent_metadata \
  -root . \
  -surface internal/cli/schema_command_surface.json \
  -output-dir "$tmp"

if ! diff -qr internal/cli/schema_agent_metadata "$tmp" >/dev/null; then
	printf '%s\n' 'generated drift: internal/cli/schema_agent_metadata is stale' >&2
	printf '%s\n' 'run: make generate-schema-agent-metadata' >&2
	diff -ru internal/cli/schema_agent_metadata "$tmp" || true
	exit 1
fi

printf 'generated drift check: ok\n'
