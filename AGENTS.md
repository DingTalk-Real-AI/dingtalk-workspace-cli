# Repository Agent Guide

This file applies to the entire repository. Keep changes scoped, preserve
unrelated work, and use `gofmt` for every modified Go file.

## Build and test

- Build: `go build ./cmd`
- Full test suite: `DWS_PACKAGE_VERSION=0.0.0-test go test ./...`
- Generate Schema assets: `go generate ./internal/cli`
- Check generated drift: `./scripts/policy/check-generated-drift.sh`
- Check the Schema contract: `./scripts/policy/check-schema-catalog.sh`

Generated Schema JSON is committed. Change its source inputs and generators,
then regenerate; do not hand-edit generated Catalog or Agent metadata files.
`internal/cli/schema_command_registry.json` is different: it is a reviewed
`CommandRegistry` source, not a generated snapshot. It is the single reviewed
source of stable canonical identity,
primary paths, aliases, and navigation. Edit it only when reviewed exposure,
identity, primary path, or aliases change; parameter, Skill, and metadata-only
changes must not rewrite it mechanically.

## Agent Schema contract

The Schema data flow is one way:

```text
1. app.NewRootCommand()
   └─ builds the real Cobra command tree and flags

2. schema_command_registry.json
   + schema_hints/metadata/<product>.json tool parameters (+ cli_path)
   └─ forms EffectiveCommandRegistry
      └─ binds exactly to real Cobra leaves and aliases

3. Parameter resolution
   Cobra flags
   + schema_parameter_bindings.json
   + metadata tool parameters
   └─ produces ParameterSpec and constraints

4. Agent and interface semantics
   schema_hints/selection/<product>.json   (selection prose)
   + schema_hints/metadata/<product>.json  (safety/interface/runtime_gate)
   + pinned MCP metadata
   └─ resolves Agent metadata by source precedence
      Markdown is evidence only; it is not concatenated into final prose

5. One typed hub
   BoundCommandRegistry
   + ParameterSpec
   + Agent metadata
   + Interface metadata
   └─ resolves every command exactly once into ToolSpec
      └─ aggregates SchemaRegistry + SchemaIndex

6. One-way publication
   SchemaRegistry
   └─ internal/cli/schema_catalog.json
      └─ dws schema list/product/group/leaf/--all
```

Parameter overlays from metadata are merged into `EffectiveCommandRegistry`
*before* Cobra binding; after that point there is no second identity source and
no identity precedence winner. The binder must reject a missing/non-runnable
Cobra path, an alias collision, and any native identity annotation that
disagrees with the effective registry. A missing native identity annotation is
allowed because annotations are implementation-side assertions, not identity
fallbacks.

The assembler resolves every bound command exactly once into one `ToolSpec`.
Build-time gates and the snapshot serializer consume that source-resolved typed
registry/index. Runtime projections and delivery gates consume the typed
registry/index returned by the production snapshot loader. Neither path may
reopen annotations, merge source records, or use a previous Catalog or other
generated JSON as a source. `schema_catalog.json` is output-only in the
generation graph. The production loader decoding the embedded published
snapshot is a delivery boundary, not source resolution; it must never create or
repair a Cobra command, flag, registry entry, or later Catalog generation.

This split is architecturally isomorphic to Lark's typed metadata registry,
navigation catalog, and schema renderer. DWS intentionally preserves its
existing flat JSON wire contract for compatibility; do not treat architectural
alignment as permission to make an unversioned wire-format change.

The reviewed `CommandRegistry` is the sole source of stable command identity
and navigation. The executable Cobra tree remains the source of truth for
whether a CLI path exists, is runnable, and which flags it accepts. Schema
coverage is bidirectional:

1. Every final `SchemaRegistry` tool, including its serialized Catalog
   projection, must resolve to an executable Cobra command.
2. Every public runnable Cobra leaf must either resolve to Schema or appear as
   an exact, reviewed exclusion with a non-empty reason in
   `internal/cli/schema_command_exclusions.json`.

Do not use prefix or wildcard exclusions: they can silently hide future
commands. Remove an exclusion when its command enters Schema; stale, invalid,
or duplicate exclusions must fail generation and CI.

