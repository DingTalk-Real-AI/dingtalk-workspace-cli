# Event consume — AI subprocess contract

Defines the stable `dws event consume` subprocess contract so an
orchestrator can determine when the consumer is ready, stop it cleanly,
and machine-read why it exited.

Scope of this branch: the six **contract** items below. Reconnect
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

### 5. Subscription-create retry orchestration and local guard

This policy covers all 16 public personal-event keys and every logical
subscription in a multi-event command. It applies only before the ready
marker; reconnecting an established Stream remains a separate mechanism.

- The `0/2/1` limits below are an **Agent/host orchestration contract**, not
  a CLI-enforced persisted total-attempt cap. Each `dws event consume`
  process sends at most one subscription-create HTTP request for a logical
  subscription and performs no in-process automatic retry. The CLI persists
  only the `in_flight`, `cooldown`, and `terminal_hold` guard states; it does
  not persist or enforce the Agent/host attempt count across invocations.
- ID resolution, `event consume`, and later `event status/stop` must use the
  same `--profile`. A user or conversation ID resolved under another profile
  must not be reused for the current subscription.
- A logical subscription is keyed by the current profile/identity, event key,
  rule type, target, and filters. A new `subscribe_id`, `trace_id`, or process
  does not create a new logical operation or reset the Agent/host budget.
- For the Agent/host, `retryable=false` means
  `max_additional_attempts=0`.
- For the Agent/host, `retryable=true` means
  `max_additional_attempts=2`. It must honor `retry_after_seconds` or
  `next_retry_at` when present and must not retry early.
- For the Agent/host, an omitted retryable value
  (`retryable=unknown`) means `max_additional_attempts=1`; a second unknown
  failure stops the operation.
- `in_flight` means the original logical request is still running.
  `cooldown` and `terminal_hold` mean a guard is already delaying or blocking
  it. These states must not recursively launch `event consume`, start a
  parallel equivalent subscription, or bypass the guard with a new subId or
  trace. The caller waits for the original request/guard or stops, while the
  Agent/host keeps its own orchestration count.
- A multi-event command remains one original operation. A caller must not
  split out a failed event, reorder events, or restart the command to bypass
  a budget. Existing startup rollback cleans subscriptions created before a
  later item fails.

#### Local guard state operations

- The default open-edition state file is
  `~/.dws/events/open/personal_stream/<identity_hash>/personal_subscription_attempts.json`.
  The config root follows `DWS_CONFIG_DIR` when set, and another edition uses
  that edition's directory instead of `open`.
- The identity directory is mode `0700`; both
  `personal_subscription_attempts.json` and
  `personal_subscription_attempts.lock` are mode `0600`.
- A failure streak resets after 24h without another failure. A
  `terminal_hold` lasts 1h. Prefer waiting until the reported
  `next_retry_at`; do not clear the file as a normal retry mechanism.
- For emergency recovery, first ensure that no subscription-create process is
  running for that identity. Delete only
  `personal_subscription_attempts.json`, never the lock file. This clears
  every protection record for that identity, not just one event.

**Verification**
- T5a (policy): skill/docs tests pin the Agent/host 0/2/1 orchestration
  contract and explicitly reject describing it as a CLI-persisted hard cap.
- T5b (CLI): one process issues at most one create request per logical
  subscription; a changed subId/trace or process restart does not bypass the
  persisted fingerprint guard.
- T5c: `in_flight`/`cooldown` does not recursively issue another create.
- T5d: multi-event startup cannot be split or reordered to bypass the guard,
  and a partial startup still rolls back earlier subscriptions.
- T5e: state-store tests cover `0700`/`0600` permissions, 24h reset, 1h
  `terminal_hold`, and identity-scoped cleanup; skill/docs tests pin the
  operational recovery instructions.

### 6. Host runtime-token handoff

When the root command carries an explicit host-supplied `--token`, personal
event control requests and the foreground Stream use that token with higher
priority than local OAuth. A detached bus receives it only through the
owner-only local IPC transport:

1. The child starts in runtime-token mode with non-sensitive identity and
   ticket metadata only; neither its argv nor environment contains the token.
2. The consumer sends `Hello` with `credential_mode=runtime_token`.
3. The bus advertises the additive `runtime_token_v1` capability and its
   in-memory credential generation in `HelloAck`.
4. Only after that capability is confirmed does the consumer send a bounded
   `credential_update` frame. The bus applies it with generation CAS, replies
   with `credential_update_ack`, and registers the consumer only on success.

The bus blocks ticket acquisition until the first runtime credential arrives.
A later invocation may rotate Token A to Token B on a compatible existing bus;
the current WebSocket remains connected and the next ticket request or natural
reconnect uses B. If a 401 rejects the current runtime token, only an already
installed newer generation is retried; the runtime path never refreshes or
falls back to a local OAuth profile and never suggests `dws auth login`.

Clients do not send a token to a bus that lacks the capability, do not stop
other consumers automatically, and fail before printing the ready marker. With
no explicit `--token`, the original OAuth, refresh, profile, and old-client to
new-bus protocol behavior remains unchanged.

**Verification**
- T6a: a stale local Token A and root Token B produce control and ticket
  requests authenticated only with B.
- T6b: compatible bus reuse supports A-to-B rotation and generation conflicts;
  401 retries only an already-installed newer runtime token.
- T6c: an old bus receives no credential and remains running; the new consumer
  exits before its ready marker.
- T6d: a canary credential is absent from child argv/environment, dry-run,
  stdout/stderr, `bus.meta`, `bus.log`, run state, and returned errors.
- T6e: no-token OAuth, refresh, multi-profile, marker/cache, and bus-reuse tests
  continue to pass.

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
