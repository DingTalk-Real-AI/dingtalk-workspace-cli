#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"
. "$ROOT/scripts/policy/search.sh"

# Same-line source inventory, not an AST/type proof. Cobra's implementation and
# tests are outside this production scan. Standalone stdlib flag.Parse is unrelated.
pattern='\.[[:space:]]*ParseFlags[[:space:]]*\(|\.(PersistentFlags|Flags)[[:space:]]*\([[:space:]]*\)[[:space:]]*\.[[:space:]]*Parse[[:space:]]*\('
matches="$(policy_search_production_go "$pattern" cmd internal pkg ./*.go || true)"
normalized="$(printf '%s\n' "$matches" | sed -E 's/:[0-9]+:[[:space:]]*/:/')"
# Allow one reviewed expression, not every future parser in the wiki file.
expected='internal/helpers/wiki.go:if err := targetCmd.ParseFlags(finalArgs); err != nil {'
if [ "$normalized" != "$expected" ]; then
 printf '%s\n' 'Manual Cobra parsing inventory changed; route failures through the prepared FlagErrorFunc and review this gate.' "$matches" >&2
 exit 1
fi
printf '%s\n' 'manual Cobra parsing inventory: 1 reviewed wiki proxy call'
