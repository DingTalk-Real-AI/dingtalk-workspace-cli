# Repository Agent Guide

This file applies to the entire repository. Keep changes scoped, preserve
unrelated work, and use `gofmt` for every modified Go file.

## Build and test

- Build: `make build` (wraps `scripts/dev/build.sh` → `go build -o dws ./cmd`; bare `go build ./cmd` fails because output name `cmd` collides with the directory)
- Full test suite: `DWS_PACKAGE_VERSION=0.0.0-test go test ./...`
- Param aliases generate: `go generate ./internal/cli` (entry point: `internal/cli/gen.go`; Catalog is not generated)
- Optional diagnostic MCP dump (not a Schema pin): `make fetch-mcp-metadata` (requires `dws auth login`; writes under `artifacts/`)
- Check generated drift + assembly determinism: `./scripts/policy/check-generated-drift.sh`
- Check the Schema contract: `./scripts/policy/check-schema-catalog.sh`
- Coverage-gate test naming: tests that carry coverage for the macOS platform
  gate must be named `TestCrossPlatformCoverage*` (or `TestAllShortcuts*`);
  `scripts/policy/run-platform-coverage-gate.sh` only selects those prefixes,
  so a covering test with any other name silently leaves its target uncovered.
- Package-var injection seams (e.g. `pipelineBuildEffectiveRegistry`): swap
  them in tests only via `testseam.Swap(t, &seam, stub)` from
  `internal/testseam` — it restores the previous value through `t.Cleanup`
  structurally. Like the manual pattern it replaces, Swap mutates global state
  and is **not** safe for `t.Parallel` tests.
- Cross-package test helpers (e.g. `StoreProductDeclRawForTest`) live in
  per-package `fortest.go` files, never scattered through production files;
  the `ForTest` suffix is the boundary and production code must not call them.

Schema Catalog delivery is **声明即 Catalog**: production assembles via
`RegisterSchemaSourceRoot` → `ResolveSchemaBuild` (factory registered in
`internal/app`). There is no
`cmd_schema_catalog` `//go:generate` delivery step. `dws schema -f json` remains
the wire projection. `cmd_schema_catalog` produces CI/local dumps only;
`internal/cli/schema_catalog/`, `internal/cli/schema_meta_index.gob`, and
`internal/cli/schema_meta_index.json` must not be committed. `schema_agent_metadata/` is retired: if that directory
(or `schema_agent_metadata_audit.json`) is present, policy fails.
Command identity is no longer a file input: it is collected from
`ContractFinal.Identity` on the live Cobra leaves
(`internal/cli/schema_identity_collect.go` → `BuildEffectiveCommandRegistry`).
The reviewed `schema_command_registry/` was retired together with that
switchover and must not reappear; identity changes happen by editing the leaf
declaration. The remaining **reviewed inputs** under `internal/cli` (see Agent
Schema contract) keep separate authorities — do not merge them with
`param_concepts.json` or promote any of them into Catalog declaration.

## Command framework declaration

- Framework definition: `docs/rfc-command-framework-convergence.md` **§5.0**
- Today: `helpers.LeafSpec` / `shortcut.Shortcut` → `corecmd.Spec` (+ optional `Contract`) → `corecmd.New`
- **Declare = final Schema source**: `Flags` / `Constraints` / `Safety` / `ConstParams` / `Contract` (`corecmd.ContractDecl`; nested fields are `contract.*`)
- Naming: `ContractDecl` is the authoring leaf declaration. "Schema" means Catalog / `ToolSpec` delivery — do not reintroduce `SchemaDecl`.
- `Safety` uses `contract.SafetySpec` (`internal/corecmd/contract` only — no `cli.*` type alias). Its `confirmation` drives the runtime gate; `effect` / `risk` / `idempotency` are published unchanged. When `Contract` is set, convert once via `contractfinal.RegisterRuntimeContractFinal` (all callers — `corecmd.New` registers internally); assembly **pass-throughs** Final.
- Package seam:
  - types / ProductDecl → `corecmd/contract` (DTO only; **no** Cobra-keyed ContractFinal store)
  - AnnotateRuntime* writers → `internal/corecmd/runtimeannotate` (framework-owned)
  - ContractFinal cobra store + Register → `internal/corecmd/contractfinal` (framework-owned)
  - homology gates → `internal/cli/homology`
  - Catalog assembly / `ResolveMeta` (`RegisterSchemaSourceRoot` → `ResolveSchemaBuild`); go:embed only for reviewed inputs → `internal/cli` root (package-local aliases for annotate/store APIs live in `runtime_schema_seam.go`; the former `cli/runtimeannotate` / `cli/contractfinal` shim packages are removed — import `corecmd/*` directly)
  - **Hard rule**: `internal/corecmd` (and its subpackages) must **not** import any `internal/cli` package
