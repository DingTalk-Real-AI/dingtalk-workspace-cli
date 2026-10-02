# Design guidance: make a valid card effective

`lint` checks JSON syntax and protocol structure. This guide helps choose information structure, layout, and visual hierarchy. Adapt visual advice to the user; verify the result on a target client. The bundled protocol governs fields, defaults, and enumerations. Use the contract lookup command in the Skill entrypoint for exact definitions.

## Global design guidance

1. **Use the host background by default.** Do not set a background on the outermost card unless the content requires it. Local content areas may use backgrounds to establish hierarchy.
2. **Show generation state when relevant.** For an Agent that is generating or executing, a `Markdown` component may prefix its `content` with `[AIGenerating]`, for example `[AIGenerating]Running a command…`. This is a DingTalk host convention for an animated GIF, subject to target-client support. Remove the marker and update the copy on completion, failure, or cancellation.
3. **Group execution details.** Use `CollapsiblePanel` with `variant: "reasoning"` for an Agent generation or execution process. Choose its expanded state and internal layout from the content.

## 1. What each check establishes

| Layer | Checks | Evidence |
|---|---|---|
| Tools | Lint checks protocol structure; new-card preflight also checks initialization, reachable references in the final snapshot, static cycles, and resource encoding | Run checks for the delivery scenario. Fix errors at their locations; decide whether an unreferenced-component warning matters for this card |
| Author review | Preserve user information, operations, and visual requirements; provide appropriate first-frame binding values; make priority, grouping, and density clear | Compare each request with its component or action and the host's existing state. Visual and content completeness are not lint diagnostics |
| Runtime acceptance | Resource loading, function evaluation, delivery, actual layout, interactions, and state writeback | Verify in the target host. Report request acceptance, conversation readback, and client rendering separately |

Passing structural checks does not establish the other two layers. Explain unsupported behavior or missing runtime evidence at delivery. Ensure that every requested piece of information, visual element, and action has an appropriate representation. Do not invent events or URLs to fill a checklist.

## Image reconstruction

**Identify regions and purpose before generating.** Record visible copy, numbers, region order, buttons, and states; distinguish interface layout from image assets before looking up components. This analysis guides implementation and need not become another file or exposed reasoning. Ask only about critical facts or interactions that cannot be determined. Do not guess unreadable content.

| Content in the image | Implementation |
|---|---|
| Interface copy, data, buttons, forms, lists | Use structured components and preserve content and state. Static reproduction may use literals; changing business values need bindings and initial values |
| Photography, marketing posters, complex illustrations | Use the original asset or a reliable crop in `Image`; preserve its subject and essential text. Poster text need not be forcibly decomposed into components |
| Brand marks and specific document or product icons | Use a real asset or a crop of the reference. Do not impersonate them with emoji, ordinary characters, or vaguely similar icons. If the asset is unavailable, report the gap and, when appropriate, use an explicit text description |
| Generic action or status icons | Prefer a registered `Icon` with matching meaning. Otherwise use a supplied asset or text; do not invent an Icon name |

Crop media to the media region itself. Do not bake surrounding interface copy or interactive controls into the image; text and marks that belong to a poster may remain inside it. Do not substitute a screenshot of the entire interface for components, or destroy poster artwork just to make it structural. Images need a working URL or correctly encoded inline data, not a placeholder path.

**Implement only known interactions.** For appearance-only reproduction, use a valid static representation and state that behavior is not connected. A pictured button does not reveal its event or URL. Implement bindings and actions when the user explicitly requires a form, data update, or host action. Extra business data and behavior require user input or a verifiable source.

**Compare before delivery.** Check key copy, numbers, dates, prices, region order, buttons, tags, and media against the source. Look for invented business facts or substituted brand assets. The component count need not equal the number of visual elements; preserve meaning and use. Report unreadable, unavailable, or unsupported content. Neither this comparison nor protocol preflight proves client visual fidelity.

## 2. Information roles and blocks

First decide what the user most needs to know or do, then arrange conclusions, evidence, details, and actions. A report may lead with the conclusion; an execution card may lead with progress. Short content needs no extra blocks. These combinations are optional layout notation, not a fixed order or required structure.

