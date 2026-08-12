# Schema 渐进查询

`dws schema` 内嵌当前二进制公开命令面的结构化契约。**Agent 选择命令、读取参数映射/约束和安全语义时优先渐进查询 leaf Schema**；真正组装执行参数前，用 `--help` 确认当前 Cobra 接受的 flags。

本节同时适用于基础/原子命令与公开内建 `+` shortcut。用户自定义或未公开 shortcut 不进入发布 Schema；是否可执行仍以当前 Cobra help 为准。

## 已知命令路径例外

当产品 Skill、意图表或任务 reference **已经给出精确 CLI path** 时：

- 不要再查产品级/分组级 Schema，也不要加载完整 Shortcut Catalog
- 可直接执行；只有参数、约束或安全语义不确定时才读该命令的 leaf Schema
- 只有当前 Cobra flags 不确定时才补读 leaf Help

稳定 command identity 已与真实 Cobra tree 绑定。不要读取 Catalog 文件、native annotation 或其他生成 JSON 来重新推断命令。

## 四层查询

```bash
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

`--all` 输出体积很大。仅在用户明确要求全量导出，或 CI / Catalog 审计 / 参数防丢 baseline 时使用。普通业务任务严禁用 `--all` 做命令发现，也不要把全量结果注入 Agent 上下文；必须按「产品概览 → 产品/分组 → leaf」渐进查询。

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
