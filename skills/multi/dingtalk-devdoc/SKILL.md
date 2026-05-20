---
name: dingtalk-devdoc
description: 钉钉开放平台开发文档搜索。Use when 用户说 开放平台文档/API 文档/接口文档/调用报错/开放接口怎么调。Distinct from dingtalk-doc(钉钉云文档)。命令前缀：dws devdoc。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉开放平台文档 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[devdoc.md](references/devdoc.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查 OAuth2 接入文档" | `dws devdoc article search --query "OAuth2 接入"` |
| "API 调用报错怎么办" | `dws devdoc article search --query "<报错关键词>"` |
| "开放接口文档" | `dws devdoc article search --query "<接口名或场景>"` |

## 跨产品协作

- 钉钉云文档（个人 / 企业内文档）→ 切到 `dingtalk-doc`
