# Helpers Package Structure Guide

This is the coding-structure contract for product commands under
`internal/helpers/`. Business teams and coding agents must follow it when
adding or moving CLI leaves. Behavioral contracts (stdout, errors, confirmation,
Schema) stay in [`internal/helpers/AGENTS.md`](../internal/helpers/AGENTS.md);
this document only owns file layout and split rules.

Reference implementations already in-tree:

| Pattern | Use as |
|---|---|
| [`sheet.go`](../internal/helpers/sheet.go) + `sheet_*.go` | Preferred product layout: thin root wiring, resource files |
| [`chat_media_upload.go`](../internal/helpers/chat_media_upload.go) | Incremental extract from a megafile without behavior change |
| `connect_*.go` | Concern-based split inside the same `helpers` package |

Anti-pattern to stop growing: single megafiles such as `chat.go` / `aitable.go`
(thousands of lines). New work must not enlarge them.

## 1. Package boundary

Keep product handlers in the flat package `helpers`:

```text
internal/helpers/                 # package helpers (default)
  register_products.go            # public product registration table
  {product}.go                    # product root: new{Product}Command()
  {product}_{resource}.go         # one resource / cohesive concern
  {product}_{resource}_test.go    # focused tests beside the file
  helpers.go / interfaces.go …    # cross-product shared machinery
```

Do **not** create `internal/helpers/{product}/` subpackages by default. The flat
package exists so leaves can share unexported helpers (`callMCPToolOnServer`,
flag validators, confirmation wrappers, transport adapters) without exporting a
public API surface. Split into a subpackage only when the concern is a real
library boundary with its own tests and almost no need for helpers-private
symbols—and get an explicit review for that exception.

Repository layers outside helpers stay unchanged:

| Layer | Owns |
|---|---|
| `internal/app` | Root/static wiring, plugin load |
| `internal/helpers` | Product Cobra trees and handler behavior |
| `internal/cobracmd` | Shared Cobra construction primitives |
| `internal/executor`, `internal/transport` | Invocation and transport |
| `internal/output`, `internal/errors`, `internal/safety` | Projection, failures, confirmation |
| `internal/cli` | Schema identity / Agent metadata |

Ordinary product leaves belong in helpers. Do not push product business mapping
into app, transport, or Schema generators.

## 2. File roles inside a product

### 2.1 Product root — `{product}.go`

Owns exactly one entry constructor, registered from `register_products.go`:

```go
func newChatCommand() *cobra.Command { /* wire subgroups only */ }
```

The root file should:

1. Define the product `cobra.Command` (`Use` / `Short` / `Long` / aliases).
2. Call resource factories and `AddCommand` them.
3. Register product-level aliases or hint stubs when needed.
4. Avoid large inline `RunE` bodies for leaves.

Target size: wiring-only, roughly under **400 lines** (see `sheet.go` ≈ 270).
If the root grows past that because leaves are inlined, extract a resource file.

### 2.2 Resource / concern file — `{product}_{resource}.go`

Split on the **CLI path segment or cohesive concern**, not on “one function per
file”:

| CLI path | File |
|---|---|
| `dws chat group …` | `chat_group.go` |
| `dws chat message …` | `chat_message.go` |
| `dws chat media …` | `chat_media_upload.go` (or `chat_media.go`) |
| `dws sheet filter-view …` | `sheet_filter_view.go` |
| `dws sheet dimension …` | `sheet_dimension.go` |

Each file exposes one or more unexported factories, for example:

```go
func newChatGroupCmd() *cobra.Command { … }
func newChatMessageCmd() *cobra.Command { … }
func newWorkbookCmds() []*cobra.Command { … }
```

The product root only wires those factories. Prefer keeping a leaf’s flags,
`RunE`, and nearby request-mapping helpers in the same resource file until a
helper is reused by multiple resources.

Soft size guide per resource file:

| Lines | Action |
|---|---|
| &lt; 600 | Normal |
| 600–1000 | Prefer splitting the next cohesive subgroup before adding more |
| &gt; 1000 | Required split before landing substantial new leaves |

“One command per file” is **not** required. Tiny sibling leaves that share
flags and mapping belong together. Split when the file holds multiple unrelated
resources or becomes hard to review.

