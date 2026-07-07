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

> ⚠️ **命令以当前 dws 二进制为准**。服务发现和动态 schema 已下线，本文档随版本内嵌发布；执行前用 `dws <cmd> --help` 或 `--dry-run` 验证 flag 与命令是否存在。


> 命令参考：[live.md](references/live.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "我的直播 / 直播列表" | `dws live stream list` |
