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

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[report.md](references/report.md)；剧本：[05-reporting.md](references/05-reporting.md)；分页查询辅助脚本：[`scripts/report_received_today.py`](scripts/report_received_today.py)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service report --format json` 读取参数、约束、风险和示例，并以 `dws report <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws report +inbox-list` | read | 列出我收到的日报（按时间范围分页） |
| `dws report +outbox-list` | read | 列出我发出的日报（可选时间/模版名过滤） |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "今天 / 最近 / 最近一周收到的日志" | `dws report inbox list --start "<ISO+08>" --end "<ISO+08>" --cursor 0 --size 20 --format json` |
| "看日志模版" | `dws report template list --format json` → `dws report template get --name "<模版名>" --format json` |
| "提交日报 / 周报（按模版）" | `dws report entry submit --template-id <id> --contents-file <tmp.json> --format json` |
| "我已发送 / 我创建 / 我发过的日志" | `dws report outbox list --cursor 0 --size 20 --format json` |
| "看日志正文 / 总结多篇日志" | 列表取 `reportId` 后逐篇 `dws report entry get --report-id <id> --format json` |
| "日志已读统计" | `dws report entry stats --report-id <id> --format json` |
| "生成日报 / 周报 / 月报 / 主题报告" | 见 [05-reporting.md](references/05-reporting.md) recipe |

## Lite Recipes

### view-report-inbox

1. 收到的日志：把用户时间词转换为 Asia/Shanghai 的完整 ISO 起止时间，再执行 `dws report inbox list --start "<YYYY-MM-DDT00:00:00+08:00>" --end "<YYYY-MM-DDT23:59:59+08:00>" --cursor 0 --size 20 --format json`；用户只说“最近 / 近期 / 最近收到 / 最近一周”时默认最近 7 天。
2. 我发过 / 我创建 / 已发送：第一条查询必须用 `dws report outbox list --cursor 0 --size 20 --format json`；有时间范围时补 `--start` / `--end`。
3. 按发件人过滤收件箱：先 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 取 `userId/staffId`，再给 `inbox list` 加 `--sender-user-ids <id>`；空结果必须说明未找到该发件人的日志，不得改选其他人。
4. 用户要正文、详情、汇总或总结多篇日志时，对选中的每篇日志逐条执行 `dws report entry get --report-id <reportId> --format json`；不足 5 篇按实际数量说明。

### check-report-read-status

`dws report entry stats --report-id <reportId> --format json`；`reportId` 来自列表结果或用户明确提供的 ID，不要用标题猜测。

## 日志查询硬约束

- 第一条查询命令必须按视角选新路径：收到/收件箱/别人发给我 → `dws report inbox list`；已发/我创建/我发过 → `dws report outbox list`。不要先生成 `report list` / `report sent` / `report detail` / `report stats` 等 deprecated alias。
- `--size` 最大只能传 20；需要更多结果时按 `cursor` 分页，禁止传 50/100。
- 查“收到的日志”必须传 `--start` / `--end`，时间一律使用 Asia/Shanghai 的 `YYYY-MM-DDT00:00:00+08:00` 到 `YYYY-MM-DDT23:59:59+08:00`；禁止 UTC `Z` / `date -u`。
- 用户说“最近 / 近期 / 最近收到”但未给具体范围时，默认最近 7 天；“最近一周”同样展开为最近 7 天。
- 按发件人过滤收件箱时，先用 `dingtalk-aisearch` 查人得到 `userId/staffId`，再调用 `dws report inbox list ... --sender-user-ids <id> --format json`；没有匹配结果时必须说明未找到该发件人的日志，不得改选其他发件人。
- 列表返回后，后续 `entry get` / `entry stats` 必须复用同一个 `reportId`；不要重新挑选、猜测或改用标题。
- 用户要正文、详情、汇总或总结多篇日志时，必须对选中的每篇日志逐一调用 `dws report entry get --report-id <reportId> --format json`，不要把 list 当正文接口。
- CSV / 群聊导出的“日志”、系统日志、聊天记录核验不属于钉钉 OA 日志；不要用本 skill 强行处理，应转到文件/群聊/表格相关 skill。

## 跨产品协作

- 日报内容来源（待办 / 听记 / OA / 邮件 / 群消息）→ 多源采集，按 dws-shared 的 conventions.md 并行执行
- 把汇总写文档 → 切到 `dingtalk-doc`（`dws doc create` + `dws doc update --mode append`）
- 注意：`submit-report` 走 report 模版提交，**不要**走 doc 写文档
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)；[Lite Recipe](references/lite-recipes.md)。
