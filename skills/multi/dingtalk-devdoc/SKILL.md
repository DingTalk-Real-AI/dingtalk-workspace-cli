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

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。22 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[devdoc.md](references/devdoc.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查 OAuth2 接入文档" | `dws devdoc article search --query "OAuth2 接入"` |
| "API 调用报错怎么办" | `dws devdoc article search --query "<报错关键词>"` |
| "开放接口文档" | `dws devdoc article search --query "<接口名或场景>"` |

## 跨产品协作

- 钉钉云文档（个人 / 企业内文档）→ 切到 `dingtalk-doc`
