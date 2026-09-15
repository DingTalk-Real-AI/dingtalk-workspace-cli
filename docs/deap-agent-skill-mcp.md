# 数字员工 Skill / MCP 命令

`dws dingtalk-tag` 将 Skill/MCP 资源管理与数字员工草稿配置分开：资源命令负责创建和查询，`dingtalk-tag manage save-draft` 只保存选择、启用状态和配置值。

## Skill

从本地 ZIP 创建：

```bash
dws dingtalk-tag capability skill create \
  --agent-uuid <agentUuid> \
  --file ./my-skill.zip \
  --format json
```

CLI 在本地检查 ZIP 扩展名、压缩包完整性、路径安全、50 MiB 上限、解压规模和 `SKILL.md`。随后使用当前登录身份，把文件流式上传到 OpenAPI `POST /v1.0/assistant/skills/upload`，在内存中取得短期 `fileUrl` 后调用 `create_skill_by_url`。CLI 不落盘、不打印临时签名 URL；上传失败和创建失败分别返回对应阶段错误。

查询命令：

```bash
dws dingtalk-tag capability skill list --agent-uuid <agentUuid> --snapshot draft --format json
dws dingtalk-tag capability skill query --agent-uuid <agentUuid> --skill-id <skillId> --snapshot draft --format json
```

## MCP

敏感配置必须放在本地 JSON 文件中，不要直接拼进命令行：

```bash
dws dingtalk-tag capability mcp create --agent-uuid <agentUuid> --config-file ./mcp.json --format json
dws dingtalk-tag capability mcp list --agent-uuid <agentUuid> --keywords 文档 --page 1 --page-size 20 --format json
dws dingtalk-tag capability mcp query --agent-uuid <agentUuid> --mcp-id <mcpId> --format json
```

CLI 对配置文件和输出执行递归敏感字段保护；服务端 detail 只应返回脱敏元数据和工具列表。

文件根节点的 name、configString 必须为非空字符串，其他字段可选；不要包在 config 中。
CLI 将配置字段展开为 create_mcp 的根参数，agentUuid 仅来自必填 --agent-uuid。
create/list/query 统一使用员工资源域，不再默认企业资源域。创建后仍需 save-draft 选择并 publish；
同员工域的原 mcpId 可直接挂载，不再克隆，旧企业/公共源资源的兼容行为由 OpenAPI 保留。
服务端部署与 MCP 平台字段调整见 [MCP 映射说明](dingtalk-tag-mcp-mapping.md)。

## 草稿配置和详情

`save-draft` 使用 JSON 数组文件传入配置：

```bash
dws dingtalk-tag manage save-draft \
  --agent-uuid <agentUuid> \
  --skills-file ./skills.json \
  --mcps-file ./mcps.json
```

字段未传表示保持该类配置，空数组表示清空，非空数组表示覆写。`detail --type draft` 返回草稿配置，`detail --type published` 仍读取已发布配置；保存草稿不会自动改变线上结果。

## 变更历史

- 2026-09-15：MCP create/list/query 必填 agentUuid，创建请求将 configString 等配置展开到工具根节点；同步 Help、Schema、内部及对外 skill。必须联动部署 OpenAPI 与 MCP 映射，不能只发布 CLI。

- 2026-09-01：Skill/MCP 资源命令统一下沉到 `dws dingtalk-tag capability skill|mcp`，与数字员工本体管理和执行查询分层。
- 2026-08-31：顶级命令从 `dws deap` 重命名为 `dws dingtalk-tag`。
- 2026-08-18：新增 Skill/MCP create/list/query、save-draft Skill/MCP 文件参数和 detail snapshot 参数；Skill create 接入 OpenAPI 流式 multipart 一步创建。原因：复用后台权威链路，同时避免在 MCP JSON、日志和用户输出中传递 ZIP 或临时签名 URL。
- 2026-08-18：当时的命令路径随 DEAP 基线调整为 `dws deap skill|mcp` 与 `dws deap manage save-draft|detail`。原因：保持新增能力与当时的顶级 DEAP 产品命令树一致，避免恢复已撤销的 `dev deap-agent` 旧入口。
- 2026-08-19：将 `skill create --file` 改为串联 OpenAPI multipart upload 与 `create_skill_by_url` 两个能力，并把 `fileUrl/uploadUrl` 纳入日志脱敏。原因：保持上传和 Skill 创建的服务契约独立，同时让本地 ZIP 通过 CLI 的 HTTP 请求体安全到达 OpenAPI。
