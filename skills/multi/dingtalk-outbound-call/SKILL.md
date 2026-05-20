---
name: dingtalk-outbound-call
description: 钉钉 AI 语音外呼。Use when 用户说 外呼/AI电话/语音外呼/批量外呼/电话提醒/AI打电话。Distinct from dingtalk-ding(电话 DING 通知)。命令前缀：dws outbound-call。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# AI 语音外呼 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[outbound-call.md](references/outbound-call.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "给张三打电话提醒明天 9 点开会" | `dws outbound-call create --callee "张三" --prompt "<提醒内容>" --greeting "<开场白>"` |
| "批量外呼（JSON 文件）" | `dws outbound-call create --batch <input.json>` |
| "查外呼任务结果" | `dws outbound-call get --task-id <id>` |

## 跨产品协作

- 紧急电话 DING（直接拨人不走 AI）→ 切到 `dingtalk-ding`（type=call）
