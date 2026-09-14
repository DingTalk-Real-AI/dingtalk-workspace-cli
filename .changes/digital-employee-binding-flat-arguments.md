---
category: Fixed
---

- **数字员工服务端绑定** — 修复 bind、rebind 与 unbind MCP 参数被错误包进 DTO 名称对象的问题，按工具 Schema 将业务字段直接发送到 `tools/call.arguments` 顶层。
- **数字员工绑定回执** — bind/rebind 按服务端契约从绑定对象的 `data.runtimeBindingId` 读取 ID，兼容旧版 `data` 字符串和顶层映射，避免有效对象响应被误留 pending；冲突、缺失或无效 ID 仍按未知结果保护，unbind 继续严格要求布尔值 `data=true`。
