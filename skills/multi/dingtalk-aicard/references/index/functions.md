# Function index (40 total; 7 host actions)

Value functions use `{"call": …, "args": {…}}` and may nest. Host actions require explicit `catalogId: urn:dingtalk:a2ui:host:v1`; the system function `@index` must not carry it. `checks[].condition` requires a boolean-returning function.

| Function | Group | Returns | Required arguments | Host action | Purpose |
|---|---|---|---|---|---|
| `and` | Core functions | boolean | values |  | Performs logical AND on a list of boolean values. |
| `email` | Core functions | boolean | value |  | Checks that the value is a valid email address. |
| `formatCurrency` | Core functions | string | currency, value |  | Formats a number as a currency string. |
| `formatDate` | Core functions | string | format, value |  | Formats a timestamp into a string using Unicode TR35 date pattern. |
| `formatNumber` | Core functions | string | value |  | Formats a number with grouping and decimal precision. |
| `formatString` | Core functions | string | value |  | Uses `${/path}` for data-path interpolation. |
| `length` | Core functions | boolean | value |  | Checks string length constraints. |
| `not` | Core functions | boolean | value |  | Performs logical NOT on a boolean value. |
| `numeric` | Core functions | boolean | value |  | Checks numeric range constraints. |
| `openUrl` | Core functions | void | url |  | Opens the specified URL in a browser or handler. |
| `or` | Core functions | boolean | values |  | Performs logical OR on a list of boolean values. |
| `pluralize` | Core functions | string | other, value |  | Returns a localized string based on CLDR plural category of the count. |
| `regex` | Core functions | boolean | pattern, value |  | Checks that the value matches a regular expression string. |
| `required` | Core functions | boolean | value |  | Checks that the value is not null, undefined, or empty. |
| `add` | Expressions | number | a, b |  | DingTalk extension function. |
| `arrayFind` | Expressions | any | target, value |  | DingTalk extension function. |
| `arrayGet` | Expressions | any | index, value |  | DingTalk extension function. |
| `cond` | Expressions | any | else, if, then |  | DingTalk extension function. |
| `div` | Expressions | number | a, b |  | DingTalk extension function. |
| `eq` | Expressions | boolean | a, b |  | Tests whether two values are equal. |
| `getLength` | Expressions | number | value |  | DingTalk extension function. |
| `gt` | Expressions | boolean | a, b |  | DingTalk extension function. |
| `gte` | Expressions | boolean | a, b |  | DingTalk extension function. |
| `lt` | Expressions | boolean | a, b |  | DingTalk extension function. |
| `lte` | Expressions | boolean | a, b |  | DingTalk extension function. |
| `mod` | Expressions | number | a, b |  | DingTalk extension function. |
| `mul` | Expressions | number | a, b |  | DingTalk extension function. |
| `ne` | Expressions | boolean | a, b |  | Tests whether two values are unequal. |
| `objectGet` | Expressions | any | key, value |  | Reads an object property by `key`. |
| `sub` | Expressions | number | a, b |  | DingTalk extension function. |
| `toDouble` | Expressions | number | value |  | DingTalk extension function. |
| `toLong` | Expressions | number | value |  | DingTalk extension function. |
| `copyText` | Host actions | void | text | Yes | Copies text. |
| `openChat` | Host actions | void | openConversationId | Yes | Opens a conversation. |
| `previewImages` | Host actions | void | urls | Yes | Previews 1–100 images. |
| `previewVideo` | Host actions | void | url | Yes | Previews a video. |
| `promptText` | Host actions | object | — | Yes | Opens a single-line input panel. |
| `showConfirm` | Host actions | object | message | Yes | Shows a confirmation dialog. |
| `showModal` | Host actions | object | buttons, message | Yes | Shows a dialog with custom buttons. |
| `@index` | System function | number | — |  | Returns the 0-based index of the current item when rendering a dynamic list fro… |

---
Generated from `references/protocol/` (fingerprint `cf4611469d2c3f5e`). Do not edit this index: protocol changes regenerate it. For exact fields and a minimal example, query a name as shown in the Skill entrypoint; the whole catalog need not enter the model context.
