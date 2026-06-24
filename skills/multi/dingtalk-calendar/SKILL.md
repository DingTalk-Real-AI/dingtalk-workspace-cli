---
name: dingtalk-calendar
description: 钉钉日历与会议室。Use when 用户说 约会议/查日程/订会议室/查闲忙/加参会人/改期/取消会议/今天的日程/本周日程/共同空闲。Distinct from dingtalk-conference(视频会议发起/预约/邀请入会/会中控制)、dingtalk-minutes(听记)、dingtalk-todo(待办)。命令前缀：dws calendar。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉日历 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[calendar.md](references/calendar.md)；剧本：[03-meeting.md](references/03-meeting.md)。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "今天 / 明天 / 本周日程" | `python scripts/calendar_today_agenda.py [today\|tomorrow\|week]` |
| "约会议（含参会人 + 会议室）" | `python scripts/calendar_schedule_meeting.py --title "<主题>" --start "<起>" --end "<止>" [--users <ids>] [--book-room]` |
| "多人共同空闲" | `python scripts/calendar_free_slot_finder.py --users <ids> --date <yyyy-MM-dd>` |
| "查闲忙" | `dws calendar event list --start "<ISO>" --end "<ISO>"` |
| "加参会人" / "订房" / "取消" | `dws calendar participant add` / `room add` / `event delete` |

## 执行硬约束

- 多轮日程任务必须保留 `eventId`，后续加人、移人、订房、换房、改描述、删除都基于同一个 `eventId` 执行；不要重新创建重复日程。
- 用户明确说"帮我订一个空闲会议室"时，`room search` 返回可用会议室后直接选择第一个可预订且不需要自定义审批的 `roomId` 执行 `room add`；不要把选择权抛回用户导致任务停住。
- 已有日程订房：`dws calendar room search --start ... --end ... --format json` → `dws calendar room add --event <EVENT_ID> --rooms <ROOM_ID> --format json` → `event get` 或 `room/busy` 验证。
- 换会议室：先 `room delete --event <EVENT_ID> --rooms <OLD_ROOM_ID>`，再 `room add --event <EVENT_ID> --rooms <NEW_ROOM_ID>`，最后回查；不要只更新 `--location`。
- 参会人变化用 `participant add/delete`，日程描述变化用 `event update --desc`，删除日程用 `event delete --id`。用户当前消息已明确要求删除/取消时可直接执行；否则先确认。
- 脚本失败或参数不完整时，立即降级到明确的 `dws calendar event/participant/room` 命令，不要停在"我要查看用法"。
- 所有 dws 命令带 `--format json`；查询时间必须显式 `--start` / `--end`。

## 跨产品协作

- 视频会议发起 / 入会链接 / 邀请入会 / 会中控制 → 切到 `dingtalk-conference`
- 会后摘要 / 待办 → 切到 `dingtalk-minutes`
- 参会人按人名 → 先用 `dingtalk-aisearch` 解析

## 注意

`schedule-meeting` 必须读 [03-meeting.md](references/03-meeting.md) 中的「两准则」「搜房失败硬门禁」，禁止假设 `roomId`。