- Authoring tiers (current, not aspirational):
  - **Tier1** — `corecmd.New` / `helpers.NewLeafCommand` (fully managed declare + execute)
  - **Tier2** — `DeclareLeafMetadata` (helpers migration; **Shortcut may also use this path — acceptable**)
  - **Tier3** — bare Cobra (should shrink over time; reviewed exclusions where needed)
  - Long-term outlook only: broader mcpbind / fewer hand-written `Execute` bodies. **Not** a current hard requirement to delete `Shortcut.Execute` or force mcpbind.
- Description declare vs delivery: construction requires `ContractDecl.Description` (evidence). Catalog delivery prefers Cobra Long → provenance `cobra_help`; without Long, declared text → `contract_final`. Title: declared first, then Short, then MCP. Do **not** read this as "declare = wire final" or dual authority.
- **Execute** = hooks (`Validate` / `Call` / `RunE` / `PostMount`) — not a second surface authority
- Declaration path has **no reviewed parallel fields**; migration-only `runtime_gate` annotate until `Safety` is declared
- **Do not add** new production `AnnotateRuntimeRisk` / `AnnotateRuntimeGate`
  (`runtime_gate`) call sites; migrate leaves to declared `Safety` /
  `ContractDecl` instead. Existing annotate sites may remain until migrated.

## flag / help / schema homology

- Decision (path A — Contract/LeafSpec is CLI-surface authority **and must embed into Schema**): `docs/flag-help-schema-homology.md`
- Hard rule: every help/Schema fact is **declared** **or** **annotated**; never inference-only (§1.1–§1.3; framework §5.0).
- Embed path: `corecmd.New` → `dws.schema.*` annotations → Schema catalog assembly
- MCP metadata must not create CLI flags; optional 1:1 passthrough is a gated subset only.
- Gate IDs: `HOM-P*`, `HOM-S*`, `HOM-I1`, `HOM-D1` (see that doc §3–§4). `HOM-P1`/`HOM-D1`/`HOM-S1`/`HOM-S2` are on the `check-schema-catalog.sh` policy whitelist; remaining IDs land incrementally.

## Agent Schema contract

The Schema data flow is one way:

```text
1. app.NewRootCommand()
   └─ builds the real Cobra command tree and flags
   └─ leaf Safety / Contract / contract.ParamDecl declare ContractFinal (declare-or-annotate)

2. CollectIdentitySpecs (ContractFinal.Identity on live Cobra leaves)
   └─ forms EffectiveCommandRegistry
      └─ binds exactly to real Cobra leaves and aliases

3. Parameter resolution
   Cobra flags
   + contract.ParamDecl.Property / native annotations (primary property authority)
   + schema_parameter_mapping_ledger.go (mapping_exclusions / removals only;
     active bindings JSON retired after Track 1 Phase 2)
   └─ produces ParameterSpec and constraints

4. Agent and interface semantics
   ProductDecl + leaf ContractFinal Selection / Safety / Interface
   + contract.ParamDecl (interface_type / property)
   └─ resolves Agent metadata by source precedence
      Markdown is evidence only; it is not concatenated into final prose
   └─ schema_hints/ and schema_mcp_metadata.json are fully retired

  5. One typed hub
   BoundCommandRegistry
   + ParameterSpec
   + Agent metadata
   + Interface metadata
   └─ resolves every command exactly once into ToolSpec
   └─ aggregates SchemaRegistry + SchemaIndex
   └─ ResolveSchemaBuild assembles at runtime; deliverySchemaCatalog wraps it (lazy, sync.Once)

  6. Runtime delivery (no generate-written Catalog authority)
   SchemaRegistry
   └─ dws schema list/product/group/leaf/--all (-f json wire)
   └─ ResolveMeta projects Identity/Safety/Selection from the same registry
   └─ CI may dump Catalog via cmd_schema_catalog for jq gates / determinism
```

