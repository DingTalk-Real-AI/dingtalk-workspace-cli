---
name: dingtalk-pat
description: 钉钉 PAT 行为授权、单 scope 授权撤回与本地浏览器策略管理。Use when 用户说 PAT 授权/行为权限/scope 授权/全部授权/一次性授权/会话授权/永久授权/批量授权/撤回一个显式 scope，或允许、禁止 PAT 授权流程打开浏览器。Distinct from dingtalk-dev（开放平台应用权限）。命令前缀：dws pat。
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
| "授权全部可操作 scope" | `dws pat chmod --all`；先 `--dry-run`，确认后加 `--yes` |
| "撤回一个显式 scope 授权" | `dws pat chmod <scope> --revoke`；先 `--dry-run`，确认后加 `--yes` |

## 安全边界

- `browser-policy` 只修改本地策略，不授予业务权限。
- `chmod` 会改变 Agent 可执行范围。`--dry-run` 会执行一次需要认证的只读服务端计划，不是本地回显；得到用户明确确认后才可加 `--yes` 写入。
- `--all` 可与产品/域过滤器组合，但不能与位置 scope 或 `--recommend` 组合；它不是 `--recommend` 的别名。
- `--revoke` 必须且只能接一个位置 scope，不能与 `--all`、产品/域选择器、`--recommend`、`--grant-type` 或 `--session-id` 组合；不得批量撤回。
- `--yes` 只表达写操作已获用户确认，不改变认证、浏览器、轮询或重试策略；与 `--dry-run` 同时出现时仍不写入。
- dry-run 的服务端 challenge/error 原样返回，不打开浏览器、轮询或认证重试，不刷新 token，也不修改本地身份、profiles、凭据/keychain 或授权状态（常规 CLI 日志/审计日志仍可记录）；`--all`/单 scope 撤回不支持时失败关闭，不降级为旧授权或批量撤回。
- 撤回只移除一个 ACTIVE 显式 grant 并恢复默认策略，服务端拒绝 DENIED；它不是 OAuth logout 或永久 DENIED。
- 授权计划/执行分别路由到 `pat.batch_plan` / `pat.batch_grant`，单 scope 撤回路由到 `pat.scope_revoke`。
- PAT 行为授权不是开放平台应用权限；后者使用 `dws dev app permission`，并切到 `dingtalk-dev`。
