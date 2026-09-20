# dingtalk-tag capability — 数字员工能力资源

命令前缀 `dws dingtalk-tag capability`。资源属于目标 `agentUuid`：MCP 创建成功会自动挂载到员工草稿，不自动发布；Skill 创建使用员工域引用链路，创建后查询 draft 确认回显。

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

MCP 敏感配置必须放在本地 JSON 文件，不要直接拼进命令行或提交代码库。创建命令 `confirmation=user_required`；确认摘要须包含“创建资源并修改员工草稿”。dry-run 不调用远端、不写草稿、不输出配置值，查询结果只返回脱敏信息。

`create` 由服务端依次 check → create → query → 读取草稿 → 在 MCP 选择中追加原 `mcpId` → saveDraft → 回读确认，成功后无需再手动保存一次挂载。用户侧只感知分开的 `skills` / `mcps`；OpenAPI 内部再合并成下游引用。保留已有 Skill、MCP 和其他选择，不克隆、不自动发布。create/list/query 均针对同一员工域；list/query 只证明资源存在，不证明当前仍被选中或已发布。

失败恢复：若错误含 `stage=query_created_mcp` 或 `stage=mount_draft`，从返回 data 或错误信息保留已创建 `mcpId`，先 query 资源及 `manage detail --type draft`，必要时按原 ID save-draft 恢复；禁止重复 create。超时或缺少 ID 时也先核查，不盲目重试。同一员工的创建、保存、发布串行执行，当前跨应用读改写不是原子事务。

此语义需配套部署 OpenAPI 自动挂载实现和 Studio saveDraft 未传字段保持逻辑，不能仅凭新版 CLI Help 判断旧服务端已具备能力。直接使用 MCP 工具时，`check_mcp` 仅校验，不保存资源或草稿；`update_mcp` 更新资源，不改变选择、不重新挂回已移除资源、不自动发布。当前 CLI 没有独立 check/update 子命令，不要猜命令。

## 核验草稿与发布

```text
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type draft --format json
dws dingtalk-tag manage publish --agent-uuid <agentUuid> --dry-run --format json
# 用户确认发布后：
dws dingtalk-tag manage publish --agent-uuid <agentUuid> --yes --format json
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type published --format json
```

创建后核验 draft 的 `mcps` 有对应 `mcpId` 回显；只有用户要求上线才执行 publish。运行时可用还须验证已发布配置和真实工具挂载，不能把创建成功当成运行态验收。用户侧只使用 `skills` / `mcps` 两类字段。

需要调整已有资源选择、配置或恢复部分失败时，使用 `dws dingtalk-tag manage save-draft`，按类型传 `--skills-file` 或 `--mcps-file`。基础字段按显式字段更新；两个文件不传时保持对应关联，显式 `[]` 清空，非空数组覆写该类选择，不能只传新增一项而丢掉原有项目。
