# CI — PR 合入门禁

The pull-request admission layer has exactly nine required external contexts:

| Required context | Contract |
|---|---|
| `Lint` | Stable PR revision/risk classification plus applicable formatting, `go vet`, and Actionlint |
| `Test` | Tier-selected race/unit/release-script tests plus representative cross-platform compilation |
| `Coverage` | Scope-matched overall non-regression and 100% changed-code coverage |
| `Policy` | Repository policy and the fail-closed CHANGELOG contract |
| `Edition` | Edition contract tests |
| `Interface Integrity` | CLI, Schema, Skill, and stable-release compatibility |
| `AI Behavior` | Base-owned policy for PRs labeled `ai-generated` |
| `CLI Smoke` | Offline help for every public top-level command |
| `Mock MCP` | HTTP and stdio MCP lifecycle smoke tests |

The workflow display name is `CI`. Parallel helper
jobs may implement `Test` and `Coverage`, but they are not ruleset contexts.
Do not require an aggregate alias or a downstream integration check in place of
the nine contracts above.

`AI Behavior` is evaluated by a `pull_request_target` workflow that never
checks out or executes PR code. It writes the exact `AI Behavior` status to the
current PR head. Its Files API read is bracketed by base/head revision checks,
so a synchronize race fails closed. The same workflow supplies a successful
`AI Behavior` check run on protected `main` pushes for release governance.

## Exact CHANGELOG-only fast path

A pull request qualifies only when GitHub reports exactly one changed file,
that file is an in-place modification of `CHANGELOG.md`, and the base and head
both retain it as a regular non-executable `100644` blob. Add, delete, rename,
symlink, executable-mode, and second-file changes do not qualify.

`Lint` classifies the Files API result only after verifying that the API's base
and head equal the event revision both before and after pagination. `Policy`
checks out GitHub's PR merge ref and verifies its parents:

```text
HEAD^1 = pull_request.base.sha
HEAD^2 = pull_request.head.sha
```

It then runs:

```sh
./scripts/policy/check-changelog-pr.sh \
  --fast-path "$PR_BASE_SHA" HEAD
```

The exact fast path remains limited to historic one-file maintenance. A
release-seal PR uses `--content-only`, which permits the generated
`CHANGELOG.md` change together with archival moves from `.changes/` to
`.changes/released/`; it receives the normal scoped admission instead of this
fast path. Ordinary PRs must not modify `CHANGELOG.md`; they add a standalone
release fragment instead. The validator and its policy dependencies in that merge tree are byte-for-byte the current base
versions. Validation targets the synthetic merge tree, not the feature-branch
tree, so a stale branch cannot supply an older validator or combine with newer
base notes into an invalid final CHANGELOG.

All nine admission contexts are still emitted and must succeed. Expensive
implementation helpers are skipped; the named contexts record that their code
surface is unaffected.

The protected `main` push keeps that fast path only when all of these
fail-closed conditions hold:

- the event is a non-forced update of the existing `refs/heads/main`;
- the event `after` SHA is the exact workflow SHA, and both event SHAs are
  complete, non-zero commit IDs;
- GitHub's comparison reports the previous main tip as the unique linear merge
  base, with no commits behind it;
- the complete resulting tree diff is exactly one in-place modification of
  `CHANGELOG.md`;
- the previous main tip already has successful GitHub Actions checks for all
  nine Code Admission contexts.

`Policy` then independently checks out the pushed revision and runs the same
`check-changelog-pr.sh --fast-path` contract from the event's `before` SHA to
its `after` SHA. If identity, ancestry, file scope, tree mode, CHANGELOG
content, or predecessor admission cannot be proved, classification falls back
to the complete main admission suite. A source change can therefore never
inherit the CHANGELOG-only result.

Any PR that touches `CHANGELOG.md` but also changes another file runs the same
content contract in `Policy` with `--content-only`. That mode accepts only
fragment archival moves (`.changes/<name>.md` to
`.changes/released/<version>/<name>.md`) alongside the changelog; source and
documentation changes are rejected. It still rejects invalid dates or
versions, missing bullets, placeholder `TODO`/`TBD`, unmanaged-section
changes, and unsafe tree modes.

## Risk tiers and downstream boundaries

`Lint` resolves the complete base/head diff before any helper is skipped.
Unknown or truncated input fails closed into the high-risk tier.

