# Maintainer Automation Notes

This document keeps agent- and maintainer-specific workflow notes out of the
repository root while preserving repo-local guidance for automation.

## Read Order

1. `README.md`
2. `CONTRIBUTING.md`
3. `docs/architecture.md`
4. This document

## Project Snapshot

- `dws` is a Go-based DingTalk Workspace CLI and MCP runtime bridge.
- Product commands are loaded dynamically via `internal/plugin` from bundled descriptors.
- Command handlers live in `internal/helpers`; runtime execution flows through `internal/executor` and `internal/transport`.

## Repository Map

- `cmd`: public CLI entrypoint
- `internal/app`: root command wiring, static utility commands, plugin loading
- `internal/helpers`: product command handlers (dev, chat, calendar, contact, etc.)
- `internal/plugin`: plugin-based dynamic command loader
- `internal/cli`: catalog types and static endpoint loader
- `internal/executor`: invocation dispatch and result handling
- `internal/transport`: MCP HTTP client and request signing
- `internal/auth`: login, token management, agent-code detection
- `internal/audit`: user operation audit log
- `internal/errors`: structured error model with categories and hints
- `internal/keychain`: OS keychain integration for credential storage
- `internal/security`: endpoint allowlist and domain trust
- `internal/pat`: PAT (Personal Access Token) authorization flow
- `docs/`: public architecture and reference docs
- `scripts/`: build, test, lint, packaging, and policy checks
- `test/`: CLI, integration, contract, unit, and skill E2E test suites

## Task Routing

- Add or fix a command path: start from `internal/helpers` (handler implementations) or `internal/app` (command tree wiring)
- Protocol or transport issues: inspect `internal/transport`
- Auth or login issues: inspect `internal/auth`, `internal/pat`, `internal/keychain`
- Error message or category issues: inspect `internal/errors`
- Audit log issues: inspect `internal/audit`
- Plugin loading or command surface: inspect `internal/plugin`
- Failure or degraded mode: inspect `internal/errors`

## Policy Checks

When command surface or plugin descriptors change, run:

- `./scripts/policy/check-command-surface.sh --strict`
- `./scripts/policy/check-open-source-assets.sh`

## Common Commands

```bash
make build
make test
make lint
./scripts/dev/ci-local.sh
git diff --check
```

## Reviewer Router GitHub App

Reviewer requests and native auto-merge intentionally use different
identities. The base-owned `pull_request_target` workflow may use its built-in
`GITHUB_TOKEN` to request reviewers, but it must mint a dedicated GitHub App
installation token before enabling auto-merge. GitHub suppresses most workflow
events created by the built-in token; using it for auto-merge prevents the
merge commit's `push` workflows from running and leaves the exact-SHA Coverage
baseline without a trusted main-scoped producer.

Configure the dedicated App before merging a workflow revision that requires
it:

- install it only on `DingTalk-Real-AI/dingtalk-workspace-cli`;
- grant only `Contents: read and write` and `Pull requests: read and write`;
- set repository variable `REVIEWER_ROUTER_APP_CLIENT_ID` to its client ID;
- set `REVIEWER_ROUTER_APP_SLUG` to its exact lowercase slug;
- set repository secret `REVIEWER_ROUTER_APP_PRIVATE_KEY` to its private key;
- create one active repository branch ruleset named `main-merge-writers`,
  targeting only `refs/heads/main`, with exactly one `Restrict updates` rule
  (`update_allows_fetch_and_merge: false`);
- give that ruleset exactly two bypass actors: the Reviewer Router App as an
  `Integration` in `pull_request` mode, and `PeterGuy326` (ID `47820304`) in
  `always` mode for Formula publication and break-glass recovery;
- never give the App bypass on `main-protection`, `main-quality`, or any other
  ruleset, and never reuse `HOMEBREW_PR_TOKEN`,
  `RELEASE_GOVERNANCE_TOKEN`, or a personal token for Reviewer Router.

