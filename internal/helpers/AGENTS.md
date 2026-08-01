# Product Command Handler Agent Guide

This file applies to `internal/helpers/`. Read the root `AGENTS.md`,
[`CONTRIBUTING.md`](../../CONTRIBUTING.md), and
[`docs/coding-agent-guide.md`](../../docs/coding-agent-guide.md) first.

## Scope and routing

`internal/helpers` owns product command construction and handler behavior. Find
the owning product file and its closest tests before editing.

| Concern | Owning surface |
|---|---|
| Root/static command wiring or plugin loading | `internal/app` |
| Product command flags and handler behavior | `internal/helpers` |
| Shared Cobra construction | `internal/cobracmd` |
| Invocation and transport | `internal/executor`, `internal/transport` |
| Structured failures and recovery hints | `internal/errors`, `internal/recovery` |
| Output encoding and projections | `internal/output` |
| Confirmation and dry-run guards | `internal/safety` and command runtime gates |
| Agent identity/Schema/parameters | `internal/cli` and [`docs/schema-contributor-guide.md`](../../docs/schema-contributor-guide.md) |

Do not modify `internal/app` or shared layers merely to register an ordinary
product leaf when the existing helper construction already owns it. Cross the
package boundary only when the task changes that shared contract.

## Command contract

- Confirm the current path and flags with `dws <path> --help` and the Cobra
  construction before changing behavior.
- Keep stdout machine-readable business data. Send progress, warnings, and
  diagnostics through the established stderr/logging paths.
- Preserve structured error category, stable exit behavior, cause, trace ID,
  and an actionable recovery hint. Do not replace a typed lower-layer failure
  with an unclassified message.
- Treat paths, JSON payloads, filenames, and remote content as untrusted input;
  use existing validation and sanitization helpers.
- Mutating or destructive commands must keep their preview/confirmation
  behavior aligned with runtime safety and published Schema metadata.
- If flags, identity, parameters, selection, or safety metadata change, follow
  the scoped `internal/cli/AGENTS.md` and regenerate from reviewed inputs.

## Verification matrix

| Change | Required focused evidence |
|---|---|
| Handler bug or behavior | package regression test including the failure path |
| Flags or request mapping | Cobra/help test plus dry-run request assertion when the command supports preview |
| Output or error behavior | structured payload, stderr/stdout, and exit-code assertions |
| Mutating behavior | preview/confirmation test; live execution only with explicit authorization and disposable data |
| Shared helper refactor | affected product tests plus `go test ./internal/helpers/...` |
| Public command surface | Schema/command gates from `internal/cli/AGENTS.md` |

Before handoff, run the narrow package tests, `make format-check`, and the
admission checks selected by `docs/coding-agent-guide.md`. Do not claim a live
round-trip when only dry-run or mocked transport was exercised.
