# Event consume — AI subprocess contract

Defines the stable `dws event consume` subprocess contract so an
orchestrator can determine when the consumer is ready, stop it cleanly,
and machine-read why it exited.

Scope of this branch: the four **contract** items below. Reconnect
resilience (keeping the stream alive across a transient upstream drop) is
tracked separately and intentionally out of scope here.

## Baseline (already present, no work)

- `--max-events N` — stop after N events (exit 0).
- `--duration D` — wall-clock budget (exit 0). Kept as `--duration`, NOT
  aliased to `--timeout`: the global `--timeout` is the HTTP request
  timeout (int seconds) and would collide (different type and meaning).
- Bus idle-shutdown fires only with **zero** consumers, so a connected
  consumer is never idle-killed.
- SIGINT/SIGTERM already cancel the run context and return cleanly.

## Improvements

### 1. Ready marker (standardized)

On connect, emit a fixed stderr line **before** any stdout event:

```
[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>
```

Parents block on stderr until this line, then read stdout. Suppressed
under `--quiet`. Replaces the ad-hoc `connected bus pid=...` line (which
omits `event_key`).

**Verification**
- T1a: stderr contains a line matching `^\[event\] ready event_key=<key>`.
- T1b: that line appears before the first stdout event (ordering).
- T1c: with `--quiet`, the line is absent.

### 2. stdin EOF = graceful exit

`consume` watches stdin; closing stdin is a shutdown signal (wired for AI
subprocess callers). To stay resident, feed a never-EOF stdin
(`< <(tail -f /dev/null)`) or run bounded (`--max-events` / `--duration`).

**Verification**
- T2a: `printf '' | dws event consume <key>` exits ≤2s, code 0, final
  line `reason: signal` (stdin-eof classified as signal).
- T2b: `dws event consume <key> < <(tail -f /dev/null)` still alive after
  5s, connection intact.
- T2c (unit): a controllable stdin reader hitting EOF makes Run return nil
  via the cleanup path.

### 3. Exit reason contract + exit codes

On exit, final stderr line:

```
[event] exited — received N event(s) in Xs (reason: <limit|timeout|signal|bus_shutdown>)
```

Exit codes: controlled exit (limit/timeout/signal/stdin-eof) = 0; startup
or runtime failure (permissions, network, params) = non-zero, with no
`exited` line and an `Error:` line instead.

**Verification**
- T3a: `--max-events 1` + 1 event → exit 0, reason=`limit`, N=1.
- T3b: `--duration 2s`, no events → exit 0, reason=`timeout`.
- T3c: SIGTERM mid-run → exit 0, reason=`signal`.
- T3d: bad params / permission failure → exit≠0, no `exited` line, has `Error:`.
- Unit tests assert (reason string, exit code) for each path.

### 4. Cleanup on exit (no `kill -9`)

Ownership-based cleanup:
- If this run **created** the subscription (no `--subscribe-id`), a clean
  exit (SIGTERM / SIGINT / stdin-EOF / limit / timeout) **unsubscribes**
  it server-side and sends Bye.
- If `--subscribe-id` was passed (reusing an existing subscription), the
  subscription is **left intact** — the caller owns its lifecycle.
- `--ephemeral` remains as an explicit "always unsubscribe" override.
- Help/docs warn: avoid `kill -9` (skips the unsubscribe → leaked
  server-side subscription: "subscription already exists" on restart,
  duplicate delivery). Prefer SIGTERM or closing stdin.

**Verification**
- T4a: start consume (self-created subscription), record subscribe_id;
  SIGTERM; afterwards `dws event status` no longer lists that subscribe_id
  and the server-side subscription is gone.
- T4b: start consume with `--subscribe-id <existing>`; SIGTERM; the
  subscription is still present (reuse case preserved).
- T4c (control): `kill -9` leaves subscribe_id lingering (documented risk;
  we only guarantee SIGTERM is clean, we do not fix kill -9 itself).

## Out of scope (next branch)

**Reconnect resilience** — today `personal source` retries only
`retryable` errors (1–30s backoff); a non-retryable error tears the bus
down and takes consume with it (the likely cause of the observed silent
drop). Making more drops retryable, keeping the bus alive across a
reconnect, and emitting `reason: source_lost` only after exhausting the
budget — tracked on its own branch, since it needs error-classification
judgement and real flaky-network testing, and would otherwise couple clean
contract work with resilience work.

## Test surface

- Unit: extend `internal/event/consume/*_test.go` with fake bus conn /
  stdin / stderr sink for T1c, T2c, T3 (all paths), T4 ownership branch.
- Integration/e2e: `--foreground` + mock source (or a short real run) for
  T1a/b, T2a/b, T3a–d, T4a/b/c — assert the stderr contract lines and exit
  codes.