| Tier | Selection | Admission work |
|---|---|---|
| Documentation-only | Only prose/documentation assets; no executable, generated, workflow, packaging, or interface surface | Documentation and repository-asset validation; expensive code helpers skip while every required context still succeeds |
| Standard | Ordinary code change with a stable package graph | Race tests for changed Go packages and their reverse dependencies; candidate and merge-base coverage over the same impacted scope and `coverpkg`; representative Darwin/Windows compilation |
| High-risk / protected `main` | Workflow/policy, package add/remove/rename, generated Schema/registry, platform, auth/keychain, installer, packaging, release, transport, recovery, or an unprovable infrastructure classification | Complete race suite and full native macOS/Windows tests, plus every affected domain gate |

Domain helpers (`Edition`, `Interface Integrity`, `CLI Smoke`, and `Mock MCP`,
for example) execute their substantive suites when the diff can affect that
contract or when the high-risk tier is selected. Otherwise their stable named
contexts still report a successful, explicit unaffected result. Release-script
tests follow the same impact rule. This preserves the ruleset contract without
charging every developer for unrelated work.

Platform-sensitive changes additionally run native changed-code coverage.
Protected `main` always runs native tests; generic portable changes are held to
the Linux changed-code gate rather than being forced to manufacture
platform-only coverage.

Complete `Multi-profile E2E` is not a PR admission context. It belongs to the
`Main Integration — 主干集成` workflow and runs only after a push to `main` (or
an explicit manual dispatch). A failing downstream run remains a real
regression and must be repaired, but it must not be represented by a synthetic
successful PR check.

```mermaid
flowchart TB
  PR["Pull request"] --> ADMISSION["CI"]
  ADMISSION --> L["Lint"]
  ADMISSION --> T["Test"]
  ADMISSION --> C["Coverage"]
  ADMISSION --> P["Policy"]
  ADMISSION --> E["Edition"]
  ADMISSION --> I["Interface Integrity"]
  ADMISSION --> A["AI Behavior"]
  ADMISSION --> S["CLI Smoke"]
  ADMISSION --> M["Mock MCP"]
  ADMISSION --> MAIN["Protected main"]
  MAIN --> NATIVE["Full native platform matrix"]
  MAIN --> E2E["Multi-profile E2E"]
  MAIN --> RELEASE["Release delivery"]
```

## Review ownership and auto-merge

A base-owned `pull_request_target` workflow routes newly opened, updated,
reopened, or newly ready PRs targeting `main` to one eligible peer reviewer. It
does not check out or execute PR code, excludes both the author and the known
latest pusher, and balances the open requested-review load across the reviewed
maintainer pool. A current-head approval or change request is preserved; after
a new push, stale activity does not suppress a fresh request, and an
outstanding change requester is preferred for continuity.

The branch ruleset keeps one human approval and all nine strict required
contexts, and requires someone other than the latest pusher to approve after
the most recent head update. Repository auto-merge is enabled for ready PRs,
so a PR merges after that approval and the current revision's nine checks are
green. If `main` advances, strict checks rerun before merge. The reviewer
router is orchestration, not a quality context, and must not be added to the
ruleset.

## Running focused gates locally

Run the contracts relevant to the change. Ordinary contributors are not
expected to repeat every CI job locally:

```sh
make build
make policy
make interface-integrity BASE_REF=<merge-base> STABLE_REF=<stable-tag> CANDIDATE_REF=<candidate-sha>
make schema-compatibility BASE_REF=<merge-base> STABLE_REF=<stable-tag> CANDIDATE_REF=<candidate-sha>
make skill-command-integrity
make cli-smoke
make mock-mcp-smoke
go test -v -count=1 ./pkg/editiontest/...
```

CI 先解析并核对精确的 merge-base、最近可达且未撤回的 stable GA tag 和已提交的 candidate
SHA，再调用 `make authoritative-interface-integrity`。本地 `make interface-integrity`
与该 CI target 都只委托给同一个 modern authoritative wrapper，不存在第二个比较
入口。省略 `BASE_REF` 时本地 target 默认比较 `origin/main`，省略 `STABLE_REF` 时自动
选择该 base 可达且未撤回的最近 stable GA tag，省略 `CANDIDATE_REF` 时比较已提交的 `HEAD`。
需要逐字复现某次 CI 时，应显式传入该次运行记录的 merge-base、stable tag 和
candidate SHA。

`make update-interface-baseline` / `make reset-interface-baseline` 只维护
`test/fixtures/cli-interface-baseline.txt` 这一份非权威 CLI Smoke fixture。底层旧
`check-interface-baseline.sh` 不再作为本地或 CI 的兼容性审批入口，也不能用于批准
flag 迁移。