**Reviewed inputs / 评审输入** (organizational family under `internal/cli`;
parallel peers, not one merged authority). These are assembly inputs only —
never Catalog declaration authority, never leaf `Contract` / `ProductDecl`
substitutes. Keep them side-by-side; do **not** fold one into another:

| Input | Path | Owns |
|---|---|---|
| Command identity | collected from `ContractFinal.Identity` on live Cobra leaves (`schema_identity_collect.go`; not a file input) | stable identity, primary CLI path, aliases, navigation |
| Param concepts | `param_concepts.json` (+ `.schema.json`) | argv synonym / concept dictionary (reduced to `param_aliases_generated.go`) |
| Exclusions | `schema_command_exclusions.go` | exact reviewed CLI paths excluded from Schema (non-empty reason) |
| Mapping ledger | `schema_parameter_mapping_ledger.go` | `mapping_exclusions` / removals (CLI flags with no direct RPC property); active bindings JSON retired |

`schema_mcp_metadata.json` is retired and must not reappear. Interface facts
(`interface_ref`, `interface_type`, …) declare on leaf `Contract` /
`contract.ParamDecl`. Retiring the pin cleared MCP-sourced `interface_type`
values from the wire; schema-compat deliberately accepts clearing (missing =
unknown for consumers) while still rejecting any change to a different
non-empty value. Re-populating a value requires an explicit `ParamDecl`
declaration, not a new pin.

**Aliases are three distinct layers** (do not conflate):

| Layer | Owns |
|---|---|
| `FlagSpec.Aliases` / Cobra flag aliases | executable flag synonyms on a leaf |
| `ContractFinal.Identity` `aliases` | reviewed CLI-path aliases for the same command identity |
| `param_concepts.json` | argv synonym / concept dictionary (central preparse normalization) |

**Visibility vs exclusions:** collected identity `visibility` is dormant (all
entries default `public`); “runnable but not Agent-visible” belongs in
`schema_command_exclusions.go`, not new `visibility` values. Native identity
annotations are consistency assertions only — they must agree with the
collected identity and never materialize or override it.

Leaf declare (`Contract` / `ParamDecl` / `Safety` / `ProductDecl`) and the live
Cobra tree remain separate from this table: declare owns semantics; Cobra owns
executability and flags.

After binding there is no second identity source and no identity precedence
winner. The binder must reject a missing/non-runnable Cobra path, an alias
collision, and any native identity annotation that disagrees with the effective
registry. A missing native identity annotation is allowed because annotations
are implementation-side assertions, not identity fallbacks.

The assembler resolves every bound command exactly once into one `ToolSpec`.
CI determinism (`check-schema-assembly.sh`) and policy jq gates consume a
fresh assembly dump; runtime consumes the same `ResolveSchemaBuild` path via
`RegisterSchemaSourceRoot`. Neither path may reopen annotations, merge source
records, or use a previous Catalog JSON as a source.

### Assembly vs consumption

**Assembly** (declare → typed registry; CI + runtime):
- Runtime entry: `RegisterSchemaSourceRoot` (`internal/app`) →
  `ResolveSchemaBuild` / `deliverySchemaCatalog` (lazy, sync.Once).
- CI tool: `cmd_schema_catalog` dumps an assembled Catalog for jq/determinism;
  it is **not** a `//go:generate` or committed delivery step.
- `gen.go` only generates `param_aliases_generated.go`.
- Inputs: **reviewed inputs** (param_concepts / exclusions / mapping ledger —
  see table above) + ProductDecl/ContractFinal (identity is collected from
  `ContractFinal.Identity`) + live Cobra tree.
  `schema_hints/`, `schema_agent_metadata/`, `schema_command_registry/`, and
  `schema_mcp_metadata.json` must not reappear.
