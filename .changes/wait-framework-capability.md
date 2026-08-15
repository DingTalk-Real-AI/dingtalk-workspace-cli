---
category: Added
---

- **Wait framework capability** — adds the reviewed `Contract.Wait`
  declaration (`contract.WaitSpec`, poll mode only: poll command, status
  query, terminal status→success/failure map, pending values, reviewed
  timeout default; event/auto modes arrive with their own execution paths —
  declaring them fails validation today). Declared commands must use the
  `ResultInvoke` dispatcher and register `--wait` / `--wait-timeout`
  (framework-owned flags that never enter MCP toolArgs); undeclared commands
  reject the flags as unknown. After a successful dispatch the framework wait
  phase polls through the leaf's `WaitPoll` hook and closes the unified
  envelope exactly once: terminal success → `success`, terminal failure →
  `failure` with new wire-stable `error.type: "wait"` (exit code 8), timeout
  → `pending` with `meta.operation.timed_out: true` and the last observed
  state (exit 0 — an accepted-but-not-terminal wait is not a process
  failure). Deadline exhaustion during a poll or between polls always closes
  as timed-out pending, never as a poll failure. The capability is projected
  into the Schema catalog (`wait` key) alongside `dry_run`. No business
  command declares it yet; approval/export/batch adoption lands separately.
