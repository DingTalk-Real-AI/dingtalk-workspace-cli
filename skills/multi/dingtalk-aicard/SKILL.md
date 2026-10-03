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

# Create and validate DingTalk AI Cards V0.8

Turn the user's content and interaction requirements into a maintainable A2UI file. Look up contracts in the bundled DingTalk protocol, check syntax and structure, and identify runtime behavior that remains unverified. Sending and rendering require a delivery tool and a target client.

The DingTalk specification version is **V0.8**, based on the official A2UI **1.0** reference. The message `version` remains `"v1.0"`, and the sending API's `protocolVersion` remains `"1.0"`; neither becomes V0.8.

## Determine content and interactions

Build the card when the available information is sufficient. Ask only when missing information affects factual content, the meaning of a required interaction, or delivery. A file-only request does not need a conversation target. Choose the layout, grouping, and a stable `surfaceId` as appropriate.

- Preserve requested components, information, and interactions; do not drop requirements to fit a pattern.
- Use provided or verified numbers, names, dates, and business URLs. Clearly label demonstration data in a prototype.
- Preserve buttons, tags, and other visual elements requested by the user or shown in a reference image. Do not invent URLs or event names when business behavior is unknown. A static `ButtonGroup` may omit each item's `action` where the protocol permits it; report that its interaction is not connected. Standalone `Button.action` is still required: do not rely on renderer tolerance to omit it. Do not add buttons without a visual or operational need.

## Surface and delivery scenarios

Choose the message boundary for the known delivery route. If the user specified creation or update, use that route directly:

| Scenario | Required messages |
|---|---|
| New card or complete snapshot loaded from an empty state | Send `createSurface`, root `updateDataModel`, then `updateComponents`; set `catalogId` explicitly |
| Host has created an empty Surface | Initialize with `updateDataModel` and `updateComponents`; do not create it again |
| Existing-card update | Reuse the original `surfaceId` and identify the card as required by the host API; send only changes, without replaying `createSurface` or unrelated form defaults |

For a new card, use `createSurface →` root `updateDataModel → updateComponents` and set `catalogId` explicitly. The current DingTalk creation and delivery API requires data initialization, even for a purely static card:

```json
{"version":"v1.0","createSurface":{"surfaceId":"same-as-following-messages","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json"}}
```

```json
{"version":"v1.0","updateDataModel":{"surfaceId":"same-as-create-message","path":"/","value":{}}}
```

Replace the empty object with real initial business data when needed. This is a DingTalk delivery requirement for complete creation, not a universal A2UI message-order rule. It does not require every string to be bound, `sendDataModel` to be enabled, or all updates to be sent at once. Preflight also accepts protocol-valid inline initialization in `createSurface.dataModel/components`; do not split such an existing file just to match the examples. Send only changed values in an incremental update.

When integrating with an existing host, determine whether its API creates the Surface. The four [protocol examples](references/protocol/examples/README.md) contain creation, data initialization, and component initialization for new cards. For a host-created empty Surface, remove `createSurface` and use the host's `surfaceId`. Removing `createSurface` from a complete file does not turn it into a safe incremental update; construct the actual delta.

## Query and validation environment

This edition uses native DWS commands for lookup, validation, and preview; it does not need Python or the standalone Skill's scripts. On first use or an unrecognized command, run `dws aicard --help` to confirm that the binary provides `explain`, `lint`, and `preview`. Copying Skill files does not install commands in an older binary.

`explain` and `lint` use the embedded protocol offline and need no Profile. Check `dws aicard explain --help` before a batch or compact query; query names individually if unsupported. Follow the current command help. Use `dws aicard explain` for lookup, not `dws aicard lint --explain`. Inspect the exit code, `ok`, and `outcome`: successful content is in `data`, failures in `error`, and structural diagnostics in `error.details`.

If a command is missing, use a DWS build that includes aicard. Preserve the actual error if embedded protocol loading fails and inspect the DWS installation. When temporarily unavailable, the indexes can still guide a draft; state that DWS validation was not run. Do not present Python or manual checks as a DWS validation result.

## Read on demand

