#!/usr/bin/env bash
# run_all_tests.sh — Run all test suites in parallel and print a summary.
# Usage: bash test/scripts/run_all_tests.sh [--verbose] [--timeout <duration>] [--jobs <n>]
#
# Options:
#   --verbose          Print full test output for every suite (default: only on failure)
#   --timeout <dur>    Per-suite go test timeout (default: 120s)
#   --jobs <n>         Max parallel suites (default: number of CPU cores)

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERBOSE=false
TIMEOUT="120s"
MAX_JOBS="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"

# ── Parse flags ──────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --verbose) VERBOSE=true; shift ;;
    --timeout) TIMEOUT="$2"; shift 2 ;;
    --jobs)    MAX_JOBS="$2"; shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

# ── Colour helpers ────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

# ── Test suite definitions ────────────────────────────────────────────────────
# Each entry: "label|package_path"
declare -a SUITES=(
  "cli|./test/cli/..."
  "contract|./test/contract/..."
  "integration/extensions|./test/integration/extensions/..."
  "integration/recovery|./test/integration/recovery/..."
  "mock_mcp|./test/mock_mcp/..."
  "scripts|./test/scripts/..."
)

# ── Temporary workspace ───────────────────────────────────────────────────────
LOG_DIR="$(mktemp -d)"
trap 'rm -rf "$LOG_DIR"' EXIT

# ── Worker: run one suite, write result files ─────────────────────────────────
# Result files written to $LOG_DIR/<safe_label>/{status,output}
run_suite_worker() {
  local label="$1"
  local pkg="$2"
  local safe_label="${label//\//_}"
  local suite_dir="$LOG_DIR/$safe_label"
  mkdir -p "$suite_dir"

  # Check whether the package directory actually exists
  local pkg_dir="${pkg//\.\//}"
  pkg_dir="${pkg_dir%/...}"
  if [[ ! -d "$REPO_ROOT/$pkg_dir" ]]; then
    echo "skip" > "$suite_dir/status"
    echo "(directory not found)" > "$suite_dir/output"
    return
  fi

  local exit_code=0
  (
    cd "$REPO_ROOT"
    go test -timeout "$TIMEOUT" -count=1 "$pkg" 2>&1
  ) > "$suite_dir/output" 2>&1 || exit_code=$?

  if [[ $exit_code -eq 0 ]]; then
    echo "pass" > "$suite_dir/status"
  elif grep -q "matched no packages\|no packages to test\|\[no test files\]" "$suite_dir/output" 2>/dev/null; then
    echo "skip" > "$suite_dir/status"
    echo "(no test files found)" > "$suite_dir/output"
  else
    echo "fail" > "$suite_dir/status"
  fi
}

export -f run_suite_worker
export REPO_ROOT TIMEOUT LOG_DIR

# ── Main ──────────────────────────────────────────────────────────────────────
echo ""
printf "${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}\n"
printf "${BOLD}║              dws — Full Test Suite Runner                    ║${RESET}\n"
printf "${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}\n"
echo ""
printf "Repo root : %s\n" "$REPO_ROOT"
printf "Timeout   : %s per suite\n" "$TIMEOUT"
printf "Parallel  : %s jobs\n" "$MAX_JOBS"
printf "Verbose   : %s\n" "$VERBOSE"
echo ""
printf "${BOLD}Launching %d suites in parallel…${RESET}\n" "${#SUITES[@]}"
echo ""

START_TIME=$(date +%s)

if [[ $MAX_JOBS -le 1 ]]; then
  for suite in "${SUITES[@]}"; do
    label="${suite%%|*}"
    pkg="${suite##*|}"

    printf "  ${CYAN}START${RESET}  %s\n" "$label"
    run_suite_worker "$label" "$pkg"
  done
else

# ── Dispatch all suites concurrently with a job-slot semaphore ────────────────
declare -a PIDS=()
declare -a PID_LABELS=()
active_jobs=0

for suite in "${SUITES[@]}"; do
  label="${suite%%|*}"
  pkg="${suite##*|}"

  printf "  ${CYAN}START${RESET}  %s\n" "$label"

  run_suite_worker "$label" "$pkg" &
  PIDS+=($!)
  PID_LABELS+=("$label")
  active_jobs=$((active_jobs + 1))

  # Throttle: wait for one slot to free up when at capacity
  if [[ $active_jobs -ge $MAX_JOBS ]]; then
    wait "${PIDS[$((${#PIDS[@]} - MAX_JOBS))]}" 2>/dev/null || true
    active_jobs=$((active_jobs - 1))
  fi
