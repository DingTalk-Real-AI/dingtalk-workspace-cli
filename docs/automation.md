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

## Homebrew Formula PR Automation

Official tag releases require the repository Actions secret
`HOMEBREW_PR_TOKEN`. Prefer a fine-grained personal access token owned by a
maintainer or release-bot account, limited to this repository with
`Contents: write` and `Pull requests: write`. If organization policy prevents
that account from targeting the repository, use a dedicated classic token with
only the `public_repo` scope. Do not reuse a broad developer token.

Store the dedicated token as the `HOMEBREW_PR_TOKEN` repository Actions secret
and rotate it before its configured expiration. Replace it immediately if it is
exposed, its owner loses repository access, or the release-bot ownership
changes. The Release workflow uses this
dedicated token only to push an `automation/homebrew-*` branch and open the
stable or beta Formula PR. It does not push Formula changes directly to `main`.
The default-branch governance preflight and every tag contract authenticate the
token before publication, reject over-scoped classic tokens, confirm its
identity, and run a controlled write canary. The canary pushes a unique
`automation/homebrew-token-canary-*` branch with a `[skip ci]` commit, creates a
draft PR, closes it, and deletes the branch with the same token. This proves both
Contents and Pull requests write access before publication without merging
anything. The gate also rejects reuse of `RELEASE_GOVERNANCE_TOKEN`.
No maintainer environment variable is required when creating a tag. Using the
built-in `GITHUB_TOKEN` is insufficient because organization policy prevents
Actions from creating pull requests, and its generated PR events may require
separate workflow approval.

## Release Governance and Recovery

Store `RELEASE_GOVERNANCE_TOKEN` as a dedicated Actions secret with only
repository `Administration: read`. The immutable-releases REST endpoint is an
administration setting and cannot be read by the workflow's built-in
`GITHUB_TOKEN`. Both the default-branch governance preflight and the tag
contract use this same credential so a missing or expired identity is detected
before an irreversible tag is created.

Create a protected `release-recovery` environment limited to protected
branches, with a required reviewer, self-review disabled, and administrator
bypass disabled. The workflow reads the environment through the GitHub API and
fails closed unless the required-reviewer, prevent-self-review, and protected-
branch rules are present.
Recovery is restricted to an existing annotated tag whose exact tag object,
commit, and failed tag-push run all match; it then reuses the normal release
jobs. Do not put publication secrets in temporary branches or create ad-hoc
recovery workflows.

## Handoff Checklist

Before handoff, include:

1. Changed files and why
2. Verification commands run and outcomes
3. Known risks or follow-up work