| Role | Purpose | Components | Layout notation |
|---|---|---|---|
| `header` | Title, source, and status | Row + Text + Tag | `Row(align=center, gap=8) > Text(bold, weight=1) + Tag` |
| `hero` | Key cover or person image | Image | `Image(variant=header, fit=cover, cornerRadius=10)` |
| `facts` | Label-value facts | Column + Row + Text | Each row: `Row(align=start, gap=8) > Text(label) + Text(value, weight=1)`; see [key-value rows](#key-value-rows) |
| `summary` | Conclusion-first prose | Markdown | Literal or bound content, for example `/content/body` |
| `metrics` | A few important measures | Row + Column + Text | `Row(gap=8) > Column(weight=1, padding=12) > Text(value, bold) + Text(label, caption)` |
| `data` | Comparison, trend, or composition | Table / Chart | Use Table for exact records and a suitable Chart for graphical data; look up values in the contract. See the [report example](protocol/examples/data-report.json) |
| `list` | Repeated entries | Column + Card + Text | — |
| `timeline` | Time- or stage-ordered progress | Column + Row + Text + Tag | — |
| `attachments` | File results | Divider + Text + File | `File(fileName, url, description, onPreview)` |
| `form` | Text and choice inputs | TextField + ChoicePicker | Group and bind business fields, such as `/form/*`; submit-button `context` refers to the relevant paths. See [form composition](patterns/form.md) |
| `actions` | Main and optional secondary action | Divider + Row + Button + Text | `Row(align=center, gap=8) > Button(default) + Button(primary)`; each Button's `child` references a Text |
| `details` | Infrequent supporting information | CollapsiblePanel + Markdown + Link | `CollapsiblePanel(variant=indented, defaultExpanded=false, maxHeight=240) > Column > Markdown + Link(openUrl)` |
| `progress` | Generation or execution stage | CollapsiblePanel + Markdown | `CollapsiblePanel(variant=reasoning) > Markdown` |
| `terminal-status` | Stable completed, failed, or cancelled state | Tag + Text + Link | — |

States, icons, illustrated entries, and highlights may support a main block or stand alone when content is simple. They do not each require a top-level section.

| Local combination | Use | Layout or choice |
|---|---|---|
| Compact header | Schedule, task, light reminder | `Card(padding=0) > Column(gap=0) > CardHeader(trailing=Tag) + Column(padding=8, gap=8)` |
| Status | Current stage or risk | Tag + Text |
| Icon | Action, source, or status | Icon or Text.icon; use a registered matching icon, or a supplied image or explanation if none exists. Do not invent a name or default to emoji for a specific icon |
| Illustrated item | Compact image-and-text content | `Row(align=center, gap=12) > Column(weight=1) + Image(variant=smallFeature)` |
| Highlight | Key conclusion or local emphasis | See [local highlights](#local-highlights); this is a design technique, not a protocol field |

Read [composition patterns](patterns/detail.md) from the Skill's task table. Approval, schedules, and tasks share the detail pattern; ongoing generation uses the [progress pattern](patterns/progress.md). For mixed content, select only needed blocks and avoid duplicating headers or actions. Choose one [message example](protocol/examples/README.md); all four are complete new-card messages. Adapt them for host-created Surfaces or updates as described in the [Skill entrypoint](../SKILL.md). The protocol and contract lookup govern fields.

## 3. Host behavior and layout

The bundled protocol governs field capabilities. Visual numbers are starting points, not extra protocol limits.

Ordinary vertical content can use `Column` directly; add a `Card` only when grouping, background, or border serves the content. Omitting root `padding` may still leave host padding and is not the same as setting it to `0`.

Use `Text` or `CardHeader` for ordinary group titles. Reserve Markdown headings for actual document sections; a small field or status label does not need heading hierarchy.

1. `Row.align=stretch` can make containers in a row the same height. Container children and leaf components are handled differently; consult the Row contract for client behavior and integration boundaries.
2. Check long titles, metrics, and multiple columns at the target conversation width. Wrap or reduce adjacent items when space is limited; do not assume identical client widths.
3. `Image.variant=smallFeature` is a starting point for illustrated cards; adjust to the asset and protocol size guidance.
4. Use `CardHeader` for a compact native header when appropriate; build other headers from actual requirements.
5. Usually emphasize one primary action within a visible action area. Assess separate Tabs or states independently, not by counting every Button in the component list. Label actions by what they do, not by phrases such as “click the button below.”
6. Remove empty rows and dividers without information value. Choose truncation, full display, or collapse for long content; do not hide necessary text with `maxLine`.
7. Check media URLs under the host policy; lint does not fetch them. Table pagination is local, so provide the data at once. Paginate, limit rows, or link to details for long tables.
8. Query a component for defaults, platform fallbacks, and conditions under which a field takes effect. The host may apply defaults when fields such as `Image.variant` are omitted.

Keep related content close and separate groups with appropriate space. Set `gap` and `padding` on the containers responsible for layout rather than stacking padding at several levels. Multiples of 2 are a reasonable spacing starting point, adjustable for host defaults, density, and user requirements; they are not validation rules.

### Key-value rows

- Put related facts in one `Column`. Each row can begin with `Row(align=start, gap=8)`; give long values `weight=1` for remaining width.
- Labels and values may both use `body`, with `common_level2_base_color` and `common_level1_base_color` respectively. Consider `caption` labels for compact content.
- Omit `weight` for a naturally sized label; do not write `weight=0`. Labels of different lengths do not guarantee aligned value columns. Stack a long label and value vertically when width is insufficient rather than assuming unsupported fixed columns.
- `Text` displays at most three lines by default. Set sufficient `maxLine` for important long values, or use full text or collapsible details. Do not truncate every value to two lines.

### Local highlights

- Give a key conclusion or risk its own block when needed, for example `Column(padding=12, gap=8) > Text(bold) + Markdown`; add a local background only when useful.
- Emphasize a short label within `Markdown.content`, for example `**Risk:** Only completed tasks are included.`, without wrapping every note in another container.
- Choose background and border for hierarchy; they need not both appear. When both are used, keep them in one semantic color family and select actual values from the [Token index](index/tokens.md).
- `Card.backgroundColorToken` can use a registered `extended_*0_color`; `borderColorToken` can use its matching `extended_*1_color`. `borderWidth` is numeric, for example `0.5`, not a string with units. `*` denotes a family only: send complete registered Token IDs and do not put Token names in hex-color fields.
- Use component defaults when no emphasis is needed. Do not add a colored container for decoration or repeat the same highlighted block in every subsection. A short bold label often suffices.
- Use only supported fields. Simplify a one-sided indent or inline treatment that cannot be expressed accurately. Do not invent a field or pad with spaces; container `padding` affects all sides.

## 4. Color discipline

Prefer registered Tokens or semantic component variants (`Tag.theme`, `CardHeader.theme`). Some fields accept hex colors; consult their contracts. For semantic colors, consider:

| Meaning | `Tag.theme` / `CardHeader.theme` | Text `colorToken` |
|---|---|---|
| Success / accepted | `green` | `common_green1_color` |
| Failure / warning / dangerous action | `red` | `common_red1_color` |
| Pending / reminder | `orange` | `common_orange1_color` |
| Information / neutral | `blue` | — |
| Primary text | — | `common_level1_base_color` |
| Secondary text | — | `common_level2_base_color` |
| Supporting / placeholder text | — | `common_level3_base_color` |
| Divider | — | `common_line_light_color` (`Divider.borderColorToken`) |

Use a Token or theme for one meaning within a card. Express important status in text, not color alone. The host resolves actual colors, so a local block needs sufficient contrast against its background. Prefer `Tag.theme` for capsule states and contract-supported Tokens for container colors. Component defaults are valid too; a card does not need a quota of explicit Tokens. See the full [Token index](index/tokens.md).

Keep ordinary key-value labels neutral; use semantic color for a value only when it conveys status or trend. Three or fewer non-gray semantic colors is a starting point for an ordinary information card, not a limit on charts, brands, or complex states.

## 5. Hierarchy and density

- **One strongest focal point:** use `bold` and `level1` for a title, a normal body, and `caption` with `level2` for explanation, as the content warrants.
- **Top-level grouping:** two to five groups can guide a complex card. A short notice may have one, and complex content may need more.
- **Group with containers:** put related fields in one `Column` or `Card`. Use `Divider` between semantic sections, such as content and actions, not between every row.
- **Contain long content:** use `CollapsiblePanel(defaultExpanded=false)` for details. Offer a “View all” `Link` when a real full-content destination exists.
- **Separate display fallback from business data:** missing information may display “—” or “None yet,” or omit an empty section. Never write display fallback into a form, number, or boolean value. Empty string, unchecked `false`, unselected `[]`, and numeric `0` retain their distinct meanings.

## 6. Revise feedback at the right field

After a user reviews the card, locate the affected fields and check related effects:

| Feedback | Change | Recheck |
|---|---|---|
| Copy or wording | The corresponding `/content/*` value in `updateDataModel`, or `Text.text` / `Markdown.content` | Factual accuracy |
| Color or style | `Tag.theme` / `CardHeader.theme` / `colorToken` | Meaning and contrast |
| Layout | `Row` ↔ `Column`, `align` / `gap` / `weight` / `padding` | Grouping and references |
| Missing information | Add it to the relevant role; initialize its model value when bound | Information and bindings |
| Too dense or too sparse | `gap` / `padding` / `maxLine`, or collapse details in `CollapsiblePanel` | Density at actual width |
| Wrong button | `action.event.name` / `context`, `variant`, `disabled` | Visual and interaction behavior separately |

Rerun lint and deliver the revised file and check results. Send a real card only when requested and supported by an available delivery route.
