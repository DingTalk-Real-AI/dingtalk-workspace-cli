# Author chart data offline

Use supplied facts, units and visual intent. Ask for missing data or a materially
ambiguous comparison; never promote sample values to business facts. Preserve
the layout, component IDs, Surface and bindings when editing an existing card,
template or mixed report. A single-chart example does not replace a whole report.

## Discover a style and data family

The bundled [43-style catalog](chart-presets.json) contains IDs, aliases, data
families, series types and presentation defaults. G16 and G25 are excluded.
This is an offline reference, not an executable preset builder. Alias names
describe sample appearances rather than fixed business item counts. Query the
named Chart contract for the actual protocol; unsupported combinations require
clarification rather than silently changing the requested chart.

| Family | Facts to collect | Option shape |
|---|---|---|
| category | Ordered categories and aligned named value arrays | Category/value axes and bar/line series; preserve grouping, stacking and orientation |
| named_values | Named values and units | Pie or funnel entries with name/value; retain requested order |
| points | Named coordinate points | Scatter/effectScatter data and compatible axes; a point may carry extra dimensions |
| matrix | Axis labels and coordinate/value triples | Heatmap axes, visual range and heatmap series |
| ohlc | Ordered categories and OHLC records | Candlestick category xAxis and value yAxis |
| radar | Indicators, bounds and named vectors | Radar indicators and aligned value arrays |
| measure | Values and display bounds | Gauge values; omitted bounds default to 0/100, at most five pointers |
| flow | Named nodes and weighted links | Acyclic Sankey links referencing existing nodes |

Use the exact family and properties in the selected catalog entry. Preserve
explicit cumulative/percentage meanings; do not infer whether values are raw,
already cumulative or already normalized.

## Build a complete message array manually

For example, supplied facts A=10 and B=20 can be expressed as:

```json
[{"version":"v1.0","createSurface":{"surfaceId":"sales","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{"chart":{"xAxis":{"type":"category","data":["A","B"]},"yAxis":{"type":"value"},"series":[{"type":"bar","name":"Sales","data":[10,20]}]}},"components":[{"id":"root","component":"Column","children":["chart"]},{"id":"chart","component":"Chart","data":{"path":"/chart"},"fallbackMarkdown":"Sales: A 10; B 20."}]}}]
```

Replace the sample facts, IDs, summary and styles. Keep fallbackMarkdown readable
and consistent with the actual data. Include a legend when several named series
need identification. Delivery content is a separate capability; preparing this
file neither creates a cloud template nor sends a card.

## Validate the card

Follow the [lint instructions for this Skill](../SKILL.md#construct-and-validate)
and select `--preflight new-card` for a complete card.

Check exit status, valid and preflight.valid. Inspect chartValidation.checked,
unverified and status. Only statically resolved, supported instances can provide
strict Chart evidence. Unknown series types warn because they may belong to a
client-registered chart template. Check the spelling or verify the registered
template before delivery. Relative template bindings and unreachable definitions
also remain unverified.
Schema, semantic checks and client rendering are different evidence layers.
For a delta use resource checks and compare against the known current state;
do not replay initialization or user-entered form defaults.
