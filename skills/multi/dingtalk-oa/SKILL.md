---
name: dingtalk-oa
description: 钉钉 OA 审批。Use when 用户说 OA/审批/待处理审批/同意审批/拒绝审批/撤销审批/已发起审批/审批记录/批量审批。Distinct from dingtalk-todo(待办任务)、dingtalk-report(日志)。命令前缀：dws oa。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉 OA 审批 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[oa.md](references/oa.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "待我处理的审批 / 7 天内待审" | `python scripts/oa_pending_review.py --days 7` |
| "查审批详情" | `dws oa approval get --id <approvalId>` |
| "同意 / 拒绝审批" | `dws oa approval approve --id <id> --yes` / `reject --id <id> --yes`（需用户确认） |
| "批量同意 / 批量拒绝" | `python scripts/oa_batch_approve.py --action approve --days 7` |
| "撤销审批" | `dws oa approval revoke --id <id>` |
| "我已发起的审批" | `dws oa approval list-mine` |

## 危险操作

`approval approve / reject` 不可撤回，必须先向用户展示摘要并获得明确同意，再加 `--yes`。

## 跨产品协作

- 催别人审批 → 在群里 @对方（`dingtalk-chat`），不要走 #1 消息剧本里的 escalate-ding
- 审批通过后建待办 → 切到 `dingtalk-todo`
