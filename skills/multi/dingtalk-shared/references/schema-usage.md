# DWS 命令发现与 Schema Inspect

`dws` 对 Agent 是一个稳定元工具；Catalog 中的命令是它的内部命令空间，不是需要向 Host 动态注册的一组新工具。`dws schema search` 用来找到未知子命令，`dws schema <canonical>` 用来检查选中命令的结构化契约，最后仍通过同一个 `dws` 执行。

本节同时适用于基础/原子命令与公开内建 `+` shortcut。用户自定义或未公开 shortcut 不进入发布 Schema；是否可执行仍以当前 Cobra help 为准。

## 已知命令路径例外

当产品 Skill、意图表或任务 reference **已经给出精确 CLI path** 时：

- 不要再查产品级/分组级 Schema，也不要加载完整 Shortcut Catalog
- 可直接执行；只有参数、约束或安全语义不确定时才读该命令的 leaf Schema
- 只有当前 Cobra flags 不确定时才补读 leaf Help

稳定 command identity 已与真实 Cobra tree 绑定。不要读取 Catalog 文件、native annotation 或其他生成 JSON 来重新推断命令。

## 未知命令：Search → Inspect → Execute

仅当当前 Skill、意图表和任务 reference 都没有给出精确 CLI path 时，才执行一次本地命令搜索：

```bash
dws schema search --query "查询群消息已读状态" --limit 5
```

1. 只从 `candidates` 选择与用户动作一致的候选；不把 rank/score 当授权或成功概率。零候选、多个语义无法区分或 `abstained=true` 时停止并澄清。
2. `exact_filtered` 表示用户精确指定的命令被过滤；立即停止，不选择同名 sibling 绕过。
3. 选中候选后，使用 response `catalog.source_hash` 和 `catalog.surface_hash` 精确 Inspect `canonical_path`：

```bash
dws schema chat.query_msg_read_status --compact --format json \
  --expected-source-hash "<search.catalog.source_hash>" \
  --expected-surface-hash "<search.catalog.surface_hash>"
```

4. 只使用 `schema-inspect.v1.tool_spec` 组装参数，并按候选的 `primary_cli_path` 调用同一个 `dws`。`reason=catalog_changed` 时丢弃旧候选，重新 Search，不混用两代 Schema。
5. 执行阶段仍以实时 profile、权限、confirmation 和 Cobra 校验为准；Search/Inspect 都不代表授权或业务成功。

例如 Inspect 返回的 ToolSpec 确认参数后，最后仍执行：

```bash
dws chat message read-status \
  --conversation-id "<真实返回的 openConversationId>" \
  --message-id "<真实返回的 openMessageId>" \
  --format json
```

### 复合意图

先把 2～4 个有时序的动作拆成 `subqueries`，不要用整句话只排一次 Top-K：

```bash
printf '%s' '{"version":"tool-search.v1","query":"给群里发文件并确认送达","subqueries":["给群里发送文件消息","查询消息发送状态","查询群消息已读状态"]}' |
  dws schema search --request-json -
```

对每个选中的 canonical 分别做双 hash Inspect；按任务依赖顺序执行，后续 ID 只能来自前一步真实返回。

## 已知产品时的四层浏览 fallback

```bash
# 只用于已知产品、但需要人类浏览组/命令层级的 fallback
# 第 1 层：产品概览（列出产品 + 工具数 + 用途摘要）
dws schema

# 第 2 层：产品级（该产品工具的 cli_path + description + effect/risk）
dws schema calendar --compact

# 第 3 层：分组级
dws schema "calendar event" --compact

# 第 4 层：Agent leaf（参数契约）
dws schema "calendar event create" --compact

# --all：全量 leaf，仅 CI / 审计 / 参数 baseline
dws schema --all --format json
```

### `--all` 边界（强制）

`--all` 输出体积很大。仅在用户明确要求全量导出，或 CI / Catalog 审计 / 参数防丢 baseline 时使用。普通业务任务严禁用 `--all` 做命令发现，也不要把全量结果注入 Agent 上下文。未知命令优先用 `schema search`；四层浏览只作已知产品时的人类导航 fallback。

完整兼容性 baseline 必须使用未裁剪的 `schema --all`；`schema --all --compact` 会移除 provenance 和接口映射字段，不得作为完整 baseline。

同一工具省略 `--compact` 的 full leaf 与 `--all` 条目是同一份 `ToolSpec` 契约；compact leaf 只做展示投影，不重新解析语义。Alias 查询只改变路径视图，不得据此重写参数。若同一视图观察到内容差异，按契约漂移报告，不要选一份继续猜。

### `--compact`

正向字段白名单：保留 `cli_path`、`canonical_path`、`description`、`effect`、`risk`、`confirmation`、`interface_mode`、`availability`、`interface_reason`、`parameters`、`constraints`、`examples`、`use_when`、`avoid_when`；新增 full/audit 字段不会自动泄漏进 Agent 上下文。它有意不返回 `interface_ref`、参数 `property/interface_type` 和 provenance（如 `agent_metadata_source`、`effect_source`、`primary_cli_path` 等）；检查这些映射事实时，用 full leaf 配合 `--jq` 精确投影。

若旧二进制报 `unknown_flag: --compact`，去掉 `--compact` 重跑同一查询；不要因此判定 leaf 不存在，也不要用 Schema 查业务数据。

## 字段速查

```jsonc
{
  "cli_path": "calendar event create",
  "effect": "write",              // read | write | destructive
  "risk": "medium",               // low | medium | high
  "confirmation": "not_required", // not_required | user_required
  "availability": "available",
  "parameters": { "title": { "type": "string", "required": true } },
  "constraints": { "require_together": [["a", "b"]] }
}
```

- `confirmation=user_required` → 先确认再加 `--yes`；协议见 [confirmation.md](./confirmation.md)
- `availability=unavailable` → 不执行；说明 `interface_reason`
- `parameters.<flag>.required=true` / `cli_required=true` → 按 Schema/Cobra 契约提供参数
- `constraints.require_together` → 列出的 flag 必须同时提供

## Schema、Help 与业务数据边界

| 信息 | 事实源 |
|---|---|
| 命令是否存在、Cobra 接受哪些 flags | `dws <cli_path> --help` |
| Agent 选择、CLI 参数/required/组合约束、risk/confirmation | `dws schema "<cli_path>" --compact` |
| CLI↔RPC 参数映射、接口绑定或 provenance 审计 | full leaf 配合 `--jq` / `--fields` 精确投影；不要把整个 full leaf 注入 Agent 上下文 |
| shortcut 同上 | 已知路径：`dws schema --cli-path "<service> +<shortcut>" --compact --format json` |
| 钉钉业务数据 | 真实 `read` / `search` / `list` 等命令 |

Schema 与 Help 冲突是**契约漂移**：执行参数只用 Cobra 接受的 flags；安全语义冲突时采用更保守确认，无法确认则停止并报告。

`dws schema` 只查询命令契约。完成发现后必须继续执行真实业务命令；不要把 Schema 结果当成业务查询结果。

### 两类易混 Schema

- `dws event schema <event_key> --flatten`：事件业务字段
- `dws schema "event consume" --compact`：CLI 命令参数

二者不可互相替代。
