## Summary

- What changed?
- Why is this change needed?

## Risk tier

- [ ] Documentation-only: prose/assets only; no executable, generated, workflow,
  packaging, or interface behavior changed
- [ ] Standard: ordinary implementation change with a stable package graph
- [ ] High-risk: workflow/policy, package graph, generated Schema/registry,
  platform, auth/keychain, installer, packaging, release, transport, recovery,
  or another fail-closed infrastructure change

## Verification

Record the smallest targeted evidence that proves the changed behavior. Do not
repeat the entire CI suite locally only to fill this checklist: CI expands the
selected tier from documentation checks, through affected-package tests, to
the complete high-risk suite.

- [ ] Exact in-place `CHANGELOG.md`-only check (otherwise `N/A`):
  `./scripts/policy/check-changelog-pr.sh --fast-path "$(git merge-base HEAD origin/main)" HEAD`
- [ ] Targeted test/check commands and results:
- [ ] Behavior evidence (test name, CLI output shape, or before/after result):
- [ ] Documentation links/content/rendering checked (documentation-only, otherwise
  `N/A`)
- [ ] Full local suite run because the change is high-risk (optional for other
  tiers; record command/result or `N/A`)
- [ ] `./scripts/policy/check-generated-drift.sh`
  (when generator inputs or generated artifacts may change)
- [ ] `./scripts/policy/check-command-surface.sh --strict` (if command surface changed)
- [ ] `./scripts/release/verify-package-managers.sh`
  (after `make package`, if packaging or installer surfaces changed)

## Notes

- Any risks, follow-up work, or intentional scope cuts

The repository automatically requests one eligible peer reviewer, including
after a new head push when another review is needed. Once the latest push has
peer approval and all nine required checks are current and green, auto-merge
completes the PR; authors do not need to coordinate a separate routine merge.
