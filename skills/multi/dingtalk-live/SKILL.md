---
name: dingtalk-live
description: 钉钉直播。Use when 用户说 直播/我的直播/直播列表。命令前缀：dws live。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉直播 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[live.md](references/live.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "我的直播 / 直播列表" | `dws live stream list` |
