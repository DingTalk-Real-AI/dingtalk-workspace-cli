# Schema Contract / Schema 契约

`dws schema` uses progressive disclosure over the current runnable command surface. Product and group queries are compact browse responses; tool queries use a GWS-style flat leaf contract.

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

- Runtime/dynamic products: schema metadata attached to actual Cobra leaf commands.
- Hardcoded helper products: registered runtime schema roots plus curated hints.
- Helper tools, including `dev app`, derive schema from their real Cobra flags and versioned metadata; schema queries never call MCP `tools/list`.
- Visible local helper leaves, such as `dev connect status`, use a `hardcoded:<product>` source.
- Sanitized CLI/MCP descriptions and parameter facts are snapshotted into `internal/cli/schema_mcp_metadata.json`; endpoints, credentials, and cache timestamps are never embedded.
- Agent affordances are generated from the versioned Skill files into the product-domain files in `internal/cli/schema_agent_metadata/` and embedded in the binary. `index.json` carries the routing and coverage summary; each product JSON carries only that product's tool metadata. Their paths are reconciled against the versioned, endpoint-free `internal/cli/schema_command_surface.json` snapshot.

## Agent Metadata

- Product overview: `use_when`, `avoid_when`, an `interface_metadata` snapshot summary, and an `agent_metadata` version/hash summary.
- Tool summaries: `use_when`, `avoid_when`, `effect`, `risk`, and `confirmation` when known.
- Tool detail: summary fields plus `examples`, `effect_source`, and `agent_source_refs`.
- Skill-derived JSON is deterministic and checked by `scripts/policy/check-generated-drift.sh`. Refresh the command-surface snapshot with `make generate-schema-command-surface`, then run `make generate-schema-agent-metadata`.
- Refresh interface metadata explicitly with `make generate-schema-interface-metadata SCHEMA_REGISTRY=/path/to/servers.json`; release builds embed the committed snapshot and never perform runtime `tools/list` discovery.

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
- Hidden commands are excluded; visible local helper commands remain queryable with a `hardcoded:<product>` source.
