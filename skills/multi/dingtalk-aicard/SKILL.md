---
name: dingtalk-aicard
description: >
  Create, edit, and validate DingTalk AI Card (A2UI) JSON files with native
  DWS commands. Use for cards built from requirements or images, structural
  protocol errors, charts, component or function lookup, and cloud
  templates created, retrieved, or revised by template ID and version.
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

Create or edit A2UI from user content or images; query needed contracts. Distinguish local validation, delivery, and rendering. DingTalk V0.8 uses A2UI 1.0: message `version` is `"v1.0"`; send API `protocolVersion` is `"1.0"`.

Follow the requested lookup, file, send, or update; references are not a mandatory sequence. Obey command and protocol requirements. Ask only if missing facts, interaction meaning, or target affect the outcome.

## Route by intent

Read only the relevant references: design defines visual hierarchy, patterns define composition and behavior, and templates provide reusable card frames. Do not preload the full protocol or every pattern.

| Intent | Start with | Optional follow-up |
|---|---|---|
| Set up lookup and validation | [Execution environment](#execution-environment) | Installed command help |
| Reuse a bundled template | Its [reuse boundary](references/templates/README.md) and selected JSON | Components outside the data model; pattern for changed behavior |
| Create or redesign from requirements | [Visual focus and layout](references/design.md) | One [pattern](#composition-patterns); a template only if compatible |
| Reconstruct an image | [Image reconstruction](references/design.md#image-reconstruction) | Selected component contracts |
| Query a known name | [Query a named contract](#query-a-named-contract) | The result's source pointer |
| Find an unknown name | [Component](references/index/components.md), [function](references/index/functions.md), or [Token](references/index/tokens.md) index | Query only the selected names |
| Construct or repair a card, or prepare an update | Current messages and [Construct and validate](#construct-and-validate) | Affected guidance, diagnostic pointer, and named contract |
| Create a new single-chart card | [Native chart preset workflow](references/dws/chart.md) | Named Chart contract and selected data family |
| Send, preview, or deliver an update | [Delivery boundary](#delivery-boundary) | Installed command help |

Use patterns for composition guidance and linked JSON for message shapes. Read a named capability example directly; use the [example index](references/protocol/examples/README.md) only to choose one. Template reuse must match structure, collection sizes, and interactions.

## Composition patterns

| Content | Reference |
|---|---|
| Notifications and reminders | [Short notifications](references/patterns/detail.md#short-notifications) |
| Object facts and decisions | [Detail](references/patterns/detail.md) |
| Illustrated content and attachments | [Content](references/patterns/content.md) |
| Metrics and reports | [Report](references/patterns/report.md) |
| Editable inputs and submission | [Form](references/patterns/form.md) |
| Generation and execution updates | [Progress](references/patterns/progress.md) |

For editable inputs, also consult the form pattern for bindings and submission,
even when another pattern describes the card.
When replaying an execution example, read [Examples and replay](references/patterns/progress.md#examples-and-replay) for its batch boundaries, even if the JSON was selected directly.

## Execution environment

Use native DWS commands. Check `dws aicard --help` for `explain`, `lint`, `chart`, `preview`, and `template`; copying this Skill does not install them. Lookup, lint, and chart build are offline and need no Profile. Check leaf help and JSON envelopes (`ok`, `outcome`, `data`, `error.details`). Never invent flags, even in drafts. Missing commands permit a draft, never a DWS validation claim.

### Cloud templates

Use only documented flags or flags confirmed by installed help, including in command plans. Check `dws --help` and the selected leaf's help (for example, `dws aicard template create --help`) before execution. The global `--profile` selects the account/organization context; the explicit MCP service must belong to the intended environment. Verify those mappings instead of turning context words into CLI flags. This contract has no template `--env`, `--environment`, `--org`, or `--organization` flags.

If help is unavailable, the documented command forms below may be planned without claiming execution or verification. Keep unresolved Profile/environment mappings and any undocumented options as prose prerequisites. Do not append guessed options to a runnable-looking command, even when labeled provisional; do not silently drop the requested context or use a default service.

| Intent | Command |
|---|---|
| Save a validated card | `dws --profile <profile> aicard template create --mcp-id <service-id> --file <file>` |
| Read a known ID and version | `dws --profile <profile> aicard template get --mcp-id <service-id> --template-id <id> --version <version> --out <new-file>` |
| Revise a template | `dws --profile <profile> aicard template update --mcp-id <service-id> --template-id <id> --file <file>` |

For `get`, the output path must not exist; write planning notes elsewhere, not to a placeholder file at `--out`.

Cloud calls need the intended Profile, organization, and environment, plus an explicit `--mcp-id` or `DWS_AICARD_TEMPLATE_MCP_ID`; there is no default. `get` needs a known ID and version, not keywords. To derive a template, get it, edit the file, and create a new one; `update` appends a version under the same ID. For writes, report the returned ID and version; never retry an uncertain write. A read reports the requested version with `versionConfirmed:false`, not a confirmed actual version. An absent version may return the latest content, so strict use must match the approved digest.

Templates may intentionally defer Chart data to business-time injection. Preview and template save preserve that content with Chart warnings/unverified results; do not fabricate data or require a complete generated-chart result before saving such an asset. Explicit new-card lint remains strict for ready-to-send snapshots. Unverified template content is not chart validation evidence.

Use approved business bindings and data paths; do not substitute local samples. Business configuration owns activation and rollback. Send complete new cards or authorized instance deltas; delivery never modifies template assets.

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

Preserve unrelated styling when editing. When reusing a template, keep its frame and insets; the new-card defaults below do not override template styling or explicit surface designs. For ordinary new cards, prefer a root `Column` without `backgroundColor` or `backgroundColorToken`. If a root `Card` is needed, use `backgroundColor: "#00FFFFFF"` and omit `backgroundColorToken`: omitting both fields gives Card an opaque default, and a configured background Token takes precedence over the color. See [design guidance](references/design.md#root-surface).

Prefer `Markdown` for rich text and static tables; use `Text` for short titles and labels. See [report composition](references/patterns/report.md) for table selection and pagination. Omit root `padding` for ordinary new cards; omission is not equivalent to explicit `0`. Do not default to `CardHeader`; preserve it when requested and check [insets](references/design.md#headers-and-card-insets). These are authoring defaults, not Schema restrictions.

Preserve supplied facts and requested interactions; label demonstration data. Replace `REPLACE_*` placeholders before delivery. Do not present unverified resources, unconnected actions, or simulated states as real and working.

Choose literals or bindings as the field contract permits. Initialize bound data with the required types and update it through `updateDataModel`; change component literals and layout through `updateComponents`. `sendDataModel` controls whether A2A messages include the full data model, not whether bindings work.

```bash
dws aicard lint --file a2ui.json --format json
```

Choose `--preflight new-card` or `--preflight resources` for the selected scenario. Use `--emit` only when serialized message strings are needed; it cannot be combined with `--fragment`. Check exit status, `data.valid`, and `data.preflight.valid` when selected. For complete charts, `data.preflight.chartValidation` reports checked and unverified content; bare lint remains Schema-only. Fix errors at their diagnostic pointers and rerun the selected check. `dws aicard lint --self-check --format json` verifies the full package.

For group-card @ semantics and preflight flags, see [Mentions](#mentions) below.

## Validation scope

Lint checks syntax and Schema. New-card preflight checks initialization, root, IDs, final-snapshot static references, cycles, and ownership. Multiple parents, repeated child slots, shared template sources, and static/template overlap fail even with empty arrays; unreachable nodes warn. Use distinct IDs per placement. Complete-card Chart JSON Pointer bindings and known series shapes are checked; legacy Chart options remain explicitly unverified. Other bindings, dynamic expansion, intermediate frames, and rendering remain unverified. Resource preflight rejects literal `data:` resource fields (`resource.inline_data_unsupported`); Markdown/`fallbackMarkdown` markers, even code/comments, only warn (`resource.markdown_data_unverified`) for review. This does not parse Markdown/HTML, evaluate bindings, or fetch resources; a clean report proves neither absence of inline media nor loading. Prefer HTTPS [assets](references/design.md#media-resources). `--fragment` never bypasses initialization. Do not silently remove requested data, components, or interactions.

Use self-check for package integrity, not named lookup. Review facts, actions, visual completeness, and first-frame values. Verify loading, functions, rendering, interaction, and writeback on the client; report delivery evidence separately.

## Natural-language chart requests

Use the [native chart preset workflow](references/dws/chart.md) to select a preset, build a new single-chart card, and validate the output. For an unsupported combination, use ordinary A2UI authoring with native lint.

Use ordinary construction and updates for existing cards, templates, and mixed reports. Preserve their layout, interactions, IDs, and bindings.

## Delivery boundary

Follow the installed dingtalk-chat A2UI guidance for creation, updates, summaries, annotations, and completion; check leaf help for `--summary`, `--support-forward`, and update flags. A missing capability blocks delivery.

Send only when requested through the installed chat Skill. Serialize emitted `data.a2uiMessages` as a JSON array in a UTF-8 file and pass it through stdin with `--content -`; avoid inline payload arguments, including on Windows. Use the generated readable fallback as `--summary`. For business-template instances, update only authorized data paths.

### Mentions

Preflight rejects IM-style `<@id>` placeholders even when `--at-open-dingtalk-ids` is omitted.

For group cards that @ members, Markdown uses `<a atId="openDingTalkId">nickname</a>` for a member and `<a atId="all">everyone</a>` for everyone. The server adds the visible @ prefix, so do not write another one in the tag content. IM-style `<@id>` placeholders are literal text in a card and never notify. The reserved `all` is not a member ID.

Pass each member ID to DWS lint using `--at-open-dingtalk-ids` together with `--preflight new-card` or `--preflight resources`. It reports declared but unused IDs (`mention.declared_unused`), used but undeclared IDs (`mention.undeclared`), and IM placeholders (`mention.im_placeholder`). Static matching accepts literal Markdown and resolvable absolute data paths in the same batch. Runtime expressions and later data updates remain unverified. Code spans, code blocks, and escaped tag examples are ignored. The recognized tag has a double-quoted `atId` as its sole attribute. Lint has no `--at-all` flag and does not enforce the send-time 100-declaration limit or everyone-tag pairing; use send dry-run for those checks.

For a group send, list member IDs with `--at-open-dingtalk-ids` (transmitted as `atOpenDingTalkIds`, maximum 100 declarations including duplicates). Add `--at-all` only when the body contains `<a atId="all">`; DWS rewrites the tag to the server's highlight alias `<a atId="10">` on the wire and sends `atAll=true`. With explicit mention flags, known mismatches fail before sending. Without flags, legacy bodies are preserved. Explicit @ flags are invalid in single chat. Invalid or unresolvable IDs, over-limit declarations, single-chat @, or alias conflicts fail at creation; undeclared tags have no highlight or notification. Preserve the exact server failure rather than retrying with changed recipient semantics.

Pass explicit forwarding policy unchanged. Forms and progress cards follow their business lifecycle; the chart-specific `/summary` FINISH delta does not apply to other cards.

Creation starts in PROCESSING. Reuse the returned `bizId`, original Profile, `surfaceId`, and component IDs. Static cards require `--flow-status FINISH` with a prepared nonempty valid delta; send only intended changes, never initialization or unrelated defaults. Stream complete messages. Resource preflight checks only the delta; validate against known current state when available.

`dws aicard preview --file a2ui.json` sends a self-preview in PROCESSING; `--dry-run` does not send. Finish a static preview with `dws chat message update-a2ui-card --flow-status FINISH`, the server-issued `data.bizId` (never request-side `bizCardId`), and `--a2ui-annotations` marking its main content component as `artifact`; otherwise the conversation summary becomes the server default. If it is absent, inspect `updateWarning` and the receipt; do not recreate automatically. `openTaskId` cannot be used with `query-send-status`.

After sending, identify this exact card from returned instance or message identifiers under the same Profile and target; generic card text and a nearby timestamp are not sufficient. Report request acceptance, readback, asynchronous summary, client rendering, and interaction separately; missing evidence stays unverified.

Report the file, checks actually run, and delivery result when requested. Local validation and request acceptance do not prove client rendering.
