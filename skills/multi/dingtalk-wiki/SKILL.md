---
name: dingtalk-wiki
description: 钉钉知识库（Wiki 空间）。Use when 用户说 知识库/wiki/创建知识库/搜索知识库空间/我的文档/知识库归档。Distinct from dingtalk-doc(单文档编辑)、dingtalk-drive(钉盘文件)。命令前缀：dws wiki。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉知识库 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[wiki.md](references/wiki.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建知识库" | `dws wiki space create --name "<名称>" [--description "<描述>"]` |
| "搜索知识库空间" | `dws wiki space search --keyword "<关键词>" [--limit <1-20>]` |
| "我的文档 / 个人知识库" | `dws wiki space list --type myWikiSpace` |
| "列出组织知识库" | `dws wiki space list [--type orgWikiSpace] [--limit <1-50>]` |

## 评测高频硬约束

- `space search` 用 `--keyword`，不要用 `--query`；`search` 没有 `--type` flag，按类型筛选请走 `space list --type myWikiSpace/orgWikiSpace`。
- 用户说"我的文档/个人空间/my workspace"时必须用 `dws wiki space list --type myWikiSpace --format json`；不要往 `search` 里拼 `--type`。
- 用户给空关键词时，不要构造空 `--keyword ""`；若语义是我的文档则走 `space list --type myWikiSpace`，否则请用户补关键词。
- 搜到空间后复用返回的 `workspaceId/id`，知识库内具体文档的创建、搜索、读写切到 `dingtalk-doc`，不要在 `wiki` 下编造 doc 子命令。
- 所有 `dws wiki` 命令加 `--format json`。

## 跨产品协作

- 知识库内具体文档读写 → 切到 `dingtalk-doc`
- 文件存储 → 切到 `dingtalk-drive`
