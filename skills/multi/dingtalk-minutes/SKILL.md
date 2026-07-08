---
name: dingtalk-minutes
description: 钉钉 AI 听记。Use when 用户说 听记/会议录音/会议纪要/AI摘要/转写/关键字/听记标题/会后待办提取/分享听记。Distinct from dingtalk-calendar(日程)、dingtalk-report(日报)、dingtalk-doc(普通文档)。命令前缀：dws minutes。URL `shanji.dingtalk.com` 一定路由到此 skill。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 AI 听记 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[minutes.md](references/minutes.md)；剧本：[07-minutes.md](references/07-minutes.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "我的听记列表" | `dws minutes list mine [--query "<关键词>"] [--start "<ISO>"] [--end "<ISO>"]` |
| "查某段时间/最近/本周/上月的听记" | `dws minutes list all --start "<ISO>" --end "<ISO>" [--query "<关键词>"]` |
| "看一篇听记摘要" | `dws minutes get summary --id <taskUuid>` |
| "看转写 / 原文" | `dws minutes get transcription --id <taskUuid>` |
| "近期听记摘要合并" | `python scripts/minutes_recent_summary.py --max 5` |
| "提取会议待办" | `python scripts/minutes_extract_todos.py --id <taskUuid>` |
| "改听记标题" | `dws minutes update title --id <taskUuid> --title "<新标题>"` |

## 评测高频硬约束

- `shanji.dingtalk.com` URL 必须走 `dws minutes`，禁止用浏览器或 `read_file` 打开链接。自动提取 taskUuid 后调用 `get info/summary/transcription/todos`。
- 用户给了时间线索（今天、本周、上周、上月、最近 N 天、某日期范围）时，必须自行计算 `--start` / `--end`，格式用 ISO-8601，如 `2026-05-11T00:00:00+08:00`。不要反问用户时间范围。
- 未指定 mine/shared 时，检索型任务默认 `list all`；如果只查"我创建的"才用 `list mine`。
- 不要全量拉取后本地过滤时间。时间范围和关键词能服务端过滤时必须放进同一条 `list all --start --end --query`。
- 列表为空时按顺序兜底：同范围 `list all` → 去掉关键词但保留时间范围 → 明确告知无数据。禁止用模板或虚构听记内容继续生成纪要/周报。
- 生成纪要、文档、待办、周报前，必须先完成 `list` → 选定真实 `taskUuid` → `get summary`；需要原文或行动项时继续 `get transcription` / `get todos`。前置数据没拿到就停止并说明卡点。
- 所有 dws 命令带 `--format json`，不要用 shell 管道、重定向、`head`、`grep`、`jq`。

## 跨产品协作

- 提取的待办批量建任务 → 切到 `dingtalk-todo`（`scripts/todo_batch_create.py`）
- 摘要发给同事 → 切到 `dingtalk-chat`
- 日程 / 会议室 → 切到 `dingtalk-calendar`
