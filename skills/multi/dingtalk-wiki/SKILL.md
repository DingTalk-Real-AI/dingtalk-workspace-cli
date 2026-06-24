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

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[wiki.md](references/wiki.md)。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建知识库" | `dws wiki space create --name "<名称>" [--desc "<描述>"]` |
| "搜索知识库空间" | `dws wiki space search --query "<关键词>" [--limit <1-20>]` |
| "我的文档 / 个人知识库" | `dws wiki space list --type myWikiSpace` |
| "列出组织知识库" | `dws wiki space list [--type orgWikiSpace] [--limit <1-50>]` |

## 评测高频硬约束

- `space search` 用 `--query`；`search` 支持 `--type myWikiSpace` 查询个人知识库，但按类型列出空间优先走 `space list --type myWikiSpace/orgWikiSpace`。
- 用户说"我的文档/个人空间/my workspace"时优先用 `dws wiki space list --type myWikiSpace --format json`。
- 用户给空关键词时，不要构造空 `--query ""`；若语义是我的文档则走 `space list --type myWikiSpace`，否则请用户补关键词。
- 搜到空间后复用返回的 `workspaceId/id`，知识库内具体文档的创建、搜索、读写切到 `dingtalk-doc`，不要在 `wiki` 下编造 doc 子命令。
- `workspaceId` 是知识库空间 ID，只能用于 `wiki space/member --workspace`、`doc --workspace` 或 `doc search --workspace-ids`；不要传给 `doc list --folder`，也不要使用不存在的 `--space-id`。
- 读取知识库内指定文档固定链路：`wiki space search/list` 取 `workspaceId` → `doc search --query "<文档名>" --workspace-ids <workspaceId>` 取 `nodeId` → `doc read --node <nodeId>`。
- 所有 `dws wiki` 命令加 `--format json`。

## 跨产品协作

- 知识库内具体文档读写 → 切到 `dingtalk-doc`
- 文件存储 → 切到 `dingtalk-drive`