The workflow limits each minted token to the current repository, requests the
two permissions explicitly, and lets the token action revoke it at job end.
It also requires the minted App slug to equal the reviewed repository variable;
there is no `GITHUB_TOKEN` fallback. Before reading App credentials, the
base-owned workflow revalidates the event's exact base/head and uses its
built-in token only to disable an existing request owned by
`github-actions[bot]` or one whose title or merge metadata requests that GitHub
skip workflows. A mint or permission failure therefore leaves that PR
manual-merge only. The built-in token's `Contents: write` permission is
isolated to this trusted cleanup job and is never used to enable auto-merge;
review routing keeps `Contents: read`. Existing requests owned by a human or
another non-built-in identity are replaced with the exact dedicated-App
request after token minting. Only an already App-owned request with the fixed
headline/body is preserved. The required `Test` context reads the live
repository settings and applied rulesets, verifies the exact writer-rule
shape, and requires its own built-in Actions identity to report
`current_user_can_bypass: never`. Before enabling or reconciling auto-merge,
the minted App independently requires `pull_requests_only` on that writer rule
and `never` on every other active main ruleset. These identity-relative checks
remain available to low-privilege tokens; GitHub deliberately hides the full
`bypass_actors` list from callers without ruleset-write access. Operators must
therefore inspect that list during rollout and keep it at the exact two actors
above. The required `Test` context then briefly waits for the concurrent
router takeover and accepts only a null request or the configured App owner
with exact fixed metadata. A null request is safe for this failure mode because
the built-in Actions identity cannot pass the writer rule; other permitted
identities emit either a protected-main push or the trusted closed-PR repair.
Draft PRs skip this identity check; the explicit `ready_for_review` trigger
reruns admission when they become merge-eligible,
while `edited` and `auto_merge_enabled` rerun both workflows when the PR title
or merge request changes. A human `auto_merge_disabled` event reruns CI without
silently re-enabling the request, leaving it available only to the designated
break-glass identity. The required `Test` context rejects GitHub workflow-skip
directives in the PR title or an existing auto-merge request and verifies the
repository's reviewed `MERGE_MESSAGE` title plus `PR_TITLE` or `BLANK` body
defaults. The dedicated App binds the mutation to the exact head OID and
supplies a fixed safe headline and body, so GitHub cannot copy an unsafe PR
title into its merge commit.
After enabling, the workflow requires the owner to equal the token action's
exact `<app-slug>[bot]` output. If the event base/head changes during the
mutation window, it removes only that App-owned request and fails the run.
The break-glass publisher must preserve a safe final commit message;
`[skip ci]`, `[ci skip]`, `[no ci]`, `[skip actions]`,
`[actions skip]`, and a `skip-checks: true` trailer are forbidden outside the
release-controlled Formula-only path below.

GitHub may suppress `pull_request_target` entirely for security-sensitive head
branch names, including names that look like commit SHAs. Such a PR receives
neither App takeover nor the closed-event repair. Rename the head branch for
the normal path; if break-glass merge is unavoidable, preserve a safe final
message so the protected-main push CI remains the authoritative producer.

After installing the App, the protected-main push that deploys this workflow
runs reconciliation automatically. The job enumerates open, ready `main` PRs
with any non-App owner, unsafe App metadata, or workflow-skip metadata. It
revalidates each base/head, converges a safe request to the exact dedicated-App
owner and fixed message, and leaves a workflow-skipping request disabled for
manual correction. It never enables auto-merge where the request was already
null. A mid-migration failure leaves the affected PR disabled for a fresh
routing event or break-glass merge. One PR failure is recorded
without preventing later legacy owners from being attempted; the batch ends
red with a per-PR summary. Manually dispatch `Reviewer routing` from `main`
until the failed count is zero.

A PR that introduces or rotates this identity still runs the old base-owned
router. Install/configure the App and activate the exact writer ruleset first;
this blocks its legacy `github-actions[bot]` request from writing `main`. After
the governance PR's final push, disable that old request, confirm the live
settings/ruleset contract and all required checks are green for the exact head,
then have only `PeterGuy326` merge that head with the repository-generated safe
merge message. Verify the resulting merge SHA has a `CI` run with `event=push`,
a successful `Coverage` context, and an exact-SHA baseline cache under
`refs/heads/main`. Confirm automatic reconciliation reports zero failures and
zero non-App owners. Finally use a normal canary PR to verify that the dedicated
App is both `enabledBy` and `mergedBy`, and that the same post-merge chain
repeats before declaring the rollout complete.

## Homebrew Formula Delivery

Official releases use the designated `HOMEBREW_PR_TOKEN` identity to update
exactly one tracked Formula after the immutable GitHub assets and their
checksums have passed verification. The publisher validates the rendered Ruby,
commits only the configured Formula path, never force-pushes `main`, and retries
from a fresh clone up to three times when `main` advances concurrently. Normal
stable and beta releases do not create a Formula PR or run a permission
canary. The workflow uses the existing repository-scoped
`HOMEBREW_PR_TOKEN` release identity because GitHub does not allow its built-in
Actions App to bypass this repository's rulesets. Its owner is the designated
always-bypass actor for controlled Formula publication and break-glass recovery,
including on `main-merge-writers`. The workflow creates the
nine Code Admission checks for the Formula-only commit only after proving its
sole parent already has all nine successful checks and the committed Formula
exactly matches this release's verified bytes. Formula commits retain
`[skip ci]`, so the sealing step exposes only the reviewed commit identity to
an independent confirmation job. That job creates the
`Coverage Baseline Cache` acknowledgement and emits the reviewed
`coverage-baseline-promote` repository dispatch. The default-branch
`Coverage Baseline Promotion` workflow independently verifies the exact
single-parent Formula commit, both parent and target admission contexts, and
default-branch containment before checking out the target. It restores only
the exact parent profile, recomputes the complete profile if that cache is
absent, and saves the Formula SHA under the `main` cache scope. Because the
cache save action treats upload errors as warnings, a second lookup must report
`cache-hit=true` for the exact target key before the producer succeeds. The
promotion completes the unique acknowledgement, and the confirmation job
waits for that exact check-run ID. npm and mirror publication depend only on
the immutable release job, so a transient
cache-service failure cannot strand an otherwise valid release between
channels; the final release-delivery gate still fails until the exact cache is
confirmed. Once Formula sealing exposes the target SHA, the confirmation job
also runs when a later immutable-package recheck fails, so a post-push failure
cannot orphan the producer. Rerun the failed promotion/confirmation path after
repairing the producer. Never add a prefix `restore-keys` fallback to this path.

