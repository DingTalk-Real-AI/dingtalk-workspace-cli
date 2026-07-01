#!/usr/bin/env sh
# connect-watchdog.sh — keep a `dws dev connect` connector alive using the
# health contract from `dws dev connect status --json`.
#
# This is the "脚本" side of the 0701 review: a small, inspectable local watchdog
# that (1) asks dws whether the connection is actually healthy — not just whether
# a process exists — and (2) relaunches it when it is down/degraded. Drop it into
# cron or launchd; it is idempotent, so running it every few minutes is safe.
#
# It consumes the machine-readable contract, so it never parses `ps` or guesses.
#
# Usage:
#   connect-watchdog.sh --client-id <clientId> [--dry-run] -- <launch command...>
#
# Example (relaunch a daemon connector if it is not healthy):
#   connect-watchdog.sh --client-id ding123 -- \
#     dws dev connect --robot-client-id ding123 --channel opencode --daemon
#
# cron (every 5 minutes):
#   */5 * * * * /path/to/connect-watchdog.sh --client-id ding123 -- \
#     dws dev connect --robot-client-id ding123 --channel opencode --daemon >> ~/.dws/connect/ding123/watchdog.log 2>&1
#
# Exit codes: 0 = healthy (no action) or relaunch issued; 1 = usage error.

set -eu

DWS="${DWS_BIN:-dws}"
CLIENT_ID=""
DRY_RUN=0

while [ $# -gt 0 ]; do
  case "$1" in
    --client-id) CLIENT_ID="$2"; shift 2 ;;
    --dry-run)   DRY_RUN=1; shift ;;
    --)          shift; break ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

if [ -z "$CLIENT_ID" ]; then
  echo "usage: connect-watchdog.sh --client-id <clientId> [--dry-run] -- <launch command...>" >&2
  exit 1
fi
if [ $# -eq 0 ]; then
  echo "error: missing launch command after --" >&2
  exit 1
fi

ts() { date '+%Y-%m-%d %H:%M:%S'; }

# Ask dws for the health verdict. `--json` is the stable contract.
STATUS_JSON="$("$DWS" dev connect status --robot-client-id "$CLIENT_ID" --json 2>/dev/null || true)"

# Extract "state" without a hard jq dependency (fall back to jq when present).
if command -v jq >/dev/null 2>&1; then
  STATE="$(printf '%s' "$STATUS_JSON" | jq -r '.state // "unknown"')"
else
  STATE="$(printf '%s' "$STATUS_JSON" | sed -n 's/.*"state"[[:space:]]*:[[:space:]]*"\([a-z_]*\)".*/\1/p' | head -n1)"
  [ -n "$STATE" ] || STATE="unknown"
fi

case "$STATE" in
  healthy)
    echo "$(ts) [watchdog] $CLIENT_ID healthy — no action"
    exit 0
    ;;
  not_running|down|degraded|unknown)
    echo "$(ts) [watchdog] $CLIENT_ID state=$STATE — relaunching: $*"
    if [ "$DRY_RUN" -eq 1 ]; then
      echo "$(ts) [watchdog] dry-run, not executing"
      exit 0
    fi
    # For down/degraded, stop the old connector first so we do not fight the
    # single-instance lock; not_running has nothing to stop (ignore failures).
    if [ "$STATE" = "down" ] || [ "$STATE" = "degraded" ]; then
      "$DWS" dev connect stop --robot-client-id "$CLIENT_ID" >/dev/null 2>&1 || true
    fi
    exec "$@"
    ;;
  *)
    echo "$(ts) [watchdog] $CLIENT_ID unexpected state=$STATE — no action" >&2
    exit 0
    ;;
esac
