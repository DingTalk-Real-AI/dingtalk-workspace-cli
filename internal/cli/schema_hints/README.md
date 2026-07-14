# DWS Agent Schema Hints

This directory contains versioned, structured Agent metadata. Hints belong to
the CLI Schema subsystem rather than either installable Skill layout. They are
excluded from embedded binaries and release Skill bundles. Files generate
`internal/cli/schema_agent_metadata/`; generated runtime metadata must not be
edited directly.

## Source kinds

- `reviewed_manual`: the selected Agent prose committed under
  `agent_hints` in `internal/cli/schema_manual_hints.json`. It has the highest
  precedence for `agent_summary`, `use_when`, `avoid_when`, and `examples`.
- `explicit`: reviewed DWS hints. Scalar fields override imported baselines.
- `imported`: sanitized metadata from a fixed external revision. It fills missing Agent semantics but cannot redefine command paths or parameter contracts.

Skill Markdown and the structured files in this directory remain authoring
evidence and lower-level safety/interface inputs. Normal generation does not
semantically combine Markdown into the final Agent prose and never rewrites
the reviewed manual file.

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
`required`; source precedence is value-neutral, so a candidate may raise or
lower the Agent-facing value when no higher-priority source wins. It cannot
override reviewed manual hints, versioned bindings, typed/native metadata, or
current Cobra/constraint facts. Cobra's executable required marker remains a
separate `cli_required` fact with provenance even when the resolved Agent
projection differs.

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

## Manual Schema and Agent hints

Agent semantic hints in this directory do not change the executable CLI
contract. The single reviewed manual input is
`internal/cli/schema_manual_hints.json`: its `commands` section is for an
existing public Cobra leaf or a CLI-facing parameter correction, while its
`agent_hints` section stores reviewed product/tool selection prose. Command
entries use one exact `cli_path`, one canonical path, `reviewed: true`, and a
non-empty reason. Tool prose is keyed by an exact Effective CommandRegistry
canonical path and also requires review reason and evidence.
The file declares `internal/cli/schema_manual_hints.schema.json` through its
top-level `$schema` field. Agents and editors should use that schema as the
field-level source of truth instead of inferring the format from generated
Catalog JSON.

Command hints may override Schema description, interface-property/type mapping,
`required`, and `required_when` for flags that already exist on that command.
They cannot create a command or flag, target a hidden/group command, define an
interface, or mark an unknown RPC available. Missing commands and flags,
wildcards, canonical conflicts, duplicate paths, and unreviewed entries fail
generation.

`agent_hints` may be authored by an offline AI pass, but the committed batch is
a reviewed manual source. Its revision table records `generated_by`, `model`,
and `prompt_version`; every product/tool points to one declared batch revision.
The initial batch covers the exact product/tool set. A later local revision may
update selected entries without relabeling unchanged prose. `go generate` only
reads, validates, and projects this data. It must never call a model, copy a
previous Catalog, or overwrite the manual source. Agent prose cannot change
command identity, flags, parameters, safety, or interface facts.

Commands intentionally kept outside Schema remain in the separate exact
reviewed exclusion file `internal/cli/schema_command_exclusions.json`. An
included command cannot also remain excluded: completeness validation treats
that exclusion as stale.

### Agent editing workflow

1. Locate the real Cobra leaf and verify its exact path and current flags.
2. Read `internal/cli/schema_manual_hints.schema.json`; preserve `$schema` and
   `version` in the data file.
3. Add only fields that need review. In `commands`, do not repeat generated
   Agent metadata, interface availability, risk, or confirmation. In
   `agent_hints`, keep only product/tool selection prose and authoring evidence;
   do not copy safety, interface, or parameter facts.
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

6. Review the generated Catalog diff. A command/parameter hint should affect
   only the intended contract. A complete Agent batch intentionally updates
   every selected product/tool projection and its hashes.
