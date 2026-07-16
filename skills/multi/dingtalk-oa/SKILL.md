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

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[oa.md](references/oa.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service oa --format json` 读取参数、约束、风险和示例，并以 `dws oa <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws oa +list-cc` | read | 获取抄送当前用户的审批单列表 |
| `dws oa +list-executed` | read | 获取当前用户已经处理过的审批单列表 |
| `dws oa +list-forms` | read | 获取当前用户可见的审批表单列表 |
| `dws oa +list-pending` | read | 查询待我处理的审批（时间范围为 epoch 毫秒） |
| `dws oa +list-submitted` | read | 获取当前用户已发起的审批单列表 |
| `dws oa +my-initiated` | read | 列出我发起（提交）的审批单据 |
| `dws oa +search-forms` | read | 按关键字模糊搜索当前用户可见的审批表单 |
<!-- VISIBLE_SHORTCUTS_END -->

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
