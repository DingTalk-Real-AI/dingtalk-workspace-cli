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
- Product commands are loaded dynamically via `internal/plugin` from bundled descriptors.
- Command handlers live in `internal/helpers`; runtime execution flows through `internal/executor` and `internal/transport`.

## Repository Map

- `cmd`: public CLI entrypoint
- `internal/app`: root command wiring, static utility commands, plugin loading
- `internal/helpers`: product command handlers (dev, chat, calendar, contact, etc.)
- `internal/plugin`: plugin-based dynamic command loader
- `internal/cli`: catalog types and static endpoint loader
- `internal/executor`: invocation dispatch and result handling
- `internal/transport`: MCP HTTP client and request signing
- `internal/auth`: login, token management, agent-code detection
- `internal/audit`: user operation audit log
- `internal/errors`: structured error model with categories and hints
- `internal/keychain`: OS keychain integration for credential storage
- `internal/security`: endpoint allowlist and domain trust
- `internal/pat`: PAT (Personal Access Token) authorization flow
- `docs/`: public architecture and reference docs
- `scripts/`: build, test, lint, packaging, and policy checks
- `test/`: CLI, integration, contract, unit, and skill E2E test suites

## Task Routing

- Add or fix a command path: start from `internal/helpers` (handler implementations) or `internal/app` (command tree wiring)
- Protocol or transport issues: inspect `internal/transport`
- Auth or login issues: inspect `internal/auth`, `internal/pat`, `internal/keychain`
- Error message or category issues: inspect `internal/errors`
- Audit log issues: inspect `internal/audit`
- Plugin loading or command surface: inspect `internal/plugin`
- Failure or degraded mode: inspect `internal/errors`, `internal/recovery`

## Policy Checks

When command surface or plugin descriptors change, run:

- `./scripts/policy/check-command-surface.sh --strict`
- `./scripts/policy/check-open-source-assets.sh`

## Common Commands

```bash
make build
make test
make lint
./scripts/dev/ci-local.sh
git diff --check
```

## Handoff Checklist

Before handoff, include:

1. Changed files and why
2. Verification commands run and outcomes
3. Known risks or follow-up work
