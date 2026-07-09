# Schema Contract / Schema 契约

`dws schema` uses a GWS-style flat schema contract over the current runnable command surface. The design follows the common Lark/GWS pattern: schema describes API/tool leaf contracts, not local infrastructure commands.

## Source Of Truth

- Runtime/dynamic products: schema metadata attached to actual Cobra leaf commands.
- Hardcoded helper products: registered runtime schema roots plus curated hints.
- Live helper tools, such as `dev app`: MCP `tools/list` from the pinned helper server, rendered into the same flat leaf shape.
- Local-only commands without an API/tool binding, such as `dev connect status`, are not schema tools.

## Path Rules

```bash
dws schema                                  # list products and primary tools
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
- `required` is inline on each parameter.
- `default` is present only when there is an explicit useful default.
- No-parameter tools use `parameters: {}`, `has_parameters: false`, and `parameter_count: 0`.

## Alignment With Lark/GWS

- Like GWS, DWS emits a flat leaf object and keeps `parameters: {}` for no-argument tools.
- Like Lark, canonical lookup must be stable and duplicate command paths are made explicit instead of silently picking one.
- Unlike Lark, DWS does not wrap leaf output in an MCP `inputSchema` envelope because agents primarily need executable CLI flags.
- Unlike GWS, DWS keeps helper live-schema support for selected helper-backed API tools, but renders them into the same flat shape.

## Validation Invariants

- `.products[].tools[].canonical_path` is unique in the list output.
- Every listed tool has `canonical_path` and `cli_path`.
- Every leaf output has `parameters`, `has_parameters`, and `parameter_count`.
- `parameter_count` equals the number of keys under `parameters`.
- Local-only helper commands are excluded unless they have an explicit API/tool binding.
