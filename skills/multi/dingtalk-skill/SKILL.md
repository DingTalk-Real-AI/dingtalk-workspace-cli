---
name: dingtalk-skill
description: 悟空技能管理（搜索 / 安装 / 发布技能）。Use when 用户说 搜索技能/找技能/安装技能/发布技能/上传技能到企业库/技能市场/企业技能库。注意：这是元 skill —— 它管理的是 dws 平台上的其他 skill。命令前缀：dws skill。
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
| "搜索技能 / 找技能" | `dws skill search --query "<关键词>" [--source DingtalkMarket\|OrgInternal]` |
| "安装技能" | `dws skill install --skill-id <id> [--force]` |
| "发布技能 / 上传技能到企业库" | `dws skill publish <path> --name <skillName> --version <semver> [--changelog "..."]` |

## 安全检测

- `securityStatus=failed` 的技能默认拒绝安装；只有明确 `--force` 才能强装
- 发布后进入安全检测流程

## 环境

- `DWS_SKILL_API_HOST` 覆盖技能 API 地址（默认 `https://aihub.dingtalk.com`）

## 兼容提示

- `dws skill find` → 用 `dws skill search --query <关键词>`
- `dws skill add` → 用 `dws skill install --skill-id <id>`
- `dws skill upload` → 用 `dws skill publish <path>`
