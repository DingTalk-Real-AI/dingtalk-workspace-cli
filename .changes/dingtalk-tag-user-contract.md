---
category: Added
---

- 统一 `dingtalk-tag manage` 的 `agentUuid` / `userId` 用户契约：创建时部门可省略，草稿按字段更新并分开 Skill/MCP 输入，本地头像可安全上传且不暴露临时凭证。
- 直属上级的输入与输出统一使用 `supervisorUserId`；HSF 历史字段名仅保留在 MCP 映射边界内部。
- 头像统一使用 `avatarUrl`；`--avatar-url` 同时支持公网 HTTP(S) 与本地图片，本地文件复用 Skill 上传封装并自动回写。
