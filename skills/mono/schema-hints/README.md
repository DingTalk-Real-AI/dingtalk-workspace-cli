# DWS Agent Schema Hints

This directory contains versioned, structured Agent metadata. Files are build inputs for `internal/cli/schema_agent_metadata/`; generated runtime metadata must not be edited directly.

## Source kinds

- `explicit`: reviewed DWS hints. Scalar fields override imported baselines.
- `imported`: sanitized metadata from a fixed external revision. It fills missing Agent semantics but cannot redefine command paths or parameter contracts.

The Agent metadata generator also reads the committed `internal/cli/schema_mcp_metadata.json` after Skill and Hint parsing. A sanitized MCP description can fill an otherwise empty `agent_summary`; it is marked `reviewed: false`, retains revision provenance, and cannot infer or override risk/effect fields.

Tool keys should use stable `canonical_path` values from `internal/cli/schema_command_surface.json`. CLI paths and aliases are also accepted and are reconciled to the canonical public tool during generation.

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

Run `make generate-schema-agent-metadata` after changing Hint or Skill sources, then `make generate-schema-catalog`. External Wukong metadata must be refreshed with an immutable revision through `make generate-schema-wukong-agent-hints`.
