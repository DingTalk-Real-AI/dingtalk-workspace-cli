# Schema Contract / Schema 契约

`dws schema` uses progressive disclosure over the versioned release command surface. Product and group queries are compact browse responses; tool queries use a GWS-style flat leaf contract. Runtime queries read the embedded catalog and do not rebuild Schema from the user's discovery cache or plugins.

The source ownership model, Agent Hint design, unified Command Catalog, quality gates, and implementation roadmap are documented in [DWS Agent Schema 统一方案](./dws-agent-schema-unified-plan.md).

## Progressive Queries

```bash
dws schema                                  # product overview only
dws schema list                             # compatibility alias for the product overview
dws schema calendar                         # tools in one product
dws schema calendar.event                   # tools in one CLI group
dws schema calendar.create_calendar_event   # one complete tool schema
dws schema --all                            # complete catalog for audit/CI
```

- The default response has `level: "products"`, product `tool_count`, and no embedded tool arrays.
- A product response has `level: "product"` and includes that product's tool summaries.
- A group response has `level: "group"` and includes only matching descendant tools.
- A tool response remains a flat leaf object for compatibility.
- `--all` preserves the complete product/tool catalog and is intentionally larger.

## Source Of Truth

- Go/Cobra commands and strong annotations define executable paths, flags, types, hidden state, required fields, and cross-parameter constraints at generation time.
- `internal/cli/schema_command_surface.json` fixes the reviewed public path/alias surface for one release.
- `internal/cli/schema_catalog.json` is the generated final catalog. It contains the 504 public tool summaries and full leaf schemas used by runtime `dws schema` queries.
- Hardcoded helper tools, including `dev app`, derive schema from their real Cobra flags and versioned metadata before the catalog is frozen.
- Visible local helper leaves, such as `dev connect status`, use a `hardcoded:<product>` source.
- Sanitized CLI/MCP descriptions and parameter facts are snapshotted into `internal/cli/schema_mcp_metadata.json`; endpoints, credentials, and cache timestamps are never embedded.
- During Agent metadata generation, a sanitized MCP description may fill a missing `agent_summary`. This fallback is revision-pinned, capped at 120 characters, marked `reviewed: false`, and never infers effect or risk.
- Agent affordances are generated from versioned Skill files and `skills/mono/schema-hints/**/*.json` into the product-domain files in `internal/cli/schema_agent_metadata/` and embedded in the binary. `index.json` carries the routing and coverage summary; each product JSON carries only that product's tool metadata. Their canonical IDs and CLI paths are reconciled against the versioned, endpoint-free `internal/cli/schema_command_surface.json` snapshot.
- `skills/mono/schema-hints/imported/wukong.json` is a sanitized build-time import from a fixed `dws-wukong` envelope revision. It carries descriptions, examples, and explicit sensitive-operation hints only; it contains no endpoints and cannot override Cobra parameter facts. Import coverage and unmatched paths are recorded in `internal/cli/schema_wukong_agent_hints_audit.json`.

The current open-source surface has 21 products and 504 canonical tools: 285 are emitted from hardcoded helper/root commands and 219 originate from the runtime CLIOverlay command tree. This distinction describes command construction only; all 504 Schema entries are frozen in the release catalog.

## Agent Metadata

