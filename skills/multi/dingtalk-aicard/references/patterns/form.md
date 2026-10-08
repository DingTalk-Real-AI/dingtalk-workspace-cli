# Form: input and feedback submission

Use for parameters, settings, ratings, and comments. For display-only information, use the [detail pattern](detail.md).

| Input need | Optional components | When to choose them |
|---|---|---|
| Free text or comments | `TextField + Button` | Submit entered text; group multiple fields by business purpose |
| Fixed choices or feedback reasons | `ChoicePicker + Button` | Single or multiple choice; a static Tag is not a selectable input |
| Number, date, or person | `NumberInput / DateTimeInput / UserPicker` | Use the native input for the field meaning instead of turning everything into text |
| Boolean setting | `Text + Switch` or `CheckBox` | A settings row or a choice with full explanation; the business flow decides whether it takes effect immediately or on submit |
| Rating and explanation | `Rating + TextField + Button` | Optional comment with a star rating; use ChoicePicker for named satisfaction choices |
| Image or variable entries | `ImageUpload` or `InputList`, with `Button` | Only when uploading or dynamic entry is required |

`ChoicePicker.variant` sets single or multiple selection. `displayStyle` sets presentation: `chips` for inline tags, `checkbox` for inline vertical choices, and `dropdown` (the default) for a compact entry opening the native picker. `chips` and `checkbox` write local state through `value` binding without firing an action; the submit Button collects their values in `context`. A configured `dropdown` action fires after confirming a selection.

Bind collected values with suitable initial values. Even single-select `ChoicePicker.value` uses an array of strings. Omitting `TextField.value` does not create a writeback path. Put submit validation on standalone `Button.checks`, not on TextField or a ButtonGroup item.

Make each field recognizable before and after input. Component labels have different roles:

| Component | Field-title choice |
|---|---|
| `TextField` | Its native `label` stays above the input. Use it as the field title; an empty label removes that title. |
| `ChoicePicker` (dropdown), `DateTimeInput` | Their `label` is an in-control prompt that gives way to the selected value. Add a nearby `Text` title when the field would otherwise lose its meaning. For inline `ChoicePicker` styles, title the group only when its options do not explain their purpose. |
| `NumberInput`, `UserPicker` | Native `label`, `title`, and `placeholder` serve the input panel or control. Add a nearby `Text` title when the field needs a persistent heading in the card. |
| `CheckBox`, `Switch` | `CheckBox.label` names the choice beside its control; a `Switch` can use an adjacent `Text` in a settings row. Do not add a duplicate heading when the choice is already clear. |
| `ImageUpload` | Its `label` is the upload control's caption. Use a nearby `Text` if the field itself needs a separate name. |

Use the component contract for exact fields. A placeholder or current value alone should not carry a field name that must remain visible. Add a short purpose or outcome line for a longer form when the surrounding context does not explain it.

Preserve user input after failure; show a completed state only after a real result. See [form interaction](../protocol/examples/form-interaction.json) for bindings, submit context, and checks, and [host action](../protocol/examples/host-action.json) for a host dialog and result writeback.
