# Digital employee parameter migration

This is a proposal for maintainer review, recorded as four `pending` Schema
enum migrations. The prerequisite PR adds governance and declarations only;
it does not change business parameters or consume its own approvals.

| Command | Parameter | Before | After |
| --- | --- | --- | --- |
| `dws dingtalk-tag manage create` | `type` | `local_agent`, `open_code` | `local_agent`, `qwenwork` |
| `dws dingtalk-tag manage list` | `type` | `local_agent`, `open_code` | `local_agent`, `qwenwork` |
| `dws dingtalk-tag manage save-draft` | `type` | `local_agent`, `open_code` | `local_agent`, `qwenwork` |
| `dws dingtalk-tag manage set-visibility` | `visibility` | unrestricted string | `ALL`, `PARTIAL` |

The exact product/tool/parameter identities and enum values are recorded in
`scripts/policy/interface-migrations/approved-schema-parameter-migrations-v1.json`.
Only those enums are covered. Command paths, parameter names, requiredness,
defaults, property bindings, other parameters and safety remain independently
checked. The hidden `--main-program-type` and `--staff-ids` aliases are retained.
`connect --channel opencode` identifies a local adapter and is unaffected.

## User migration and compatibility

The business PR changes the cloud employee type to `qwenwork`; `local_agent`
keeps its meaning. This is a breaking input change, including for the hidden
type alias. Update `--type open_code` to `--type qwenwork` in create, list filters
and explicit draft updates. Return-value consumers must recognize `qwenwork`.
An omitted list type still means no filter; an omitted draft type still means
leave the type unchanged. Create continues to require an explicit type.

The new CLI rejects `open_code` locally even if a server accepts older clients.
Before distributing the business change, maintainers must independently verify
that the public service accepts the new type, continues to support older
clients as required, and returns compatible employee identities. This document
does not assert that any deployment has completed. No historical employee
data, UUIDs or cloud/local assignment is migrated by this CLI change.

The visibility update expresses these constraints consistently in Help,
Schema, execution and dry-run:

- `ALL` cannot include member or department targets.
- `PARTIAL` requires at least one nonempty member user ID or department ID.
- Member and department lists are full replacements; omitted dimensions clear.
- Invalid requests fail before the MCP call with validation / exit code 3,
  including in dry-run. Scripts that relied on a server-side rejection may see
  a different failure time and message.

## Approval and delivery order

Follow [the migration governance](cli-interface-flag-migrations.md). Merge the
prerequisite mechanism and `pending` declarations after maintainer review.
Only then can the business PR switch to the exact after enums and consume the
same records without changing their reasons or scope. Candidate declarations
cannot authorize themselves. The old base checker still compares the complete
business contracts; bootstrap validation uses an empty approval manifest and
only checks that pending declarations preserve the exact before state.

Validate new and old type values, `local_agent`, omitted optional types,
visibility positive/negative cases, dry-run and older-client service behavior
separately. Source-project results are not validation of the public candidate.

Before rolling back a business change, check both CLI and server return-value
consumers. Keep compatible server behavior available. Do not move historical
tags or baselines. Governance and receipts must remain while a consumer or a
historical GA still needs them; do not remove pending/consumed records merely
to silence compatibility findings.
