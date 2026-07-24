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

## Homebrew Formula Delivery

Official releases use the Release workflow's built-in `GITHUB_TOKEN` to update
exactly one tracked Formula after the immutable GitHub assets and their
checksums have passed verification. The publisher validates the rendered Ruby,
commits only the configured Formula path, never force-pushes `main`, and retries
from a fresh clone up to three times when `main` advances concurrently. Normal
stable and beta releases do not create a Formula PR or run a permission
canary. The workflow uses the existing repository-scoped
`HOMEBREW_PR_TOKEN` release identity because GitHub does not allow its built-in
Actions App to bypass this repository's rulesets. That identity is the sole
user bypass actor on the two default-branch rulesets. The workflow creates the
nine Code Admission checks for the Formula-only commit only after proving its
sole parent already has all nine successful checks and the committed Formula
exactly matches this release's verified bytes.

Keep `HOMEBREW_PR_TOKEN` repository-scoped with `Contents: write` and
`Pull requests: write` (the latter remains necessary for withdrawal rollback),
keep its owner as the designated ruleset bypass actor, and do not reuse
`RELEASE_GOVERNANCE_TOKEN`. The workflow and publisher provide the Formula-only
path restriction; GitHub rulesets do not infer that restriction from the token.

## Release Governance and Recovery

Store `RELEASE_GOVERNANCE_TOKEN` as a dedicated Actions secret with only
repository `Administration: read`. The immutable-releases REST endpoint is an
administration setting and cannot be read by the workflow's built-in
`GITHUB_TOKEN`. Both the default-branch governance preflight and the tag
contract use this same credential so a missing or expired identity is detected
before an irreversible tag is created.

Recovery is restricted to an existing annotated tag whose exact tag object,
commit, sealed metadata, original failed run/attempt, requester identity and
Release state all match; it then reuses the normal release jobs without a
second-person environment approval. A same-run “Re-run failed jobs” is even
lighter: the seal job may adopt an existing tag only when its complete
authority matches that run and its original attempt is not newer than the
current attempt. Do not put publication secrets in temporary branches or
create ad-hoc recovery workflows.

Cloud-sealed releases mirror to OSS only when the repository variable
`ENABLE_OSS_MIRROR` is exactly `true`. Leave the variable unset while no Bucket
is provisioned; GitHub, npm, and Homebrew delivery can then complete without
running the OSS step. Once enabled, missing credentials, an invalid Bucket, or
an upload failure remains fail-closed. The cloud tag immutably records the
decision as `OSS-Mirror: enabled|deferred`; publication and withdrawal consume
that sealed value instead of the variable's later state. Deferred releases
cannot use `repair_oss_version`; enabling OSS applies to later release tags
until an audited immutable repair marker is implemented.

If an immutable GitHub Release and npm package were delivered but an enabled
downstream China mirror failed, dispatch the normal `Release` workflow from the
protected default branch with exactly one of `repair_gitee_version` or
`repair_oss_version`. Channel repair accepts a fully successful exact release,
or a failed exact-tag run only when its latest attempt completed the release
contract, build, Apple signature, immutable GitHub publication, and npm
delivery checks for the exact tagged commit. OSS repair additionally requires
the tag's sealed policy to be `enabled`. It then downloads and re-verifies the
immutable assets before invoking only the selected mirror. For a failed
release, an OSS repair requires the OSS step itself to be the recorded failure.
A Gitee repair accepts either a failed Gitee job or a Gitee job that was
skipped behind that OSS failure; the latter is an explicit Gitee backfill and
does not claim that OSS has been repaired. Gitee repair requires `GITEE_TOKEN`,
`GITEE_USER`, and `GITEE_REPO`; OSS repair requires `OSS_ACCESS_KEY_ID`,
`OSS_ACCESS_KEY_SECRET`, `OSS_ENDPOINT`, and `OSS_BUCKET` (with optional
`OSS_PREFIX`) as Actions secrets. Missing credentials fail the selected repair
closed.

## Handoff Checklist

Before handoff, include:

1. Changed files and why
2. Verification commands run and outcomes
3. Known risks or follow-up work