- Gates: `make generate-schema` (param aliases + assembly determinism),
  `check-generated-drift.sh`, `check-schema-catalog.sh`.

**Consumption** (runtime, unified API):
- Entry point: `ResolveMeta(cliPath) → CommandMeta{Identity, Safety, Selection}`
  in `internal/cli/command_meta.go` — projected from the assembled registry
  when the app factory is registered.
- Consumers: `--help` (Safety annotation via `RenderSafetyAnnotation`),
  agent selection, future skill generation; `dws schema` uses the same
  assembled Catalog (`-f json` wire unchanged).
- `SafetyForCLIPath` delegates to `ResolveMeta` (backward compatible).

This split is architecturally isomorphic to Lark's typed metadata registry,
navigation catalog, and schema renderer. DWS intentionally preserves its
existing flat JSON wire contract for compatibility; do not treat architectural
alignment as permission to make an unversioned wire-format change.

The identity collected from `ContractFinal.Identity` (via
`CollectIdentitySpecs`) is the sole source of stable command identity and
navigation. The executable Cobra tree remains the source of truth for whether
a CLI path exists, is runnable, and which flags it accepts. Schema coverage is
bidirectional:

1. Every final `SchemaRegistry` tool, including its serialized Catalog
   projection, must resolve to an executable Cobra command.
2. Every public runnable Cobra leaf must either resolve to Schema or appear as
   an exact, reviewed exclusion with a non-empty reason in
   `internal/cli/schema_command_exclusions.go` (central Go groups; not JSON).

Do not use prefix or wildcard exclusions: they can silently hide future
commands. Remove an exclusion when its command enters Schema; stale, invalid,
or duplicate exclusions must fail generation and CI.

When adding or changing an Agent-visible command, review all relevant inputs:

- Leaf `ContractFinal.Identity` for canonical identity, primary CLI path,
  aliases, and stable navigation. Identity is collected from the live Cobra
  leaves (`CollectIdentitySpecs`); there is no separate identity file. Invalid
  canonical paths, alias collisions, stale paths, and drift fail collection,
  binding, and policy.
- Leaf `Safety` / `Contract` (`corecmd.ContractDecl`) / `contract.ParamDecl`
  (helpers `LeafSpec` or shortcut `Contract`) for parameter facts, interface
  disposition, safety, and Agent selection prose. Delivered provenance is
  `contract_final` from `corecmd.contract` (description may stamp `cobra_help`
  when Cobra Long wins). Product routing uses `ProductDecl`
  (`internal/corecmd/contract`; provenance label remains `cli.product_decl`).
- `internal/cli/schema_hints/` is fully retired. Do not reintroduce HintFiles,
  audit JSON, or `imported/` baselines; declare on ProductDecl / the owning
  leaf instead.
- Native Runtime Schema identity annotations, when present, as consistency
  assertions against `EffectiveCommandRegistry`. They must agree exactly and
  must never materialize, infer, or override registry identity.
- Flag-to-interface property mappings and required/default semantics.
- Do not expect generate-written Catalog delivery. Run
  `make generate-schema` only to refresh param aliases and prove assembly
  determinism. Do not expect or commit `schema_agent_metadata/`.

Run the reverse-completeness tests whenever the Cobra tree changes. A command
that works through `dws <path>` but cannot be found through the matching
`dws schema` lookup is a contract failure unless it has a reviewed exact
exclusion.

`RegisterSchemaHints` / `ToolSchemaHint` overlays are fully removed. Parameter
and selection facts must be declared on the owning leaf (`contract.ParamDecl` /
`Contract`) or via `ProductDecl`; do not reintroduce overlay registries.

For Agent-authored selection edits:

1. Confirm the exact command and flag names in the current Cobra tree.
2. Declare selection prose on the owning leaf (`Contract.Selection` /
   `DeclareLeafMetadata`) and product routing via `ProductDecl`; declare
   safety / parameters / interface on the same leaf.
3. Do not copy generated Catalog fields into source inputs.
4. Run generation, drift, Schema policy, and the focused CLI tests before
   proposing the change.

## Agent curation workflow

