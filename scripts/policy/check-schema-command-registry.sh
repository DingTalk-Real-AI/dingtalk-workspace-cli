#!/bin/sh
set -eu

# Validate the reviewed CommandRegistry as an input contract. This check is
# deliberately independent of Catalog/interface/provenance output policy so a
# malformed identity source cannot be hidden by a healthy generated snapshot.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/search.sh"

if [ -e internal/cli/schema_native_contracts.go ] ||
	[ -e internal/cli/schema_native_contracts_generated.go ] ||
	policy_search_go 'ApplyNativeRuntimeSchemaContracts|nativeRuntimeSchemaContracts|runtimeSchemaIdentityCandidate' internal/cli; then
	printf '%s\n' 'native Schema identity materialization must not be reintroduced' >&2
	exit 1
fi

# Legacy hint maps used to select primary CLI paths and discover helper roots.
# They are identity/navigation sources, so bringing any of them back would
# silently reintroduce a second source beside CommandRegistry.
if policy_search_go '(schemaPrimaryCLIPath|RuntimeSchemaRootHint|RegisterRuntimeSchemaRoot|PrimaryCLIPaths|RegisterSchemaProductVisibility|SchemaProductVisibilityFor|productVisibility)' \
	internal/cli; then
	printf '%s\n' 'legacy Schema hint navigation or visibility sources must not be reintroduced' >&2
	exit 1
fi

if [ ! -f internal/cli/schema_command_registry.schema.json ] ||
	! jq -e '
	  ."$schema" == "./schema_command_registry.schema.json" and
	  .version == 1 and
	  (.products | type) == "array" and
	  (.products | length) > 0
	' internal/cli/schema_command_registry.json >/dev/null; then
	printf '%s\n' 'reviewed CommandRegistry is missing its local JSON Schema contract' >&2
	exit 1
fi

# The registry argument is validation-only and all go:generate outputs must be
# downstream assets. Reject any directive that targets the reviewed input.
if ! grep -Eq '^//go:generate .*cmd_schema_agent_metadata .* -registry internal/cli/schema_command_registry\.json ' internal/cli/schema_agent_metadata.go; then
	printf '%s\n' 'go generate must validate the reviewed CommandRegistry explicitly' >&2
	exit 1
fi
if policy_search_go '^//go:generate .*-(output|output-dir|audit-output)(=|[[:space:]]+)([^[:space:]]*/)?schema_command_registry\.json([[:space:]]|$)' \
	internal/cli ||
	policy_search_go '^//go:generate .*(>|>>)[[:space:]]*([^[:space:]]*/)?schema_command_registry\.json([[:space:]]|$)' \
		internal/cli; then
	printf '%s\n' 'go generate must never overwrite the reviewed CommandRegistry' >&2
	exit 1
fi

# Embedded Agent/MCP/parameter metadata is intentionally expensive and must be
# parsed only through its sync.Once accessor. Each raw loader is allowed at
# exactly two production locations: its declaration and the assignment inside
# that accessor. Any third reference is an eager initializer or an accessor
# bypass and fails this static check.
check_schema_loader_references() {
	loader="$1"
	allowed="$2"
	references="$(policy_search_production_go "${loader}\\(" internal/cli || true)"
	count="$(printf '%s\n' "$references" | awk 'NF { count++ } END { print count + 0 }')"
	if [ "$count" -ne 2 ]; then
		printf 'Schema loader %s has %s production references, want exactly declaration + lazy accessor\n' "$loader" "$count" >&2
		printf '%s\n' "$references" >&2
		exit 1
	fi
	unexpected="$(printf '%s\n' "$references" | grep -Ev "$allowed" || true)"
	if [ -n "$unexpected" ]; then
		printf 'Schema loader %s is called outside its lazy accessor:\n%s\n' "$loader" "$unexpected" >&2
		exit 1
	fi
}

check_schema_loader_references \
	'loadEmbeddedAgentMetadata' \
	'^internal/cli/schema_agent_metadata\.go:[0-9]+:(func loadEmbeddedAgentMetadata\(\) embeddedAgentMetadata \{|[[:space:]]*runtimeEmbeddedAgentMetadataLazy\.metadata = loadEmbeddedAgentMetadata\(\))$'
check_schema_loader_references \
	'loadEmbeddedMCPMetadata' \
	'^internal/cli/runtime_schema\.go:[0-9]+:(func loadEmbeddedMCPMetadata\(\) embeddedMCPMetadata \{|[[:space:]]*runtimeEmbeddedMCPMetadataLazy\.metadata = loadEmbeddedMCPMetadata\(\))$'
check_schema_loader_references \
	'loadSchemaParameterBindings' \
	'^internal/cli/schema_parameter_bindings\.go:[0-9]+:(func loadSchemaParameterBindings\(\) \(schemaParameterBindingSnapshot, error\) \{|[[:space:]]*runtimeSchemaParameterBindingsLazy\.snapshot, runtimeSchemaParameterBindingsLazy\.err = loadSchemaParameterBindings\(\))$'

# Catch the common direct eager form statically; the fresh-process tests below
# additionally catch indirect or multi-line package initializers.
if policy_search_production_go '^[[:space:]]*var .*=[[:space:]]*(runtimeAgentMetadata|runtimeMCPMetadata|runtimeSchemaParameterBindingData)\(' \
	internal/cli; then
	printf '%s\n' 'Schema metadata accessors must not be called from package-scope variable initializers' >&2
	exit 1
fi

# Root construction may register the schema command, but app production code
# must never parse or inspect generation metadata. The schema command reads the
# already embedded Catalog only when it is actually executed.
if policy_search_production_go '(loadEmbeddedAgentMetadata|loadEmbeddedMCPMetadata|loadSchemaParameterBindings|runtimeAgentMetadata|runtimeMCPMetadata|runtimeSchemaParameterBindingData|EmbeddedSchemaParameterBindings)\(' \
	internal/app; then
	printf '%s\n' 'root/app production code must not access Schema generation metadata loaders or accessors' >&2
	exit 1
fi

go test ./internal/cli \
	-run '^(TestCommandRegistry.*|TestDecodeCommandRegistry.*|TestBuildEffectiveCommandRegistry.*|TestBindEffectiveCommandRegistry.*|TestRuntimeSchemaMetadataLoadsOnlyOnDemand)$' \
	-count=1

go test ./internal/app \
	-run '^TestOrdinaryRootCommandsDoNotLoadSchemaMetadata$' \
	-count=1

printf 'schema CommandRegistry check: ok (%s reviewed commands)\n' \
	"$(jq '[.products[].tools[]] | length' internal/cli/schema_command_registry.json)"
