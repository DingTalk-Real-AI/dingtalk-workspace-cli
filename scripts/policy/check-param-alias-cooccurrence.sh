#!/bin/sh
set -eu

# Scan every runnable command for parameter-concept co-occurrence. A concept
# whose members intersect two or more VISIBLE real flags on one command cannot
# be auto-reduced (picking one would silently mis-map the other) and must be an
# explicitly reviewed `ambiguous` entry in param_concepts.json, or reduction
# fails. This re-runs the single reduction algorithm — cli.ReduceParamAliases,
# invoked by the parameter-alias generator — against the live Cobra tree built
# by app.NewRootCommand(), so a future command or flag that introduces an
# unreviewed co-occurrence (or a scoped_aliases/bind override targeting a flag
# that is not real) turns this gate red before it can ship a silent mis-map.
#
# It is intentionally distinct from check-generated-drift.sh: drift proves the
# committed table is byte-current, whereas this gate isolates and reports the
# co-occurrence / override-target contract with a focused failure message.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

exec_tmp="$(policy_runtime_mktemp_dir dws-param-cooccurrence)"
# The generator refuses to write inside the repository tree, so the throwaway
# output goes to a system temp dir (mirrors check-generated-drift.sh).
out_tmp="$(mktemp -d)"
generator="$exec_tmp/param-aliases"
out="$out_tmp/param_aliases_generated.go"
err="$exec_tmp/err.log"
trap 'rm -rf "$exec_tmp" "$out_tmp"' EXIT HUP INT TERM

"${GO:-go}" build -o "$generator" ./internal/generator/cmd_param_aliases

if ! "$generator" -root . -output "$out" 2>"$err"; then
	printf '%s\n' 'param-alias co-occurrence gate: FAIL' >&2
	printf '%s\n' 'an unreviewed concept co-occurrence (>=2 visible real flags) or an invalid scoped/bind override target was found:' >&2
	cat "$err" >&2
	exit 1
fi

printf 'param-alias co-occurrence gate: ok\n'
