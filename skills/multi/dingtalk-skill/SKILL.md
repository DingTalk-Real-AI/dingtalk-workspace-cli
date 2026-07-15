---
name: dingtalk-skill
description: 悟空技能管理（搜索 / 安装技能）。Use when 用户说 搜索技能/找技能/安装技能/技能市场/企业技能库。注意：这是元 skill —— 它管理的是 dws 平台上的其他 skill。命令前缀：dws skill。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 悟空技能管理 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。22 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[skill.md](references/skill.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "搜索技能 / 找技能" | `dws skill search --query "<关键词>" [--source "DingtalkMarket OrgInternal"]` |
| "下载技能包到本地临时目录" | `dws skill get --skill-id <id>` |
| "安装技能到 Agent 目录" | `dws skill install <skillId> <target>`（target: claude / cursor / codex / opencode / qoder / .） |

## 环境

- `DWS_SKILL_API_HOST` 覆盖技能 API 地址（默认 `https://mcp.dingtalk.com`）

## 兼容提示

- `dws skill find` → 用 `dws skill search --query <关键词>`
- `--scopes` 已废弃（会打印 `Flag --scopes has been deprecated, 请使用 --source 替代` 警告）→ 用 `--source`
- 前置：`skill search` 依赖企业开通技能市场；未开通时报「当前企业暂未开放此功能」，属企业级开关，非命令写法问题
