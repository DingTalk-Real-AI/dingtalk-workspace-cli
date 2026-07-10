# Pull request quality gates

The repository defines four focused checks in addition to its existing CI:

- **Interface Integrity** compares every public command, alias, and directly
  defined flag with `test/fixtures/cli-interface-baseline.txt`.
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

## Updating an intentional CLI interface change

Run:

```sh
make build
make update-interface-baseline
make interface-integrity
make cli-smoke
```

Commit the baseline diff with the implementation so reviewers can see the
exact additions, removals, aliases, and flag type changes.

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