`Coverage Baseline Repair` is the independent safety net for every merged PR.
Its base-owned `pull_request_target: closed` job never checks out or executes PR
content: it binds the closed event to the current merged PR, verifies the exact
head/base/merge SHA and `main` containment, then emits only a
`coverage-baseline-repair` repository dispatch. Workflow-skip directives alone
do not suppress `pull_request_target`, subject to GitHub's separate
security-sensitive branch-name restriction described above. The low-trust
trigger is forbidden from writing the default-branch cache directly. Before
dispatching, it gives Actions event delivery one minute to expose a run from
the exact protected `.github/workflows/ci.yml` workflow and exits if that normal producer already
owns the SHA, avoiding a duplicate full-suite run. A successful CI producer
must hard-verify its exact cache key. If that run instead completes with any
non-success conclusion, a separate base-owned `workflow_run` dispatcher binds
the exact workflow ID/path, run ID/attempt, conclusion, repository, branch, and
head SHA before requesting repair. `workflow_run` also has read-only
default-branch cache access, so both dispatchers use the reviewed
`repository_dispatch` exception. The dispatched default-branch producer
revalidates the corresponding merged-PR or failed-CI identity before checkout,
restores only the exact target key, recomputes the complete profile on a miss,
and verifies `cache-hit=true` after saving. An hourly schedule refreshes the
event-time `main` SHA after direct break-glass pushes or cache eviction;
`workflow_dispatch` provides the same current-main repair on demand. The
dedicated App identity remains mandatory because events created by the built-in
`GITHUB_TOKEN` can suppress both the main push and the closed-PR event.

Keep `HOMEBREW_PR_TOKEN` repository-scoped with `Contents: write` and
`Pull requests: write` (the latter remains necessary for withdrawal rollback),
keep its owner as the designated ruleset bypass actor, and do not reuse
`RELEASE_GOVERNANCE_TOKEN`. The workflow and publisher provide the Formula-only
path restriction; GitHub rulesets do not infer that restriction from the token.

## Release Governance and Recovery

Store `RELEASE_GOVERNANCE_TOKEN` as a dedicated Actions secret with only
repository `Administration: read`. The immutable-releases REST endpoint is an
administration setting and cannot be read by the workflow's built-in
`GITHUB_TOKEN`. Both the default-branch governance preflight and the tag
contract use this same credential so a missing or expired identity is detected
before an irreversible tag is created.

Recovery is restricted to an existing annotated tag whose exact tag object,
commit, sealed metadata, original failed run/attempt, requester identity and
Release state all match; it then reuses the normal release jobs without a
second-person environment approval. A same-run “Re-run failed jobs” is even
lighter: the seal job may adopt an existing tag only when its complete
authority matches that run and its original attempt is not newer than the
current attempt. Do not put publication secrets in temporary branches or
create ad-hoc recovery workflows.

Cloud-sealed releases mirror to OSS only when the repository variable
`ENABLE_OSS_MIRROR` is exactly `true`. Leave the variable unset while no Bucket
is provisioned; GitHub, npm, and Homebrew delivery can then complete without
running the OSS step. Once enabled, missing credentials, an invalid Bucket, or
an upload failure remains fail-closed. The cloud tag immutably records the
decision as `OSS-Mirror: enabled|deferred`; publication and withdrawal consume
that sealed value instead of the variable's later state. Deferred releases
cannot use `repair_oss_version`; enabling OSS applies to later release tags
until an audited immutable repair marker is implemented.

If an immutable GitHub Release and npm package were delivered but an enabled
downstream China mirror failed, dispatch the normal `Release` workflow from the
protected default branch with exactly one of `repair_gitee_version` or
`repair_oss_version`. Channel repair accepts a fully successful exact release,
or a failed exact-tag run only when its latest attempt completed the release
contract, build, Apple signature, immutable GitHub publication, and npm
delivery checks for the exact tagged commit. OSS repair additionally requires
the tag's sealed policy to be `enabled`. It then downloads and re-verifies the
immutable assets before invoking only the selected mirror. For a failed
release, an OSS repair requires the OSS step itself to be the recorded failure.
A Gitee repair accepts either a failed Gitee job or a Gitee job that was
skipped behind that OSS failure; the latter is an explicit Gitee backfill and
does not claim that OSS has been repaired. Gitee repair requires `GITEE_TOKEN`,
`GITEE_USER`, and `GITEE_REPO`; OSS repair requires `OSS_ACCESS_KEY_ID`,
`OSS_ACCESS_KEY_SECRET`, `OSS_ENDPOINT`, and `OSS_BUCKET` (with optional
`OSS_PREFIX`) as Actions secrets. Missing credentials fail the selected repair
closed.

## Handoff Checklist

Before handoff, include:

1. Changed files and why
2. Verification commands run and outcomes
3. Known risks or follow-up work
