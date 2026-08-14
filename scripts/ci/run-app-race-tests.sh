#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

usage() {
	printf '%s\n' \
		"usage: $0 verify <app-package>" \
		"       $0 run <app-package>" >&2
	exit 2
}

[ "$#" -eq 2 ] || usage
mode="$1"
app_package="$2"

case "$mode" in
	verify|run) ;;
	*) usage ;;
esac

workdir="$(mktemp -d "${TMPDIR:-/tmp}/dws-app-race-tests.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT HUP INT TERM

tests="$workdir/tests"
duplicates="$workdir/duplicates"
list_output="$workdir/list-output"

cd "$ROOT"
if ! go test "$app_package" -list '^(Test|Example|Fuzz)' > "$list_output"; then
	printf 'app race partition discovery failed for %s\n' "$app_package" >&2
	exit 1
fi
awk '/^(Test|Example|Fuzz)/ { print $1 }' "$list_output" > "$tests"

if [ ! -s "$tests" ]; then
	printf 'app race partition discovery found no tests in %s\n' "$app_package" >&2
	exit 1
fi

LC_ALL=C sort "$tests" | uniq -d > "$duplicates"
if [ -s "$duplicates" ]; then
	printf '%s\n' 'app race partition discovery found duplicate top-level tests:' >&2
	sed 's/^/  /' "$duplicates" >&2
	exit 1
fi

schema_count=0
ab_count=0
c_count=0
dr_count=0
sz_count=0
unmatched_count=0

while IFS= read -r test_name; do
	case "$test_name" in
		Test*Schema*) schema_count=$((schema_count + 1)) ;;
		Test[A-B]*) ab_count=$((ab_count + 1)) ;;
		TestC*) c_count=$((c_count + 1)) ;;
		Test[D-R]*) dr_count=$((dr_count + 1)) ;;
		Test[S-Z]*|Example*|Fuzz*) sz_count=$((sz_count + 1)) ;;
		*)
			printf 'unmatched app race test: %s\n' "$test_name" >&2
			unmatched_count=$((unmatched_count + 1))
			;;
	esac
done < "$tests"

if [ "$unmatched_count" -ne 0 ]; then
	exit 1
fi

for partition in \
	"schema:$schema_count" \
	"a-b:$ab_count" \
	"c:$c_count" \
	"d-r:$dr_count" \
	"s-z-example-fuzz:$sz_count"
do
	name="${partition%%:*}"
	count="${partition#*:}"
	if [ "$count" -eq 0 ]; then
		printf 'app race partition %s is empty\n' "$name" >&2
		exit 1
	fi
done

total_count="$(wc -l < "$tests" | tr -d ' ')"
assigned_count=$((schema_count + ab_count + c_count + dr_count + sz_count))
if [ "$assigned_count" -ne "$total_count" ]; then
	printf 'app race partitions assigned %s tests, want %s\n' "$assigned_count" "$total_count" >&2
	exit 1
fi

printf 'app race partitions cover %s top-level tests exactly once: schema=%s a-b=%s c=%s d-r=%s s-z-example-fuzz=%s\n' \
	"$total_count" "$schema_count" "$ab_count" "$c_count" "$dr_count" "$sz_count"

if [ "$mode" = "verify" ]; then
	exit 0
fi

run_partition() {
	name="$1"
	run_pattern="$2"
	skip_pattern="${3:-}"

	printf 'running internal/app race partition %s\n' "$name"
	if [ -n "$skip_pattern" ]; then
		go test -v -race -count=1 -timeout=15m \
			-run "$run_pattern" -skip "$skip_pattern" "$app_package"
	else
		go test -v -race -count=1 -timeout=15m \
			-run "$run_pattern" "$app_package"
	fi
}

# Schema assembly has the largest transient memory footprint. Run those tests
# in a fresh process, then keep each remaining name range in its own process so
# command trees retained by process-global registries are released between
# partitions. The complementary run/skip patterns preserve the full test set.
schema_pattern='^Test.*Schema'
run_partition schema "$schema_pattern"
run_partition a-b '^Test[A-B]' "$schema_pattern"
run_partition c '^TestC' "$schema_pattern"
run_partition d-r '^Test[D-R]' "$schema_pattern"
run_partition s-z-example-fuzz '^(Test[S-Z]|Example|Fuzz)' "$schema_pattern"
