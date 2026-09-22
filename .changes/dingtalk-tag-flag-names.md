---
category: Changed
---

- `dingtalk-tag manage create/list/save-draft` 将 `--main-program-type` 更名为 `--type`；创建仍要求显式指定 `open_code` 或 `local_agent`，MCP 字段映射不变。
- `dingtalk-tag manage set-visibility` 将 `--staff-ids` 更名为 `--user-ids`，调用 MCP 时仍发送 `staffIds`；同步帮助、Schema 契约、示例与 mono/multi 技能文档。
- `dingtalk-tag manage publish` 移除未对应服务端字段的 `--allow-join-group` 参数。使用旧参数的脚本需同步更新。