When adding or changing an Agent-visible command, review all relevant inputs:

- `internal/cli/schema_command_registry.json` for the reviewed
  `CommandRegistry`: canonical identity, primary CLI path, aliases, and stable
  navigation. It is the identity source and is not a generated artifact.
- `internal/cli/schema_command_registry.schema.json` is its closed,
  machine-readable editing contract. Preserve the local `$schema` reference;
  unknown fields, invalid visibility values, stale paths, and collisions fail
  Go validation and policy.
- `internal/cli/schema_hints/metadata/<product>.json` for safety, interface,
  `runtime_gate`, and optional parameter overlays (`parameters` / `cli_path`).
- `internal/cli/schema_hints/selection/<product>.json` for reviewed Agent
  selection prose (`agent_summary`, `use_when`, `avoid_when`, `examples`).
- `internal/cli/schema_hints/index.json` only maps product IDs to those files.
- Native Runtime Schema identity annotations, when present, as consistency
  assertions against `EffectiveCommandRegistry`. They must agree exactly and
  must never materialize, infer, or override registry identity.
- Flag-to-interface property mappings and required/default semantics.
- Generated files under `internal/cli/schema_agent_metadata/` and
  `internal/cli/schema_catalog.json` after running generation.

Run the reverse-completeness tests whenever the Cobra tree changes. A command
that works through `dws <path>` but cannot be found through the matching
`dws schema` lookup is a contract failure unless it has a reviewed exact
exclusion.

Metadata parameter overlays must reference an exact public runnable Cobra leaf
and real flags. They may override Schema description, interface-property/type
mapping, `required`, and `required_when`; they must not create commands or
flags, define an interface, or advertise an unknown RPC. Every authored entry
requires `reviewed: true` and a non-empty review reason.

For Agent-authored metadata or selection edits:

1. Confirm the exact command and flag names in the current Cobra tree.
2. Edit only the owning block (`metadata/` or `selection/`); do not mix fields.
3. Add the smallest possible entry; do not copy generated Catalog fields into
   the input.
4. Describe user-visible semantics in `review_reason` and parameter
   descriptions.
5. Run generation, drift, Schema policy, and the focused CLI tests before
   proposing the change.

## Agent curation workflow (Schema hints)

Use this workflow when refreshing Agent selection prose and confirmation
alignment. Prefer **agent-authored review** over bulk merge scripts that dump
`selection-review.json` or Skill Markdown into Catalog fields.

Human-authored inputs are split into two blocks:

| Block | Path | Owns |
|---|---|---|
| **metadata** | `internal/cli/schema_hints/metadata/<product>.json` | `effect` / `risk` / `confirmation` / `idempotency` / `interface_*` / `runtime_gate` / optional `parameters` |
| **selection** | `internal/cli/schema_hints/selection/<product>.json` | `agent_summary` / `use_when` / `avoid_when` / `examples` (+ product routing) |

`index.json` only maps product IDs to those files. Do not mix selection fields
into metadata files or metadata fields into selection files.

### Goals

1. **Selection prose** is decision-oriented (Feishu/Lark style): trigger intent,
   sibling-command routing, and outcome shape — not a restatement of the
   summary. Delivered Catalog provenance is `reviewed_explicit` from
   `selection/`.
2. **Safety** follows Runtime: `confirmation=user_required` iff the tool's
   metadata `runtime_gate != none` (for example `confirm_delete`, `typed_yes`,
   `confirm_dangerous`).
3. **Parameter overrides** (former Manual `commands`) live on metadata tools as
   `parameters` (+ `cli_path`) and are applied into EffectiveCommandRegistry.

### Authoring

For every curated tool:

1. Edit `metadata/<product>.json` for safety/interface/gates/parameters.
2. Edit `selection/<product>.json` for selection prose (`reviewed: true`,
   `review_reason`, `source_refs`).
3. Run `make generate-schema`. Do not hand-edit generated
   `schema_agent_metadata/` or `schema_catalog.json`.

### Pull live MCP descriptions (personal token)

Pinned `internal/cli/schema_mcp_metadata.json` is a sanitized baseline. Prefer
live Schema from a logged-in personal session:

