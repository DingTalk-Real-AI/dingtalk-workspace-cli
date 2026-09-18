---
category: Added
---

- 统一 `dingtalk-tag manage` 的 `agentUuid` / `userId` 用户契约：创建时部门可省略，草稿按字段更新并分开 Skill/MCP 输入，本地头像可安全上传且不暴露临时凭证。
- 直属上级的输入与输出统一使用 `supervisorUserId`；HSF 历史字段名仅保留在 MCP 映射边界内部。
- 头像统一使用 `avatarUrl`；`--avatar-url` 同时支持公网 HTTP(S) 与本地图片，本地文件复用 Skill 上传封装并自动回写。
- 创建、更新、列表的主程序类型在 MCP 契约中统一为顶层 `type`；CLI 保留语义明确的 `--main-program-type`，详情查询的 `--type` 仍表示 draft/published。
- `open_code` 创建、切换及发布必须至少配置一个 `responseMode`；`local_agent` 可省略。DWS 提前校验可确定的参数组合，OpenAPI 按草稿合并后的最终状态兜底校验。
