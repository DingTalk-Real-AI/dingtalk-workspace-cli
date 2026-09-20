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

# Schema-cache protobuf must match scripts/generate-schema-cache-proto.sh.
# SCHEMA_CACHE_PROTO_CHECK=1 (set by `make policy`, which the required CI
# Policy job runs) makes the check mandatory: when PATH has no libprotoc 35.1,
# the pinned protoc release is bootstrapped into the policy temp dir and used.
# Bootstrap or download failures fail the check instead of skipping it. Local
# runs without the variable and without protoc keep the friendly skip hint.
schema_cache_bootstrap_protoc() {
	asset=
	sha=
	case "$(uname -s)/$(uname -m)" in
		Linux/x86_64) asset='protoc-35.1-linux-x86_64.zip'; sha='6930ebf62bd4ea607b98fff052596c6ee564b9835b4ce172c75a3f53ae9d91b7' ;;
		Linux/aarch64) asset='protoc-35.1-linux-aarch_64.zip'; sha='01bf9d08808c7f96678b63f4bd8efa559bb4f83d5a7a270d5edaf507f9d5d9cf' ;;
		Darwin/arm64) asset='protoc-35.1-osx-aarch_64.zip'; sha='193289af0470c6a1aada357d4fba0bbf8d78bfaac8b5e42ca30af2ef75583de2' ;;
		Darwin/x86_64) asset='protoc-35.1-osx-x86_64.zip'; sha='537d73604a344ded6fc94e98e07e529d4fe3e4a0b09e59905353950fafc2a1f7' ;;
		*)
			printf '%s\n' "schema cache proto check: cannot bootstrap protoc 35.1 for $(uname -s)/$(uname -m); install libprotoc 35.1 or set PROTOC" >&2
			return 1
			;;
	esac
	bootstrap_dir="${exec_tmp}/protoc-35.1"
	mkdir -p "$bootstrap_dir"
	zip_path="$bootstrap_dir/$asset"
	curl -fsSL --retry 3 -o "$zip_path" "https://github.com/protocolbuffers/protobuf/releases/download/v35.1/$asset" || {
		printf '%s\n' "schema cache proto check: failed to download $asset" >&2
		return 1
	}
	checksum_tool=
	if command -v sha256sum >/dev/null 2>&1; then
		checksum_tool='sha256sum'
	elif command -v shasum >/dev/null 2>&1; then
		checksum_tool='shasum -a 256'
	else
		printf '%s\n' 'schema cache proto check: no sha256 tool (sha256sum/shasum) available' >&2
		return 1
	fi
	actual="$($checksum_tool "$zip_path" | awk '{print $1}')"
	if [ -z "$actual" ] || [ "$actual" != "$sha" ]; then
		printf '%s\n' "schema cache proto check: $asset sha256 mismatch (want $sha, got ${actual:-unknown})" >&2
		return 1
	fi
	unzip -q -o "$zip_path" -d "$bootstrap_dir" || {
		printf '%s\n' "schema cache proto check: failed to unpack $asset" >&2
		return 1
	}
	if [ ! -x "$bootstrap_dir/bin/protoc" ]; then
		printf '%s\n' "schema cache proto check: bootstrapped archive has no bin/protoc" >&2
		return 1
	fi
	PROTOC="$bootstrap_dir/bin/protoc" "$ROOT/scripts/generate-schema-cache-proto.sh" --check
}

schema_cache_proto_check() {
	if command -v protoc >/dev/null 2>&1 && [ "$(protoc --version 2>/dev/null || true)" = "libprotoc 35.1" ]; then
		"$ROOT/scripts/generate-schema-cache-proto.sh" --check
		return
	fi
	if [ "${SCHEMA_CACHE_PROTO_CHECK:-}" = "1" ]; then
		schema_cache_bootstrap_protoc
		return
	fi
	printf '%s\n' 'schema cache proto check: skipped (install libprotoc 35.1 or set SCHEMA_CACHE_PROTO_CHECK=1)'
}
schema_cache_proto_check

printf 'generated drift check: ok\n'
