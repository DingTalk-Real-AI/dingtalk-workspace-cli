# DWS Agent Schema Hints

This directory contains versioned, structured Agent metadata. Files are build inputs for `internal/cli/schema_agent_metadata/`; generated runtime metadata must not be edited directly.

## Source kinds

- `explicit`: reviewed DWS hints. Scalar fields override imported baselines.
- `imported`: sanitized metadata from a fixed external revision. It fills missing Agent semantics but cannot redefine command paths or parameter contracts.

The Agent metadata generator also reads the committed `internal/cli/schema_mcp_metadata.json` after Skill and Hint parsing. A sanitized MCP description can fill an otherwise empty `agent_summary`; it is marked `reviewed: false`, retains revision provenance, and cannot infer or override risk/effect fields.

Tool keys should use stable `canonical_path` values from `internal/cli/schema_command_surface.json`. CLI paths and aliases are also accepted and are reconciled to the canonical public tool during generation.

`selection-review.json` fixes the reviewed selection contract for every public
tool: `use_when`, `avoid_when`, safe examples, `interface_mode`, availability,
and the reason for local, composite, or unavailable implementations. These
values are build inputs; the generator must not derive them from the previous
Catalog.

`reference-review.json` classifies every Skill command reference that is not a
current public leaf. `alias` entries bind an old or cross-product path to an
explicit current target. `group`, `stale`, and `out_of_surface` entries remain
visible in the audit but are never fuzzy-matched to a leaf.

`interface_ref` is a separate interface binding. Use it when a public helper/canonical tool calls a differently named MCP RPC or another source product:

```json
{
  "version": 1,
  "source": {"kind": "explicit", "name": "reviewed-interface-map"},
  "tools": {
    "chat.bot_search": {
      "interface_ref": {
        "product_id": "bot",
        "rpc_name": "search_my_robots"
      }
    }
  }
}
```

An entry containing only `interface_ref` participates in interface projection but does not count as Agent semantic coverage. It cannot add a command, change a flag, or expose a Wukong-only tool.

`interface_mode` has four reviewed values:

- `mcp`: exactly one fixed `interface_ref` implements the command.
- `composite`: multiple RPC/local steps implement the command; a singular ref would be misleading.
- `local`: the command only changes local process or policy state.
- `unavailable`: the compatibility command is retained but no reviewed backend is shipped.

The missing `notify` MCP service is separately dispositioned in
`internal/cli/schema_mcp_service_review.json`; it is outside the public command
surface and must not trigger runtime discovery.

`internal/cli/schema_mcp_metadata.json.coverage.surface_tools` describes only
the immutable MCP import at its declared `source_revision`; it is not the
current CLI/Catalog tool count. Its `coverage.surface_scope` must remain
`source_revision`, and policy verifies the snapshot's internal matched and
unmatched arithmetic. Current Catalog interface coverage is instead proved for
every generated tool: each tool must have one valid `interface_mode` /
availability disposition and retain provenance to `selection-review.json` or
`runtime-surface-completeness.json`. This makes newly added CLI tools explicit
without rewriting historical MCP evidence.

Interface metadata may enrich type and description, but MCP `required` never promotes an optional Cobra flag. CLI required/one-of/conditional rules must be represented by current Cobra markers, typed runtime constraints, or reviewed parameter hints.

```json
{
  "version": 1,
  "source": {
    "kind": "explicit",
    "name": "calendar-schema-review"
  },
  "products": {
    "calendar": {
      "agent_summary": "管理日程、参与人、会议室和闲忙信息"
    }
  },
  "tools": {
    "calendar.get_calendar_detail": {
      "agent_summary": "读取一个日程的完整详情",
      "use_when": ["已经取得 eventId，需要查看详情"],
      "effect": "read",
      "reviewed": true
    }
  }
}
```

Run `make generate-schema` after changing Hint or Skill sources. External Wukong metadata must be refreshed by the controlled offline import pipeline with an immutable revision, then committed together with its audit before regenerating the Catalog; runtime refresh is forbidden.

## Manual Schema hints

Agent semantic hints in this directory do not change the executable CLI
contract. When an existing public Cobra leaf needs to enter Schema or its
CLI-facing parameter projection needs a reviewed correction, edit
`internal/cli/schema_manual_hints.json` instead. Each entry must use one exact
`cli_path`, one canonical path, `reviewed: true`, and a non-empty reason.
The file declares `internal/cli/schema_manual_hints.schema.json` through its
top-level `$schema` field. Agents and editors should use that schema as the
field-level source of truth instead of inferring the format from generated
Catalog JSON.

Manual hints may override Schema description, interface-property/type mapping,
`required`, and `required_when` for flags that already exist on that command.
They cannot create a command or flag, target a hidden/group command, define an
interface, or mark an unknown RPC available. Missing commands and flags,
wildcards, canonical conflicts, duplicate paths, and unreviewed entries fail
generation.

Commands intentionally kept outside Schema remain in the separate exact
reviewed exclusion file `internal/cli/schema_command_exclusions.json`. An
included command cannot also remain excluded: completeness validation treats
that exclusion as stale.

### Agent editing workflow

1. Locate the real Cobra leaf and verify its exact path and current flags.
2. Read `internal/cli/schema_manual_hints.schema.json`; preserve `$schema` and
   `version` in the data file.
3. Add only fields that need review. Do not repeat generated Agent metadata,
   interface availability, examples, risk, or confirmation here.
4. Use `property` and `interface_type` only for a real CLI-to-interface
   conversion. `required` and `required_when` describe the Schema projection;
   they do not modify Cobra execution validation.
5. Run:

   ```bash
   make generate-schema
   ./scripts/policy/check-generated-drift.sh
   ./scripts/policy/check-schema-catalog.sh
   go test ./internal/cli ./internal/app
   ```

6. Review the generated Catalog diff. A typical Manual Schema Hint change
   should affect the intended tool and hashes, not unrelated commands.
