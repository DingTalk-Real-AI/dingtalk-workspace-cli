# Progress: Agent execution and results

Use for generation, search, analysis, tool execution, and other ongoing processes such as uploads. An ordinary task detail does not need a reasoning panel; use the [detail pattern](detail.md).

For one status, a short Text or Markdown block, optionally in one panel, is enough. Group tools only when useful. Default to concise operation summaries; include necessary input/output details when the user requests diagnostics and the content is appropriate to disclose. For a known ratio, use `ProgressBar`; for stages or counts, use Text with an optional `Row + Tag`, without inventing a percentage.

## State and presentation

For an Agent reasoning or tool-execution flow, use this recommended pattern. Ordinary upload or task progress does not require reasoning styling or generation effects:

- The outer `CollapsiblePanel(variant=reasoning)` shows the current operation in its title, with the host's `[AIGenerating]` convention and `textEffect: shimmer` while running. On completion, show completion text and omit both the generating marker and `textEffect`.
- Group related tools in nested `indented` panels when useful. While running, set `icon` to the light loading GIF URL and `darkIcon` to the dark loading GIF URL. For the requested DingTalk named-icon completion style, replace `icon` with `"Check_L_outlined"` when the tool group succeeds and omit the loading `darkIcon`. Do not show a success check for failed or cancelled groups.
- Panel icons accept image URLs or registered icon names. Tool-row named icons use `Icon.name`.
- Tool rows use `Icon.name`, chosen from [IconName](../protocol/common-types-visual.json#/$defs/IconName) for the tool type, not image URLs. `SpinLoading_L_outlined` alone does not enable rotation. Keep tool icons separate from panel status images; the outer panel uses the generation marker rather than a loading icon.
- For the compact example style, use `common_level2_base_color` on panel titles and tool rows, and `common_action_text_style__font_size` on tool-row Text. These are styling defaults, not protocol requirements; preserve requested styling. Panel-title size is renderer-owned; do not invent a size field.
- Children may contain any contract-supported composition, including Markdown, File, charts, or forms. Content placement and reveal timing are task choices, not restrictions imposed by the execution state.

| State | Title and effects | Content |
|---|---|---|
| Running | Name the current operation; keep generation effects active | Show actual steps and any available partial result |
| Completed | Show completion; remove the generating marker, shimmer, and loading indicators | Show the final content or artifact |
| Failed or cancelled | Name the actual terminal state; stop generation effects | Preserve valid results; offer retry only through a real action |

Distinguish generated results from user confirmation. Completion describes the A2UI content state, not a delivery API status.

## Updates and interaction

- Start panels with reachable content so expansion reveals useful information. Set `defaultExpanded` only for the initial state; later updates must respect the user's local toggle state.
- `CollapsiblePanel.title` is a nonempty string and `textEffect` is a string enum, not a `{path: ...}` binding. Update them through `updateComponents`; update bound body content through `updateDataModel`.
- Keep the same `surfaceId` and existing component IDs. When adding a step, update its parent's `children` too. Supply the replacement component definition when clearing effects; do not send an unsupported empty `textEffect`.
- Shimmer, generation indicators, loading GIFs, and collapse interactions are supported across iOS, Android, HarmonyOS, Windows, macOS, and Web. Check the actual card's resource loading and interactions separately from protocol validation.

Initialize new bound fields with correctly typed values before using them. A text update through `updateDataModel` replaces the value at its path; it does not append characters. Do not replay initial values for existing editable fields.

## Examples and replay

The message positions below are one-based. Read one matching JSON, not both by default.

| Example | Batch 1 | Batch 2 | Batch 3 |
|---|---|---|---|
| [agent-run-progress.json](../protocol/examples/agent-run-progress.json) | Messages 1–3: initialize reading | Message 4: finish reading, start analysis | None; intentionally stays running |
| [agent-run.json](../protocol/examples/agent-run.json) | Messages 1–3: initialize reading | Message 4: finish reading, start analysis | Message 5: complete and reveal results |

The batches describe successive A2UI states on the same Surface; applying the entire array at once may hide intermediate states. Timing belongs to the consuming application, not the JSON. Keep the running-only example unfinished when demonstrating that state. For a requested complete lifecycle, include the actual completed, failed, or cancelled content state; do not infer success from the end of a message array.

The complete example reveals Markdown and File outside the panel in its final batch. Its File is a public sample PDF, not a generated report, and has no `onPreview` callback. Replace sample resources for real tasks and configure file interaction when required.
