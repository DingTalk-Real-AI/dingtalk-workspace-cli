#!/bin/sh

# Shared runtime workspace for policy helpers that compile and execute Go
# programs. Callers may override DWS_POLICY_TMPDIR and GOTMPDIR.
policy_prepare_runtime() {
	policy_root="$1"
	DWS_POLICY_TMPDIR="${DWS_POLICY_TMPDIR:-$policy_root/.worktrees/policy-tmp}"
	mkdir -p "$DWS_POLICY_TMPDIR/go"
	DWS_POLICY_TMPDIR="$(CDPATH= cd -- "$DWS_POLICY_TMPDIR" && pwd)"
	export DWS_POLICY_TMPDIR

	if [ -z "${GOTMPDIR:-}" ]; then
		GOTMPDIR="$DWS_POLICY_TMPDIR/go"
	fi
	mkdir -p "$GOTMPDIR"
	GOTMPDIR="$(CDPATH= cd -- "$GOTMPDIR" && pwd)"
	export GOTMPDIR
}

policy_runtime_mktemp_dir() {
	prefix="$1"
	mktemp -d "$DWS_POLICY_TMPDIR/${prefix}.XXXXXX"
}
