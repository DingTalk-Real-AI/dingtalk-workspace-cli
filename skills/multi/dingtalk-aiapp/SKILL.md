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

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[aiapp.md](references/aiapp.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建一个 XX 应用 / 系统 / 工具" | `python scripts/aiapp_create_and_poll.py --prompt "<用户原始需求>"`（自动轮询进度） |
| "查 AI 应用 / 修改已有应用" | 见 [aiapp.md](references/aiapp.md) |

## 跨产品协作

- 已存在的工作台应用列表 → 切到 `dingtalk-workbench`
