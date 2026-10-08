# Notification: short results and status reminders

Use for a short result, risk alert, or status change. When there are many facts or a decision, use the [detail pattern](detail.md).

Start with the [content hierarchy guidance](../design.md#content-hierarchy). A notice with a cause, two observations, and a proposed fix needs distinct semantic groups even when it is short. Keep the source and status quiet, make the consequence scannable, keep related evidence together, and place the next step after the explanation. Do not flatten all roles into one Markdown paragraph or create one equally prominent colored box per sentence.

| Needed presentation | Optional components | When to choose them |
|---|---|---|
| Compact reminder | `Row > Icon + Text + Link`, then `Text` or `Markdown` | Title, source, and detail destination in one row; add icon and link only if useful |
| Title and explanation | `Column > Text + Markdown` | Add status or actions only when needed |
| Result or exception | `Row > Tag + Text`, then explanation | A distinct status and body; a very short result may use text alone |
| Capacity or quota | `Text + ProgressBar + Text` | A real computable ratio; show a number directly if only balance or remaining amount is known |

`Tag` supplies status styling without hand-built colors. For a requested `CardHeader`, use the [header and inset guidance](../design.md#headers-and-card-insets).

Do not select this pattern from words such as “notification”, “reminder”, or “issue” alone. First infer the product job from the user's intent, the event semantics, the reading behavior, and the expected action. A message is notification-shaped when its primary job is to make a recipient aware of an event, status change, risk, or result; a detailed investigation, approval decision, periodic report, or data-entry request may need a different pattern even if its copy contains notification words.

Keep the event or issue title as the focal fact; do not replace it with a generic header or duplicate the sole title in a separate identity strip.

Validate at the actual target container width, including narrower layouts when supported by the host. A 360px preview is only a sample design starting point, not a minimum width or an acceptance gate. Check wrapping, overflow and actions at the widths the card will actually use.

`ButtonGroup.buttons[]` contains inline objects. For static reproduction, each item may omit `action` and keep only its noninteractive appearance. Standalone `Button.action` remains required by the protocol. Use [image reconstruction](../design.md#image-reconstruction) for a branded title icon.
