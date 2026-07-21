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

Run the verification commands that match the surface you changed before you hand work back.

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

## Pull Request Checklist

1. Keep implementation and tests in sync.
2. Run `./scripts/dev/ci-local.sh`.
3. Run `./scripts/policy/check-command-surface.sh --strict` when command paths/flags change. CI also runs `./scripts/policy/check-command-compatibility.sh --base-ref <main-ref> --stable-ref <latest-GA-tag>` against both the target branch and latest stable release.
4. Run `./scripts/policy/check-generated-drift.sh` when generated artifacts may change.
5. Run `./scripts/release/verify-package-managers.sh` when packaging or installer surfaces change (run `make package` first).
6. Update docs and `CHANGELOG.md` for behavior/interface changes.
7. Include verification evidence in your PR description.

## Submission Flow

1. Make the smallest atomic change that satisfies the task.
2. Keep doc edits factual and limited to implemented behavior.
3. Run the relevant verification commands.
4. Report the validation results with the handoff.
