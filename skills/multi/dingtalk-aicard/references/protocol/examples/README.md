---
status: authoritative
updated: 2026-09-21
scope: Four task-oriented message examples of DingTalk AICard Protocol V0.8
---

# DingTalk AICard Protocol V0.8 scenario examples

Read one matching scenario when composing bindings, actions or updates. For a single field, read its component or function definition instead. These are adaptable examples, not required layouts.

Each JSON file is a complete new-card message array: createSurface with the public catalogId, root updateDataModel, then updateComponents and any scenario updates. For a host-created empty Surface, omit createSurface; for an existing-card update, construct only the actual changes instead of replaying initial data and components. Keep each surfaceId consistent. See [Surface and delivery scenarios](../../../SKILL.md) for delivery boundaries. FULL_DATA / DELTA wrappers are not part of these arrays.

| Example | Demonstrates | Relevant shards |
|---|---|---|
| [form-interaction.json](form-interaction.json) | Persistent field titles, typed initial values, local input state, upload events and submit-button checks | [catalog-components-common.json](../catalog-components-common.json), [catalog-components-inputs.json](../catalog-components-inputs.json), [catalog-functions-core.json](../catalog-functions-core.json) |
| [host-action.json](host-action.json) | Explicit host catalog, promptText result writeback and item-owned button metadata | [catalog-components-common.json](../catalog-components-common.json), [catalog-components-composition.json](../catalog-components-composition.json), [catalog-functions-host-actions.json](../catalog-functions-host-actions.json) |
| [agent-progress.json](agent-progress.json) | Nested collapsed execution panels, incremental steps and a completed state | [catalog-components-common.json](../catalog-components-common.json), [catalog-components-composition.json](../catalog-components-composition.json) |
| [data-report.json](data-report.json) | Tabs, local table pagination, chart data, relative Loop bindings and Stack references | [catalog-components-common.json](../catalog-components-common.json), [catalog-components-composition.json](../catalog-components-composition.json), [catalog-components-specialized.json](../catalog-components-specialized.json) |

Shard names describe reading dependencies, not runtime catalog IDs. Follow field $refs into the common-type files as needed. Do not load this whole directory into every prompt.

## Interaction notes

- In the form, single selection still binds a string array. `required(false)` passes: the agreement check therefore reads the boolean binding directly. ImageUpload reports only after a successful upload; canceled or failed uploads do not report an event.
- Host actions explicitly use `urn:dingtalk:a2ui:host:v1`. promptText takes `initialValue`, then writes `{status: "success", data: {text: "..."}}` to the declared resultPath. Before a call, result slots are empty; a missing-initial-value warning at these asynchronous output bindings is expected. Cancellation preserves the previous result; a failed result has no data.text. The static hint and bounded result text provide an empty state. ButtonGroup items own their metadata and never inherit a parent writeback path.
- The progress example uses an outer reasoning panel and a nested tool panel. Titles and shimmer are literals supported by the current public contract; subsequent updateComponents messages replace those definitions. Adding a step also updates its parent children. The final update removes shimmer. Current shimmer animation is iOS-only; other clients show plain text.
- The table receives its complete dataset once; pageSize 2 with three rows demonstrates local pagination. Loop item bindings are relative to each row. The chart Stack has both a base child and a referenced corner label.

All examples are checked structurally and semantically. Host callbacks and device rendering are not executed by static validation. No user or tool activity is performed by reading these files.
