# Choose an example

Read only the matching JSON. Adapt its content and layout to the task.

| Need | JSON example | Guidance |
|---|---|---|
| Form bindings, upload events, and submit checks | [form-interaction.json](form-interaction.json) | [Form](../../patterns/form.md) |
| Host dialog and result writeback | [host-action.json](host-action.json) | [Form](../../patterns/form.md#example-interaction-notes) |
| Running state with generation effects and tool icons | [agent-run-progress.json](agent-run-progress.json) | [Progress](../../patterns/progress.md) |
| Execution updates through completion | [agent-run.json](agent-run.json) | [Progress](../../patterns/progress.md) |
| Tabs, table pagination, charts, Loop, and Stack | [data-report.json](data-report.json) | [Report](../../patterns/report.md) |

Execution replay requires the [batch instructions](../../patterns/progress.md#examples-and-replay); sending the whole array at once may hide intermediate states.

These arrays initialize new cards. For existing cards, send only intended changes, not initialization. See [Construct and validate](../../../SKILL.md#construct-and-validate) for creation and update scenarios.
