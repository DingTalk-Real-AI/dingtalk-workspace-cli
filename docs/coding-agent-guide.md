# Coding Agent Workflow

This is the task intake and self-check contract for coding agents working in
this repository. It is intentionally independent of external Wiki systems:
the checked-out repository is the execution context and evidence source.

## 1. Normalize the task input

Start from the copyable
[`coding-agent-task-template.md`](coding-agent-task-template.md). Keep one
primary outcome per task. Fill unknown fields from the issue, nearby code,
tests, and versioned docs; state any assumption that can affect behavior.

For a saved, filled template, run `make coding-agent-task TASK=path/to/task.md`.
The local checker rejects missing required fields, unsupported task kinds, and
unresolved placeholders before implementation begins.

Do not invent acceptance criteria that expand the requested behavior. Stop for
user input only when the unresolved choice would change externally visible
behavior, compatibility, destructive scope, credentials, or external state.

## 2. Establish the baseline

1. Run `git status --short` and identify pre-existing changes.
2. Read the applicable guides linked from the root `AGENTS.md`.
3. Locate the implementation and its closest tests with `rg`/`rg --files`.
4. Reproduce the bug or capture the current contract before changing it.
5. Pick the smallest owning layer; avoid duplicating policy in a caller when a
   shared typed layer already owns it.

Never clean, overwrite, stage, or reformat unrelated user changes. If a
required file is already modified, inspect the overlap and preserve both
intents or stop with the exact conflict.

## 3. Implement from authoritative inputs

- Go behavior belongs in the package that owns the contract, with focused
  tests beside it.
- Public CLI paths and flags must match the live Cobra tree and compatibility
  policies.
- Schema and Agent-facing changes start from reviewed source inputs; generated
  outputs are publication artifacts.
- Documentation describes behavior that exists in the same change.
- Secrets, tokens, local identities, and private endpoints must not enter code,
  fixtures, logs, or handoff output.

For generated files, run the repository generator and inspect the resulting
diff. A large or unrelated generated diff is a signal to stop and find the
wrong input or nondeterminism, not something to accept automatically.

## 4. Select validation by change surface

Run the narrow check while iterating, then the applicable admission checks
before handoff. `make help` is the authoritative target list.

| Changed surface | Focused check | Admission checks |
|---|---|---|
| Documentation only | inspect links/examples | `git diff --check` |
| Go implementation | `go test ./path/to/package` | `make format-check`, `make test` |
| CLI paths or flags | focused command/help tests | `make build`, `./scripts/policy/check-command-surface.sh --strict`, `make interface-integrity` |
| Schema registry, hints, or generators | focused generator/app tests | `make generate-schema`, `./scripts/policy/check-generated-drift.sh`, `./scripts/policy/check-schema-catalog.sh`, `make test-schema-agent-examples` |
| Skill command examples | inspect referenced `dws` help | `make skill-command-integrity` |
| CI or test sharding | run the affected script/test | `make test-plan`, `make lint`, and the CI workflow's pinned actionlint command when workflows change |
| Packaging or installers | focused release-script tests | `make package`, `./scripts/release/verify-package-managers.sh` |
| Authentication, transport, or OS-specific code | focused tests, including failure paths | `make test`; run relevant platform checks or disclose the unavailable platform |

`make policy` is the combined policy gate and is appropriate for command,
Schema, generated-asset, or broad cross-cutting changes. Platform credentials
and live services are not prerequisites for ordinary unit tests; never turn a
missing credential into permission to skip deterministic checks.

The guide contract itself is executable. Run `make coding-agent-harness` after
changing `AGENTS.md`, this guide, the Schema contributor guide, or their routed
commands and paths. This remains a local, opt-in check and is not wired into CI.

## Design references

This repository adapts two patterns without copying their product-specific
rules:

- [Lark CLI's contributor guide](https://github.com/larksuite/cli/blob/5efaf65aec59c33899475bb90e6bff1bc3b5b65c/AGENTS.md): one primary goal, machine-consumable errors/output, and validation selected by behavior surface.
- [WeCom CLI's root routing guide](https://github.com/WecomTeam/wecom-cli/blob/9eb7898b959861af879495e211e37431fa908f19/AGENTS.md) and [human helper template](https://github.com/WecomTeam/wecom-cli/blob/9eb7898b959861af879495e211e37431fa908f19/src/helpers/HUMANS.md): a thin root guide, scoped implementation guidance, and a copyable request format.

## 5. Pre-handoff self-check

Confirm every applicable item:

- The diff implements the stated goal and no unrelated cleanup.
- Pre-existing changes are still present and were not attributed to this task.
- New behavior has a regression test; removed behavior has an explicit reason.
- Public command paths, flags, output, exit behavior, and compatibility remain
  intentional.
- Destructive or mutating operations retain the required confirmation path.
- Generated files came from their reviewed inputs and generation is clean.
- Docs and examples use commands accepted by current help/Schema.
- Errors preserve actionable context without leaking secrets.
- `git diff --check` passes and the final diff has been read.
- Every reported check is labeled passed, failed, or not run with a reason.

Use this compact handoff shape:

```text
Outcome: what is now true
Files: intentional files changed
Validation: exact commands and results
Limits: unrun checks, environment constraints, follow-ups
```
