# Contributing

This repository uses repo-local documentation, scripts, and tests as the source of truth for active behavior and validation.

## License and Contribution Terms

By submitting a contribution, you agree that your contribution is licensed
under the project [Apache License 2.0](./LICENSE).

## Before You Start

1. Read `README.md`.
2. Read the relevant docs under `docs/`.
3. Inspect the code and tests for the area you will change.
4. Decide the smallest safe change that satisfies the request.

Maintainers and automation authors should also read
`docs/automation.md` for repo-local release and agent workflow
notes that are intentionally kept out of the repository root.

## Working Rules

- Keep changes minimal and atomic.
- Update tests together with implementation changes.
- Prefer main-thread integration for shared or high-conflict files.
- Do not let multiple agents edit the same code region at the same time.

## Local Checks

Run the verification commands that match the surface you changed before you
hand work back. The goal is useful, change-specific evidence, not a second
local execution of every CI job.

Common repository checks already used here include:

```bash
./scripts/dev/ci-local.sh
./scripts/policy/check-open-source-assets.sh
go test ./...
make test
make test-plan
make lint
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-command-surface.sh --strict
./scripts/release/verify-package-managers.sh
git diff --check
```

Select the PR risk tier before choosing checks:

| Tier | Typical scope | Developer evidence | CI expansion |
|---|---|---|---|
| Documentation-only | Prose and documentation assets with no executable, generated, workflow, packaging, or interface change | Links/content/rendering plus repository asset checks | Lightweight documentation validation; all nine named contexts still report |
| Standard | Ordinary implementation work with a stable package graph | Focused unit/integration tests and observable behavior for the changed path | Race tests for changed packages and their reverse dependencies, scope-matched HEAD/base coverage, and representative Darwin/Windows compilation |
| High-risk | Workflow/policy, package graph, generated Schema/registry, platform, auth/keychain, installer, packaging, release, transport, recovery, or an unprovable infrastructure change | Relevant full or domain suite plus focused behavior evidence | Complete race suite, native platform tests, and all affected domain gates; protected `main` uses this tier |

Classification fails closed: an incomplete diff, package add/remove/rename, or
uncertain dependency graph selects the high-risk suite. Native changed-code
coverage is additionally selected for platform-sensitive code.

## Pull Request Checklist

1. Keep implementation and tests in sync.
2. Select the documentation-only, standard, or high-risk tier and run the
   smallest checks that prove the change. Use `./scripts/dev/ci-local.sh` when
   a complete local pass is warranted; it is not required for every ordinary
   PR.
3. Include both the commands/results and user-visible or contract-level
   behavior evidence in the PR description.
4. Run `./scripts/policy/check-command-surface.sh --strict` when command
   paths/flags change. CI also runs
   `./scripts/policy/check-command-compatibility.sh --base-ref <main-ref> --stable-ref <latest-GA-tag>`
   against both the target branch and latest stable release.
5. Run `./scripts/policy/check-generated-drift.sh` when generated artifacts may
   change.
6. Run `./scripts/release/verify-package-managers.sh` when packaging or
   installer surfaces change (run `make package` first).
7. Update docs and `CHANGELOG.md` for behavior/interface changes.

## Submission Flow

1. Make the smallest atomic change that satisfies the task.
2. Keep doc edits factual and limited to implemented behavior.
3. Run the relevant verification commands.
4. Report the validation results and risk tier with the handoff.
5. Open a ready PR against `main`. Base-owned automation assigns one eligible
   peer reviewer, balancing the current open-review load and excluding the
   author. A new head push re-enters the same routing flow when the latest
   revision still needs review.
6. After the latest push has one peer approval and the exact nine required
   contexts are current and green, auto-merge completes the PR. If `main`
   advances first, strict status checks revalidate the branch; no separate
   routine merge request is needed.