Schema compatibility 使用同一组 base、stable、candidate refs，以及 base-owned flag
与 command migration ledgers。merge-base-owned checker 分别规范化 merge-base 与
stable 的完整 Schema，并让 candidate 对两份历史 contract 独立执行检查；它只把已通过
Interface lifecycle 的 exact rename、command move 或 flag extraction 规范化到当前历史
副本，不会维护第二份 allowlist，也不会放宽其他 Schema 历史字段。

For a release-seal branch that archives rendered fragments:

```sh
base_ref=$(git merge-base HEAD origin/main)
./scripts/policy/check-changelog-pr.sh --content-only "$base_ref" HEAD
```

`make coverage-gate` is an enforcement step, not a profile generator. For a
standard PR, CI derives changed packages and their reverse-dependency test
closure, then generates candidate and merge-base profiles with the same test
scope and `coverpkg`. High-risk and protected-main runs use the complete
profiles. The complete candidate profile is produced by disjoint per-shard
helper jobs (`scripts/ci/test-packages.sh list-coverage`, kept serial with
`-p 1` inside each shard; `verify` proves the shard union equals the
full-suite scope exactly once) and concatenated in the aggregate job before
enforcement. The complete merge-base profile is restored from an exact-key
cache written by the last green `main` push of that same commit (key:
merge-base SHA plus resolved Go version); any miss falls back to recomputing
it in a merge-base worktree. The trusted `main` producer and PR consumer use
the same dedicated cache profile path because GitHub includes that path in the
cache version; the runtime-facing candidate and baseline filenames remain
separate. Near-miss reuse is forbidden — the caches carry no prefix restore
keys, because a neighbouring commit's profile would compare the candidate
against the wrong baseline. Supporting and (when
platform-selected) native profiles are generated before the aggregate
`Coverage` context evaluates them. The
aggregate and native gates require 100% coverage for changed executable Go
statements. Overall coverage remains an unrounded, zero-tolerance,
scope-matched merge-base non-regression check. Candidate and baseline profiles
are evaluated by the same block-deduplicating checker; supporting policy and
shortcut profiles contribute to changed-code coverage only. The checked-in
badge is presentation only and is never read as a gate input.

CLI 兼容检查只使用 modern Interface Snapshot 这一处权威比较 seam，并从 PR
merge-base 和最近的可达 stable release 生成权威快照。本治理机制合入后，
merge-base 拥有生成器、比较器和已审批迁移清单，因此 candidate 不能通过修改
helper、fixture 或在同一 PR 新增 self-approval 记录来放行 breaking change。首次
bootstrap 仍由 merge-base 已有的 modern helper 做无豁免比较，并只接受 candidate
提交中的规范空清单；完整边界见下方治理文档。

精确的两阶段 flag 迁移生命周期见
[CLI flag 兼容迁移治理](cli-interface-flag-migrations.md)。治理 PR 只能在
surface 未变化时新增 `pending`；后续产品 PR 达到审批的精确 surface 后，才能
消费 base-owned 记录并改为 `consumed`。在 main 与 stable 都达到 after 状态前
必须保留该回执，之后再由单独 PR 清理。机制只放行记录中的 legacy
visible-to-hidden，以及 canonical required 新增或提升；删除、type、scope、
shorthand、no-opt 和任何无关漂移仍然阻塞。Schema 可以新增；历史 product、
tool、parameter、mapping、positional execution、constraint 与 safety 语义继续
受保护。`alias_of` 只是一项由 `FlagSpec.Aliases` 产生的框架关系证据，不是 payload
等价证明；产品 PR 仍须证明 canonical 与 legacy 的最终运行 payload 等价并在 transport
前拒绝冲突输入。当前迁移清单为空，不授权 PR #904。

## Required GitHub repository settings

The `main` quality ruleset must enable strict required-status-check policy
(`strict_required_status_checks_policy=true`) so a PR is revalidated whenever
`main` advances. It must require these exact contexts and no legacy aliases:

- `Lint`
- `Test`
- `Coverage`
- `Policy`
- `Edition`
- `Interface Integrity`
- `AI Behavior`
- `CLI Smoke`
- `Mock MCP`

Do not require helper jobs, `Multi-profile E2E`, or an aggregate admission
alias. Update ruleset contexts only after the new names have appeared on the
protected branch, so a rename cannot silently remove enforcement or leave an
unproducible required context.

The branch ruleset also requires one approval after the latest push. Enable
repository auto-merge and automatic head-branch deletion; keep the base-owned
reviewer router outside the required-context list.
