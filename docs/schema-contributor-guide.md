# Schema and Agent Contract Contributor Guide

Read this guide when changing public CLI commands, Schema identity or
parameters, Agent selection/safety metadata, or generated Schema assets.

## Ownership and data flow

The publication graph is one way:

```text
Cobra command tree
  + reviewed CommandRegistry identity/navigation
  + reviewed metadata parameter overlays
  -> EffectiveCommandRegistry and executable binding
  + parameter bindings
  + reviewed selection and safety/interface metadata
  + pinned MCP metadata
  -> one resolved ToolSpec registry/index
  -> generated Agent metadata and embedded Schema Catalog
  -> dws schema projections and runtime metadata lookup
```

The owning sources are:

| Concern | Authoritative input |
|---|---|
| Executable paths and accepted flags | Cobra tree built by `app.NewRootCommand()` |
| Stable canonical identity, primary path, aliases, navigation | `internal/cli/schema_command_registry.json` |
| Registry editing contract | `internal/cli/schema_command_registry.schema.json` |
| Safety, interface, runtime gates, parameter overlays | `internal/cli/schema_hints/metadata/<product>.json` |
| Agent selection prose and examples | `internal/cli/schema_hints/selection/<product>.json` |
| Product-to-hint-file routing | `internal/cli/schema_hints/index.json` |
| Flag/property bindings | `internal/cli/schema_parameter_bindings.json` |
| Sanitized interface fallback | `internal/cli/schema_mcp_metadata.json` |
| Exact reviewed omissions from Schema | `internal/cli/schema_command_exclusions.json` |

Generated files under `internal/cli/schema_agent_metadata/` and
`internal/cli/schema_catalog.json` are output only. Runtime loading is a delivery
boundary: it must not create or repair commands, flags, registry entries, or
generation inputs.

## Invariants

1. Every delivered tool resolves to a public runnable Cobra leaf.
2. Every public runnable Cobra leaf resolves to Schema or has one exact,
   reviewed exclusion with a non-empty reason. Wildcard/prefix exclusions are
   forbidden.
3. The reviewed CommandRegistry is the only stable identity/navigation source.
   Native annotations, when present, are consistency assertions and must agree.
4. Metadata overlays may describe or constrain real flags; they cannot create
   commands, flags, interfaces, or unknown RPCs.
5. Cobra-required flags are a hard floor. An overlay may make an optional flag
   required but cannot make a Cobra-required flag optional.
6. Each tool is resolved once into one typed `ToolSpec`; all Catalog, `schema
   --all`, leaf, summary, safety, and runtime projections derive from it.
7. Provenance winner values must equal delivered values. Same-precedence
   conflicts fail instead of being merged silently.
8. `confirmation=user_required` requires user confirmation before `--yes`.
   Do not infer confirmation mechanically from risk/effect; keep runtime gates
   and published metadata consistent.
9. Stored examples use real primary/alias paths and accepted flags, satisfy all
   required/constraint rules, contain no shell comments, and never add `--yes`.
10. `schema --all` remains the complete compatibility export. Routine discovery
    should use overview, product/group, then leaf queries.

When Help and shipped Schema disagree, treat it as contract drift. Cobra still
defines executable flags; use the safer interpretation for confirmation or
stop rather than guessing.

Parameter and safety resolution is source-precedence based and otherwise
value-neutral: a value must not win merely because it looks stricter. Preserve
all candidates and the selected source, and fail same-precedence conflicts.
Command text resolves from reviewed tool hints, then command-specific Cobra
help, then MCP metadata; generic RPC prose must not replace a specialized
leaf's description. An alias lookup may change only view fields such as
`cli_path` and `is_alias`, never the resolved command contract.

## Editing workflow

1. Confirm the live path and flags in the Cobra tree and current `--help`.
2. Change only the owning reviewed block. Do not copy generated Catalog fields
   into inputs or mix selection fields into metadata files.
3. Keep registry edits limited to intentional identity/navigation changes.
4. For selection prose, write decision-oriented routing: when to choose the
   command, when a sibling is better, and the result shape. Do not restate help.
5. For parameter overlays, use an exact runnable leaf and real flags; set
   `reviewed: true` with a concrete review reason.
6. Regenerate the complete snapshot; publication is deterministic even when
   only one product input changed.
7. Inspect authored and generated diffs separately, then run the gates below.

Pinned MCP metadata is a sanitized fallback. When a task requires refreshing
it and a personal session is available, inspect live metadata with `dws auth
status`, `dws cache refresh`, and `dws schema <canonical> -f json`. Never print
or commit tokens. Evidence precedence is Runtime/Cobra, live MCP, pinned MCP,
then Skill prose as evidence only.

## Required checks

```bash
make generate-schema
./scripts/policy/check-runtime-confirmation-truth.sh
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-schema-catalog.sh
./scripts/policy/check-command-surface.sh --strict
make test-schema-agent-examples
```

Also run focused tests for the changed binder, generator, command, or runtime
consumer. Run reverse-completeness tests whenever the Cobra tree changes.

Agent examples are contract-checked by default. Eligible reviewed dry-run
examples are additionally exercised by `make test-schema-agent-examples` with
isolated state; a runtime failure must not be converted into an ad hoc skip.
Live-model selection evaluation is optional and never a normal CI dependency.

An example enters runtime dry-run only when its final typed contract publishes
an explicit reviewed dry-run capability. Risk or confirmation metadata does
not manufacture preview support, and the harness never injects `--yes`. A
narrow precondition that cannot be derived from the contract may use an exact,
reviewed `example_dispositions` entry to narrow dry-run to contract-only; it
must not become a general skip or a fallback applied after execution fails.

Every `use_when` entry is a positive selection fixture and every `avoid_when`
entry is a negative fixture for that tool. The deterministic gate checks
coverage and contradictions; it does not claim to prove natural-language
understanding. When explicitly requested, the optional live-model smoke test
can be run with:

```bash
DWS_AGENT_SELECTION_LIVE=1 \
ARK_API_KEY=... ARK_BASE_URL=... ARK_MODEL=... \
go test ./internal/app -run TestManualAgentSelectionArkLive -count=1
```

Use `DWS_AGENT_SELECTION_FULL=1` for the full fixture or
`DWS_AGENT_SELECTION_CASES=<comma-separated-ids>` for selected cases. A custom
HTTPS provider must be explicitly allowlisted; plaintext is accepted only for
a loopback test server so credentials are not sent over arbitrary clear text.

## Runtime boundaries

- `schema list` is a progressive overview; `schema --all` is the complete,
  non-compact compatibility baseline with full parameters, constraints, and
  safety semantics.
- `--compact` saves discovery context but is not a full compatibility export.
- `dws <path> --help` decides whether a path and its flags are executable. Leaf
  Schema owns Agent selection, mapping, constraints, and safety semantics.
- Help and Schema describe commands; they do not return DingTalk business
  data. Execute the real read/list/search command after discovery.
