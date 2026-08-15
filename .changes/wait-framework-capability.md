---
category: Added
---

- **Wait framework capability** — adds the reviewed `Contract.Wait`
  declaration (`contract.WaitSpec`): a command may declare terminal-state
  waiting (mode poll/event/auto, poll command, status query, terminal
  status→outcome map, pending values, reviewed timeout default). Declared
  commands register `--wait` / `--wait-timeout` (framework-owned flags that
  never enter MCP toolArgs); undeclared commands reject the flags as unknown
  instead of silently ignoring them. After a successful dispatch the framework
  wait phase polls through the leaf's `WaitPoll` hook: terminal success closes
  the unified envelope to `success`, a terminal failure closes it to
  `failure` with new wire-stable `error.type: "wait"` (exit code 8), and a
  timeout keeps the envelope `pending` with `meta.operation.timed_out: true`
  and the last observed state (exit 0 — an accepted-but-not-terminal wait is
  not a process failure). The capability is projected into the Schema catalog
  (`wait` key) alongside `dry_run`. No business command declares it yet;
  approval/export/batch adoption lands separately.
