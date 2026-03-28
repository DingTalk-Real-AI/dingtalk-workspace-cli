# Maintainer Automation Notes

This document keeps agent- and maintainer-specific workflow notes out of the
repository root while preserving repo-local guidance for automation.

## Read Order

1. `README.md`
2. `CONTRIBUTING.md`
3. `docs/architecture.md`
4. This document

## Project Snapshot

- `dws` is a Go-based DingTalk Workspace CLI and MCP runtime bridge.
- One internal Tool IR drives canonical CLI, schema, docs, skills, and snapshots.
- Compatibility and helper surfaces are overlays, not the canonical truth.

## Repository Map

- `cmd`: public CLI entrypoint
- `internal/app`: root command wiring and command tree mount points
- `internal/auth`: authentication and token management
- `internal/platform`: shared config, errors, and i18n
- `internal/runtime`: discovery, market, transport, executor, cache, ir, and safety
- `internal/surface`: canonical CLI command surface and output formatting
- `internal/skillgen`: CLI/schema/docs/skills generation pipeline
- `docs/`: public architecture and reference docs
- `hack/`: developer-only helper commands not shipped as public binaries
- `scripts/`: build, test, lint, packaging, and policy checks
- `test/`: CLI parity, integration, contract, and script validation suites

## Task Routing

- Add or fix a command path: start from `internal/app` and the related module under `internal/*`
- Discovery or protocol issues: inspect `internal/runtime/discovery`, `internal/runtime/market`, `internal/runtime/transport`
- Generated output drift: inspect `internal/skillgen` and run drift checks
- CLI parity mismatch: inspect `internal/surface/cli`, `test/compat`, and `test/cli`
- Failure or degraded mode: inspect `internal/runtime/discovery`, `internal/platform/errors`

## Generated Artifacts

Prefer editing source logic instead of generated files directly.

- Generated-heavy paths:
  - `docs/generated/`
  - `skills/generated/`
  - `test/golden/generated_outputs/`
- When generator or command surface changes, run:
  - `./scripts/policy/check-generated-drift.sh`
  - `./scripts/policy/check-command-surface.sh --strict`

## Common Commands

```bash
make build
make test
make lint
./scripts/dev/ci-local.sh
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-command-surface.sh --strict
./scripts/policy/check-open-source-assets.sh
git diff --check
```

## Handoff Checklist

Before handoff, include:

1. Changed files and why
2. Verification commands run and outcomes
3. Known risks or follow-up work
