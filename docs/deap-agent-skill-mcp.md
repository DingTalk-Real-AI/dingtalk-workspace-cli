# 数字员工 Skill / MCP 命令

`dws dingtalk-tag capability` 统一管理目标员工的 Skill/MCP 资源，二者都提供 `create|update|delete|list|query`。create 自动挂载草稿，update 保持现有挂载，delete 清理草稿挂载；所有写操作都不自动发布，list/query 只读。`check_mcp` 仅由 create/update 内部调用，不作为 CLI 命令暴露。

## Skill

从本地 ZIP 创建：

```bash
dws dingtalk-tag capability skill create \
  --agent-uuid <agentUuid> \
  --file ./my-skill.zip \
  --dry-run --format json
```

确认预览后，把 `--dry-run` 替换为 `--yes` 执行。CLI 在本地检查 ZIP 扩展名、压缩包完整性、路径安全、50 MiB 上限、解压规模和 `SKILL.md`。随后使用当前登录身份，把文件流式上传到 OpenAPI `POST /v1.0/assistant/skills/upload`，在内存中取得短期 `fileUrl` 后调用 `create_skill_by_url`，创建员工域 Skill、自动挂载草稿并查询确认。CLI 不落盘、不打印临时签名 URL；上传失败和创建失败分别返回对应阶段错误，不自动发布。

更新或删除：

```bash
dws dingtalk-tag capability skill update --agent-uuid <agentUuid> --skill-id <skillId> --enabled=false --dry-run --format json
dws dingtalk-tag capability skill update --agent-uuid <agentUuid> --skill-id <skillId> --file ./updated-skill.zip --dry-run --format json
dws dingtalk-tag capability skill delete --agent-uuid <agentUuid> --skill-id <skillId> --dry-run --format json
```

`update` 至少提供 `--enabled=true|false` 或 `--file` 之一；未提供的字段保持原值。替换 ZIP 时由 CLI 完成校验和上传。`delete` 删除资源并清理草稿挂载。两者都不自动发布。

查询命令：

```bash
dws dingtalk-tag capability skill list --agent-uuid <agentUuid> --snapshot draft --format json
dws dingtalk-tag capability skill query --agent-uuid <agentUuid> --skill-id <skillId> --snapshot draft --format json
```

## MCP

敏感配置必须放在本地 JSON 文件中，不要直接拼进命令行：

```bash
dws dingtalk-tag capability mcp create --agent-uuid <agentUuid> --config-file ./mcp.json --dry-run --format json
dws dingtalk-tag capability mcp update --agent-uuid <agentUuid> --mcp-id <mcpId> --enabled=false --dry-run --format json
dws dingtalk-tag capability mcp update --agent-uuid <agentUuid> --mcp-id <mcpId> --config-file ./mcp.json --dry-run --format json
dws dingtalk-tag capability mcp delete --agent-uuid <agentUuid> --mcp-id <mcpId> --dry-run --format json
dws dingtalk-tag capability mcp list --agent-uuid <agentUuid> --keywords 文档 --page 1 --page-size 20 --format json
dws dingtalk-tag capability mcp query --agent-uuid <agentUuid> --mcp-id <mcpId> --format json
```

确认预览后，把 `--dry-run` 替换为 `--yes` 执行。dry-run 不发远端请求、不写草稿。create 和带 `--config-file` 的 update 会先在 CLI 内部调用只读 `check_mcp`，校验通过后才提交写入；`check_mcp` 不直接暴露给用户。CLI 对配置文件和输出执行递归敏感字段保护；服务端 detail 只应返回脱敏元数据和工具列表。

文件根节点的 name、configString 必须为非空字符串，其他字段可选；不要包在 config 中。
CLI 将配置字段展开为 create_mcp 的根参数，agentUuid 仅来自必填 --agent-uuid。
create/update/delete/list/query 统一使用员工资源域，不再默认企业资源域。create 依次执行 check → create → query → 读取草稿 → 追加原 mcpId 到 selectedSkills → saveDraft → 回读确认，完成后才返回成功。update 至少提供 `--enabled=true|false` 或 `--config-file` 之一，未传字段保持原值且不改变挂载关系；delete 删除资源并清理草稿挂载。
create 追加时保留已有 Skill/MCP 和其他选择，不克隆、不自动发布；无需再手动保存挂载。旧企业/公共源资源的兼容行为由 OpenAPI 保留。

失败信息含 `stage=query_created_mcp` 或 `stage=mount_draft` 时，从 data 或错误信息保留已创建 mcpId，先 query 资源和 draft，再按 trace 排查挂载；不要重复 create。超时或缺少 ID 时先核查是否已创建。同一员工的创建、更新、删除、保存和发布应串行，当前跨应用读改写不是原子事务。
服务端部署与 MCP 平台字段调整见 [MCP 映射说明](dingtalk-tag-mcp-mapping.md)。

## 草稿配置和详情

create 成功后可查 `manage detail --type draft`，确认资源已挂载；update 后可核对内容或启停状态；delete 后可确认草稿挂载已清理。list/query 的资源存在不等于当前已发布。用户要求上线时再显式执行 `manage publish`，并查询 published 详情与运行态挂载。

`manage save-draft` 只更新名称、职责、头像、部门、人设和运行配置等基础草稿字段，不再接受 `--skills-file`/`--mcps-file`，普通用户无需手工读取、合并并回传完整 Skill/MCP 数组。Skill/MCP 生命周期统一通过 capability 子命令管理。
