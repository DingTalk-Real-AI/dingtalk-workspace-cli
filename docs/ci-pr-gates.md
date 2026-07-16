# Pull request quality gates

The repository defines five focused checks in addition to its existing CI:

- **Interface Integrity** enforces backwards compatibility. Every historical
  command path and alias must still resolve, every historical command must
  still render `-h`, and historical flags must keep their type and shorthand.
  New commands, aliases, and flags are allowed. The same job compares the full
  complete `dws schema --all` contract with the PR merge-base, blocking removed
  products/tools/parameters, incompatible parameter or interface mappings,
  constraint drift, and safety-semantic drift. It also checks that executable
  `dws ...` references in `skills/**/*.md` resolve to real commands.
  Help compatibility covers command/alias/flag spelling, flag type and
  shorthand; descriptive prose may evolve without breaking the gate.
- **Coverage** runs unit tests on every pull request and prints both overall and
  changed-code statement coverage. During the migration to the 80% repository
  target, overall coverage may not regress from a profile generated from the
  merge-base with the same test command, while changed production Go
  statements must meet 80%. Linux, Windows, and macOS each generate a native
  coverage profile for changed packages and enforce the threshold against
  changed files buildable on that platform, so build-tagged source cannot be
  hidden by an Ubuntu-only profile. Overall non-regression allows 0.1 percentage point of measurement
  variance to avoid failing unchanged code on test-path noise. Set
  `COVERAGE_ENFORCE_OVERALL=true` once repository coverage reaches 80% to make
  the overall target fail closed as well.
- **CLI Smoke** builds the release binary, reads the root command list from the
  structured Interface contract, and renders offline help for every public
  top-level command. It rejects Cobra's unknown-command root-help fallback and
  fails when the checked-in development fixture is stale.
- **Mock MCP Smoke** runs the existing HTTP and stdio MCP lifecycle tests
  (`Initialize -> ListTools -> CallTool`).
- **AI Behavior Check** applies to pull requests labeled `ai-generated`. It
  limits the change to 30 files and blocks release/CI infrastructure changes,
  including policy implementations and the checked-in Interface fixture.
  It uses `pull_request_target` without checking out PR code, so the policy
  cannot be bypassed by changing the workflow in the same pull request. The
  evaluator writes an `AI Behavior Check` commit status to the PR head SHA so
  GitHub rulesets can require it.

## Running the compatibility gates

Run:

```sh
make build
make interface-integrity
make authoritative-interface-integrity BASE_REF=<merge-base>
make schema-compatibility BASE_REF=<merge-base>
make skill-command-integrity
make cli-smoke
# Run on the corresponding native runner with its generated profile:
make coverage-gate-platform BASE_REF=<merge-base> PROFILE=<coverage-profile>
```

`make coverage-gate` is the enforcement step, not a profile generator. It
expects the candidate, policy, and merge-base profiles (`coverage.txt`,
`coverage-policy.txt`, and `coverage-base.txt`) produced by the preceding CI
steps. A clean local checkout can reproduce the Linux/overall CI gate with:

```sh
base_ref=$(git merge-base HEAD origin/main)
root=$(pwd)
base_worktree=$(mktemp -d "${TMPDIR:-/tmp}/dws-coverage-base.XXXXXX")
rmdir "$base_worktree"
cleanup() { git worktree remove --force "$base_worktree" >/dev/null 2>&1 || true; }
trap cleanup EXIT HUP INT TERM

go test -count=1 -coverprofile=coverage.txt -covermode=atomic \
  ./ ./cmd/... ./internal/... ./skills/...
go test -count=1 -coverprofile=coverage-policy.txt -covermode=atomic \
  ./pkg/... ./scripts/policy/...
git worktree add --detach "$base_worktree" "$base_ref"
(
  cd "$base_worktree"
  go test -count=1 -coverprofile="$root/coverage-base.txt" -covermode=atomic \
    ./ ./cmd/... ./internal/... ./skills/...
)
make coverage-gate BASE_REF="$base_ref"
```

The native-platform target likewise expects `PROFILE` to have already been
generated on that operating system. CI owns those generation steps; copying
only either enforcement command into a clean checkout is intentionally an
incomplete invocation.

CI derives the authoritative Interface snapshots from both the PR merge-base
and the latest reachable stable release tag. The complete Schema snapshot comes
from the PR merge-base, which contains the registry-first Schema introduced on
`main`. The candidate branch cannot bless a breaking change by editing a
fixture. Schema additions are allowed; historical products, tools, parameters,
parameter mappings, positional execution fields, constraints, and safety
semantics remain protected. Positional descriptions are documentation and may
change without breaking compatibility.

`make update-interface-baseline` still extends the local checked-in Interface
fixture used by `make interface-integrity`. Updates are monotonic: they add new
commands and flags without removing history.

For an intentional compatibility reset at a major-version boundary, run
`make reset-interface-baseline`. This replaces all CLI compatibility history
with the current command tree and must receive explicit human review.

## Required GitHub repository settings

Create a ruleset for `main` that requires pull requests and code-owner review,
then mark these aggregate status checks as required:

- `CI Gate`
- `Multi Profile E2E`
- `AI Behavior Check`

`CI Gate` fails closed unless every first-layer CI job succeeds, including
lint, tests, native Linux/Windows/macOS coverage, policy,
Interface/Schema/Skill integrity, and smoke tests. Requiring the aggregate
check keeps repository rules stable when an internal job is renamed or split.

The `ai-generated` label must be applied by the PR-creation automation or by a
maintainer; GitHub cannot infer reliably whether a human-authored PR contains
AI-generated code.
