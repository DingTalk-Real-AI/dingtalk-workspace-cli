# Detail: object facts and decisions

Use for an approval request, schedule item, task, order, or another object with facts and actions. Select only needed combinations; a title, status, and button are not mandatory on every card.

| Needed presentation | Optional components | When to choose them |
|---|---|---|
| Title and current status | `CardHeader(trailing → Tag)` or `Row > Text + Tag` | Native themed header or a custom compact header |
| Labels and values | `Column > Row > Text + Text` | A few object attributes; see [key-value rows](../design.md#key-value-rows) for long values |
| Fact and adjacent destination | `Row > Text + Link` | A role and "View role," or an attachment name and its details |
| Reason or supporting material | `Markdown`, `File`, optionally within `CollapsiblePanel` | Choose separately for prose, a real file, and collapsible detail |
| Adjacent text actions | `ButtonGroup` | Inline labels and actions; enable narrow-width wrapping if the contract permits |
| Action with submit validation | `Button > Text` | Use a standalone Button when `checks` are needed; arrange several Buttons in a Row or Column |

`CardHeader.trailing` is one component ID. `Button.child` references Text; the current renderer does not support wrapping arbitrary layouts such as Row in a Button. `ButtonGroup` items are not child component IDs and do not take `checks`. For a clickable illustrated block, see the container-action combination in [content](content.md).

Preserve recognizable date and timezone in schedules, and show old and new values together when relevant. Update decision and task state from a real business result; clicking alone does not mean success. Add only necessary inputs from the [form pattern](form.md) when the user must provide a comment or parameters.
