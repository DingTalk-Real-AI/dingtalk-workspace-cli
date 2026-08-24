#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

usage() {
	printf '%s\n' \
		"usage: $0 verify <app-package>" \
		"       $0 run <app-package> [partition]" \
		"       $0 list-partitions" >&2
	exit 2
}

# Single source of truth for the partition set. CI runs one job per partition and
# pins its shard names to this list, so a name that appears here without a
# dispatch entry below fails closed rather than silently skipping tests.
APP_PARTITIONS='schema a-b c-a-l c-m-o c-p-r c-s-z c-other d-r s-z-example-fuzz'

mode="${1:-}"
partition=""
case "$mode" in
	list-partitions)
		[ "$#" -eq 1 ] || usage
		for name in $APP_PARTITIONS; do
			printf '%s\n' "$name"
		done
		exit 0
		;;
	verify)
		[ "$#" -eq 2 ] || usage
		app_package="$2"
		;;
	run)
		[ "$#" -eq 2 ] || [ "$#" -eq 3 ] || usage
		app_package="$2"
		partition="${3:-}"
		;;
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
cal_count=0
cmo_count=0
cpr_count=0
csz_count=0
cother_count=0
dr_count=0
sz_count=0
unmatched_count=0

while IFS= read -r test_name; do
	case "$test_name" in
		Test*Schema*) schema_count=$((schema_count + 1)) ;;
		Test[A-B]*) ab_count=$((ab_count + 1)) ;;
		TestCrossPlatformCoverage[A-L]*) cal_count=$((cal_count + 1)) ;;
		TestCrossPlatformCoverage[M-O]*) cmo_count=$((cmo_count + 1)) ;;
		TestCrossPlatformCoverage[P-R]*) cpr_count=$((cpr_count + 1)) ;;
		TestCrossPlatformCoverage[S-Z]*) csz_count=$((csz_count + 1)) ;;
		TestC*) cother_count=$((cother_count + 1)) ;;
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

# The loop variable is deliberately not named "partition": that name holds the
# partition requested on the command line, and run mode still executes this
# discovery pass before dispatching.
classified=''
for spec in \
	"schema:$schema_count" \
	"a-b:$ab_count" \
	"c-a-l:$cal_count" \
	"c-m-o:$cmo_count" \
	"c-p-r:$cpr_count" \
	"c-s-z:$csz_count" \
	"c-other:$cother_count" \
	"d-r:$dr_count" \
	"s-z-example-fuzz:$sz_count"
do
	name="${spec%%:*}"
	count="${spec#*:}"
	classified="$classified $name"
	if [ "$count" -eq 0 ]; then
		printf 'app race partition %s is empty\n' "$name" >&2
		exit 1
	fi
done

# The counters above and APP_PARTITIONS must describe the same set in both
# directions. A counted partition missing from APP_PARTITIONS would never be
# dispatched by any CI job, and a dispatchable partition with no counter would
# escape the exact-coverage check above.
for name in $APP_PARTITIONS; do
	case " $classified " in
		*" $name "*) ;;
		*)
			printf 'app partition %s has no coverage counter\n' "$name" >&2
			exit 1
			;;
	esac
done
for name in $classified; do
	case " $APP_PARTITIONS " in
		*" $name "*) ;;
		*)
			printf 'counted partition %s is not a dispatchable app partition\n' "$name" >&2
			exit 1
			;;
	esac
done

total_count="$(wc -l < "$tests" | tr -d ' ')"
assigned_count=$((schema_count + ab_count + cal_count + cmo_count + cpr_count + csz_count + cother_count + dr_count + sz_count))
if [ "$assigned_count" -ne "$total_count" ]; then
	printf 'app race partitions assigned %s tests, want %s\n' "$assigned_count" "$total_count" >&2
	exit 1
fi

printf 'app race partitions cover %s top-level tests exactly once: schema=%s a-b=%s c-a-l=%s c-m-o=%s c-p-r=%s c-s-z=%s c-other=%s d-r=%s s-z-example-fuzz=%s\n' \
	"$total_count" "$schema_count" "$ab_count" "$cal_count" "$cmo_count" "$cpr_count" "$csz_count" "$cother_count" "$dr_count" "$sz_count"

if [ "$mode" = "verify" ]; then
	exit 0
fi

run_partition() {
	name="$1"
	instrumentation="$2"
	run_pattern="$3"
	skip_pattern="${4:-}"

	# Fail closed on an unrecognized mode: a typo must not silently drop race
	# instrumentation from a partition that is supposed to carry it.
	case "$instrumentation" in
		race|no-race) ;;
		*)
			printf 'unknown instrumentation %s for app partition %s\n' \
				"$instrumentation" "$name" >&2
			exit 1
			;;
	esac

	printf 'running internal/app %s partition %s\n' "$instrumentation" "$name"
	set -- -v -count=1 -timeout=15m -run "$run_pattern"
	if [ -n "$skip_pattern" ]; then
		set -- "$@" -skip "$skip_pattern"
	fi
	if [ "$instrumentation" = race ]; then
		set -- -race "$@"
	fi
	go test "$@" "$app_package"
}

# Schema assembly has the largest transient memory footprint. Run those tests
# in a fresh process, then keep each remaining name range in its own process so
# command trees retained by process-global registries are released between
# partitions. The complementary run/skip patterns preserve the full test set.
#
# The schema partition runs uninstrumented. Its tests assert structural
# Schema-to-Cobra contracts over a single goroutine: none of them call
# t.Parallel or start a goroutine, so the race detector has no concurrent access
# to observe here. The process-global lazy metadata that does need race coverage
# (schema_source_root's atomic.Value, the parameter-binding lazy loaders) is
# exercised by internal/cli's concurrent tests, which stay instrumented. The
# instrumentation is not free on this partition: its shared sync.Once Catalog
# build is allocation-heavy, and -race made the partition roughly 11x slower
# (26s -> 291s locally, 357s in CI) without being able to report anything.
schema_pattern='^Test.*Schema'

# Dispatch table for the partition set declared in APP_PARTITIONS. CI passes one
# partition per job so they run concurrently; running without a partition keeps
# the original end-to-end behaviour for local use and for any caller that wants
# the whole package in one invocation.
run_named_partition() {
	case "$1" in
		schema) run_partition schema no-race "$schema_pattern" ;;
		a-b) run_partition a-b race '^Test[A-B]' "$schema_pattern" ;;
		c-a-l) run_partition c-a-l race '^TestCrossPlatformCoverage[A-L]' "$schema_pattern" ;;
		c-m-o) run_partition c-m-o race '^TestCrossPlatformCoverage[M-O]' "$schema_pattern" ;;
		c-p-r) run_partition c-p-r race '^TestCrossPlatformCoverage[P-R]' "$schema_pattern" ;;
		c-s-z) run_partition c-s-z race '^TestCrossPlatformCoverage[S-Z]' "$schema_pattern" ;;
		c-other) run_partition c-other race '^TestC' '^Test.*Schema|^TestCrossPlatformCoverage' ;;
		d-r) run_partition d-r race '^Test[D-R]' "$schema_pattern" ;;
		s-z-example-fuzz)
			run_partition s-z-example-fuzz race '^(Test[S-Z]|Example|Fuzz)' "$schema_pattern"
			;;
		*)
			printf 'unknown app partition: %s\n' "$1" >&2
			exit 1
			;;
	esac
}

if [ -n "$partition" ]; then
	run_named_partition "$partition"
	exit 0
fi

for name in $APP_PARTITIONS; do
	run_named_partition "$name"
done
