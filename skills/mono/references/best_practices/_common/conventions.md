# 业务域通用规范

> 仅服务本仓库已迁入的文档与 AI 表格行动指南。安全门控、危险操作确认、`--format json` 等已在根 [SKILL.md](../../../SKILL.md) 中定义，此处不重复。

## 批量查询规范

| # | 规范 |
|---|------|
| 1 | **并行查详情**：拿到多个 ID 后，用 `&` 合并到同一条 Shell 命令并行执行 + `wait`，**严禁逐条串行** |
| 2 | **翻页**：分页接口须拉全直至无更多 |
| 3 | **优先批量 API**：有批量接口则用批量；无则按 #1 并行 |
| 4 | **列表少轮次**：带条件搜索/列表 → 一次采全详情；**禁止**无新参数时重复同一 `list` / `search` |

## 多源并行采集（公共模式）

> recipe 引用方式：`按「多源并行采集」执行（关键词=<X>，时间=<Y>至<Z>）`。

- 同条 Shell：`&` 并行 + `wait`；分页须采全。
- 只保留与主题相关的数据，无关丢弃。
- 有批量详情接口优先；否则并行拉详情（见上表 #1）。
- 具体采哪些产品列表由对应 **行动指南 recipe** 与 [SKILL 产品参考](../../../SKILL.md) 决定；不要引入本文档未覆盖的产品路线。

## 字段术语与 ID 传递

> list 返回 JSON 后，必须提取下表字段传给后续命令。**禁止用其他字段替代。**

| 字段 | 来源 | 传递给 |
|------|------|--------|
| `nodeId` | `doc search` | `doc read/update/copy/move/rename --node` |
| `nodeId` | `doc list` 中的 folder 类型节点 / `doc folder create` | `doc list --folder`、`doc create --folder`、`doc upload --folder`、`doc copy/move --folder` |
| `baseId` / `tableId` | `aitable base search` | `aitable record query --base-id --table-id` |
| `dentryUuid` | `drive list` / `drive mkdir` | `drive info/download --file-id`、`drive list/mkdir/upload --parent-id` |
| `workspaceId` | `wiki space search/list/create` | `doc list/search/create --workspace`、`wiki member * --workspace` |

**ID 边界硬约束**：遇到 `drive --parent-id`、`doc --folder`、`doc --node` 时，只能使用 `dentryUuid` / `nodeId` / 文档 URL。若当前上下文只有数字型 `dentryId`，必须先重新 `drive list` / `doc list` / `doc search` 获取正确 ID，不能把该数字直接代入后续命令。
