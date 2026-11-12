---
name: dingtalk-aicard
description: >
  Create, edit, and validate DingTalk AI Card (A2UI) JSON files with native
  DWS commands. Use for cards built from requirements or images, structural
  protocol errors, named component or function contract lookup, and cloud
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

Design supplies visual principles; patterns give reusable content and interaction strategies; templates are complete scene examples. Read only the relevant reference. Do not preload the complete protocol, every pattern, or the whole guide.

| Intent | Start with | Optional follow-up |
|---|---|---|
| Set up lookup and validation | [Execution environment](#execution-environment) | Installed command help |
| Reuse a bundled template | Its [reuse boundary](references/templates/README.md) and selected JSON | Components outside the data model; pattern for changed behavior |
| Create or redesign from requirements | [Visual focus and layout](references/design.md) | One [pattern](#composition-patterns); a template only if compatible |
| Reconstruct an image | [Image reconstruction](references/design.md#image-reconstruction) | Selected component contracts |
| Query a known name | [Query a named contract](#query-a-named-contract) | The result's source pointer |
| Find an unknown name | [Component](references/index/components.md), [function](references/index/functions.md), or [Token](references/index/tokens.md) index | Query only the selected names |
| Construct or repair a card, or prepare an update | Current messages and [Construct and validate](#construct-and-validate) | Affected guidance, diagnostic pointer, and named contract |
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

Use native DWS commands. Check `dws aicard --help` for `explain`, `lint`, `preview`, and `template`; copying this Skill does not install them. Lookup and lint are offline and need no Profile. Check leaf help and JSON envelopes (`ok`, `outcome`, `data`, `error.details`). Missing commands permit a draft, never a DWS validation claim.

### Cloud templates

| Intent | Command |
|---|---|
| Save a validated card | `dws aicard template create --file <file>` |
| Read a known ID and version | `dws aicard template get --template-id <id> --version <version> --out <new-file>` |
| Revise a template | `dws aicard template update --template-id <id> --file <file>` |

Cloud calls need the intended Profile, organization, and environment, plus an explicit `--mcp-id` or `DWS_AICARD_TEMPLATE_MCP_ID`; there is no default. `get` needs a known ID and version, not keywords. To derive a template, get it, edit the file, and create a new one; `update` appends a version under the same ID. Report the returned ID and version; never retry an uncertain write. A read of an absent version may return the latest content, so strict use must match the approved digest.

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

Choose `--preflight new-card` or `--preflight resources` for the selected scenario. Use `--emit` only when serialized message strings are needed; it cannot be combined with `--fragment`. Check exit status, `data.valid`, and `data.preflight.valid` when selected. Fix errors at their diagnostic pointers and rerun the selected check. `dws aicard lint --self-check --format json` verifies the full package.

For group cards that @ members, Markdown content uses `<a atId="openDingTalkId">nickname</a>` for a member and `<a atId="all">everyone</a>` for everyone. Pass the declared ids with `--at-open-dingtalk-ids` alongside `--preflight`: lint then fails on ids declared but unused (`mention.declared_unused`) or used but undeclared (`mention.undeclared`). IM-style `<@id>` placeholders never notify in cards; explicit lint preflight reports `mention.im_placeholder`, excluding Markdown code examples. The reserved `all` stays in the Markdown body only; it is never listed. Lint has no `--at-all` option and does not enforce the send-time 100-declaration limit or everyone-tag pairing; run send dry-run for those checks. Static matching accepts literal Markdown and resolvable absolute data paths from the same batch; runtime expressions remain unverified. Markdown code spans, code blocks and escaped tag examples are ignored. Matching uses a double-quoted `atId` as the sole attribute.

## Validation scope

Lint checks syntax and Schema. New-card preflight checks initialization, root, IDs, final-snapshot static references, cycles, and ownership. Multiple parents, repeated child slots, shared template sources, and static/template overlap fail even with empty arrays; unreachable nodes warn. Use distinct IDs per placement. Bindings, dynamic expansion, intermediate frames, and rendering remain unverified. Resource preflight rejects literal `data:` resource fields (`resource.inline_data_unsupported`); Markdown/`fallbackMarkdown` markers, even code/comments, only warn (`resource.markdown_data_unverified`) for review. This does not parse Markdown/HTML, evaluate bindings, or fetch resources; a clean report proves neither absence of inline media nor loading. Prefer HTTPS [assets](references/design.md#media-resources). `--fragment` never bypasses initialization. Do not silently remove requested data, components, or interactions.

Use self-check for package integrity, not named lookup. Review facts, actions, visual completeness, and first-frame values. Verify loading, functions, rendering, interaction, and writeback on the client; report delivery evidence separately.

## Delivery boundary

Follow the installed dingtalk-chat A2UI guidance for creation, updates, summaries, annotations, and completion; check leaf help for `--summary`, `--support-forward`, and update flags. A missing capability blocks delivery.

Send only when requested. Resolve exactly one target under the sending Profile: `--conversation-id` for a conversation or `--open-dingtalk-id` for a person. Names are not target IDs. JSON-encode the emitted `data.a2uiMessages` as one `--content` argument for `dws chat message send-a2ui-card`, with a readable fallback `--summary` and a policy-based `--support-forward=true|false` (default true; pass false to disable forwarding). For a card instantiated from a business template, update only the data paths its business configuration authorizes.

A group card that @ members must keep the Markdown `<a atId>` tags and the send flags consistent: list each member id with `--at-open-dingtalk-ids` (sent as `atOpenDingTalkIds`, at most 100 declarations including duplicates) and add `--at-all` only when the body uses `<a atId="all">` (sent as `atAll`; the CLI rewrites the tag to the server highlight alias `<a atId="10">` on the wire). With explicit mention flags, a statically known mismatch fails with exit code 3. Valid Markdown bindings may receive their body after placeholder creation; the CLI cannot certify future values. Without mention flags, legacy bodies are preserved, including direct-chat markup. Single-chat cards reject explicit @ flags. The server fails closed: an invalid or unresolvable declared id, over-limit declarations, @ in a single chat, or an alias conflict rejects card creation with INVALID_PARAM and a nonzero exit; body tags that were never declared get no highlight or notification.

Creation starts in PROCESSING. Update with `dws chat message update-a2ui-card --biz-id <bizId> --content <message-strings> --flow-status <state>` under the original profile. Keep the same `surfaceId` and component IDs; send only intended `updateDataModel` or `updateComponents` changes, not another `createSurface` or unrelated defaults. Static cards require `--flow-status FINISH` with a nonempty valid delta; prepare it before sending and preserve business data. Stream complete messages, not JSON tokens. Resource preflight checks only the delta. If current state is known, validate it with the delta; Schema validity does not prove business safety.

`dws aicard preview --file a2ui.json` sends a self-preview in PROCESSING; `--dry-run` does not send. Finish a static preview with `dws chat message update-a2ui-card --flow-status FINISH`, the server-issued `data.bizId` (never request-side `bizCardId`), and `--a2ui-annotations` marking its main content component as `artifact`; otherwise the conversation summary becomes the server default. If it is absent, inspect `updateWarning` and the receipt; do not recreate automatically. `openTaskId` cannot be used with `query-send-status`.

After sending, identify this exact card from returned instance or message identifiers under the same Profile and target; generic card text and a nearby timestamp are not sufficient. Report request acceptance, readback, asynchronous summary, client rendering, and interaction separately; missing evidence stays unverified.

Report the file, checks actually run, and delivery result when requested. Local validation and request acceptance do not prove client rendering.
