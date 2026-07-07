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

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令以当前 dws 二进制为准**。服务发现和动态 schema 已下线，本文档随版本内嵌发布；执行前用 `dws <cmd> --help` 或 `--dry-run` 验证 flag 与命令是否存在。


> 命令参考：[ding.md](references/ding.md)。

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
