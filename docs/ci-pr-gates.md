# Pull request quality gates

The repository defines four focused checks in addition to its existing CI:

- **Interface Integrity** enforces backwards compatibility. Every historical
  command path and alias must still resolve, every historical command must
  still render `-h`, and historical flags must keep their type and shorthand.
  New commands, aliases, and flags are allowed. The same job also checks that
  `dws schema list` does not lose products/tools/parameters and that executable
  `dws ...` references in `skills/**/*.md` resolve to real commands.
  Help compatibility covers command/alias/flag spelling, flag type and
  shorthand; descriptive prose may evolve without breaking the gate.
- **Coverage** runs unit tests on every pull request and prints both overall and
  changed-code statement coverage. During the migration to the 80% repository
  target, overall coverage may not regress from a profile generated from the
  merge-base with the same test command, while changed production Go
  statements must meet 80%. Overall non-regression allows 0.1 percentage point
  of measurement variance to avoid failing unchanged code on test-path noise. Set
  `COVERAGE_ENFORCE_OVERALL=true` once repository coverage reaches 80% to make
  the overall target fail closed as well.
- **CLI Smoke** builds the release binary and renders offline help for the root
  and every public top-level command.
- **Mock MCP Smoke** runs the existing HTTP and stdio MCP lifecycle tests
  (`Initialize -> ListTools -> CallTool`).
- **AI Behavior Check** applies to pull requests labeled `ai-generated`. It
  limits the change to 30 files and blocks release/CI infrastructure changes.
  It uses `pull_request_target` without checking out PR code, so the policy
  cannot be bypassed by changing the workflow in the same pull request. The
  evaluator writes an `AI Behavior Check` commit status to the PR head SHA so
  GitHub rulesets can require it.

## Extending the compatibility baselines

Run:

```sh
make build
make update-interface-baseline
make update-schema-baseline
make interface-integrity
make schema-compatibility
make skill-command-integrity
make cli-smoke
make coverage-gate BASE_REF=<merge-base>
```

Baseline updates are monotonic: they add newly supported contracts but retain
all historical commands and schema entries. Running an update therefore cannot
bless a removal. Commit baseline additions with the implementation for review.

For an intentional compatibility reset at a major-version boundary, run
`make reset-interface-baseline`. This replaces all CLI compatibility history
with the current command tree and must receive explicit human review.

The current open-source static-endpoint build returns an empty `products` array
from `dws schema list`, so its initial schema baseline contains zero products.
To protect schema entries exposed by an older dynamic or private distribution,
seed this baseline from that distribution's last supported `schema list` output.

## Required GitHub repository settings

Create a ruleset for `main` that requires pull requests and code-owner review,
then mark these status checks as required:

- `Lint`
- `Test`
- `Coverage`
- `Policy Check`
- `Edition Contract Tests`
- `Multi Profile E2E`
- `Interface Integrity`
- `CLI Smoke`
- `Mock MCP Smoke`
- `AI Behavior Check`

The `ai-generated` label must be applied by the PR-creation automation or by a
maintainer; GitHub cannot infer reliably whether a human-authored PR contains
AI-generated code.
