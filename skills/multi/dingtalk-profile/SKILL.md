---
name: dingtalk-profile
description: 钉钉多组织 / profile 管理与跨组织取数。Use when 用户说 切换组织/换组织/跨组织/另一个钉钉/别的公司/多组织/看登录了哪些组织/profile，或在当前组织找不到群/人/数据需要去其他组织找。命令前缀：dws profile / dws auth / 全局 --profile。
cli_version: ">=1.0.40"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉多组织 / profile Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准；接口、命名、跨 skill 引用后续可能调整。生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

dws 可同时登录多个钉钉组织，一个 profile = 一个已登录组织（corp）。当前 profile 决定本次命令用哪个组织的身份（corpId / userId 按当前 profile 自动注入）。

## 触发条件（命中任一即用本 skill）
- 显式：用户提到 切换 / 换 / 跨组织、另一个钉钉、别的公司、看登录了哪些组织、当前是哪个组织、某人 / 某群 / 某数据在别的组织
- 隐式（最常见、易漏）：在当前组织读 / 搜没找到目标（群 / 人 / 数据），且 `dws profile list` 显示已登录 ≥2 个组织 —— 别急着判「不存在」，按下方跨组织铁律去其他组织找
- 需要跨多个组织汇总 / 对比数据
- 用户问认证状态 / 登录了哪些组织 / 主组织是哪个

**不触发**：只登录 1 个组织时，按当前组织正常处理，不带 `--profile`。

## 命令
- `dws profile list` — 列出已登录组织（主 / 当前标记、状态、有效期），只读元数据
- `dws profile switch <名称|corpId|->` — 持久切换当前组织；`-` 切回上一个；无参数在交互终端弹选择器（非交互须显式传参）。`dws profile use` 是别名
- 全局 `--profile <名称|corpId>` — 单次指定本命令用哪个组织，一次性、不改当前组织
- `dws auth login` — 再登一个组织即新增 profile（自动从授权账号取 corpId / corpName）；同组织重复 login = 刷新
- `dws auth status [--profile <名称>]` — 查看认证状态

## 跨组织铁律（必须执行，不得跳过）
「找群 / 找人 / 找数据」（chat search、aisearch / contact、doc / wiki 搜索等读 / 搜场景）在当前组织没命中、且 `dws profile list` 显示 ≥2 个组织时，对每个组织带一次性 `--profile <corpId>` 各搜一遍；命中即用，全部组织都没有才追问用户。禁止在当前组织搜不到就判定「不存在」或直接甩给用户选。

## 跨组织聚合（agent 编排，无内置 --all-orgs）
① `dws profile list` 拿到所有已登录组织 → ② 对每个组织带 `--profile <corpId>` 各取一次数 → ③ 合并并标注来源组织；某组织失败则标「该组织暂不可用」并继续返回其余。

## 安全护栏（务必遵守）
- 只有 `dws profile list` 显示 ≥2 个组织才启用跨组织逻辑；单组织直接按当前组织走，不带 `--profile`。
- 自动跨组织只对「读 / 搜」。写 / 发 / 删 / 撤回等操作默认只在当前组织做；确需带 `--profile` 跨组织写时，必须先与用户确认目标组织。
- 持久切换 `dws profile switch`（改默认组织）按写操作对待：未经用户明确要求不得执行。跨组织找数一律用一次性 `--profile`，不改当前组织。
- `dws auth logout` 默认退出所有已登录组织；只退一个加 `--profile <名称|corpId>`。退主组织不会被拦截，会静默改选新主，执行前必须向用户确认。