Use this workflow when refreshing Agent selection prose and confirmation
alignment. Prefer **agent-authored review** over bulk merge scripts that dump
Skill Markdown into Catalog fields.

Human-authored inputs:

| Block | Path | Owns |
|---|---|---|
| **declaration** | helpers / shortcut `Safety` + `Contract` / `contract.ParamDecl` + `ProductDecl` | `effect` / `risk` / `confirmation` / `idempotency` / `interface_*` / parameter facts / selection prose (`contract_final`) |

`schema_hints/` is fully retired. Do not reintroduce HintFiles or audit JSON.

### Goals

1. **Selection prose** is decision-oriented (Feishu/Lark style): trigger intent,
   sibling-command routing, and outcome shape — not a restatement of the
   summary. Delivered Catalog provenance is `contract_final` from leaf
   `Contract.Selection` / `ProductDecl`.
2. **Safety** follows Runtime: `confirmation=user_required` when the leaf
   Contract/Safety (or remaining `runtime_gate` annotate) requires a user gate
   (for example `confirm_delete`, `typed_yes`, `confirm_dangerous`).
3. **Parameter facts** are declared on the leaf (`contract.ParamDecl` /
   `Contract.Parameters` / FlagSpec). Do not reintroduce HintFile or
   `RegisterSchemaHints` overlays.

### Authoring

For every curated tool:

1. Declare safety/interface/parameters/selection on the owning leaf
   (`DeclareLeafMetadata` / `Shortcut.Contract` / `contract.ParamDecl`) and product routing
   via `ProductDecl` when needed.
2. Run `make generate-schema` (param aliases + assembly determinism). Do not
   create or commit `schema_catalog/` or Schema meta-index fixtures.

### Pull live MCP descriptions (personal token)

Schema delivery no longer embeds a pinned MCP JSON. Prefer live Schema from a
logged-in personal session when reviewing interface facts before declaring them
on the leaf:

```bash
dws auth status                 # token_valid should be true
dws schema <mcp-canonical> --jq '{canonical_path,interface_ref,parameters}' -f json
# or CLI path: dws schema --cli-path "drive copy" --jq '{canonical_path,interface_ref,parameters}' -f json
```

Resolve MCP identity via declared `interface_ref` when CLI canonical ≠ MCP path
(example: CLI `drive.copy_document` → live `doc.copy_document`). On pull
failure, fall back to Skill + Cobra Help, and record evidence
(for example `live-dws-schema:<path>#FAILED`). Never print or commit tokens.
`make fetch-mcp-metadata` writes an optional diagnostic dump under `artifacts/`
only — do not commit it as a Schema pin.

Precedence when sources disagree: **Runtime/Cobra / leaf Contract > live MCP >
Skill (evidence only)**.

### Parallel product agents

Split work by product groups. Each agent must:

- Read Skill, Cobra/`--help`, Runtime confirmation sites, and live
  `dws schema <leaf> --compact` for its tools. Mapping/interface/provenance
  audits may query the full leaf only through a narrow `--jq` / `--fields`
  projection; do not load an entire full leaf into Agent context.
- Hand-write selection prose and leaf Contract / ProductDecl declarations;
  forbid wholesale JSON merges from review dumps.
- Edit only its product’s leaf declarations (and `ProductDecl` when needed).
- **Never** `git checkout` unrelated product files to “clean scope”.

### Regenerate and gates

```bash
make generate-schema
./scripts/policy/check-runtime-confirmation-truth.sh
go test ./internal/app -run '^TestSheetFinalSchemaConfirmationMatchesRuntimeGuards$' -count=1
```

`check-runtime-confirmation-truth.sh` compares live ContractFinal.Safety with the assembled ToolSpec `confirmation=user_required` and probes the runtime gate.
`schema_hints/` must stay absent.

Example rules (fail generation otherwise):

- At most two examples per tool; no `--yes` in stored examples.
- Examples must match live Cobra argv (path, flags, required groups).
- No shell comments in examples.

After generation, spot-check Catalog: selection and safety/interface
provenance are `contract_final` from ProductDecl / leaf declarations
(`user_required` must match Runtime confirmation gates).

