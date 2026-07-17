---
name: dingtalk-ding
description: DING 紧急消息（应用内 / 短信 / 电话）。Use when 用户说 DING一下/紧急通知/电话DING/短信DING/必达消息/电话叫人/以我的名义发DING/个人DING/撤回DING。不做普通群聊消息（走 dingtalk-chat）、企业外呼（走 dingtalk-outbound-call）。命令前缀：dws ding。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 DING 紧急消息 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[ding.md](references/ding.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service ding --format json` 读取参数、约束、风险和示例，并以 `dws ding <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws ding +list` | read | 查询 DING 消息列表 |
| `dws ding +recall-personal` | high-risk-write | 撤回本人发起的 DING |
| `dws ding +receiver-status` | read | 查询 DING 消息接收人已读状态 |
| `dws ding +send-personal` | write | 以本人身份发送 DING 给指定人 |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "DING 张三" / "应用内紧急通知" | `dws ding message send --type app --users <userId> --content "<内容>"` |
| "短信 DING" | `dws ding message send --type sms --users <userId> --content "<内容>"` |
| "电话 DING" / "电话叫人" | `dws ding message send --type call --users <userId> --content "<内容>"` |
| "撤回 DING" | `dws ding message recall --robot-code <robotCode> --id <openDingId>` |
| "以我的名义发 DING / 个人 DING" | `dws ding message send-personal --users <openDingTalkId> --content "<内容>"` |
| "以我的名义撤回 DING" | `dws ding message recall-personal --id <openDingId>` |
| "DING 历史 / 接收状态" | `dws ding message list` / `dws ding message receiver-status` |

## 跨产品协作

- 接收人是人名 → 先用 `dingtalk-aisearch` 拿 `userId`
- 普通通知（不需必达）→ 切到 `dingtalk-chat`
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)；[Lite Recipe](references/lite-recipes.md)。
