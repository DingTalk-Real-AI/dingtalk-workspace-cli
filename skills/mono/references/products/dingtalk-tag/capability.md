# dingtalk-tag capability — 数字员工能力资源

命令前缀 `dws dingtalk-tag capability`。Skill 和 MCP 均提供 `create|update|delete|list|query`：create 校验后自动挂载草稿，update 保持现有挂载，delete 删除资源并清理草稿挂载，所有写操作都不自动发布；list/query 只查询资源。

## Skill

```text
dws dingtalk-tag capability skill create --agent-uuid <agentUuid> --file ./skill.zip --dry-run --format json
# 用户确认预览后：
dws dingtalk-tag capability skill create --agent-uuid <agentUuid> --file ./skill.zip --yes --format json
dws dingtalk-tag capability skill update --agent-uuid <agentUuid> --skill-id <skillId> --enabled=false --dry-run --format json
dws dingtalk-tag capability skill update --agent-uuid <agentUuid> --skill-id <skillId> --file ./updated-skill.zip --dry-run --format json
dws dingtalk-tag capability skill delete --agent-uuid <agentUuid> --skill-id <skillId> --dry-run --format json
dws dingtalk-tag capability skill list --agent-uuid <agentUuid> --snapshot draft --format json
dws dingtalk-tag capability skill query --agent-uuid <agentUuid> --skill-id <skillId> --snapshot draft --format json
```

Skill ZIP 最大 50 MiB，必须包含 `SKILL.md`。create 和带 `--file` 的 update 会先校验本地 ZIP，再通过 OpenAPI multipart 上传；临时上传地址不会输出或落盘。update 至少传 `--enabled=true|false` 或 `--file` 之一，省略的字段保持原值。create/update/delete 均为 `confirmation=user_required`，必须先预览并取得用户确认。

## MCP

```text
dws dingtalk-tag capability mcp create --agent-uuid <agentUuid> --config-file ./mcp.json --dry-run --format json
# 用户确认预览后：
dws dingtalk-tag capability mcp create --agent-uuid <agentUuid> --config-file ./mcp.json --yes --format json
dws dingtalk-tag capability mcp update --agent-uuid <agentUuid> --mcp-id <mcpId> --enabled=false --dry-run --format json
dws dingtalk-tag capability mcp update --agent-uuid <agentUuid> --mcp-id <mcpId> --config-file ./mcp.json --dry-run --format json
dws dingtalk-tag capability mcp delete --agent-uuid <agentUuid> --mcp-id <mcpId> --dry-run --format json
dws dingtalk-tag capability mcp list --agent-uuid <agentUuid> --keywords <关键词> --page 1 --page-size 20 --format json
dws dingtalk-tag capability mcp query --agent-uuid <agentUuid> --mcp-id <mcpId> --format json
```

`mcp.json` 根节点必填非空字符串 `name`、`configString`，可选 `description`、`detailIntro`、`userQuestionTips`、`configType`、`envs`、`toolsDisabled`；文件最大 1 MiB。`configString` 是配置 JSON 的字符串，不是 JSON 对象；不要再包装一层 `config`。CLI 将这些字段直接展开到 MCP 工具根节点。文件不能放 `agentUuid` 或 `identity`；员工域只由必填的 `--agent-uuid` 指定，调用人身份仍来自所选 Profile。

MCP 敏感配置必须放在本地 JSON 文件，不要直接拼进命令行或提交代码库。create 和带 `--config-file` 的 update 会先在 CLI 内部调用 `check_mcp`，校验通过才写入；`check_mcp` 不暴露为 CLI 命令。update 至少传 `--enabled=true|false` 或 `--config-file` 之一，省略字段保持原值。create/update/delete 均需确认；dry-run 不调用远端、不写草稿、不输出配置值，查询结果只返回脱敏信息。

create 成功后自动挂载草稿并保留已有选择；update 不改变挂载关系，也不会重新挂回已移除资源；delete 删除资源并清理草稿挂载。三者都不自动发布。list/query 只证明资源存在，不证明当前仍被选中或已发布。

失败恢复：若错误含 `stage=query_created_mcp` 或 `stage=mount_draft`，从返回 data 或错误信息保留已创建 `mcpId`，先 query 资源及 `manage detail --type draft`，禁止重复 create。普通用户不通过 `manage save-draft` 手工拼装完整 Skill/MCP 数组；挂载部分失败交由服务端运维链路恢复。超时或缺少 ID 时也先核查，不盲目重试。同一员工的创建、更新、删除和发布串行执行，当前跨应用读改写不是原子事务。

## 核验草稿与发布

```text
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type draft --format json
dws dingtalk-tag manage publish --agent-uuid <agentUuid> --dry-run --format json
# 用户确认发布后：
dws dingtalk-tag manage publish --agent-uuid <agentUuid> --yes --format json
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type published --format json
```

创建后核验 draft 已挂载，update 后核对内容或启停状态，delete 后确认草稿挂载已清理；只有用户要求上线才执行 publish。运行时可用还须验证已发布配置和真实工具挂载，不能把资源写成功当成运行态验收。`manage save-draft` 仅更新基础草稿字段，不接受 Skill/MCP 完整数组。
