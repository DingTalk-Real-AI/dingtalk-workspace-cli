---
name: dingtalk-workbench
description: 钉钉工作台应用管理。Use when 用户说 工作台/工作台应用/查应用列表/应用详情。Distinct from dingtalk-aiapp(AI 应用生成)。命令前缀：dws workbench。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉工作台 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[workbench.md](references/workbench.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "工作台所有应用" | `dws workbench app list` |
| "查应用详情" | `dws workbench app get --ids <id1>,<id2>`（批量并行） |

## 跨产品协作

- 创建新 AI 应用 → 切到 `dingtalk-aiapp`