Choose components from the content, then look up their fields. Use the host's default background unless the content needs a local treatment. Short content does not require a title, metric, or button. Group content as needed and implement explicit interaction requirements.

For image reconstruction, identify visible copy, controls, states, and media regions before querying the needed components. Use structured components for interface text and controls. Photography, posters, and complex illustrations may retain their original image regions. Use a real asset or reliable crop for a brand mark; do not imitate it with an emoji or character. Before delivery, compare key text, numbers, region order, and controls with the image. Do not infer business behavior from the image alone. See [image reconstruction](references/design.md#image-reconstruction).

Read only the relevant section of [design guidance](references/design.md) when layout, color, density, or progress-state advice is needed. Do not load the entire guide, all indexes, or the whole catalog by default.

For a known component, function, or Token name, query its contract directly. If the name is unknown, read only the relevant [component index](references/index/components.md), [function index](references/index/functions.md), or [Token index](references/index/tokens.md):

```bash
dws aicard explain Tabs --format json
dws aicard explain Text Row Column --compact --format json
```

Replace `Tabs` with the component, function, common type, or Token name. In a deduplicated batch result, `$contractRef` points to the corresponding `definitions` entry. These are lookup results, not A2UI fields. Use a normal query when fields need to be expanded directly. A query returns one definition's fields and example without loading the entire catalog. It includes essential nested structures and event-specific slots. If a function's arguments are missing, query that function by name rather than recursively loading every function.

Read one component composition pattern when it fits the content; consult only the needed parts of another pattern for mixed content. Compositions are optional blocks, not complete-card templates. The user's requirements and current protocol determine the final layout and fields.

If the card has editable inputs, also consult [form](references/patterns/form.md) for field titles, bindings, and submission behavior, even when another pattern describes the overall card.

| Current task | Reference |
|---|---|
| Short result, alert, or status reminder | [notification](references/patterns/notification.md) |
| Object facts and actions: approval, schedule, task, or order | [detail](references/patterns/detail.md) |
| Illustrated content, recommendations, media, or attachments | [content](references/patterns/content.md) |
| Metrics, records, charts, or analytical conclusions | [report](references/patterns/report.md) |
| Inputs, settings, ratings, or feedback submission | [form](references/patterns/form.md) |
| Agent generation, execution, and result updates | [progress](references/patterns/progress.md) |

Simple content can use known components directly. In the pattern notation, `+` combines items, `>` shows a container and its contents, and `/` lists alternatives; none is sendable JSON. Fill actual reference fields from the contract. Reconstruct an image from what is visible, without adding unsupported content or actions because a pattern suggests them. General visual guidance is in `references/design.md`.

The `source` field in a lookup result points to a protocol file and JSON Pointer. It also marks omitted deep structures or long descriptions; inspect that location rather than reloading the catalog. The [example index](references/protocol/examples/README.md) provides complete new-card form, host-action, agent-progress, and report scenarios. Read one that matches the task. Adapt its messages for a host-created Surface or existing-card update using the delivery scenarios above. Do not guess unread fields; confirm them in the protocol or a contract query.

## Construction constraints

- Use `references/protocol/` as the contract for A2UI messages, common types, and DingTalk components, expressions, operators, host actions, and visual Tokens. An identically named official component or function is not necessarily identical to this package's definition.
- Write an ordered JSON message array. A complete component tree has root `id` `root`; keep `surfaceId` stable within one card. Include `createSurface` according to the delivery scenario.
- Match a binding's initial-value type to its literal counterpart. For example, `DynamicNumber` uses a JSON number and `ChoicePicker.value` uses an array of strings. A dynamic list's `path` points to an array; templates may use relative paths and `@index`.
- Follow each function contract for placement, arguments, return type, and host `catalogId`. Select colors, font sizes, and icons from their Token definitions.
- Replace `REPLACE_*` markers in prototypes or existing files before delivery. Do not describe unconnected events, inaccessible resources, or unverified client behavior as implemented.

## Validate before delivery

Validate this file:

```bash
dws aicard lint --file card.a2ui.json --format json
```

Add `--preflight new-card` for a new card, or `--preflight resources` to check inline resources in a delta or host-created content. Confirm option support with `dws aicard lint --help`; an older binary cannot claim to have run preflight.

Read `data.valid` on success, plus `data.preflight.valid` and diagnostics when preflight is enabled. Warnings do not block. Structural or preflight errors cause a nonzero exit with details in `error.details`; `valid` still refers only to Schema checks. An environment or read failure is not a pass.

Add `--emit` when the sending layer needs an array of message strings. After structural validation, `data.a2uiMessages` contains individually serialized messages. JSON-encode the whole array for the sending parameter; do not hand-write shell escaping. `--fragment` and `--emit` are mutually exclusive. Run `dws aicard lint --self-check --format json` separately to check embedded protocol integrity.

Lint checks JSON syntax and the bundled A2UI protocol structure: Schema constraints on messages, components, functions, and DingTalk extensions, including field names, required fields, types, and enumerations. It does not merge data, execute functions, check reference closure or binding initial values, or grade design. `--fragment` accepts one component, a component array, or one message under the same structural rules as the default message-array input.

Generic lint accepts host-created content, so passing lint alone does not mean a new file can be delivered. Run the preflight appropriate to the delivery scenario; do not hide missing new-card initialization with `--fragment`. `new-card` merges the final component snapshot per Surface and checks the root, duplicate IDs within a message, existence of reachable static child references and template targets, and static cycles. Reusing an ID across update messages is valid. Unreachable components produce warnings only; retain them when they serve a purpose rather than deleting them to silence a warning. Template expansion, binding evaluation, intermediate frames, image decoding, and client support still require runtime verification. `resources` checks resource encoding without requiring a delta to be self-contained.

Use media URLs provided by the user or otherwise verified. Have a program read and encode inline media into the standard resource field, then run resource preflight; do not paste tool logs into Base64. Mark unknown schemes and client SVG support as unverified rather than inventing a protocol allowlist. Follow [image reconstruction](references/design.md#image-reconstruction) when choosing image assets.

Static copy may be literal. Bind values that must update and provide first-frame initial values. `sendDataModel` controls whether the model accompanies an A2A message; it is not a switch that enables bindings. If delivery fails, check the initialization sequence, current sending API, and server receipt. A successful repair of one card does not justify enabling that field or binding every string in every card.

**Fix structural errors and errors from the selected preflight.** Apply design suggestions according to the user and scenario; they are not lint gates. Confirm binding initial values, resource availability, and interaction behavior during authoring and runtime acceptance.

Use a diagnostic's `pointer` to locate and edit the original field, then rerun validation. Do not silently rewrite user data. If several attempts fail, recheck the relevant field contract and diagnostic rather than replacing a requested component or downgrading an interaction.

## Delivery and rendering boundaries

Deliver the file path, its purpose (new card, host-created initialization, or existing-card delta), validation results, and unresolved issues that affect use. `valid: true` means syntax and protocol structure passed. `renderingVerified: false` in the validation report means there is no evidence of client rendering.
The `metrics` object is reserved for output compatibility and is currently empty; do not infer measurements from it.

Local validation does not execute functions, invoke host actions, or simulate a client. Function evaluation and results, remote-resource availability, client-version support, actual layout, and button callbacks need runtime verification. If the user asks for client confirmation, use available delivery and preview capabilities in that environment. Request acceptance or conversation readback alone does not establish successful rendering or interaction.

If the user requests a preview sent to themselves, use `dws aicard preview --file card.a2ui.json`. It really sends a card and requires a complete new-card sequence. `--dry-run` checks the local sending boundary without resolving identity or sending. To send to another person or a group, follow the current `dws chat message send-a2ui-card` contract for target and parameters.

`preview` reports `success` only when the card creation request is explicitly accepted, even if its receipt contains `openTaskId`. Keep that ID in the receipt, but do not pass it to `dws chat message query-send-status`, which is for current-user message sends. `deliveryVerified` and `renderingVerified` remain `false`. Conversation readback is separate evidence; generic card text and a nearby timestamp do not uniquely identify this card. The default sending status is `PROCESSING`. Update or finish through `dws chat message update-a2ui-card`, locating the original card by `bizId` without replaying creation. Investigate an unknown result before retrying; a receipt is not client acceptance evidence.
