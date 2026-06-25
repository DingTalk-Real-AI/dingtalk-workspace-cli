---
name: dingtalk-devdoc
description: 钉钉开放平台开发文档搜索。Use when 用户说 开放平台文档/API 文档/接口文档/调用报错/开放接口怎么调。Distinct from dingtalk-doc(钉钉云文档)。命令前缀：dws devdoc。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉开放平台文档 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[devdoc.md](references/devdoc.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查 OAuth2 接入文档" | `dws devdoc article search --query "OAuth2 接入"` |
| "API 调用报错怎么办" | `dws devdoc error diagnose --query "<报错关键词>"` |
| "requestId 15r6h45w0muec 为什么失败" | `dws devdoc error diagnose --request-id 15r6h45w0muec` |
| "错误码 33012" | `dws devdoc error diagnose --error-code 33012` |
| "开放接口文档" | `dws devdoc article search --query "<接口名或场景>"` |

## 跨产品协作

- 钉钉云文档（个人 / 企业内文档）→ 切到 `dingtalk-doc`
