#!/bin/sh
set -eu

# Ensure confirmation=user_required matches live typed Contract SafetySpec and
# that every such leaf has an executable confirmation gate (DeclareLeafMetadata,
# Sheet protect marker, or framework ConfirmSafety / RunE).
#
# Do NOT compare Catalog fields to Catalog provenance labels: that is a tautology
# (both sides read the same embedded snapshot). The real homology gate is
# TestUserRequiredSafetyHomologyWithRuntimeGate in
# internal/cli/homology/safety_homology_test.go — it walks the live
# Cobra tree, reads ContractFinal.Safety, compares to AssembleSchemaRegistry
# ToolSpec.Confirmation, and probes the runtime gate.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

if [ -e internal/cli/schema_hints ]; then
	printf '%s\n' 'retired schema_hints/ must not be present' >&2
	exit 1
fi

go test ./internal/cli/homology \
	-run '^TestUserRequiredSafetyHomologyWithRuntimeGate$' \
	-count=1

printf '%s\n' 'runtime confirmation truth ok (live Contract SafetySpec homology)'
