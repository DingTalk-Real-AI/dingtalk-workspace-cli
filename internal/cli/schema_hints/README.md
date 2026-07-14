# DWS Agent Schema Hints

This directory contains versioned, structured Agent metadata. Hints belong to
the CLI Schema subsystem rather than either installable Skill layout. They are
excluded from embedded binaries and release Skill bundles. Files generate
`internal/cli/schema_agent_metadata/`; generated runtime metadata must not be
edited directly.

## Layout

Human-authored inputs live in two blocks:

- `index.json` (`format: dws-agent-hint-index`) — maps product IDs to metadata
  and selection files, plus reference review
- `metadata/<product>.json` — command metadata: safety, interface,
  `runtime_gate`, and optional parameter overrides
- `selection/<product>.json` — Agent selection prose
- `imported/` — sanitized baseline from a fixed external revision

When `index.json` is present, the generator loads `imported/` plus the
metadata and selection files listed in the index. Sibling review JSON files in
this directory remain CI/audit inputs and are not applied as Agent metadata
sources.

For the end-to-end Agent curation workflow, see `AGENTS.md` § “Agent curation
workflow (Schema hints)”.

## Source kinds

- `reviewed_explicit`: reviewed metadata or selection HintFiles under
  `metadata/` / `selection/` (`reviewed: true`). Selection prose is delivered
  only from `selection/`.
- `explicit`: explicit but not per-tool-reviewed DWS hints.
- `imported`: sanitized metadata from a fixed external revision. It fills missing Agent semantics but cannot redefine command paths or parameter contracts.

Skill Markdown and audit JSON in this directory remain authoring evidence.
Normal generation does not semantically combine Markdown into the final Agent
prose and never rewrites metadata/selection files.

The Agent metadata generator also reads the committed `internal/cli/schema_mcp_metadata.json` after Skill and Hint parsing. A sanitized MCP description can fill an otherwise empty `agent_summary`; it is marked `reviewed: false`, retains revision provenance, and cannot infer or override risk/effect fields.

Tool keys should use stable `canonical_path` values from `internal/cli/schema_command_registry.json`. CLI paths and aliases are also accepted and are reconciled to the canonical public tool during generation.

`selection-review.json` fixes the reviewed command-selection contract for every
public tool: `use_when`, `avoid_when`, safe examples, and the ordinary direct
interface disposition. Exceptional or formerly unmatched wrappers are owned by
the interface-only reviewed source `zz-interface-disposition-review.json`.
`runtime-surface-completeness.json` remains unreviewed selection evidence and
must not promote its other fields merely to classify an interface. These values
are build inputs; the generator must not derive them from the previous Catalog.

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

`interface_mode` and `availability` are orthogonal reviewed fields.
`interface_mode` has exactly three values:

- `mcp`: exactly one pinned `interface_ref` implements the command, with an auditable parameter mapping and semantically equivalent execution contract.
- `composite`: multiple RPCs, conditional routing, local projection, or a reviewed remote adapter absent from pinned metadata implements the command; a singular ref would be misleading.
- `local`: the command is fully implemented by the local process, static data, or local policy. An unpinned remote RPC is never `local`.

`availability` is `available` or `unavailable`. An unavailable command keeps
its real implementation mode, must not carry `interface_ref`, and must include
an explicit `interface_reason`; `unavailable` is never an interface mode.

The missing `notify` MCP service is separately dispositioned in
`internal/cli/schema_mcp_service_review.json`; it is outside the public command
surface and must not trigger runtime discovery.

`internal/cli/schema_mcp_metadata.json.coverage.surface_tools` describes only
the immutable MCP import at its declared `source_revision`; it is not the
current CLI/Catalog tool count. Its `coverage.surface_scope` must remain
`source_revision`, and policy verifies the snapshot's internal matched and
unmatched arithmetic. Current Catalog interface coverage is instead proved for
every generated tool: each tool must have one valid `interface_mode` /
availability disposition and retain reviewed provenance to
`selection-review.json`, `zz-interface-disposition-review.json`, or another
reviewed interface source. This makes newly added CLI tools explicit without
rewriting historical MCP evidence or promoting an unreviewed selection hint.

Interface metadata contributes lower-priority typed candidates, including
`required`; source precedence is value-neutral for most fields, so a candidate
may raise or lower the Agent-facing value when no higher-priority source wins.
It cannot override reviewed manual hints, versioned bindings, typed/native
metadata, or current Cobra/constraint facts for those fields. `required` is
special: Cobra `MarkFlagRequired` is a hard floor that cannot be projected away
as optional. `cli_required` remains the executable Cobra marker with its own
provenance.

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

## Metadata parameters and selection prose

Agent semantic hints in this directory do not change the executable CLI
contract beyond reviewed parameter overlays on metadata tools.

| Block | Owns |
|---|---|
| `metadata/<product>.json` | safety, interface, `runtime_gate`, optional `parameters` / `cli_path` |
| `selection/<product>.json` | `agent_summary`, `use_when`, `avoid_when`, `examples` |

Parameter overlays may override Schema description, interface-property/type
mapping, `required`, and `required_when` for flags that already exist on that
command. They cannot create a command or flag, target a hidden/group command,
define an interface, or mark an unknown RPC available. Missing commands and
flags, wildcards, canonical conflicts, duplicate paths, and unreviewed entries
fail generation.

Selection prose is committed as a reviewed source. `go generate` only reads,
validates, and projects this data. It must never call a model, copy a previous
Catalog, or overwrite metadata/selection files. Selection cannot change command
identity, flags, parameters, safety, or interface facts.

Commands intentionally kept outside Schema remain in the separate exact
reviewed exclusion file `internal/cli/schema_command_exclusions.json`. An
included command cannot also remain excluded: completeness validation treats
that exclusion as stale.

### Agent editing workflow

1. Locate the real Cobra leaf and verify its exact path and current flags.
2. Edit only the owning block (`metadata/` or `selection/`); do not mix fields.
3. Add only fields that need review. Do not copy generated Catalog fields into
   the input.
4. Use `property` and `interface_type` only for a real CLI-to-interface
   conversion. `required` and `required_when` describe the Schema projection;
   they do not modify Cobra execution validation.
5. Run:

   ```bash
   make generate-schema
   ./scripts/policy/check-generated-drift.sh
   ./scripts/policy/check-schema-catalog.sh
   ./scripts/policy/check-runtime-confirmation-truth.sh
   go test ./internal/cli ./internal/app
   ```

6. Review the generated Catalog diff. A parameter overlay should affect only
   the intended contract. A selection edit updates prose provenance to
   `reviewed_explicit` from `selection/`.
