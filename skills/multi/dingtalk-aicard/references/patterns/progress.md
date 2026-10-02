# Progress: Agent execution and results

Use for generation, search, analysis, tool execution, and other ongoing processes. An ordinary task detail does not need a reasoning panel; use the [detail pattern](detail.md).

| Process need | Optional components | When to choose them |
|---|---|---|
| Collapsible execution | `CollapsiblePanel(variant=reasoning) > Markdown/Column` | Process title and steps; choose expanded state from the content |
| Current generation hint | `Markdown` or `Text(textEffect=shimmer)` | Markdown may use the host's `[AIGenerating]` convention; choose either treatment as appropriate |
| Stage or counts | `Row > Text + Tag`, or several numeric groups | Show known stage, number searched, or number analyzed |
| Known progress ratio | `Text + ProgressBar` | Use a real 0–100 `value`; do not infer a percentage from a stage alone |
| Final result | `Markdown`, `File`, data, or actions | Choose by result type; the full process need not remain visible |

## Group execution and result

For a demonstration task that summarizes data, steps can be grouped as below. This is structural notation, not sendable JSON.

```text
Column
├─ CollapsiblePanel (variant=reasoning, execution)
│  └─ Column
│     ├─ Text (demonstration data read)
│     └─ CollapsiblePanel (variant=indented, summary details)
│        └─ Row
│           ├─ Icon (registered tool or status icon)
│           └─ Text (current step summary)
└─ Markdown (final result, outside the process panel)
```

For one status, one panel containing Markdown or Text is sufficient. Add a nested panel only when several tool records benefit from grouping. Show a short operation summary, not raw tool inputs and outputs.

| Stage | Process area | Result area |
|---|---|---|
| Running | "Summarizing demonstration data"; append actual steps and show generation effect if useful | May be hidden or show an available partial result |
| Completed | "Summary complete"; update step copy and remove the generation effect | Show final content or artifact |
| Failed or cancelled | Show the actual terminal state and necessary explanation; stop the generation effect | Retain still-valid results; offer retry only when a real action exists |

## Incremental updates and display boundary

- `CollapsiblePanel.title` is a nonempty string and `textEffect` is a string enum; neither accepts `{path: ...}`. Change the title by updating that component with `updateComponents`. Bound body text may be changed through `updateDataModel`.
- `defaultExpanded` controls only the initial state; do not use it to override a user's later expansion or collapse. `shimmer` depends on client support. Use the `[AIGenerating]` host convention described in [global design guidance](../design.md#global-design-guidance) only in appropriate text, not every field.
- Reuse the original `surfaceId` and component IDs in deltas. Distinguish "result generated" from "user confirmed." For message shapes, appended steps, and removing the effect on completion, see the [agent progress example](../protocol/examples/agent-progress.json).
