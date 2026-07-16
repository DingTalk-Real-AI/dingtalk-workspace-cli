---
name: dingtalk-ding
description: DING 紧急消息（应用内 / 短信 / 电话）。Use when 用户说 DING一下/紧急通知/电话DING/短信DING/必达消息/电话叫人。Distinct from dingtalk-chat(普通群聊消息)、dingtalk-outbound-call(企业外呼)。命令前缀：dws ding。
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

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


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
| "撤回 DING" | `dws ding message recall --id <openDingId>` |

## 跨产品协作

- 接收人是人名 → 先用 `dingtalk-aisearch` 拿 `userId`
- 普通通知（不需必达）→ 切到 `dingtalk-chat`