```bash
dws auth status                 # token_valid should be true
dws cache refresh               # refresh discovery / tools cache
dws schema <mcp-canonical> -f json
# or CLI path: dws schema --cli-path "drive copy" -f json
```

Resolve MCP identity via `interface_ref` when CLI canonical ≠ MCP path
(example: CLI `drive.copy_document` → live `doc.copy_document`). On pull
failure, fall back to Skill + Cobra Help + pinned MCP, and record evidence
(for example `live-dws-schema:<path>#FAILED`). Never print or commit tokens.

Precedence when sources disagree: **Runtime/Cobra > live MCP > pinned MCP >
Skill (evidence only)**.

### Parallel product agents

Split work by product groups. Each agent must:

- Read Skill, Cobra/`--help`, Runtime confirmation sites, and live `dws schema`
  for its tools.
- Hand-write selection + metadata; forbid wholesale JSON merges from review
  dumps.
- Edit only its `metadata/<product>.json` and `selection/<product>.json`.
- **Never** `git checkout` unrelated product files to “clean scope”.

### Regenerate and gates

```bash
make generate-schema
./scripts/policy/check-runtime-confirmation-truth.sh
go test ./internal/app -run '^TestSheetFinalSchemaConfirmationMatchesRuntimeGuards$' -count=1
```

Example rules (fail generation otherwise):

- At most two examples per tool; no `--yes` in stored examples.
- Examples must match live Cobra argv (path, flags, required groups).
- No shell comments in examples.

After generation, spot-check Catalog: selection provenance is
`reviewed_explicit` from `selection/`, and `user_required` count equals
metadata `runtime_gate != none`.

`make generate-schema` is a full deterministic snapshot rebuild, not an
incremental patch over the previous Catalog. It rereads every reviewed input,
removes stale generated product metadata, and rewrites the exact metadata and
Catalog projections. Incremental work happens only when an Agent or human
edits selected `metadata/` or `selection/` entries; the next publication still
recomputes all outputs. Generated files must never be read back as merge input,
and byte guards fail generation if it changes the hint inputs or CommandRegistry.

Selection prose may choose a more or less restrictive recommendation. It cannot
create a Cobra command or flag, change parameter facts, invent an
RPC/interface, alter safety metadata, or bypass command completeness. Examples
must use an executable primary/alias path and flags accepted by the live Cobra
command; never add `--yes` to stored examples.

Every example is always checked against its real `BoundCommand`: exact path,
accepted flags, Cobra required flags/positionals, and the effective
`require_one_of`, `require_together`, and `mutually_exclusive` constraints must
all pass before execution eligibility is considered. A missing required value,
constraint failure, runtime error, or MCP resolution error is a contract bug;
none is a valid reason to skip an example.

Example execution defaults to contract validation only. Runtime execution is
opt-in: an example enters `dry_run` only when its final `ToolSpec` publishes an
explicit reviewed dry-run capability. The test never injects `--yes`, and
`risk`/`confirmation` values do not manufacture preview support. A narrow
runtime precondition that cannot be derived from the typed contract may use an
exact zero-based `example_dispositions` entry with `mode=contract_only`,
`reviewed=true`, one of the schema-enumerated reason codes, and a concrete
non-empty reason. Such a disposition may only narrow an explicit dry-run
capability; it cannot turn an ordinary contract-only example into a skip.
Duplicate, missing, and out-of-range indexes fail validation. Never catch a
dry-run failure and dynamically downgrade it to `contract_only`.

Normal Go tests run the exhaustive contract gate. Run
`make test-schema-agent-examples` to additionally execute the eligible subset
through the real Cobra `--dry-run` path with isolated HOME and blocked proxies.
The test reports stable `total`, `contract`, `dry_run`, `contract_only`,
`reviewed_manual`, and per-reason counts; changing those counts requires a
review of the corresponding typed dry-run capability or manual disposition.
This target is also part of `make policy`.

Treat every tool `use_when` entry as a reviewed positive selection scenario
whose expected result is that tool's canonical path, and every `avoid_when`
entry as a reviewed negative scenario that must not choose that tool. The
deterministic gate derives a typed evaluation fixture from these same fields;
it requires exact tool coverage, a real runnable `BoundCommandRegistry`
primary command, at least one positive and negative assertion per tool, and no
literal contradictory expectations. It does not claim that string matching
proves natural-language understanding.

