---
name: dingtalk-wiki
description: 钉钉知识库（Wiki 空间）。Use when 用户说 知识库/wiki/创建知识库/搜索知识库空间/我的文档/知识库归档。Distinct from dingtalk-doc(单文档编辑)、dingtalk-drive(钉盘文件)。命令前缀：dws wiki。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉知识库 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[wiki.md](references/wiki.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建知识库" | `dws wiki space create --name "<名称>" [--description "<描述>"]` |
| "搜索知识库空间" | `dws wiki space search --keyword "<关键词>" [--limit <1-20>]` |
| "我的文档 / 个人知识库" | `dws wiki space list --type myWikiSpace` |
| "列出组织知识库" | `dws wiki space list [--type orgWikiSpace] [--limit <1-50>]` |

## 评测高频硬约束

- `space search` 用 `--keyword`，不要用 `--query`；`search` 支持 `--type myWikiSpace` 查询个人知识库，但按类型列出空间优先走 `space list --type myWikiSpace/orgWikiSpace`。
- 用户说"我的文档/个人空间/my workspace"时优先用 `dws wiki space list --type myWikiSpace --format json`。
- 用户给空关键词时，不要构造空 `--keyword ""`；若语义是我的文档则走 `space list --type myWikiSpace`，否则请用户补关键词。
- 搜到空间后复用返回的 `workspaceId/id`，知识库内具体文档的创建、搜索、读写切到 `dingtalk-doc`，不要在 `wiki` 下编造 doc 子命令。
- 所有 `dws wiki` 命令加 `--format json`。

## 跨产品协作

- 知识库内具体文档读写 → 切到 `dingtalk-doc`
- 文件存储 → 切到 `dingtalk-drive`
