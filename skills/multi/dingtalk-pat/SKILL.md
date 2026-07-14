---
name: dingtalk-pat
description: 钉钉 PAT 行为授权与本地浏览器策略管理。Use when 用户说 PAT 授权/行为权限/scope 授权/一次性授权/会话授权/永久授权/批量授权，或允许、禁止 PAT 授权流程打开浏览器。Distinct from dingtalk-dev（开放平台应用权限）。命令前缀：dws pat。
cli_version: ">=1.0.15"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# PAT 行为授权 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, Schema discovery, error handling, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[pat.md](references/pat.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "允许 / 禁止 PAT 授权时打开浏览器" | `dws pat browser-policy --enabled` / `dws pat browser-policy --enabled=false` |
| "预览某产品或 scope 的行为授权" | `dws pat chmod ... --dry-run --format json` |
| "授予一次性 / 会话 / 永久行为权限" | `dws pat chmod ...`；先预览并确认，再加 `--yes` 执行 |

## 安全边界

- `browser-policy` 只修改本地策略，不授予业务权限。
- `chmod` 会改变 Agent 可执行范围。先用 `--dry-run` 展示 scope、授权类型和有效期，得到用户明确确认后才可加 `--yes`。
- PAT 行为授权不是开放平台应用权限；后者使用 `dws dev app permission`，并切到 `dingtalk-dev`。
