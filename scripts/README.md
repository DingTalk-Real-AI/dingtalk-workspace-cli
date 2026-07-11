# Go Bootstrap Scripts

These repo-local entrypoints are the supported shell entrypoints for building, testing, packaging, and policy checks.

- `make build`: build the `dws` CLI from `cmd`
- `make test`: run `go test ./...`
- `make lint`: run formatting checks and required `golangci-lint`
- `make fmt`: format Go source files under `cmd/`, `internal/`, and `test/`
- `make package`: build all release artifacts locally
- `make release-pre VERSION=vX.Y.Z-beta.N`: validate a prerelease (`PUBLISH=1` pushes its tag)
- `make release-stable VERSION=vX.Y.Z FROM_BETA=vX.Y.Z-beta.N`: validate a stable promotion (`PUBLISH=1` pushes its tag)

Script groups:

- Root installers: `./scripts/install.sh`, `./scripts/install.ps1`, `./scripts/install-skills.sh`
- Product convenience installers: `./scripts/install-devapp.sh`, `./scripts/install-devapp.ps1`, `./scripts/install-event.sh`
- Dev helpers: `./scripts/dev/build.sh`, `./scripts/dev/lint.sh`, `./scripts/dev/ci-local.sh`, `./scripts/dev/run-mock-e2e.sh`, `./scripts/dev/coverage.sh`
- Policy checks: `./scripts/policy/check-generated-drift.sh`, `./scripts/policy/check-command-surface.sh`, `./scripts/policy/check-command-compatibility.sh --base-ref <main-ref> --stable-ref <latest-GA-tag>`, `./scripts/policy/check-open-source-assets.sh`
- Release entrypoint and contract: `./scripts/release/release.sh`, `./scripts/release/release-contract.sh`; operator guide: [`docs/releasing.md`](../docs/releasing.md)
- Release packaging helpers: `./scripts/release/post-goreleaser.sh`, `./scripts/release/stage-npm-package.sh`, `./scripts/release/pack-npm-package.sh`, `./scripts/release/verify-delivered-stable.sh`, `./scripts/release/verify-release-artifacts.sh`, `./scripts/release/verify-github-release-assets.sh`, `./scripts/release/verify-github-tag-authority.sh`, `./scripts/release/verify-package-managers.sh`, `./scripts/release/publish-homebrew-formula.sh`
