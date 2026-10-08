# Design guidance: make a valid card effective

`lint` checks JSON syntax and protocol structure. This guide helps choose information structure, layout, and visual hierarchy. Adapt visual advice to the user; verify the result on a target client. The bundled protocol governs fields, defaults, and enumerations. Use the contract lookup command in the Skill entrypoint for exact definitions.

## Global design guidance

1. **Prefer a transparent root for ordinary new cards.** Let the host provide the outer background instead of approximating its color. Preserve explicit surface designs and unrelated existing styling; see [root surface](#root-surface).
2. **Show generation state when relevant.** For an Agent that is generating or executing, a `Markdown` component may prefix its `content` with `[AIGenerating]`. DingTalk supports this convention across its six platforms; remove it on completion, failure, or cancellation. For panel titles and execution updates, see the [progress pattern](patterns/progress.md).
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
| Interface copy, data, buttons, forms, lists | Preserve content and state with structured components. Bind data-driven fields only where supported, with typed initial values; update other fields through `updateComponents` |
| Photography, marketing posters, complex illustrations | Use the original asset or a reliable crop in `Image`; preserve its subject and essential text. Poster text need not be forcibly decomposed into components |
| Brand marks and specific document or product icons | Use a real asset or a crop of the reference. Do not impersonate them with emoji, ordinary characters, or vaguely similar icons. If the asset is unavailable, report the gap and, when appropriate, use an explicit text description |
| Generic action or status icons | Prefer a registered `Icon` with matching meaning. Otherwise use a supplied asset or text; do not invent an Icon name |

Crop media to the media region itself. Do not bake surrounding interface copy or interactive controls into the image; text and marks that belong to a poster may remain inside it. Do not substitute a screenshot of the entire interface for components, or destroy poster artwork just to make it structural. Images need a working URL or correctly encoded inline data, not a placeholder path.

**Implement only known interactions.** For appearance-only reproduction, use a valid static representation and state that behavior is not connected. A pictured button does not reveal its event or URL. Implement bindings and actions when the user explicitly requires a form, data update, or host action. Extra business data and behavior require user input or a verifiable source.

**Compare before delivery.** Check key copy, numbers, dates, prices, region order, buttons, tags, and media against the source. Look for invented business facts or substituted brand assets. The component count need not equal the number of visual elements; preserve meaning and use. Report unreadable, unavailable, or unsupported content. Neither this comparison nor protocol preflight proves client visual fidelity.

## 2. Information roles and blocks

First rank the content: order the facts, group them by relationship, and assign visual weight before selecting components. A report may lead with the conclusion; an execution card may lead with progress. Short content needs no extra blocks. These combinations are optional layout notation, not a fixed order or required structure.

| Role | Purpose | Components | Layout notation |
|---|---|---|---|
| `header` | Title, source, and status | Row + Text + Tag | `Row(align=center, gap=8) > Text(bold, weight=1) + Tag` |
| `hero` | Key cover or person image | Image | `Image(variant=header, fit=cover, cornerRadius=10)` |
| `facts` | Label-value facts | Column + Row + Text | One-line rows use `align=center`; wrapping values use `align=start` and matching label/value typography so the label aligns with the value's first line; see [key-value rows](#key-value-rows) |
| `summary` | Conclusion-first prose | Markdown | Literal or bound content, for example `/content/body` |
| `metrics` | A few important measures | Row + Column + Text | `Row(gap=8) > Column(weight=1, padding=12) > Text(value, bold) + Text(label, caption)` |
| `data` | Comparison, trend, or composition | Markdown / Chart | For static tabular data, prefer Markdown table syntax over a separate `Table` component. Use a suitable Chart for graphical data and query its contract. |
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
| Compact header | Schedule, task, light reminder | `Text`; add a `Row` with a `Tag` only when there is a status to show |
| Content section | Related content needing a bounded visual surface | `Card(padding=12, cornerRadius=8, borderWidth=0.5) > Column(gap=8) > content` |
| Status | Current stage or risk | Tag + Text |
| Icon | Action, source, or status | Icon or Text.icon; use a registered matching icon, or a supplied image or explanation if none exists. Do not invent a name or default to emoji for a specific icon |
| Illustrated item | Compact image-and-text content | `Row(align=center, gap=12) > Column(weight=1) + Image(variant=smallFeature)` |
| Highlight | Key conclusion or local emphasis | See [local highlights](#local-highlights); this is a design technique, not a protocol field |

Select a relevant pattern from the Skill's [composition-pattern table](../SKILL.md#composition-patterns). Approval, schedules, and tasks share the detail pattern; ongoing generation uses the [progress pattern](patterns/progress.md). For mixed content, select only needed blocks and avoid duplicating headers or actions. When concrete message shapes would help, read the pattern's linked JSON; use the [example index](protocol/examples/README.md) only if unsure which fits. Examples initialize new cards; adapt them for host-created Surfaces or updates as described in the [Skill entrypoint](../SKILL.md#construct-and-validate). The protocol and contract lookup govern fields.

## 3. Host behavior and layout

The bundled protocol governs field capabilities. Visual numbers are starting points, not extra protocol limits.

Reserve Markdown headings for actual document sections; a small field or status label does not need heading hierarchy. Keep independent facts, statuses, and controls in appropriate components rather than flattening the card into one Markdown block.

1. `Row.align=stretch` can make containers in a row the same height. Container children and leaf components are handled differently; consult the Row contract for client behavior and integration boundaries.
2. Check long titles, metrics, and multiple columns at the target conversation width. Wrap or reduce adjacent items when space is limited; do not assume identical client widths.
3. `Image.variant=smallFeature` is a starting point for illustrated cards; adjust to the asset and protocol size guidance.
4. Usually emphasize one primary action within a visible action area. Assess separate Tabs or states independently, not by counting every Button in the component list. Label actions by what they do, not by phrases such as “click the button below.”
5. Remove empty rows and dividers without information value. Choose truncation, full display, or collapse for long content; do not hide necessary text with `maxLine`.
6. Check media URLs under the host policy; lint does not fetch them. Prefer Markdown table syntax for static tabular data. Limit rows or link to details for long tables. If a separate `Table` is explicitly required, its pagination is local, so provide the data at once.
7. Query a component for defaults and conditions under which a field takes effect. The host may apply defaults when fields such as `Image.variant` are omitted.

Keep related content close and separate groups with appropriate space. Set `gap` and `padding` on the containers responsible for layout rather than stacking padding at several levels. Multiples of 2 are a reasonable spacing starting point, adjustable for host defaults, density, and user requirements; they are not validation rules.

### Root surface

For ordinary new cards, prefer a transparent root so the host supplies the outer background. A configured color or theme token is not guaranteed to match the message surface across clients, themes, or tenants. Do not use `common_bg_color` merely to imitate the host background.

| Intent | Root and background |
|---|---|
| Ordinary vertical content | Prefer `Column`; omit `backgroundColor` and `backgroundColorToken` |
| A root `Card` is needed without a separate surface | Set `backgroundColor: "#00FFFFFF"`; omit `backgroundColorToken` |
| An explicitly requested theme-aware surface | Use a registered `backgroundColorToken` and verify the result in the target host |
| An explicitly requested custom `Card` surface | Use `backgroundColor` with `light` and `dark` values, or a single color when a fixed color is intended |
| An edit to an existing card | Preserve the root, background, and unrelated styling unless the requested change requires otherwise |

The public `Card` contract uses `#FFFFFFFF` in light mode and `#FF1F1F1F` in dark mode when both background fields are omitted. Omission is therefore not a request for transparency. `backgroundColorToken` takes precedence over `backgroundColor`; leave it out when requesting a transparent surface. `Column` also supports borders and corner rounding, so those features alone do not require an extra `Card` wrapper.

Use filled panels for local grouping or emphasis, not as an automatic full-body layer beneath a transparent root. The content-section `Card` above illustrates such a local panel. Preserve business-content backgrounds required by the user or reference design; transparency is a default, not a reason to override that design. When reconstructing a screenshot, do not reproduce the host chat bubble or window background as the card's own fill unless the user explicitly requests a separate surface.

These are authoring defaults, not Schema restrictions or cross-client rendering guarantees. Default, opaque, and themed `Card` backgrounds remain valid. Check background, insets, and readability in the target client's light and dark themes before claiming visual consistency.

### Headers and card insets

Omit root `padding` for ordinary cards: omission allows default insets, while explicit `0` can leave mobile content flush against the edge. Do not add a second whole-body inset by default; local content panels may still need padding.

`CardHeader` can trigger header-specific spacing that differs across hosts. When explicitly requested, preserve it and verify insets and any requested corner rounding on target clients; do not assume a full-bleed header or identical spacing. These are authoring defaults, not Schema restrictions: `CardHeader` and explicit zero padding remain valid.

### Key-value rows

- Put related facts in one `Column`. For a single-line key-value pair use `Row(align=center, gap=8)`. If the value can wrap, use `Row(align=start, gap=8)` and the same text size/line height for label and value; color may mute the label. This keeps the label centered against the value's first line instead of the entire paragraph. Give the value `weight=1` for remaining width.
- End a visible key label with a colon (`：` in Chinese text), such as `ISO 8601：` or `Elapsed time:`. Keep the value in a separate Text component so it can wrap without moving the label.
- Labels and values may both use `body`, with `common_level2_base_color` and `common_level1_base_color` respectively. Consider `caption` labels for compact content.
- Omit `weight` for a naturally sized label; do not write `weight=0`. Labels of different lengths do not guarantee aligned value columns. Stack a long label and value vertically when width is insufficient rather than assuming unsupported fixed columns.
- `Text` displays at most three lines by default. Set sufficient `maxLine` for important long values, or use full text or collapsible details. Do not truncate every value to two lines.

### Local highlights

- An ordinary content section may use `Card.backgroundColorToken=common_fg_z1_color`, `borderWidth=0.5`, and `borderColorToken=common_line_hard_color`.
- A highlighted content section may use a registered `extended_*0_color` for `Card.backgroundColorToken`, the matching color family's `extended_*1_color` for `borderColorToken`, and `borderWidth=0.5`. For example, pair `extended_blue0_color` with `extended_blue1_color`; never mix families.
- `padding=12` and `cornerRadius=8` are starting points for a local `Card`; adjust to the content and host spacing. `Card` accepts one `child`, so wrap multiple elements in a `Column` or `Row`.
- Give a key conclusion or risk its own block when needed, for example `Column(padding=12, gap=8) > Text(bold) + Markdown`; add a local background only when useful.
- Emphasize a short label within `Markdown.content`, for example `**Risk:** Only completed tasks are included.`, without wrapping every note in another container.
- Choose background and border independently according to the hierarchy; neither is mandatory. When both appear, keep them in one semantic color family and select actual values from the [Token index](index/tokens.md).
- `borderWidth` is numeric, for example `0.5`, not a string with units. `*` denotes a family only: send complete registered Token IDs and do not put Token names in hex-color fields.
- Use component defaults when no emphasis is needed. Do not add a colored container for decoration or repeat the same highlighted block in every subsection. A short bold label often suffices.
- Use only supported fields. Simplify a one-sided indent or inline treatment that cannot be expressed accurately. Do not invent a field or pad with spaces; container `padding` affects all sides.

## 4. Color discipline

Follow the user's requested style. When color helps emphasize key metrics, titles, or focal areas, choose a suitable color family; neutral component defaults are also valid. A local panel can pair a pale background with stronger accents from the same family while ordinary body text stays neutral. A transparent root does not require a monochrome card. Avoid coloring every section, and preserve business-status meanings independently of the visual palette.

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

Choose the main visual area from the content: an available relevant image for illustrated content, key metrics for a report, or the current goal for a task. Give it more emphasis than supporting details without inventing content for decoration. When useful, put secondary details in a collapsed section or a details Tab; keep essential risks and required inputs visible, and do not add Tabs to simple cards.

- **Text hierarchy:** emphasize focal `Text` with `bold=true` when needed. Use `variant=body` for normal text or `variant=caption` for smaller supporting text. Optional `colorToken` values include `common_level1_base_color` for primary text and `common_level2_base_color` for secondary text; preserve requested typography.
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
| Button label or appearance | Referenced `Button.child` Text or its binding; `ButtonGroup.buttons[].label`; supported style or disabled fields | Preserve the action when only presentation changes |
| Button behavior | Inspect the existing `action.event` or `action.functionCall` and change only the requested behavior; event data belongs in `action.event.context` | Intended behavior, parameters, and existing result bindings |

Rerun lint and deliver the revised file and check results. Send a real card only when requested and supported by an available delivery route.

## Media resources

Prefer verified HTTPS assets, including avatars, Markdown images and bound values. Preserve explicitly requested inline Base64 resources and disclose unverified client compatibility. Valid encoding does not establish image decoding, transport limits or client support. Resource preflight adds `resource.base64_image_unverified` warnings and rejects malformed encoding; it does not fetch URLs or resolve bindings and Markdown destinations.

Use supplied or verified assets. If one is unavailable, disclose the gap or ask for an accessible URL; do not invent URLs, impersonate a real avatar with an unrelated image, or silently upload private screenshots. Any agreed placeholder must be identified as a substitution, not faithful reconstruction.

## Content hierarchy

Choose a focal conclusion, group related evidence, and keep supporting metadata quiet. Do not add titles, metrics, or actions merely to fill a template. See [information roles](#2-information-roles-and-blocks) and [hierarchy and density](#5-hierarchy-and-density) for layout choices.
