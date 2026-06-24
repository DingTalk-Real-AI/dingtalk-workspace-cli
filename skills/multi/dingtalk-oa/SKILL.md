---
name: dingtalk-oa
description: 钉钉 OA 审批。Use when 用户说 OA/审批/待处理审批/同意审批/拒绝审批/撤销审批/已发起审批/审批记录/批量审批。Distinct from dingtalk-todo(待办任务)、dingtalk-report(日志)。命令前缀：dws oa。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 OA 审批 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[oa.md](references/oa.md)。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "待我处理的审批 / 7 天内待审" | `python scripts/oa_pending_review.py --days 7` |
| "查审批详情" | `dws oa approval detail --instance-id <processInstanceId>` |
| "同意 / 拒绝审批" | `dws oa approval approve --instance-id <id> --task-id <taskId>` / `reject --instance-id <id> --task-id <taskId> --remark "<原因>"`（需用户确认） |
| "批量同意 / 批量拒绝" | `python scripts/oa_batch_approve.py --action approve --days 7` |
| "撤销审批" | `dws oa approval revoke --instance-id <id>` |
| "我已发起的审批" | `dws oa approval list-initiated --process-code <code> --start "<ISO-8601>"`（processCode 来自 `dws oa approval list-forms`） |

## 危险操作

`approval approve / reject` 不可撤回，必须先向用户展示摘要并获得明确同意，再加 `--yes`。

## 跨产品协作

- 催别人审批 → 在群里 @对方（`dingtalk-chat`），不要走 #1 消息剧本里的 escalate-ding
- 审批通过后建待办 → 切到 `dingtalk-todo`
