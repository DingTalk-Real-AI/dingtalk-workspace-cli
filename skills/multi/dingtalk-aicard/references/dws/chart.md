# Build a chart from business facts

Use native DWS chart commands for a **new standalone chart card**. Existing cards,
templates, and mixed reports use ordinary construction/updates: preserve layout,
IDs, interactions, and bindings. Use the native DWS executor for this edition;
the separately distributed standalone Skill supports offline Python authoring.
Never rebuild an existing instance to edit one chart.

## Choose and build

Collect the required business facts and resolve material ambiguity before
choosing a preset. If facts are missing, ask for the exact period, labels,
values and units. A generic request for an analysis chart with only category
values leaves the goal open. Ask whether the chart should compare absolute
values or show each category's share. At this stage, return the clarification questions only.
Leave the payload and command plan empty until the user answers.

When the request is for a new standalone chart and native DWS is available,
use the preset workflow below. If the current session prohibits running DWS
commands, use plan-only mode for a supported chart. In this mode,
the draft is a provisional ChartSpec with the supplied facts and a list,
explain, build and lint plan. Mark fields that require preset confirmation.
The builder produces the A2UI message array when it runs.

1. Use supplied facts and visual intent. Ask for missing categories, values, units,
   or materially ambiguous intent. Never copy business values from examples. Users
   need not supply A2UI metadata, preset IDs, or an internal ChartSpec.
2. Run `dws aicard chart list --format json`, then
   `dws aicard chart explain <id-or-alias> --format json` for the chosen preset.
   There are 43 presets (G01–G45 excluding G16/G25). Aliases describe sample
   appearances: `five_part` and `five_stage` do not fix item counts. Prefer the
   requested compatible appearance; clarify distinct equally suitable families.
   Unsupported combinations must not silently change chart types.
3. Normalize facts into `specVersion: "1.0"`, `presetId`, `title`, and `data`, using
   the explained category, named_values, points, matrix, ohlc, radar, measure, or flow
   shape. Unknown fields and wrong casing fail. Homogeneous category presets
   accept 1–32 aligned series; mixed presets retain their explained type/count
   pairing. Unsupported combinations use ordinary A2UI authoring and full-card
   validation; never discard extra series.
4. Build offline:
   `dws aicard chart build --file <spec.json> --out <new-card.json> --format json`.
   `--file -` reads stdin; `--dry-run` validates without writing. Existing paths
   fail; reuse saved artifacts after success. Files contain the creation JSON and
   `.artifact.json` with create/FINISH content, annotations, summary, and Surface ID.
   Stdout reports metadata and paths without duplicating full payloads. Build
   never sends or uses a Profile.
5. Run `dws aicard lint --file <new-card.json> --preflight new-card --format json`.
   Require `data.valid: true`, `data.preflight.valid: true`, and
   `data.preflight.chartValidation` with `status: "valid"`, `checked: 1`,
   `unverified: 0`. For multi-chart cards check the expected chart count instead.
   These are offline checks, not rendering evidence.

G31 requires `stackValues: "raw"` or `"already_percent"`; G34 requires
`cumulativeMode: "increments"` or `"already_cumulative"`. Never infer these modes.
Limits: 80 category labels, 24 pie items, 12 funnel stages, 64 Sankey nodes,
256 links, 2000 total points, and 256 KiB per option. Sankey graphs must be acyclic
without isolated nodes; gauge minimum is zero. Renderer support requires specific
client/profile evidence.

## Send only when requested

1. Resolve one permitted recipient and Profile through the chat Skill. Read saved
   `createContent`, pass it through stdin with `--content -`, and use the generated
   summary. Avoid inline JSON arguments: large cards can exceed platform command-line
   limits. Preserve the forwarding policy.
2. After explicit create acceptance, retain the **server-issued `bizId`**. Send saved
   `finishContent` and `finishAnnotations` using the update leaf and
   `--flow-status FINISH`, under the same Profile. This nonempty `/summary` delta
   belongs only to the generated card and preserves its chart and Surface.
3. Keep both receipts and read back the same instance. Query original identifiers
   before retrying unknown create/FINISH outcomes. Report acceptance, readback,
   asynchronous summary, rendering, and interaction separately. Do not send a
   preview unless requested.

## Validation boundaries

Bare lint checks Schema. Complete-card preflight resolves Chart bindings and checks
series semantics and local renderer safety. Presentation fields are not restricted
to shapes inferred from preset samples. Legacy Chart options remain Schema-only
and unverified. Resource-only delta preflight lacks an earlier Surface snapshot:
Chart validity is unknown (`valid: null`, `status: "unverified"`).

Explicit lint and build are strict. Preview and template save preserve existing
content and expose invalid/unresolved Chart data as preflight warnings. Inspect
those warnings; compatibility acceptance is not chart validation evidence.
