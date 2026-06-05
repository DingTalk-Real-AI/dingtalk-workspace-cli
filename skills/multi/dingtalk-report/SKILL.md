---
name: dingtalk-report
description: 钉钉日志（日报 / 周报 / 月报）。Use when 用户说 写日报/写周报/写月报/提交日志/查日志/收件箱日志/已发送日志/已读统计/按主题汇总报告。Distinct from dingtalk-doc(普通文档)、dingtalk-todo(待办)、dingtalk-minutes(听记)。命令前缀：dws report（别名 dws log）。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉日志 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[report.md](references/report.md)；剧本：[05-reporting.md](references/05-reporting.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "今天收到的日志" | `python scripts/report_received_today.py` |
| "看日志模版" | `dws report template list` → `dws report template detail --name "<模版名>"` |
| "提交日报 / 周报（按模版）" | `dws report create --template-id <id> --contents '[...]' |
| "以指定员工身份提交日志" | `dws report entry submit --sender-user-id <userId> --template-id <id> --contents-file <file>` |
| "我已发送的日志" | `dws report sent --start <ISO> --end <ISO>` |
| "日志已读统计" | `dws report stats --report-id <id>` |
| "生成日报 / 周报 / 月报 / 主题报告" | 见 [05-reporting.md](references/05-reporting.md) recipe |

## 日志查询硬约束

- 查“收到的日志”必须用 `dws report list --start "<ISO>" --end "<ISO>" --cursor 0 --size 20 --format json`，并把“今天 / 最近 30 天”等时间词先展开成完整 ISO 起止时间。
- 列表返回后，后续 `detail` / `stats` 必须复用同一个 `reportId`；不要重新挑选、猜测或改用标题。
- 用户要正文时用 `dws report detail --report-id <reportId>`；用户要已读/统计时用 `dws report stats --report-id <reportId>`。

## 指定发送人提交

- 未传 `--sender-user-id` 时，提交继续走默认 MCP `report.create_report`。
- 传 `--sender-user-id` 时，CLI 改走钉钉 OAPI `/topapi/report/create`，日志发送人是指定的 userId。
- OAPI 代提交要求配置自有应用 AppKey/AppSecret，并为应用申请“管理员工日志数据”权限；默认 MCP 凭证不能用于此路径。

## 跨产品协作

- 日报内容来源（待办 / 听记 / OA / 邮件 / 群消息）→ 多源采集，按 dws-shared 的 conventions.md 并行执行
- 把汇总写文档 → 切到 `dingtalk-doc`（`dws doc create` + `dws doc update --mode append`）
- 注意：`submit-report` 走 report 模版提交，**不要**走 doc 写文档
