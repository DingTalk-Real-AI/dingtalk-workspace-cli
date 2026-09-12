---
category: Changed
---

- **Selection guidance in --help** (#1022) — `dws <command> --help` now renders the Selection guidance sections `Avoid when` / `Prerequisites` / `Tips` (when declared) after the intent prose and before the `参数约束` (parameter constraints) section, so agents exploring via help see the same routing boundaries the Runtime Schema publishes; `use_when` is not rendered because every declared entry currently restates the intent prose.
- **Schema --compact guidance allowlist** (#1022) — `dws schema --compact` now publishes the `prerequisites` and `tips` fields alongside `use_when` / `avoid_when` / `examples`, aligning the Agent-view allowlist with the help surface; no command declares the new fields yet, so existing compact output is unchanged.
