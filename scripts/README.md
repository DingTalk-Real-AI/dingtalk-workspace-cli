# Go Bootstrap Scripts

These repo-local entrypoints are the supported shell entrypoints for building, testing, packaging, and policy checks.

- `make build`: build the `dws` CLI from `cmd`
- `make test`: run every default package returned by `go list ./...`
- `make test-plan`: verify every default Go package belongs to exactly one CI test shard
- `make lint`: run the repository-wide format check, `go vet`, and required `staticcheck`
- `make format-check`: check every tracked or non-ignored untracked repository Go source file with `gofmt`
- `make fmt`: format every tracked or non-ignored untracked repository Go source file
- `make policy`: reuse the current `dws` binary and run the complete policy suite (`make build` first)
- `make package`: build all release artifacts locally
- `make release-pre VERSION=vX.Y.Z-beta.N`: validate a prerelease (`PUBLISH=1` pushes its tag)
- `make release-stable VERSION=vX.Y.Z FROM_BETA=vX.Y.Z-beta.N`: validate a stable promotion (`PUBLISH=1` pushes its tag)

Script groups:

- Root installers: `./scripts/install.sh`, `./scripts/install.ps1`, `./scripts/install-skills.sh`
- Product convenience installers: `./scripts/install-devapp.sh`, `./scripts/install-devapp.ps1`, `./scripts/install-event.sh`
- CI package plan: `./scripts/ci/test-packages.sh`
- Dev helpers: `./scripts/dev/build.sh`, `./scripts/dev/lint.sh`, `./scripts/dev/ci-local.sh`, `./scripts/dev/run-mock-e2e.sh`, `./scripts/dev/coverage.sh`
- Policy checks: `./scripts/policy/check-schema-command-registry.sh`, `./scripts/policy/check-generated-drift.sh`, `./scripts/policy/check-command-surface.sh`, `./scripts/policy/check-command-compatibility.sh --base-ref <main-ref> --stable-ref <latest-GA-tag>`, `./scripts/policy/check-schema-catalog.sh`, `./scripts/policy/check-schema-binary.sh`, `./scripts/policy/check-open-source-assets.sh`
- Policy runtime files: set `DWS_POLICY_TMPDIR` to override the default `.worktrees/policy-tmp` workspace
- Release entrypoint and contract: `./scripts/release/release.sh`, `./scripts/release/release-contract.sh`; operator guide: [`docs/releasing.md`](../docs/releasing.md)
- Release packaging helpers: `./scripts/release/post-goreleaser.sh`, `./scripts/release/stage-npm-package.sh`, `./scripts/release/pack-npm-package.sh`, `./scripts/release/verify-delivered-stable.sh`, `./scripts/release/verify-release-artifacts.sh`, `./scripts/release/verify-github-release-assets.sh`, `./scripts/release/verify-github-tag-authority.sh`, `./scripts/release/verify-package-managers.sh`, `./scripts/release/publish-homebrew-formula.sh`