### 2.3 Shared product helpers — `{product}_{concern}.go`

Use a concern suffix when code is shared across resources and is not itself a
command tree:

- `sheet_validate.go` — shared validation
- `chat_args.go` / similar — grant/arg builders used by several leaves
- `connect_command.go` — slash-command parsing used by the daemon

Do not park unrelated products’ utilities in these files. Cross-product
machinery belongs in non-prefixed shared files (`helpers.go`, `interfaces.go`,
output/error helpers), never copied per product.

### 2.4 Tests

- Name tests after the file or scenario: `chat_message_search_test.go`,
  `sheet_filter_view_test.go`.
- Prefer exercising public product construction (`newChatCommand().Find(…)`)
  for command-surface regressions.
- Pure helpers may be unit-tested directly in the same package.
- A mechanical file split without behavior change should keep existing tests
  green with no assertion edits; if tests must change, the split leaked a
  behavior or visibility change and needs review.

## 3. Registration and naming

1. Public products enter the CLI only through the table in
   `register_products.go` (`new{Product}Command` factories). Do not invent a
   second registration path for ordinary product leaves.
2. Constructor names stay unexported (`new…`) unless a deliberate test helper
   requires otherwise.
3. File names are lowercase snake_case. Match the CLI resource token when
   practical (`filter-view` → `filter_view`).
4. Do not hand-edit generated sync markers in `register_products.go` outside
   the product-registration workflow that owns that file.

## 4. How to add a new command (business-team checklist)

1. Confirm the owning product and CLI path with `dws <product> --help`.
2. Open `{product}.go` only to wire `AddCommand`; put the leaf in
   `{product}_{resource}.go`.
3. If the product is still a megafile (`chat.go`, `aitable.go`, …) and your
   resource file does not exist yet, **create the resource file and move only
   the subgroup you need** (or add the new leaf there and wire it from the
   root). Do not append another large leaf into the megafile.
4. Keep stdout / stderr / error / confirmation contracts from
   `internal/helpers/AGENTS.md`.
5. If path, flags, safety, or Agent selection change, follow
   [`schema-contributor-guide.md`](schema-contributor-guide.md).
6. Add or extend the closest test; run
   `go test ./internal/helpers -count=1` (or a tighter `-run`) plus the checks
   selected by [`coding-agent-guide.md`](coding-agent-guide.md).

## 5. Migrating an existing megafile

Mechanical splits are welcome and preferred over “big-bang” rewrites.

Rules for a split PR:

1. **Behavior-neutral**: same command paths, flags, help text, request mapping,
   confirmation, and output.
2. **Stable entrypoint**: keep `new{Product}Command()` as the registration
   symbol; only its body becomes wiring.
3. **Move by resource**: extract one CLI subgroup per commit/PR when possible
   (`group`, `message`, `category`, …).
4. **No drive-by cleanups** in the same PR (renames, flag redesign, Schema
   edits) unless the task explicitly includes them.
5. **Stop growing the megafile**: after the first extract, new leaves for that
   resource go to the new file only.

Suggested first-wave split for `chat` (illustrative, not mandatory order):

| Extract to | Contents |
|---|---|
| `chat.go` | `newChatCommand()` wiring only |
| `chat_permission.go` | `chmod`, `data-auth` |
| `chat_group.go` | `group` tree |
| `chat_message.go` | `message` tree |
| `chat_category.go` | `category` tree |
| `chat_bot.go` | `bot` tree |
| `chat_conversation.go` | top-level conversation ops (`set-top`, mute, red-point, …) |
| existing `chat_media_upload.go` | keep; optionally rename to `chat_media.go` only in a dedicated rename PR |

Apply the same pattern to `aitable`, `attendance`, `mail`, and `doc` when those
products take new work.

## 6. What this guide does not change

- Wire format, MCP method names, or Schema identity rules.
- The choice to keep `package helpers` flat.
- Runtime confirmation / dry-run policy (still owned by safety + Schema metadata).
- Permission to skip tests because a change was “only a move”—moves still need
  the product’s focused tests green.

## 7. Harness check

After editing this guide or its links from `AGENTS.md` /
`internal/helpers/AGENTS.md`, run:

```bash
make coding-agent-harness
```

This check is a local, opt-in agent aid; it is not part of `make policy` or CI.
