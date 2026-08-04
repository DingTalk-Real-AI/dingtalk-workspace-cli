# Schema Runtime and Publication Agent Guide

This file applies to `internal/cli/`. Read
[`docs/schema-contributor-guide.md`](../../docs/schema-contributor-guide.md)
before editing.

## Owning inputs

| Change | Edit |
|---|---|
| Canonical identity, primary path, aliases, navigation | `schema_command_registry/` |
| Reviewed parameter synonym policy | `param_concepts.json` |
| Parameter/property mapping | `schema_parameter_bindings.json` or reviewed metadata overlay |
| Safety, interface, runtime gate | `schema_hints/metadata/<product>.json` |
| Agent selection and examples | `schema_hints/selection/<product>.json` |
| Exact reviewed omission | `schema_command_exclusions.json` |
| Runtime query/projection behavior | Go implementation and focused tests in this package |

The executable Cobra tree outside this package owns whether a command exists
and which flags it accepts. Do not create a command or flag in Schema inputs.

## Generated boundary

- `schema_catalog/`, `schema_agent_metadata/`, and
  `param_aliases_generated.go` are generated outputs.
- Never hand-edit, merge from, or use a previous generated output as an input.
- Change the owning reviewed input or generator, run `make generate-schema`,
  and inspect authored and generated diffs separately.
- A broad unrelated generated diff is a failure signal, not acceptable churn.
- Runtime consumers use `ResolveMeta` from `command_meta.go`; do not reopen
  authored inputs or generated JSON in a second resolution path.

## Self-check

- Every public runnable Cobra leaf is bound or has one exact reviewed
  exclusion; every delivered tool binds back to a runnable leaf.
- Registry identity and native annotations agree; aliases do not mutate the
  resolved contract.
- Parameters reference real flags and retain Cobra-required floors.
- Selection examples use executable paths/flags and never include `--yes`.
- Runtime confirmation gates and published safety metadata agree.
- Full, compact, summary, alias, and Catalog projections derive from the same
  typed `ToolSpec` and preserve provenance winners.

Run the focused package/generator tests and the complete command list in
`docs/schema-contributor-guide.md`, beginning with `make generate-schema`.
