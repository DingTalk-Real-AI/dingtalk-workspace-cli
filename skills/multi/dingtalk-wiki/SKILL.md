---
name: dingtalk-wiki
description: 钉钉知识库与空间管理。Use when 用户说 知识库/wiki/创建知识库/搜索知识库空间/我的文档/团队空间/空间成员/空间内节点创建/列出/搜索/复制/移动/删除。空间内单文档内容读写先用本 skill 定位再切到 dingtalk-doc；钉盘文件操作走 dingtalk-drive。命令前缀：dws wiki。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉知识库 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[wiki.md](references/wiki.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service wiki --format json` 读取参数、约束、风险和示例，并以 `dws wiki <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws wiki +space-search` | read | 搜索知识库 |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建知识库" | `dws wiki space create --name "<名称>" [--desc "<描述>"]` |
| "搜索组织知识库空间" | `dws wiki space search --query "<关键词>"` |
| "我的文档 / 个人知识库" | `dws wiki space list --type myWikiSpace` |
| "列出知识库" | `dws wiki space list` |
| "列出我的文档空间" | `dws wiki space list --type myWikiSpace` |
| "列出知识库里的文件/节点" | `dws wiki node list --workspace <WS_ID>` |
| "在知识库里搜" | `dws wiki node search --workspace <WS_ID> --query "<关键词>"` |
| "在知识库里创建文档节点" | `dws wiki node create --workspace <WS_ID> --type adoc --name "<名称>"` |
| "复制 / 移动库内节点" | `dws wiki node copy` / `dws wiki node move` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 workspaceId/nodeId。每条命令必须带 `--format json`。`workspaceId`/`nodeId` 一律先查后用。

### SOP-1 找知识库（find-space）

**触发**：找知识库/列表空间/某知识库在哪。

1. **执行（必须）**：`dws wiki space list --type orgWikiSpace --format json`；按名称找 `dws wiki space search --query "<名称>" --format json`。用户明确说“我的文档/个人空间”时改用 `dws wiki space list --type myWikiSpace --format json`。
2. **解析（必须）**：取真实 `workspaceId`；多候选让用户确认，**禁止**默认取第一个。`hasMore=true` 时将返回的 `nextCursor` 传给 `--cursor` 翻页。

**禁止**：编造 workspaceId、把空间名当 ID。

### SOP-2 浏览 / 查节点（list-nodes）

**触发**：知识库里有哪些文档/列节点/看某节点。

1. **前置（必须）**：先按 SOP-1 拿 `workspaceId`。
2. **执行（必须）**：`dws wiki node list --workspace <workspaceId> --format json`（按需 `--folder <nodeId>` 看子节点）；按内容找 `dws wiki node search --workspace <workspaceId> --query "<关键词>" --format json`。
3. **解析（必须）**：取真实 `nodeId` + `nodeType`/`type`；锁定目标后按需切 `dingtalk-doc` 读写节点内容。

**禁止**：跳过 SOP-1 直接猜 workspaceId、用 `node list` 替代 `node search` 做关键词查找。

### SOP-3 建节点（create-node）

**触发**：在知识库建文档/页面。

1. **执行（必须）**：`dws wiki node create --workspace <workspaceId> --type adoc --name "<名称>" --format json`（按需 `--folder <父节点>`）；返回取 `nodeId`。
2. **写内容（必须）**：节点内容编辑切 `dingtalk-doc`，用 `dws doc update --node <nodeId> --mode overwrite|append --content-file <tmp.md> --yes`；写后 `doc read` 回读。
3. **验证（必须）**：`dws wiki node list --workspace <workspaceId> --format json` 复核节点已建。

**禁止**：在 wiki 内直接拼内容（应切 doc 写）、建后不回读。

### SOP-4 移动 / 复制 / 删节点（mutate-node）

**触发**：移动节点/复制节点/删节点。

1. **执行（必须）**：移动 `dws wiki node move --workspace <workspaceId> --node <nodeId> --folder <目标文件夹nodeId> --format json`；复制 `dws wiki node copy --workspace <workspaceId> --node <nodeId> --folder <目标文件夹nodeId> --format json`；删除 `dws wiki node delete --workspace <workspaceId> --node <nodeId> --yes --format json`（**必须**先与用户确认，确认后才能加 `--yes`）。

**禁止**：未确认就删除/移动、编造 nodeId/targetParentId。

## 高频硬约束

- `space search` / `node search` 统一用 `--query`，不要生成遗留别名 `--keyword`；组织知识库搜索无需额外传 `--type`。
- 用户说"我的文档/个人空间/my workspace"时必须用 `dws wiki space list --type myWikiSpace --format json`。
- 用户给空关键词时，不要构造空 `--query ""`；若语义是我的文档则用 `space list --type myWikiSpace`，否则请用户补关键词。
- 搜到空间后复用返回的 `workspaceId/id`；空间内节点创建/列表/搜索/复制/移动用 `wiki node`，具体文档内容读写切到 `dingtalk-doc`；钉盘文件的全局搜索、上传下载和移动复制走 `dingtalk-drive`。
- 所有 `dws wiki` 命令加 `--format json`。

## 跨产品协作

- 知识库内具体文档读写 → 切到 `dingtalk-doc`
- 钉盘文件存储、上传下载与全局文件搜索 → 切到 `dingtalk-drive`
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)；[Lite Recipe](references/lite-recipes.md)。
