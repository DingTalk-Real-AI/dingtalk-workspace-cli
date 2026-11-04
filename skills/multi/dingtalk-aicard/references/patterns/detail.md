# Detail: object facts and decisions

Use for an approval request, schedule item, task, order, or another object with facts and actions. Select only needed combinations; a title, status, and button are not mandatory on every card.

| Needed presentation | Optional components | When to choose them |
|---|---|---|
| Title and current status | `Text`, or `Row > Text + Tag` | Add a Tag only for an actual status; for CardHeader see [header layouts](../design.md#headers-and-card-insets) |
| Labels and values | `Column > Row > Text + Text` | A few object attributes; see [key-value rows](../design.md#key-value-rows) for long values |
| Fact and adjacent destination | `Row > Text + Link` | A role and "View role," or an attachment name and its details |
| Reason or supporting material | `Markdown`, `File`, optionally within `CollapsiblePanel` | Choose separately for prose, a real file, and collapsible detail |
| Adjacent text actions | `ButtonGroup` | Inline labels and actions; enable narrow-width wrapping if the contract permits |
| Action with submit validation | `Button > Text` | Use a standalone Button when `checks` are needed; arrange several Buttons in a Row or Column |

`CardHeader.trailing` is one component ID. `Button.child` references Text; the current renderer does not support wrapping arbitrary layouts such as Row in a Button. `ButtonGroup` items are not child component IDs and do not take `checks`. For a clickable illustrated block, see the container-action combination in [content](content.md).

Preserve recognizable date and timezone in schedules, and show old and new values together when relevant. Update decision and task state from a real business result; clicking alone does not mean success. Add only necessary inputs from the [form pattern](form.md) when the user must provide a comment or parameters.

## Short notifications

Use for a short result, risk alert, or status change. Choose by the event semantics, reading behavior, and expected action, not by words such as "notification" or "reminder" alone. A decision, investigation, report, or data-entry request may need a different composition.

Make the event and its consequence easy to scan. Group the cause and related observations together, then place the next step after the explanation. Keep source and status secondary unless the status itself is the focal fact. A short notice can still need these distinct semantic groups; do not flatten them into one paragraph or give every sentence an equally prominent colored box.

| Needed presentation | Optional components | When to choose them |
|---|---|---|
| Compact reminder | `Row > Icon + Text + Link`, then `Text` or `Markdown` | Title, source, and detail destination in one row; add icon and link only if useful |
| Title and explanation | `Column > Text + Markdown` | Add status or actions only when needed |
| Result or exception | `Row > Tag + Text`, then explanation | A distinct status and body; a very short result may use text alone |
| Capacity or quota | `Text + ProgressBar + Text` | A real computable ratio; show a number directly if only balance or remaining amount is known |

`Tag` supplies status styling without hand-built colors. For a requested `CardHeader`, use the [header and inset guidance](../design.md#headers-and-card-insets); use [image reconstruction](../design.md#image-reconstruction) for a branded title icon.

- Keep the event or issue title as the focal fact. Do not replace it with a generic header or duplicate the sole title in a separate identity strip.
- For appearance-only reproduction, `ButtonGroup.buttons[]` items may omit `action` and keep their noninteractive appearance. Standalone `Button.action` remains required by the protocol.

Use [information roles](../design.md#2-information-roles-and-blocks), [hierarchy and density](../design.md#5-hierarchy-and-density), and [target-width guidance](../design.md#3-host-behavior-and-layout) for grouping, emphasis, and wrapping.
