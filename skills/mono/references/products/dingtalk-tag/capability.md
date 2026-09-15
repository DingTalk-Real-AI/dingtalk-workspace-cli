# dingtalk-tag capability — 数字员工能力资源

命令前缀 `dws dingtalk-tag capability`。Skill/MCP 命令负责创建和查询能力资源，不会自动把资源关联到数字员工。

## Skill

```text
dws dingtalk-tag capability skill create --agent-uuid <agentUuid> --file ./skill.zip --dry-run --format json
# 用户确认预览后：
dws dingtalk-tag capability skill create --agent-uuid <agentUuid> --file ./skill.zip --yes --format json
dws dingtalk-tag capability skill list --agent-uuid <agentUuid> --snapshot draft --format json
dws dingtalk-tag capability skill query --agent-uuid <agentUuid> --skill-id <skillId> --snapshot draft --format json
```

Skill ZIP 最大 50 MiB，必须包含 `SKILL.md`。创建会先校验本地 ZIP，再通过 OpenAPI multipart 上传，最后调用 Skill Center 创建并查询资源；临时上传地址不会输出或落盘。该创建命令 `confirmation=user_required`，必须先预览并取得用户确认。

## MCP

```text
dws dingtalk-tag capability mcp create --agent-uuid <agentUuid> --config-file ./mcp.json --dry-run --format json
# 用户确认预览后：
dws dingtalk-tag capability mcp create --agent-uuid <agentUuid> --config-file ./mcp.json --yes --format json
dws dingtalk-tag capability mcp list --agent-uuid <agentUuid> --keywords <关键词> --page 1 --page-size 20 --format json
dws dingtalk-tag capability mcp query --agent-uuid <agentUuid> --mcp-id <mcpId> --format json
```

`mcp.json` 根节点必填非空字符串 `name`、`configString`，可选 `description`、`detailIntro`、`userQuestionTips`、`configType`、`envs`、`toolsDisabled`；文件最大 1 MiB。`configString` 是配置 JSON 的字符串，不是 JSON 对象；不要再包装一层 `config`。CLI 将这些字段直接展开到 MCP 工具根节点。文件不能放 `agentUuid` 或 `identity`；员工域只由必填的 `--agent-uuid` 指定，调用人身份仍来自所选 Profile。

MCP 敏感配置必须放在本地 JSON 文件，不要直接拼进命令行或提交代码库。创建命令 `confirmation=user_required`；dry-run 不输出配置值，查询结果只返回脱敏信息。create/list/query 均针对同一员工域；创建得到的 mcpId 可直接在该员工下挂载，无需再克隆。资源存在不等于已选中或已发布。

## 关联到数字员工

资源创建成功后，使用 `dws dingtalk-tag manage save-draft`，并按资源类型传 `--skills-file` 或 `--mcps-file` 保存关联配置。基础与档案字段仍是全量覆写，执行前必须先查完整 draft 并保留所有仍需配置的字段；`skills-file` / `mcps-file` 自身不传时保持原关联，显式 `[]` 才清空。
