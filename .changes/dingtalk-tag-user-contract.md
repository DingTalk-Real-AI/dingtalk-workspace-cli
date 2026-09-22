---
category: Added
---

- 统一 `dingtalk-tag manage` 的 `agentUuid` / `userId` 用户契约：创建时部门可省略，草稿按字段更新并分开 Skill/MCP 输入，本地头像可安全上传且不暴露临时凭证。
- 直属上级的输入与输出统一使用 `supervisorUserId`；HSF 历史字段名仅保留在 MCP 映射边界内部。
- 头像统一使用 `avatarUrl`；`--avatar-url` 同时支持公网 HTTP(S) 与本地图片，本地文件复用 Skill 上传封装并自动回写。
- 创建、更新的主程序类型映射到 MCP `digitalTagEmployeeProfile.type`；列表筛选仍使用顶层 `type`。CLI 参数统一为 `--type`（create/list/save-draft），详情查询改为 `--snapshot draft|published` 并发送 `snapshot`，兼容旧 `--type` 别名。
- 创建时响应模式缺省或为空，默认发送 `mention_only`；更新未传则保留原值，显式响应模式不会被默认值覆盖。发布前的完整配置由服务端校验。
- 登录/连接使用已发布详情的 `profile.corpId/userId`，不再要求旧 `robotUid/staffId` 字段；换票后仍在线核验身份，再保存精确 Profile。
- MCP 创建/替换配置的帮助、Schema 与技能示例明确要求显式填写 `configString.mcpServers.<名称>.type`（`streamable-http` 或 `sse`），避免只有 URL 时服务端校验失败。
- 创建数字员工必须显式指定 `--type open_code|local_agent`，DWS 本地拒绝缺失或空值，帮助/Schema/调用说明同步标为必填；更新不传仍保留原值。
- `set-visibility` 的可见成员参数在 CLI 侧为 `--user-ids`（调用 MCP `set_visibility` 时仍发送 `staffIds`）；`publish` 不再暴露 `--allow-join-group`。