`make generate-schema` refreshes `param_aliases_generated.go` and runs
assembly determinism (`check-schema-assembly.sh`). It does not rewrite a
committed Catalog as delivery authority — runtime reassembles from
declarations. Byte guards fail if generation mutates parameter-concept
inputs; policy fails if the retired `schema_command_registry/` reappears.

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
`DWS_AGENT_SELECTION_LIVE=1 ARK_API_KEY=... ARK_BASE_URL=... ARK_MODEL=... go test ./internal/app -run TestAgentSelectionArkLive -count=1`.
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
final Agent projection must keep `required=true` and cannot be lowered by a
lower-precedence source. A higher-precedence declaration may still raise an
optional flag to required. `cli_required` continues to mirror the executable
Cobra marker.

For command-level description: **declare required, delivery Long may win**.
`ContractDecl.Description` is mandatory at construction (declaration evidence).
Catalog delivery prefers Cobra Long when present (provenance `cobra_help`,
resolution `cobra_help_preferred`); without Long, the declared Description is
delivered as `contract_final`. Title keeps declared ContractDecl /
ContractFinal first, then Cobra Short, then MCP metadata. This is one authority
chain with an explicit delivery preference — not two competing sources.
Generic RPC prose may remain an unselected provenance candidate (and
parameter-level `interface_description`); it must not overwrite a specialized
leaf's title or description.

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

## Unified result Schema and performance

The unified runtime envelope and the per-command Schema result declaration are
related but distinct contracts:

- Runtime owns the outer machine envelope (`ok`, `outcome`, `data`, `error`,
  `meta`) and derives it through `internal/output`. Business commands return a
  `CommandResult`; they must not hand-author the outer JSON shape.
- A leaf `Contract.Result` / `contract.ResultSpec` describes the reviewed
  business value inside `data`. It may declare `outcomes`, `data_schema`, and
  `sensitive_paths`. `Contract.Pagination` is a separate command capability
  because pagination is emitted under envelope `meta`, not inside `data`.
- `outcomes` is the set of results a command may produce; it is not the outcome
  of the current invocation. `data_schema` is a JSON Schema object for business
  data and must not duplicate the framework envelope.
- Result declarations are delivered in the full leaf and in the reviewed
  `--compact` Agent projection. Compact retains the normalized `result` object
  verbatim but still omits provenance, interface bindings, and other audit-only
  fields. Product/group summaries remain navigation views and need not repeat
  every leaf Result. When an Agent needs return-shape facts, query the compact
  leaf directly; do not load the whole full Catalog.
- A missing `result` means “no reviewed return-value declaration is published
  for this leaf.” It does **not** prove that the runtime is legacy, and it must
  not be filled by inference from examples, MCP samples, or previous command
  output. Runtime rollout remains an internal per-command fact.
- The public contract has no `contract_version`, no `--output-contract`, and no
  Agent-selectable protocol alias. Agents continue to request machine output
  with `--format json`; migrated commands use the unified result directly and
  unmigrated commands retain their current legacy output.
- Existing `dev` / `devapp` pilot coverage is gradual. Active reviewed
  `devapp` shortcuts are gated on a non-empty Result declaration, while `dev`
  currently has representative Result coverage. Do not describe that as
  repository-wide coverage. Any newly activated Agent-visible command should
  add and test its Result declaration; the remaining pilot gaps should shrink,
  not expand.

The compact/full leaf `result` object has one stable shape:

```json
{
  "result": {
    "outcomes": ["success", "pending", "partial_failure", "failure"],
    "data_schema": {
      "type": "object",
      "properties": {
        "items": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "id": {"type": "string", "description": "Stable resource ID"},
              "name": {"type": "string", "description": "Display name"}
            }
          }
        }
      }
    },
    "sensitive_paths": ["credential.secret"]
  },
  "pagination": {
    "kind": "cursor",
    "cursor_parameter": "cursor",
    "meta_path": "meta.pagination",
    "endpoint_exhausted_path": "meta.pagination.endpoint_exhausted",
    "next_token_path": "meta.pagination.next_token"
  }
}
```

Field rules:

