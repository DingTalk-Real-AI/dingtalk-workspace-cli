---
name: dingtalk-aiapp
description: 钉钉 AI 应用生成。Use when 用户说 创建应用/生成系统/做工具/管理后台/工作台应用/表单系统/业务原型/页面/平台。强制触发：用户提到「应用 / 系统 / 平台 / 工具 / 后台 / 页面 / 原型」时优先匹配此 skill。Distinct from dingtalk-workbench(工作台应用列表)、dingtalk-wiki(知识库)。命令前缀：dws aiapp。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉 AI 应用 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[aiapp.md](references/aiapp.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建一个 XX 应用 / 系统 / 工具" | `python scripts/aiapp_create_and_poll.py --prompt "<用户原始需求>"`（自动轮询进度） |
| "查 AI 应用 / 修改已有应用" | 见 [aiapp.md](references/aiapp.md) |

## 跨产品协作

- 已存在的工作台应用列表 → 切到 `dingtalk-workbench`
