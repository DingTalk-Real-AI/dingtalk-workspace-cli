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

dws 可同时登录多个钉钉账号，同一组织也可保留多个账号。一个 profile = 一个 `corpId + userId` 身份；当前 profile 决定本次命令注入哪个身份。

## 触发条件（命中任一即用本 skill）
- 显式：用户提到 切换 / 换 / 跨组织、另一个钉钉、别的公司、看登录了哪些组织、当前是哪个组织、某人 / 某群 / 某数据在别的组织
- 隐式（最常见、易漏）：在当前组织读 / 搜没找到目标（群 / 人 / 数据），且 `dws profile list` 去重后显示已登录 ≥2 个组织 —— 别急着判「不存在」，按下方跨组织铁律去其他组织找
- 需要跨多个组织汇总 / 对比数据
- 用户问认证状态 / 登录了哪些组织或账号 / 当前账号是哪个

**不触发**：只登录 1 个组织时，按当前组织正常处理，不带 `--profile`。

## 命令
- `dws profile list --format json` — 默认列出全部账号；`profile` 是稳定选择器 `corpId:userId`，Token 状态现场读取且不刷新
- `dws profile switch <selector|->` — 持久切换账号；`-` 切回上一个
- 全局 `--profile <selector>` — 单次指定身份，不改当前 profile
- `dws auth login` — 新账号新增 profile；同一 `corpId + userId` 重登只刷新该账号
- `dws auth status [--profile <selector>]` — 查看并按需刷新指定身份；刷新失败返回未认证和真实原因

`selector` 支持 `corpId:userId`、`corpId:userName`、`corpName:userId`、`corpName:userName`，也兼容单独的 corpId、唯一 corpName 和本地 profile 名。名称只用于输入；重名时必须按报错候选改用 `profile list` 返回的稳定 `corpId:userId`。

只传组织时必须存在唯一 `isOrgCurrent=true` 账号。多账号组织没有默认账号时先让用户指定账号；禁止选择第一项、最近登录或最近使用账号。`primaryProfile/isPrimary` 仅兼容输出，不参与选择。

## 跨组织铁律（必须执行，不得跳过）
「找群 / 找人 / 找数据」（chat search、aisearch / contact、doc / wiki 搜索等读 / 搜场景）在当前组织没命中、且 `dws profile list` 按 `corpId` 去重后显示 ≥2 个组织时，每个组织使用唯一 `isOrgCurrent=true` 项的 `profile` 各搜一遍；命中即用，全部组织都没有才追问用户。多账号组织没有默认账号时先询问用户。禁止把同一组织的多个账号重复当成多个组织。

## 跨组织聚合（agent 编排，无内置 --all-orgs）
① `dws profile list --format json` 按 `corpId` 分组 → ② 每组取唯一 `isOrgCurrent=true` 的稳定 `profile`；没有则询问用户 → ③ 对每个稳定 `profile` 各取一次数 → ④ 合并并标注来源组织和账号。

## 安全护栏（务必遵守）
- 只有 `dws profile list` 按 `corpId` 去重后显示 ≥2 个组织才启用跨组织逻辑；同组织多账号不算多组织。
- 自动跨组织只对「读 / 搜」。写 / 发 / 删 / 撤回等操作默认只在当前组织做；确需带 `--profile` 跨组织写时，必须先与用户确认目标组织。
- 持久切换 `dws profile switch`（改默认组织）按写操作对待：未经用户明确要求不得执行。跨组织找数一律用一次性 `--profile`，不改当前组织。
- `dws auth logout` 默认退出全部账号；组织选择器退出该组织全部账号；精确选择器或本地 profile 名只退出一个账号。执行前必须确认目标范围。