- Product overview: one concise `agent_summary` or first `use_when`, tool count, drill-down path, an `interface_metadata` snapshot summary, and an `agent_metadata` version/hash summary.
- Tool summaries: `agent_summary`, `use_when`, `avoid_when`, `effect`, `risk`, and `confirmation` when known.
- Tool detail: summary fields plus `prerequisites`, `tips`, `idempotency`, `workflow_refs`, `reviewed`, `examples`, `effect_source`, and provenance when available.
- Agent summary precedence is explicit Hint/Skill/Wukong metadata first, followed by the fixed MCP description fallback. MCP-derived summaries expose `agent_summary_source` and a source reference to the committed interface snapshot.
- Skill/Hint-derived JSON is deterministic and checked by `scripts/policy/check-generated-drift.sh`. Refresh the command-surface snapshot, fixed Wukong import, interface metadata, Agent metadata, and final catalog in that order.
- Refresh interface metadata explicitly with `make generate-schema-interface-metadata SCHEMA_REGISTRY=/path/to/servers.json SCHEMA_TOOLS_DIR=/path/to/tools`; release builds embed the committed snapshot and never perform runtime `tools/list` discovery.
- Run `make generate-schema-catalog` last. It rejects local/plugin tools outside the reviewed surface and rejects surface paths missing from the candidate Cobra tree.

## Path Rules

```bash
dws schema                                  # compact product overview
dws schema list                             # compatibility alias for the compact root overview
dws schema calendar                         # list one product's tools
dws schema calendar.event                   # list one command group's tools
dws schema --all                            # full catalog
dws schema ding.send_ding_message           # canonical path: product.rpc_name
dws schema ding.message.send                # dotted CLI path
dws schema "ding message send"              # space CLI path
dws schema --cli-path "ding message send"   # explicit CLI-path flag
```

- `canonical_path` is stable and uses `product.rpc_name`.
- `cli_path` is the executable CLI path.
- If multiple CLI paths map to one canonical tool, the list shows only `primary_cli_path`; other paths appear in `aliases`.
- Querying an alias path is valid and returns the same `canonical_path` with `is_alias: true`.

## Leaf Shape

```json
{
  "name": "query_records",
  "canonical_path": "aitable.query_records",
  "path": "aitable.query_records",
  "cli_path": "aitable record query",
  "primary_cli_path": "aitable record query",
  "aliases": ["aitable record list"],
  "is_alias": false,
  "source": "hardcoded:aitable",
  "interface_ref": {
    "product_id": "aitable",
    "rpc_name": "query_records"
  },
  "product_id": "aitable",
  "parameters": {
    "base-id": {
      "property": "baseId",
      "type": "string",
      "description": "Base ID。",
      "required": true
    }
  },
  "has_parameters": true,
  "parameter_count": 1
}
```

## Parameters

- `parameters` is always present.
- Parameter keys are real CLI flag names, without the `--` prefix.
- `property` is the field sent to the MCP/API tool.
- `required` is inline on each parameter and means unconditionally required.
- `required_when` is present only for a conditional requirement; it explains the flag dependency without incorrectly marking the parameter globally required.
- `default` is present only when there is an explicit useful default.
- No-parameter tools use `parameters: {}`, `has_parameters: false`, and `parameter_count: 0`.
- `interface_ref` identifies the actual MCP owner and RPC when it differs from the public canonical path. It is interface provenance, not another executable CLI path.

## Alignment With Lark/GWS

- Like GWS, DWS emits a flat leaf object and keeps `parameters: {}` for no-argument tools.
- Like Lark, canonical lookup must be stable and duplicate command paths are made explicit instead of silently picking one.
- Unlike Lark, DWS does not wrap leaf output in an MCP `inputSchema` envelope because agents primarily need executable CLI flags.
- Unlike GWS, DWS includes visible hardcoded helper commands in the same flat schema shape.

## Validation Invariants

- `.products[].tools[].canonical_path` is unique in the list output.
- The compact root's `tool_count` equals the number of primary tools in `--all`.
- Every listed tool has `canonical_path` and `cli_path`.
- Every leaf output has `parameters`, `has_parameters`, and `parameter_count`.
- `parameter_count` equals the number of keys under `parameters`.
- Hidden/internal commands are excluded from both root help and Schema; compatible paths remain executable and resolve as aliases.
- The catalog tool keys exactly equal the reviewed command-surface canonical paths.
- Catalog, Agent metadata, and interface metadata report the same surface tool count.
- Normal-cache and empty-cache Schema output must be byte-identical for the same binary.