done

# Wait for all remaining background jobs
for pid in "${PIDS[@]}"; do
  wait "$pid" 2>/dev/null || true
done

fi

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

# ── Collect results in original suite order ───────────────────────────────────
TOTAL=0
PASSED=0
FAILED=0
SKIPPED=0

declare -a FAILED_SUITES=()
declare -a PASSED_SUITES=()
declare -a SKIPPED_SUITES=()

echo ""
printf "${BOLD}Results:${RESET}\n"
echo ""

for suite in "${SUITES[@]}"; do
  label="${suite%%|*}"
  safe_label="${label//\//_}"
  suite_dir="$LOG_DIR/$safe_label"
  status_file="$suite_dir/status"
  output_file="$suite_dir/output"

  TOTAL=$((TOTAL + 1))

  if [[ ! -f "$status_file" ]]; then
    # Worker never wrote a status — treat as failure
    printf "  ${RED}FAIL${RESET}  %s  (worker did not complete)\n" "$label"
    FAILED=$((FAILED + 1))
    FAILED_SUITES+=("$label")
    continue
  fi

  status="$(cat "$status_file")"

  case "$status" in
    pass)
      PASSED=$((PASSED + 1))
      PASSED_SUITES+=("$label")
      printf "  ${GREEN}PASS${RESET}  %s\n" "$label"
      if $VERBOSE; then
        sed 's/^/    /' "$output_file"
      fi
      ;;
    skip)
      SKIPPED=$((SKIPPED + 1))
      SKIPPED_SUITES+=("$label")
      printf "  ${YELLOW}SKIP${RESET}  %s  %s\n" "$label" "$(cat "$output_file")"
      ;;
    fail)
      FAILED=$((FAILED + 1))
      FAILED_SUITES+=("$label")
      printf "  ${RED}FAIL${RESET}  %s\n" "$label"
      echo "  ── output ──────────────────────────────────────────────────────"
      sed 's/^/  /' "$output_file"
      echo "  ────────────────────────────────────────────────────────────────"
      ;;
  esac
done

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
printf "${BOLD}══════════════════════════════════════════════════════════════${RESET}\n"
printf "${BOLD}  Summary  (elapsed: %ds, parallel: %s jobs)${RESET}\n" "$ELAPSED" "$MAX_JOBS"
printf "${BOLD}══════════════════════════════════════════════════════════════${RESET}\n"
printf "  Total   : %d\n" "$TOTAL"
printf "  ${GREEN}Passed  : %d${RESET}\n" "$PASSED"
printf "  ${RED}Failed  : %d${RESET}\n" "$FAILED"
printf "  ${YELLOW}Skipped : %d${RESET}\n" "$SKIPPED"
echo ""

if [[ ${#FAILED_SUITES[@]} -gt 0 ]]; then
  printf "${RED}${BOLD}Failed suites:${RESET}\n"
  for suite in "${FAILED_SUITES[@]}"; do
    printf "  ${RED}✗${RESET}  %s\n" "$suite"
  done
  echo ""
fi

if [[ ${#SKIPPED_SUITES[@]} -gt 0 ]]; then
  printf "${YELLOW}Skipped suites:${RESET}\n"
  for suite in "${SKIPPED_SUITES[@]}"; do
    printf "  ${YELLOW}–${RESET}  %s\n" "$suite"
  done
  echo ""
fi

if [[ ${#PASSED_SUITES[@]} -gt 0 ]]; then
  printf "${GREEN}Passed suites:${RESET}\n"
  for suite in "${PASSED_SUITES[@]}"; do
    printf "  ${GREEN}✓${RESET}  %s\n" "$suite"
  done
  echo ""
fi

# ── Exit code ─────────────────────────────────────────────────────────────────
if [[ $FAILED -gt 0 ]]; then
  printf "${RED}${BOLD}Result: FAILED${RESET}\n\n"
  exit 1
else
  printf "${GREEN}${BOLD}Result: ALL PASSED${RESET}\n\n"
  exit 0
fi
