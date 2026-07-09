---
name: dingtalk-live
description: 钉钉直播。Use when 用户说 直播/我的直播/直播列表。命令前缀：dws live。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉直播 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws runtime command surface 为准**。本文档列出的命令基于 runtime schema 与本仓库 v1.0.30 实测；实际调用前用 `dws <cmd> --help`、`dws schema "<cmd>"` 或 `--dry-run` 验证。若报 `unknown command` / `unknown flag` 或 fall back 到父级 help，必须按当前 help/schema 调整，禁止编造路径或 flag。


> 命令参考：[live.md](references/live.md)。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "我的直播 / 直播列表" | `dws live stream list` |
