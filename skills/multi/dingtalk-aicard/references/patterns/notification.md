# Notification: short results and status reminders

Use for a short result, risk alert, or status change. When there are many facts or a decision, use the [detail pattern](detail.md).

| Needed presentation | Optional components | When to choose them |
|---|---|---|
| Compact reminder | `Row > Icon + Text + Link`, then `Text` or `Markdown` | Title, source, and detail destination in one row; add icon and link only if useful |
| Themed notification | `Column > CardHeader + Markdown + ButtonGroup` | Native themed header and connected actions after the body |
| Result or exception | `Row > Tag + Text`, then explanation | A distinct status and body; a very short result may use text alone |
| Capacity or quota | `Text + ProgressBar + Text` | A real computable ratio; show a number directly if only balance or remaining amount is known |

`CardHeader` supplies a coordinated title and background theme; `Tag` supplies status styling without hand-built colors. An outer `Column` is enough unless a local background, border, or grouping calls for `Card`.

`ButtonGroup.buttons[]` contains inline objects. For static reproduction, each item may omit `action` and keep only its noninteractive appearance. Standalone `Button.action` remains required by the protocol. Use [image reconstruction](../design.md#image-reconstruction) for a branded title icon.
