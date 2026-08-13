---
name: dingtalk-drive
description: 钉钉文件管理（存储层，覆盖钉盘与文档空间两个存储域）。Use when 用户说 钉盘/上传文件/下载文件/文件夹/查文件/找文件/全局搜索文件/复制/移动/重命名/删除/回收站/还原删除文件/权限管理/普通文件下载；也承接钉钉文档的这些管理动作（doc 侧同名原子命令已弃用）。文档正文编辑与导出 docx/markdown/pdf 走 dingtalk-doc，知识库空间与空间内节点组织走 dingtalk-wiki。命令前缀：dws drive。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉盘 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 命令参考：[drive.md](references/drive.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`drive` 当前有 28 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图按下方路由。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws schema search --query "<用户意图>" --product drive --limit 5` 同时搜索原子命令和 shortcut。`dws shortcut list` 只作人工审计/兼容 fallback；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "看钉盘文件 / 文件夹列表" | `dws drive +list [--folder <dentryUuid>]` |
| "钉盘目录树" | `python scripts/drive_tree_list.py --depth 2` |
| "查文件元数据/统计/公开状态/封面" | `dws drive +inspect --node <dentryUuid> [--include-stats/--include-publish/--include-cover]` |
| "搜文件 / 找文件" | `dws drive +search --query "<关键词>"` |
| "下载文件" | `dws drive +download --node <dentryUuid> --output <工作目录内相对路径>` |
| "上传文件" | `dws drive +upload --file <工作目录内相对路径> [--folder <id>]` |
| "建钉盘文件夹 / 建快捷方式" | `dws drive +create-folder ...` / `dws drive +create-shortcut ...` |
| "复制/移动/重命名/删除" | `dws drive +copy/+move/+rename/+delete ...` |
| "回收站 / 还原删除的文件" | `dws drive +recycle-list` / `dws drive +recycle-restore --id <recycleItemId>` |
| "收藏 / 取消收藏 / 收藏列表" | `dws drive +star-add/+star-remove/+star-list ...` |
| "查/关互联网公开" | `dws drive +publish-get/+publish-unset ...`；开启公开当前无已验证 eligible 节点，不推荐 Agent 调用 |
| "普通文件历史版本" | `dws drive +version-history/+version-get/+version-download/+version-revert ...` |
| "在线文档评论/导入导出/权限" | 切到 `dws doc +comment-* / +import / +export / +access-*` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 dentryUuid/nodeId。每条命令必须带 `--format json`。破坏性操作（删除/移动/覆盖/公开）**必须**先与用户确认。

### SOP-1 找文件（find-file）

**触发**：找文件/搜文件/我的文件/最近文件/某文档在哪。

1. **选源（必须）**：最近访问 → `dws drive +recent --limit <n> --format json`（翻页用上次返回的 `nextCursor` 传 `--cursor`）；按内容/名称全局搜 → `dws drive +search --query "<关键词>" --format json`；浏览某目录 → `dws drive +list --folder <dentryUuid> --format json`。
2. **解析（必须）**：取真实 `dentryUuid`（= `id`/`nodeId`）；多候选让用户确认，**禁止**默认取第一个。
3. **下钻（必须）**：根目录没命中时，进入最相关文件夹继续 `drive +list --folder`，必要时 `python scripts/drive_tree_list.py --depth 2` 递归，**禁止**只看根目录就放弃。
4. **回读元数据（必须）**：命中后 `dws drive +inspect --node <dentryUuid> --format json`，按 `extension` 确认类型。

**禁止**：编造 dentryUuid、只看根目录放弃、用 `drive +list` 替代 `drive +search` 做全局查找。

### SOP-2 上传 / 下载（upload-download）

**触发**：上传文件/下载文件/传到钉盘/用本地文件覆盖已有文件。

1. **上传（必须）**：先把文件暂存到工作目录，再执行 `dws drive +upload --file <相对路径> [--folder <dentryUuid>] --format json`；shortcut 内部已提交并回读远端元数据，返回取 `data.nodeId`。
2. **覆盖（必须）**：先 `dws drive +inspect --node <dentryUuid> --format json`，记录真实 `extension` 和原 `name`。`extension=md` 切 `dingtalk-misc` 的 `references/markdown.md`；其他普通文件在用户确认后执行 `dws drive +upload --node <dentryUuid> --file <相对路径> --file-name "<原name>" --format json`。`adoc` / `axls` / `able` 切对应内容 skill/reference，不按普通文件覆盖。
3. **下载（必须）**：先 `dws drive +inspect --node <dentryUuid> --format json` 判断类型——`extension=adoc` 切 `dingtalk-doc` 用 `doc +export`；普通文件执行 `dws drive +download --node <dentryUuid> --output <工作目录内相对路径> --format json`，并校验 `data.sizeBytes > 0` 和本地文件真实存在。

**禁止**：对在线文档用 `drive +download`（会失败）、普通文件覆盖时省略 `--file-name` 导致隐式重命名、只看退出码而不检查统一结果。

### SOP-3 文件夹 / 复制 / 移动 / 重命名（folder-ops）

**触发**：建文件夹/复制/移动/重命名。

1. **执行（必须）**：建钉盘文件夹 `dws drive +create-folder --name "<名称>" [--folder <id>]`；复制 `drive +copy --node <dentryUuid> --folder <目标>`；移动 `drive +move --node <dentryUuid> --folder <目标>`；重命名 `drive +rename --node <dentryUuid> --name "<新名>"`。全部加 `--format json`。
2. **验证（必须）**：这些 shortcut 内部已经读回；调用方仍须检查 `ok=true`、`outcome=success` 和 `data` 中的 nodeId/对象，不能只看进程退出码。

**禁止**：未确认就移动/覆盖他人文件、跳过回读。

### SOP-4 回收站（recycle）

**触发**：删文件/回收站/还原。

1. **删除（必须）**：`dws drive +delete --node <dentryUuid> --format json`（**必须**先与用户确认，再由执行层添加 `--yes`）。
2. **还原（必须）**：`dws drive +recycle-list --format json` 按 `originalName/originalPath` 确认目标并取 `recycleItemId` → 用户确认后执行 `dws drive +recycle-restore --id <recycleItemId> --format json`；成功时 Shortcut 会用服务返回的 nodeId 读回。

**禁止**：未确认就删除、把 `dentryUuid` 当 `recycleItemId` 传给 restore。

### SOP-5 互联网公开（publish）

**触发**：互联网公开/取消公开/查公开状态。

1. **执行（必须）**：查状态 `dws drive +publish-get --node <dentryUuid> --format json`；用户确认后关闭公开 `dws drive +publish-unset --node <dentryUuid> --format json`。`+publish-set` 在普通文件和在线文档真实夹具上均返回 `operation.notSupported`，当前不进入 Agent 公开入口；只有未来先找到服务端明确支持的 eligible 节点并通过真实 set→get→unset 闭环后才能启用。
2. **边界（必须）**：对外公开前**必须**与用户确认边界与后果。

**禁止**：未确认就改变公开状态；把 `operation.notSupported` 当成功；在没有 eligible 节点真实闭环的情况下启用 `+publish-set`。

## 高频硬约束

- 查找文件不要只看根目录后放弃；根目录没命中时，进入最相关的目标文件夹继续 `drive +list --folder <dentryUuid>`，必要时用目录树脚本递归到合理深度。
- `drive +list` 默认 `--limit 20`，自动化场景里保守使用 `--limit 50` 以内并处理 `nextCursor` 翻页；不要因为参数边界报错反复重试。
- 全局找文件优先 `drive +search --query`；指定目录浏览用 `drive +list`，命中后必须 `drive +inspect --node <dentryUuid> --format json` 回读元数据。
- 删除、覆盖、移动等破坏性操作必须确认；上传、创建文件夹、下载后要读回或列目录验证。
- 所有 `dws drive` 命令加 `--format json`。

## 跨产品协作

- 文件内容编辑（钉钉文档）→ 切到 `dingtalk-doc`
- 知识库空间 → 切到 `dingtalk-wiki`
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
