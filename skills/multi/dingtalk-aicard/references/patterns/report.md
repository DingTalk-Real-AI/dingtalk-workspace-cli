# Report: metrics, records, and conclusions

Use for a data summary, analytical report, monitoring, or execution result. Select combinations for the content; a text-only report does not need a chart.

| Needed presentation | Optional components | When to choose them |
|---|---|---|
| Conclusion and analysis | `Markdown` | Continuous explanation, paragraphs, and inline emphasis |
| A few adjacent metrics | `Row > Column(Text, Text, Text)` | Label, value, and change in each group; an optional Card can provide a local background |
| Metrics at wide and narrow widths | `ColumnLayout > metric groups` | Query and enable `responsive` when narrow layouts must become vertical; this is not an automatic wrapping grid |
| Status or exception list | `Column/List > Row(Icon/Tag, Text)` | Status and separate explanation per item; choose a registered icon |
| Static tabular records | `Markdown` with table syntax | Exact comparison by column; prefer this over a separate `Table` component |
| Trend or distribution | `Chart` | Data suited to a graphic; add a table separately when useful |
| Method and full report | `CollapsiblePanel`, `File`, or `Link` | Supplemental details, an attachment, or a real detail destination |

For static tabular data, use table syntax in `Markdown.content` by default. Use a separate `Table` only when explicitly requested or when a required capability calls for it; query its contract first. `Table.data` is structured data and its pagination runs locally. `Chart.data` holds chart type and data, not component IDs. `ColumnLayout.columns` does not automatically wrap any number of items into groups; build actual columns according to its responsive contract.

Preserve numeric units, time ranges, and comparison baselines. Do not represent missing data as zero. Use [layout guidance](../design.md#3-host-behavior-and-layout), [text hierarchy](../design.md#5-hierarchy-and-density), and [color guidance](../design.md#4-color-discipline) rather than copying cramped, truncated, or gradient-heavy sample cards.

The [data report example](../protocol/examples/data-report.json) illustrates protocol capabilities and field shapes; it does not override the Markdown recommendation for static tables.

In that example, the full table dataset is supplied once for local pagination. Loop bindings are relative to each row. The Stack includes both its base child and a referenced corner label.
