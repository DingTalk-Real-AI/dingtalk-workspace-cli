# drive 存储容量（quota / quota apps）

> **前置条件（MUST READ）：** 执行本组命令前，必须先用 Read 工具读取以下文件：
> 1. [`../drive.md`](../drive.md) — 命令路由 + 场景索引 + 意图判断 + 工作流

---

## 存储容量（quota）

```
Usage:
  dws drive quota [flags]
Example:
  dws drive quota
  dws drive quota --app <APP_ID>
  dws drive quota --space <SPACE_ID>
Flags:
      --app string    应用 ID，查应用级用量 (可选，与 --space 互斥)
      --space string  空间 ID，查空间级用量 (可选，与 --app 互斥)
```

三种查询维度（`--app` 与 `--space` 互斥，同时传会报错）：

| 场景 | 命令 |
|------|------|
| 企业级总用量（默认，不加参数） | `dws drive quota` |
| 某个应用的存储用量 | `dws drive quota --app <APP_ID>` |
| 某个空间的存储用量 | `dws drive quota --space <SPACE_ID>` |

## 应用用量列表（quota apps）

```
Usage:
  dws drive quota apps [flags]
Example:
  dws drive quota apps
  dws drive quota apps --limit 50
  dws drive quota apps --cursor <nextToken>
  dws drive quota apps --order-by used-quota --order desc
Flags:
      --limit int        每页返回数量，默认 20，最大 50 (可选)
      --cursor string    分页游标，从上次返回的 nextToken 获取 (可选)
      --order-by string  排序字段: used-quota / standard-used-quota / exclusive-used-quota (可选)
      --order string     排序方向: asc|desc，默认 desc (可选)
```

盘点企业内各应用的存储用量排行，支持分页与排序：

- `--limit` 最大 50；结果返回 `nextToken`，翻页时传给 `--cursor`。
- `--order-by` 三种排序口径：`used-quota`（总用量）/ `standard-used-quota`（标准存储）/ `exclusive-used-quota`（专属存储），默认按 `used-quota` 降序。

## quota vs quota apps 区分

- 查**单个**应用或空间的用量 → `quota`（`--app` / `--space` 二选一）。
- 列出**全部**应用并按用量排序、翻页拉全量 → `quota apps`（`--cursor` 传上次返回的 `nextToken`）。

## 参考

- [`../drive.md` §意图判断](../drive.md#意图判断)（如何路由到本组命令）