| Field | Required | Contract |
|---|---|---|
| `outcomes` | yes | Non-empty unique subset of `success`, `pending`, `partial_failure`, `failure`; normalization publishes canonical order. |
| `data_schema` | yes | One recursive JSON Schema **object** describing only the runtime envelope's `data` value. Every named `properties` child must have a non-empty `description`. It must not duplicate `ok`, `outcome`, `error`, or `meta`. |
| `sensitive_paths` | no | Unique safe dot paths relative to `data`; renderers/redaction consumers must not treat them as shell/JQ expressions. |

Optional members are omitted, never emitted as `null`. A leaf without a
reviewed Result omits the entire `result` key. Compact must preserve the same
normalized Result value as the full leaf; it must not summarize, infer, rename,
or independently rebuild any Result field. Product/group summaries do not
aggregate child Result objects.

`pagination` is a sibling of `result`, not a child. It declares the canonical
CLI cursor parameter and the fixed framework paths under `meta.pagination`.
Product response fields used to derive that metadata remain mapper internals;
they are not part of `result.data_schema`. Do not execute a second request to
derive pagination metadata.

Invalid result declarations fail closed during normalization: unknown or
duplicate outcomes, a non-object/multiple `data_schema`, unsafe or duplicate
sensitive paths, unsupported pagination kinds, attempts to override framework
meta paths, and an invalid cursor parameter must be rejected rather than
silently removed.
Full-leaf wire round trips must
preserve the normalized Result exactly. Do not commit generated Schema JSON as
evidence; tests construct contracts in Go and runtime/CI assemble the Catalog
from declarations.

### Performance model and rules

- Catalog construction is declaration-driven and cached through the existing
  lazy `sync.Once` delivery path. Do not reassemble or reopen annotations per
  command invocation, per leaf lookup, or per renderer.
- Normalizing one Result declaration is linear in the size of that declaration.
  Full `schema --all` is linear in tools + parameters + Result schema bytes and
  is an audit/compatibility export, not the normal Agent discovery path.
  Overview → compact product/group → compact leaf remains the normal route;
  only the final leaf carries its Result declaration.
- Constructing a `CommandResult` defensively clones result data and validates
  invariants; rendering is buffer-first and then writes once. Both CPU cost and
  transient memory are O(payload size), with roughly one additional in-memory
  rendered copy. This buys immutability and prevents partial JSON leakage, but
  it is not free.
- Large list/search commands must use bounded pages and publish continuation
  facts. The current emitter buffers one command result/page before publishing;
  pagination is the memory bound. Continuous event streams are a separate,
  command-specific protocol and are not described by `ResultSpec`.
- A `dual_validate` command must execute the business request exactly once,
  validate a shadow unified result, and preserve legacy bytes. Never obtain
  validation by issuing a second network or write request.
- Filters and alternate formats are render-time work over the same in-memory
  result. They must not rerun the business operation or rebuild Schema.
- Performance changes must preserve the one-result, buffer-first, fail-closed,
  and atomic `--output` guarantees. Do not trade correctness for a microbenchmark
  improvement. For a material hot-path change, benchmark representative small
  and page-sized payloads and report allocations/bytes as well as latency.

## Current Schema boundaries

- `schema list` remains a progressive overview. `schema --all` is the stable
  full-export contract: every final `SchemaIndex` tool must contain its
  complete leaf parameters, constraints, and safety semantics, including an empty
  `parameters` object for commands without flags. Keep it suitable for the #602
  compatibility baseline and fail rather than silently emitting a partial
  export.
- `schema --all` is not normal command discovery. Use overview -> compact
  product/group -> compact leaf for routine Agent work. `--compact` is the
  reviewed positive-field allowlist for Agent context: new full/audit fields
  must not appear there until explicitly reviewed. A compact full export is not
  a complete compatibility baseline.
- `dws <path> --help` defines whether Cobra exposes a path and which flags the
  executable accepts. A compact leaf defines Agent selection, CLI parameters,
  constraints, safety/confirmation semantics, and any reviewed `result`
  contract. Full leaf fields such as `property`, `interface_ref`, and
  provenance are audit facts. A conflict is contract drift, not permission to
  guess.
- Schema and Help describe commands; neither returns DingTalk business data.
  After discovery, execute the real read/search/list command to obtain data.
