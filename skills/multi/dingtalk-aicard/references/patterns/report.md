# Report: metrics, records, and conclusions

Use for a data summary, analytical report, monitoring, or execution result. Select combinations for the content; a text-only report does not need a chart.

| Needed presentation | Optional components | When to choose them |
|---|---|---|
| Conclusion and analysis | `Markdown` | Continuous explanation, paragraphs, and inline emphasis |
| A few adjacent metrics | `Row > Column(Text, Text, Text)` | Label, value, and change in each group; an optional Card can provide a local background |
| Metrics at wide and narrow widths | `ColumnLayout > metric groups` | Query and enable `responsive` when narrow layouts must become vertical; this is not an automatic wrapping grid |
| Status or exception list | `Column/List > Row(Icon/Tag, Text)` | Status and separate explanation per item; choose a registered icon |
| Homogeneous records | `Table` | Exact comparison by column |
| Trend or distribution | `Chart` | Data suited to a graphic; add a table separately when useful |
| Method and full report | `CollapsiblePanel`, `File`, or `Link` | Supplemental details, an attachment, or a real detail destination |

`Table.data` is structured data and pagination runs locally. `Chart.data` holds chart type and data, not component IDs. `ColumnLayout.columns` does not automatically wrap any number of items into groups; build actual columns according to its responsive contract.

Preserve numeric units, time ranges, and comparison baselines. Do not represent missing data as zero. Use [design guidance](../design.md#3-host-behavior-and-layout) for metric type size, spacing, and color rather than copying cramped, truncated, or gradient-heavy sample cards.

See the [data report example](../protocol/examples/data-report.json) for field and data shapes.
