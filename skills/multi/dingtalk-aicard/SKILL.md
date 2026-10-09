---
name: dingtalk-aicard
description: >
  Create, edit, and validate DingTalk AI Card (A2UI) JSON files offline.
  Use for cards built from requirements or images, structural protocol errors,
  and named component or function contract lookup through native DWS commands.
  Send a card to the current user only when a preview is requested.
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
  cliHelp: "dws aicard --help"
  aicardVersion: "V0.8"
  protocolVersion: "1.0"
  catalogId: https://dingtalk.com/card/a2ui/catalogs/public/catalog.json
---

# DingTalk AI Cards V0.8

Create or edit an A2UI file from the user's content or image, look up only the contracts needed, and distinguish local validation from delivery and client rendering. This Skill uses the DingTalk V0.8 package based on A2UI 1.0: message `version` is `"v1.0"` and send API `protocolVersion` is `"1.0"`.

Follow the user's requested lookup, file, send or update directly; the references are not a mandatory sequence. Obey command and protocol requirements. Ask only when missing facts, interaction meaning or the target affect the outcome.

## Route by intent

Read only the relevant reference. Do not preload the complete protocol, every pattern, or the whole guide.

| Intent | First reference | Optional follow-up |
|---|---|---|
| Set up lookup and validation | [Execution environment](#execution-environment) | Installed command help |
| Create or redesign from content or images | [Visual focus and layout](references/design.md) | One relevant [pattern](#composition-patterns) or example |
| Query a known name | [Query a named contract](#query-a-named-contract) | The result's source pointer |
| Find an unknown name | [Component](references/index/components.md), [function](references/index/functions.md), or [Token](references/index/tokens.md) index | Query only the selected names |
| Construct or repair a card | [Construct and validate](#construct-and-validate) | Diagnostic pointer and named contract |
| Send, preview, or update | [Delivery boundary](#delivery-boundary) | Installed command help |

Use patterns for composition guidance and linked JSON for message shapes. Read a named example directly; use the [example index](references/protocol/examples/README.md) only to choose one.

## Composition patterns

| Content | Reference |
|---|---|
| Notifications and reminders | [Notification](references/patterns/notification.md) |
| Object facts and decisions | [Detail](references/patterns/detail.md) |
| Illustrated content and attachments | [Content](references/patterns/content.md) |
| Metrics and reports | [Report](references/patterns/report.md) |
| Editable inputs and submission | [Form](references/patterns/form.md) |
| Generation and execution updates | [Progress](references/patterns/progress.md) |

For editable inputs, also consult the form pattern for bindings and submission,
even when another pattern describes the card.
When replaying an execution example, read [Examples and replay](references/patterns/progress.md#examples-and-replay) for its batch boundaries, even if the JSON was selected directly.

## Execution environment

This edition uses native DWS commands, not Python. Confirm `dws aicard --help` exposes `explain`, `lint`, and `preview`; copying the Skill does not install commands. Lookup and lint are offline and need no Profile. Check each command's current help and structured envelope (`ok`, `outcome`, `data`, or `error.details`). If unavailable, references can guide a draft, but do not claim DWS validation.

## Query a named contract

```bash
dws aicard explain Tabs --format json
dws aicard explain Text Row Column --compact --format json
```

Query only needed names. Compact `$contractRef` and `definitions` are lookup metadata, not A2UI fields. Query without `--compact` for expanded fields; `source` locates omitted details.

## Construct and validate

Use `references/protocol/` as the contract. Write an ordered JSON message array.
Within one card, keep the same `surfaceId`; choose an ID suitable for that card
when creating it. A complete new card requires Surface creation, root data
initialization (even `{}` for static content), and a component with ID `root`.
The recommended explicit form is:

```json
[
  {"version":"v1.0","createSurface":{"surfaceId":"example-surface","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json"}},
  {"version":"v1.0","updateDataModel":{"surfaceId":"example-surface","path":"/","value":{}}},
  {"version":"v1.0","updateComponents":{"surfaceId":"example-surface","components":[{"id":"root","component":"Text","text":"Example"}]}}
]
```

This is a structural example, not a business card to send. Protocol-valid inline
initialization in `createSurface.dataModel` and `createSurface.components` is also
accepted; three separate messages are not the only valid representation.

| Scenario | Messages and checks |
|---|---|
| Complete new card | Create and initialize as above; use `--preflight new-card` |
| Host-created empty Surface | Initialize its existing ID without `createSurface`; use Schema and resource checks |
| Existing card update | Keep IDs and send only intentional changes; use Schema and resource checks, then check against known current state |

Removing `createSurface` from a complete file does not make it a safe delta.
Do not replay form defaults or unrelated initial data: that can overwrite user
input. Resource checks alone cannot prove an update preserves state or references.

Preserve unrelated styling when editing. For ordinary new cards, prefer a root `Column` without background fields. If a root `Card` is needed, use `backgroundColor: "#00FFFFFF"` and no background token. Honor explicit surface designs; see [design guidance](references/design.md#root-surface).

Prefer `Markdown` for rich text and static tables; `Text` for short titles and labels. Omit root `padding` for ordinary cards. Do not default to `CardHeader`; preserve it when requested and check [insets](references/design.md#headers-and-card-insets).

Preserve supplied facts and requested interactions; label demonstration data. Replace `REPLACE_*` placeholders before delivery. Do not present unverified resources, unconnected actions, or simulated states as real and working.

Choose literals or bindings as the field contract permits. Initialize bound data with the required types and update it through `updateDataModel`; change component literals through `updateComponents`. `sendDataModel` controls whether A2A messages include the full data model, not whether bindings work.

```bash
dws aicard lint --file card.a2ui.json --format json
```

Choose `--preflight new-card` or `--preflight resources` for the selected scenario, and use `--emit` only when serialized message strings are needed; it cannot be combined with `--fragment`. Check exit status and `data.valid` plus `data.preflight.valid` when selected. `dws aicard lint --self-check --format json` verifies the full package.

## Validation scope

Lint checks syntax and Schema fields. New-card preflight additionally checks initialization, root, duplicate IDs within a message, static references and cycles in the final component snapshot, not every intermediate streaming frame. Unreachable components are warnings. Resource preflight checks inline encoding; neither mode evaluates bindings or proves image loading. `--fragment` checks component/message fragments; it is not a way to bypass new-card initialization. Do not silently rewrite user data, remove requested components, or downgrade interactions to pass a check.

A successful named lookup is not a full package check. Use the self-check supported by the selected execution environment; local checks do not establish client behavior.

## Delivery boundary

With native `--emit`, read `data.a2uiMessages`.

Send only when requested. Resolve exactly one target under the sending profile: `--conversation-id` for a conversation or `--open-dingtalk-id` for a person. Names are not target IDs. Inspect installed command help, JSON-encode the emitted message-string array as one `--content` argument, and call `dws chat message send-a2ui-card`. Preserve the request, target, profile and returned `bizId`. An uncertain result must be checked before another create; do not resend automatically.

Creation starts in PROCESSING. Update with `dws chat message update-a2ui-card --biz-id <bizId> --content <message-strings> --flow-status <state>` under the original profile. Keep the same `surfaceId` and component IDs; send only intended `updateDataModel` or `updateComponents` changes, not another `createSurface` or unrelated defaults. Static cards require `--flow-status FINISH` with a nonempty valid delta; prepare it before sending and preserve business data. Stream complete messages, not JSON tokens. Resource preflight checks only the delta. If current state is known, validate it with the delta; Schema validity does not prove business safety.

`dws aicard preview --file card.a2ui.json` sends a self-preview in PROCESSING; `--dry-run` does not send. For a completed static preview, finish it under the same profile with `dws chat message update-a2ui-card --biz-id <data.bizId> --content <message-strings> --flow-status FINISH`, using a nonempty state-preserving delta for the same Surface. Do not replay form defaults. Use server-issued `data.bizId`, never request-side `bizCardId`. If absent, inspect `updateWarning` and the receipt; do not recreate automatically. `openTaskId` cannot be used with `query-send-status`.

After sending, use the available readback capability under the same profile and target to identify this exact card from returned instance/message identifiers. Generic card text and a nearby timestamp are not sufficient. Report request acceptance, same-target readback, client rendering and interaction separately. If readback is unavailable or inconclusive, mark it unverified; do not claim the user received or correctly rendered the card from API acceptance alone.

Report the file, checks actually run, and delivery result when requested. Local validation and request acceptance do not prove client rendering.
