---
category: Added
---

- **数字员工管理与能力资源** — 新增 `dws dingtalk-tag manage`、`capability`、`run`，覆盖创建、草稿保存、发布、查询、Skill ZIP 上传、员工域 MCP 创建与运行追踪；统一 `agentUuid`，支持双响应模式与 `open_code` / `local_agent` 类型。MCP 创建自动追加草稿依赖配套服务端，不自动发布；Skill/MCP 选择支持省略保留、空数组清空和非空数组替换。
- **员工身份登录** — `manage login` 串联授权码申请、换票、在线身份核验和精确 Profile 保存/刷新，保持主管当前 Profile；补充外部 `auth exchange`，不在普通输出中返回授权码或 Token。
- **本地 Agent 接入** — `dingtalk-tag connect` 支持本地 Agent Adapter 与 DSH，提供员工隔离的事件处理、生命周期、设备绑定及失败恢复；仅登录员工身份使用 `manage login`。同步 Help、Schema、mono/multi Skill 与必要接口文档。
