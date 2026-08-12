#!/bin/sh
set -eu

# Validate the reviewed parameter concept dictionary
# (internal/cli/param_concepts.json) against its closed schema and every loader
# semantic invariant: globally unique concept members, reviewed exact command
# scopes, members disjoint from their own excludes, bind targets referencing a
# declared concept, no unresolved confirm/investigate decisions, audited
# single/list and ID/name boundaries, closed (unknown-field-rejecting) decoding
# at every level, and the fixture did-you-mean sentinel allow-list. These
# assertions run through the real
# embedded loader (cli.LoadParamConcepts / decodeParamConcepts), not a
# re-implementation, so the shell gate and the runtime agree by construction.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

if ! "${GO:-go}" test -count=1 ./internal/cli -run \
	'^(TestParamConceptsJSONSchemaDocumentsClosedShape|TestEmbeddedParamConceptsLoadsAndSatisfiesInvariants|TestParamConceptRiskAuditBoundaries|TestDecodeParamConceptsRejectsUnknownFieldsAtEveryLevel|TestDecodeParamConceptsEnforcesReviewedConstraints|TestGeneratedParamAliasesAreWellFormed)$'; then
	printf '%s\n' 'param concepts check: FAIL (reviewed dictionary violates its schema or a loader invariant)' >&2
	exit 1
fi

printf 'param concepts check: ok\n'
