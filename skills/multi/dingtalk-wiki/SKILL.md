---
name: dingtalk-wiki
description: 钉钉知识库与空间管理。Use when 用户说 知识库/wiki/创建知识库/搜索知识库空间/我的文档/团队空间/空间成员/在指定知识库内的节点创建/列出/搜索/复制/移动/删除/知识库动态。知识库空间与空间内节点管理走本 skill（节点操作需 workspace）；未指定空间的全局文件管理与搜索走 dingtalk-drive，空间内单文档内容读写先用本 skill 定位再切到 dingtalk-doc。命令前缀：dws wiki。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉知识库 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 命令参考：[wiki.md](references/wiki.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "wiki +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws wiki <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service wiki --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws wiki +resolve-space` | read | 按名称搜索知识空间并解析出唯一 spaceId（只读） |
| `dws wiki +wiki-new-doc` | write | 在指定名称的知识库下新建一个文档节点（自动按空间名解析 workspaceId） |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建知识库" | `dws wiki space create --name "<名称>" [--desc "<描述>"]` |
| "搜索组织知识库空间" | `dws wiki space search --type orgWikiSpace --query "<关键词>"` |
| "我的文档 / 个人知识库" | `dws wiki space search --type myWikiSpace` |
| "列出知识库" | `dws wiki space list` |
| "列出我的文档空间" | `dws wiki space list --type myWikiSpace` |
| "列出知识库里的文件/节点" | `dws wiki node list --workspace <WS_ID>` |
| "在知识库里搜" | `dws wiki node search --workspace <WS_ID> --query "<关键词>"` |
| "在知识库里创建文档节点" | `dws wiki node create --workspace <WS_ID> --type adoc --name "<名称>"` |
| "知识库动态 / 最近有什么更新 / 谁改了什么" | `dws wiki feed list --workspace <WS_ID>` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 workspaceId/nodeId。每条命令必须带 `--format json`。`workspaceId`/`nodeId` 一律先查后用。

### SOP-1 找知识库（find-space）

**触发**：找知识库/列表空间/某知识库在哪。

1. **执行（必须）**：`dws wiki space list --format json`（列所有知识库/钉盘空间）；按名称找 `dws wiki space search --type orgWikiSpace --query "<名称>" --format json`。
2. **解析（必须）**：取真实 `workspaceId`；多候选让用户确认，**禁止**默认取第一个。`hasMore=true` 用 `nextPageToken` 翻页。

**禁止**：编造 workspaceId、把空间名当 ID。

### SOP-2 浏览 / 查节点（list-nodes）

**触发**：知识库里有哪些文档/列节点/看某节点。

1. **前置（必须）**：先按 SOP-1 拿 `workspaceId`。
2. **执行（必须）**：`dws wiki node list --workspace <workspaceId> --format json`（按需 `--folder <nodeId>` 看子节点）；按内容找 `dws wiki node search --workspace <workspaceId> --query "<关键词>" --format json`。
3. **解析（必须）**：取真实 `nodeId` + `nodeType`/`type`；锁定目标后按需切 `dingtalk-doc` 读写节点内容。

**禁止**：跳过 SOP-1 直接猜 workspaceId、用 `node list` 替代 `node search` 做关键词查找。

### SOP-3 建节点（create-node）

**触发**：在知识库建文档/页面。

1. **执行（必须）**：`dws wiki node create --workspace <workspaceId> --type adoc --name "<名称>" --format json`（按需 `--parent-id <父节点>`）；返回取 `nodeId`。
2. **写内容（必须）**：节点内容编辑切 `dingtalk-doc`，用 `dws doc update --node <nodeId> --mode overwrite|append --content-file <tmp.md> --yes`；写后 `doc read` 回读。
3. **验证（必须）**：`dws wiki node list --workspace <workspaceId> --format json` 复核节点已建。

**禁止**：在 wiki 内直接拼内容（应切 doc 写）、建后不回读。

### SOP-4 移动 / 复制 / 删节点（mutate-node）

**触发**：移动节点/复制节点/删节点。

1. **执行（必须）**：移动 `dws wiki node move --workspace <workspaceId> --node <nodeId> --folder <目标文件夹nodeId> --format json`；复制 `dws wiki node copy --workspace <workspaceId> --node <nodeId> --folder <目标文件夹nodeId> --format json`；删除 `dws wiki node delete --workspace <workspaceId> --node <nodeId>`（**必须**先与用户确认，再加 `--yes`）。
2. **验证（必须）**：移动/复制后执行 `dws wiki node list --workspace <workspaceId> --folder <目标文件夹nodeId> --format json`，从真实返回确认目标节点；删除后回查确认节点已不存在或进入回收站。

**禁止**：未确认就删除、编造 nodeId/folder、把 `--node-id` / `--target-parent-id` 当作公开参数。

## 高频硬约束

- `space search` 用 `--query`，不要用 `--keyword`；组织知识库显式加 `--type orgWikiSpace`。
- 用户说"我的文档/个人空间/my workspace"时必须用 `dws wiki space search --type myWikiSpace --format json`；该模式不需要 keyword。
- 用户给空关键词时，不要构造空 `--query ""`；若语义是我的文档则用 `--type myWikiSpace`，否则请用户补关键词。
- 搜到空间后复用返回的 `workspaceId/id`；空间内节点创建/列表/搜索用 `wiki node`，具体文档内容读写切到 `dingtalk-doc`，复制/移动切到 `dingtalk-drive`。
- 所有 `dws wiki` 命令加 `--format json`。

## 跨产品协作

- 知识库内具体文档读写 → 切到 `dingtalk-doc`
- 普通钉盘文件存储及复制移动 → 切到 `dingtalk-drive`
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