Semantic selection is an explicit opt-in live-model check. Run the smoke set
(one positive and one negative scenario per product) with
`DWS_AGENT_SELECTION_LIVE=1 ARK_API_KEY=... ARK_BASE_URL=... ARK_MODEL=... go test ./internal/app -run TestManualAgentSelectionArkLive -count=1`.
Add `DWS_AGENT_SELECTION_FULL=1` to evaluate every committed tool scenario, or
set `DWS_AGENT_SELECTION_CASES` to comma-separated fixture case IDs. Normal CI
never calls a model; its blockers remain the reproducible fixture, binding,
example, provenance, and final-delivery facts.

The live evaluator sends only case IDs/scenarios plus one same-product
candidate table; expected/forbidden assertions stay local and must never be
included in the model prompt. Built-in Ark HTTPS bases are allowlisted. A
different HTTPS provider requires its exact base in
`DWS_AGENT_SELECTION_ALLOWED_BASE_URLS`; plaintext HTTP is accepted only for a
loopback test server so API credentials are never sent to an arbitrary clear
text endpoint.

## Safety metadata

Parameter and safety resolution is mostly source-precedence based and
value-neutral: do not choose a winner because one value looks stricter. A
higher-priority reviewed metadata/explicit source may intentionally raise or
lower description, mapping, `effect`, `risk`, `confirmation`, or `idempotency`.
Preserve all candidates and the selected source in provenance, and fail
same-precedence conflicts rather than silently merging them.

`required` is the exception. Cobra `MarkFlagRequired` is a hard floor: the
final Agent projection must keep `required=true` and cannot be lowered by
manual/hint overlays. Overlays may still raise an optional flag to required.
`cli_required` continues to mirror the executable Cobra marker.

For command text, reviewed `ToolSchemaHint` wins first, then command-specific
Cobra Help, then MCP metadata. Generic RPC prose may remain an unselected
provenance candidate (and parameter-level `interface_description`); it must not
overwrite a specialized leaf's title or description.

For every delivered `ToolSpec` and `ParameterSpec` field, the provenance
winner value must exactly equal the delivered value. Checking only source,
count, presence, or hash is not a sufficient final-delivery invariant.

The same resolved `ToolSpec` must drive every projection. The full leaf payload
must equal the corresponding tool in `schema --all` and the full Catalog tool.
Overview/product/group summaries and Catalog summaries must equal
`ToolSpec.ToSummaryPayload()`. An alias lookup may change only the view fields
`cli_path` and `is_alias`; it must not re-resolve or mutate the command
contract.

This build-time rule is distinct from runtime drift handling. If shipped Help
and leaf Schema disagree, pass only flags accepted by Cobra. For conflicting
safety information, do not silently take the less restrictive behavior: use
the safer interpretation or stop and report the contract drift.

Do not infer one safety field from another. In particular, `effect=destructive`
or `risk=high` does not mechanically rewrite `confirmation`; the final
precedence winner for each field is authoritative. When
`confirmation=user_required`, obtain confirmation before adding `--yes`.
Keep CLI confirmation behavior and Schema metadata consistent, and add a
semantic regression test through the final embedded loader/query delivery
path; a generator unit test or JSON count alone is insufficient.

## Current Schema boundaries

- `schema list` remains a progressive overview. `schema --all` is the stable
  full-export contract: every final `SchemaIndex` tool must contain its
  complete leaf parameters, constraints, and safety semantics, including an empty
  `parameters` object for commands without flags. Keep it suitable for the #602
  compatibility baseline and fail rather than silently emitting a partial
  export.
- `schema --all` is not normal command discovery. Use overview -> product/group
  -> leaf for routine Agent work. `--compact` is supported for context-saving
  projections, but a compact full export is not a complete compatibility
  baseline.
- `dws <path> --help` defines whether Cobra exposes a path and which flags the
  executable accepts. A leaf Schema defines Agent selection, parameter mapping
  and constraints, and safety/confirmation semantics. A conflict is contract
  drift, not permission to guess.
- Schema and Help describe commands; neither returns DingTalk business data.
  After discovery, execute the real read/search/list command to obtain data.
