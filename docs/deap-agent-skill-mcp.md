# 数字员工 Skill / MCP 命令

`dws dingtalk-tag capability` 管理目标员工的 Skill/MCP 资源。MCP create 成功后自动追加到该员工草稿，不自动发布；`dingtalk-tag manage save-draft` 用于调整已有资源的选择、启用状态和配置值。

## Skill

从本地 ZIP 创建：

```bash
dws dingtalk-tag capability skill create \
  --agent-uuid <agentUuid> \
  --file ./my-skill.zip \
  --dry-run --format json
```

确认预览后，把 `--dry-run` 替换为 `--yes` 执行。CLI 在本地检查 ZIP 扩展名、压缩包完整性、路径安全、50 MiB 上限、解压规模和 `SKILL.md`。随后使用当前登录身份，把文件流式上传到 OpenAPI `POST /v1.0/assistant/skills/upload`，在内存中取得短期 `fileUrl` 后调用 `create_skill_by_url`，创建员工域 Skill 及引用。CLI 不落盘、不打印临时签名 URL；上传失败和创建失败分别返回对应阶段错误。创建后查询 draft 确认回显，不自动发布。

查询命令：

```bash
dws dingtalk-tag capability skill list --agent-uuid <agentUuid> --snapshot draft --format json
dws dingtalk-tag capability skill query --agent-uuid <agentUuid> --skill-id <skillId> --snapshot draft --format json
```

## MCP

敏感配置必须放在本地 JSON 文件中，不要直接拼进命令行：

```bash
dws dingtalk-tag capability mcp create --agent-uuid <agentUuid> --config-file ./mcp.json --dry-run --format json
dws dingtalk-tag capability mcp list --agent-uuid <agentUuid> --keywords 文档 --page 1 --page-size 20 --format json
dws dingtalk-tag capability mcp query --agent-uuid <agentUuid> --mcp-id <mcpId> --format json
```

确认“创建并挂载到目标员工草稿”后，把 `--dry-run` 替换为 `--yes` 执行。dry-run 不发远端请求、不写草稿。CLI 对配置文件和输出执行递归敏感字段保护；服务端 detail 只应返回脱敏元数据和工具列表。

文件根节点的 name、configString 必须为非空字符串，其他字段可选；不要包在 config 中。
CLI 将配置字段展开为 create_mcp 的根参数，agentUuid 仅来自必填 --agent-uuid。
create/list/query 统一使用员工资源域，不再默认企业资源域。服务端 create 依次执行 check → create → query → 读取草稿 → 追加原 mcpId 到 selectedSkills → saveDraft → 回读确认，完成后才返回成功。
追加时保留已有 Skill/MCP 和其他选择，不克隆、不自动发布；无需为本次创建再手动保存挂载。已有资源的选择可以继续通过 save-draft 调整，旧企业/公共源资源的兼容行为由 OpenAPI 保留。

失败信息含 `stage=query_created_mcp` 或 `stage=mount_draft` 时，从 data 或错误信息保留已创建 mcpId，先 query 资源和 draft，再按原 ID 恢复 save-draft；不要重复 create。超时或缺少 ID 时先核查是否已创建。同一员工的创建、保存、发布应串行，当前跨应用读改写不是原子事务。
独立 `check_mcp` 只校验，不创建或挂载；`update_mcp` 更新资源，不修改挂载选择、不恢复已移除资源、不自动发布。当前 CLI 无独立 check/update 子命令。
服务端部署与 MCP 平台字段调整见 [MCP 映射说明](dingtalk-tag-mcp-mapping.md)。

## 草稿配置和详情

MCP 创建成功后，先查 `manage detail --type draft`，确认原 mcpId 已在 selectedSkills 且 mcps 正常回显。list/query 的资源存在不等于当前被选中或已发布。用户要求上线时再显式 publish，并查询 published 详情与运行态挂载。

调整已有选择或恢复部分失败时，`save-draft` 使用 JSON 数组文件传入配置：

```bash
dws dingtalk-tag manage save-draft \
  --agent-uuid <agentUuid> \
  --skills-file ./skills.json \
  --mcps-file ./mcps.json \
  --dry-run --format json
```

两个文件未传表示保持该类配置，空数组表示清空，非空数组表示覆写该类选择，必须包含仍需保留的项目。基础与档案字段仍需先读完整 draft 并保留未修改项；上例只展示资源参数，不是完整覆写模板。确认预览后才用 `--yes` 执行。`detail --type draft` 返回草稿配置，`detail --type published` 仍读取已发布配置；保存草稿不会自动改变线上结果。

## 变更历史

- 2026-09-15：对齐 OpenAPI MCP 创建自动追加草稿行为；同步 CLI Help/Schema、双份 Skill 和工具说明。依赖部署新版 OpenAPI 与 Studio saveDraft 未传字段保持逻辑；文档和本地测试不代表预发/线上已生效。

- 2026-09-15：MCP create/list/query 必填 agentUuid，创建请求将 configString 等配置展开到工具根节点；同步 Help、Schema、内部及对外 skill。必须联动部署 OpenAPI 与 MCP 映射，不能只发布 CLI。

- 2026-09-01：Skill/MCP 资源命令统一下沉到 `dws dingtalk-tag capability skill|mcp`，与数字员工本体管理和执行查询分层。
- 2026-08-31：顶级命令从 `dws deap` 重命名为 `dws dingtalk-tag`。
- 2026-08-18：新增 Skill/MCP create/list/query、save-draft Skill/MCP 文件参数和 detail snapshot 参数；Skill create 接入 OpenAPI 流式 multipart 一步创建。原因：复用后台权威链路，同时避免在 MCP JSON、日志和用户输出中传递 ZIP 或临时签名 URL。
- 2026-08-18：当时的命令路径随 DEAP 基线调整为 `dws deap skill|mcp` 与 `dws deap manage save-draft|detail`。原因：保持新增能力与当时的顶级 DEAP 产品命令树一致，避免恢复已撤销的 `dev deap-agent` 旧入口。
- 2026-08-19：将 `skill create --file` 改为串联 OpenAPI multipart upload 与 `create_skill_by_url` 两个能力，并把 `fileUrl/uploadUrl` 纳入日志脱敏。原因：保持上传和 Skill 创建的服务契约独立，同时让本地 ZIP 通过 CLI 的 HTTP 请求体安全到达 OpenAPI。
