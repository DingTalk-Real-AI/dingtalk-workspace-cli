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

## Agent Schema contract

The executable Cobra tree is the source of truth for public CLI commands.
Schema coverage is bidirectional:

1. Every Catalog tool must resolve to an executable Cobra command.
2. Every public runnable Cobra leaf must either resolve to Schema or appear as
   an exact, reviewed exclusion with a non-empty reason in
   `internal/cli/schema_command_exclusions.json`.

Do not use prefix or wildcard exclusions: they can silently hide future
commands. Remove an exclusion when its command enters Schema; stale, invalid,
or duplicate exclusions must fail generation and CI.

When adding or changing an Agent-visible command, review all relevant inputs:

- `internal/cli/schema_command_surface.json` for the stable public surface.
- Runtime Schema annotations/root hints for the canonical command identity.
- Flag-to-interface property mappings and required/default semantics.
- `skills/mono/schema-hints/` for selection, interface, and safety metadata.
- Generated files under `internal/cli/schema_agent_metadata/` and
  `internal/cli/schema_catalog.json` after running generation.

Run the reverse-completeness tests whenever the Cobra tree changes. A command
that works through `dws <path>` but cannot be found through the matching
`dws schema` lookup is a contract failure unless it has a reviewed exact
exclusion.

## Safety metadata

Metadata from canonical paths and CLI aliases may reconcile to the same live
tool. Safety merges are conservative: `high` outranks `medium` and `low`, and
`user_required` outranks `not_required`. Inferred or default metadata must not
downgrade a stricter reviewed/imported contract.

For destructive or high-risk operations:

- Require `risk=high` and `confirmation=user_required`.
- Keep CLI confirmation behavior and Schema metadata consistent.
- Add a semantic regression test for the final generated Catalog, not only a
  generator unit test.

## Current Schema boundaries

- `schema list` and `schema --all` currently expose overview/tool summaries,
  not a complete parameter baseline suitable for the #602 compatibility gate.
  Do not claim full parameter-loss coverage until a stable full-export contract
  is implemented.
- Lazy-loading the embedded Catalog alone does not prove lower startup cost:
  root construction still annotates commands and other embedded metadata may
  load eagerly. Measure end-to-end startup before making performance claims.
