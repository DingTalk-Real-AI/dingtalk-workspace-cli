---
name: dingtalk-skill
description: 悟空技能管理（搜索 / 安装技能）。Use when 用户说 搜索技能/找技能/安装技能/技能市场/企业技能库。注意：这是元 skill —— 它管理的是 dws 平台上的其他 skill。命令前缀：dws skill。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 悟空技能管理 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[skill.md](references/skill.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "搜索技能 / 找技能" | `dws skill search --query "<关键词>" [--scopes "DingtalkMarket OrgInternal"]` |
| "下载技能包到本地临时目录" | `dws skill get --skill-id <id>` |
| "安装技能到 Agent 目录" | `dws skill install <skillId> <target>`（target: claude / cursor / codex / opencode / qoder / .） |

## 环境

- `DWS_SKILL_API_HOST` 覆盖技能 API 地址（默认 `https://aihub.dingtalk.com`）

## 兼容提示

- `dws skill find` → 用 `dws skill search --query <关键词>`
