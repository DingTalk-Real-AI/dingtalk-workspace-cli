## Summary

- What changed?
- Why is this change needed?

## Verification

For an exact in-place `CHANGELOG.md`-only pull request, the full-suite checks
may be marked `N/A`, but the targeted CHANGELOG check is required. For every
other pull request, mark the targeted check `N/A` and complete the applicable
full-suite checks.

- [ ] Exact `CHANGELOG.md`-only check (otherwise `N/A`):
  `./scripts/policy/check-changelog-pr.sh --fast-path "$(git merge-base HEAD origin/main)" HEAD`
- [ ] `make build`
- [ ] `make lint`
- [ ] `make test`
- [ ] `make policy`
- [ ] `./scripts/policy/check-generated-drift.sh`
- [ ] `./scripts/policy/check-command-surface.sh --strict` (if command surface changed)

## Notes

- Any risks, follow-up work, or intentional scope cuts
