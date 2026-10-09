---
category: Added
---

- **Wait framework capability** — adds the reviewed `Contract.Wait`
  declaration (`contract.WaitSpec`) with three execution modes: `poll`
  (cadence-poll the leaf's `WaitPoll` hook), `event` (consume the leaf's
  `WaitEvents` push stream, correlate events to the accepted resource via
  `match_field`/`resource_query`, apply the same terminal map), and `auto`
  (event first, fall back to polling when the stream ends or the
  subscription fails — one deadline spans both phases). Declared commands
  must use the `ResultInvoke` dispatcher; mode and hooks are paired at
  construction (poll↔WaitPoll, event↔WaitEvents, auto↔both; surplus hooks
  are rejected too). Declared commands register `--wait` /
  `--wait-timeout` (framework-owned flags that never enter MCP toolArgs);
  undeclared commands reject the flags as unknown. The wait phase closes
  the unified envelope exactly once: terminal success → `success`,
  terminal failure → `failure` with new wire-stable `error.type: "wait"`
  (exit code 8), timeout → `pending` with `meta.operation.timed_out: true`
  and the last observed state (exit 0). Deadline exhaustion during a poll,
  during event consumption, or between polls always closes as timed-out
  pending, never as a poll/stream failure; a correlated event with an
  unknown status fails closed exactly like a poll. The pairing also closes
  the overlay path: `AttachContract` / Tier2 metadata attaches cannot
  publish a wait declaration (no hooks, no flags, no wait phase behind
  it) — only the managed `New` construction can. At Schema assembly every
  `poll_command` is resolved against the bound registry and must name a
  delivered, agent-visible read command, so the declared manual resume
  path cannot drift from the command tree. The capability is
  projected into the Schema catalog (`wait` key) alongside `dry_run`. No
  business command declares it yet; approval/export/batch adoption lands
  separately.
