# Go Bootstrap Scripts

These repo-local entrypoints are the supported shell entrypoints for building, testing, packaging, and policy checks.

- `make build`: build the `dws` CLI from `cmd`
- `make test`: run `go test ./...`
- `make lint`: run formatting checks and required `golangci-lint`
- `make fmt`: format Go source files under `cmd/`, `internal/`, and `test/`
- `make build-all`: build cross-platform archives locally without `goreleaser`
- `make package`: build all release artifacts locally via `goreleaser --snapshot`
- `make release`: build and publish a release via `goreleaser`
- `make setup-hooks`: configure `scripts/hooks` as the repo-local git hooks path

Script groups:

- Root installers: `./scripts/install.sh`, `./scripts/install.ps1`, `./scripts/install-skills.sh`
- Dev helpers: `./scripts/dev/build.sh`, `./scripts/dev/build-all.sh`, `./scripts/dev/lint.sh`, `./scripts/dev/ci-local.sh`, `./scripts/dev/run-mock-e2e.sh`, `./scripts/dev/coverage.sh`
- Git hooks: `./scripts/hooks/pre-commit`
- Policy checks: `./scripts/policy/check-generated-drift.sh`, `./scripts/policy/check-command-surface.sh`, `./scripts/policy/check-open-source-assets.sh`
- Release helpers: `./scripts/release/post-goreleaser.sh`, `./scripts/release/verify-package-managers.sh`, `./scripts/release/publish-homebrew-formula.sh`
