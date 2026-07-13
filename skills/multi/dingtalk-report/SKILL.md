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

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。22 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[report.md](references/report.md)；剧本：[05-reporting.md](references/05-reporting.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "今天收到的日志" | `python scripts/report_received_today.py` |
| "看日志模版" | `dws report template list` → `dws report template get --name "<模版名>"` |
| "提交日报 / 周报（按模版）" | `dws report entry submit --template-id <id> --contents-file <CWD 下的 .json>` |
| "我收到的日志" | `dws report inbox list --start <ISO> --end <ISO> --cursor 0 --size 20` |
| "我已发送的日志" | `dws report outbox list --cursor 0 --size 20` |
| "查看某条日志正文" | `dws report entry get --report-id <id>` |
| "日志已读统计" | `dws report entry stats --report-id <id>` |
| "生成日报 / 周报 / 月报 / 主题报告" | 见 [05-reporting.md](references/05-reporting.md) recipe |

> 已废弃的旧命令 `report list` / `report detail` / `report sent` / `report stats` / `report create` / `report template detail` 均标 [deprecated]，请一律改用上表的 `inbox list` / `outbox list` / `entry get` / `entry stats` / `entry submit` / `template get`。

## 日志查询硬约束

- 查“收到的日志”必须用 `dws report inbox list --start "<ISO>" --end "<ISO>" --cursor 0 --size 20 --format json`，并把“今天 / 最近 30 天”等时间词先展开成完整 ISO 起止时间；查“我发出的日志”用 `dws report outbox list --cursor 0 --size 20 --format json`。
- 列表返回后，后续 `entry get` / `entry stats` 必须复用同一个 `reportId`（在返回的 `_internalDetailCommands[].command` 里）；不要重新挑选、猜测或改用标题。
- 用户要正文时用 `dws report entry get --report-id <reportId>`；用户要已读/统计时用 `dws report entry stats --report-id <reportId>`。

## 跨产品协作

- 日报内容来源（待办 / 听记 / OA / 邮件 / 群消息）→ 多源采集，按 dws-shared 的 conventions.md 并行执行
- 把汇总写文档 → 切到 `dingtalk-doc`（`dws doc create` + `dws doc update --mode append`）
- 注意：`submit-report` 走 report 模版提交，**不要**走 doc 写文档
