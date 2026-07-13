---
name: dws-shared
description: dws 多 skill 模式的公共参考——认证、全局参数、多组织 / --profile 规则、安全底线。所有 dingtalk-* 子 skill 执行前先读本 skill。命令前缀：dws。
cli_version: ">=1.0.40"
metadata:
  category: productivity
  stability: experimental
  requires:
    bins:
      - dws
---

# DWS 公共参考（dws-shared）

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。

每个 dingtalk-* 子 skill 都把本 skill 列为 PREREQUISITE：执行任何产品命令前先读这里的认证、全局参数与多组织规则。`dws` 必须在 PATH 上。

## 认证
- `dws auth login`（新登一个组织即新增 profile）；`--device` 无头 / SSH 登录；`--recommend` 无交互批量授权
- `dws auth status [--profile <名称|corpId>]` 查看认证状态
- macOS 出现 `ciphertext_key_mismatch` 且普通终端仍可登录时：先运行 `env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --dry-run --format json`，通过后加 `--yes` 迁移；不要直接 `auth reset`

## 全局参数
- 所有命令加 `--format json` 取可解析输出
- 全局 `--profile <名称|corpId>`：单次指定本命令用哪个组织，一次性、不改默认组织
- 危险 / 写 / 删操作执行前先向用户确认

## 多组织 / --profile（关键规则）
dws 可同时登录多个钉钉组织，一个 profile = 一个已登录组织（corp）。当前 profile 决定本次命令用哪个组织的身份（corpId / userId 自动注入）。

- **跨组织铁律**：任何「找群 / 找人 / 找数据」（如 chat / aisearch / contact / doc / wiki / aitable / sheet / minutes / mail / report / todo / calendar / oa 的搜索、列表、查询）在当前组织没命中、且 `dws profile list` 显示 ≥2 个组织时，对每个组织带一次性 `--profile <corpId>` 各搜一遍；命中即用，全部组织都没有才追问用户。禁止在当前组织搜不到就判定「不存在」或直接甩给用户选。
- **单组织**：`dws profile list` 只有 1 个组织时，按当前组织正常处理，不带 `--profile`。
- **安全护栏**：自动跨组织只对「读 / 搜」；写 / 发 / 删 / 撤回等操作默认只在当前组织做，确需带 `--profile` 跨组织写时先与用户确认目标组织；持久切换 `dws profile switch`（改默认组织）属写操作，未经用户明确要求不得执行。
- 完整命令与跨组织聚合见 `dingtalk-profile` skill。

## 错误处理
- `unknown command` / `unknown flag`：先跑 `dws <path> --help` 查证再修正一次，别把自然语言当命令 / flag
- 服务端 token 过期：提示用户 `dws auth login` 重新登录；本地密钥不匹配按上面的 macOS 迁移流程处理，不要混为 token 过期
- 业务错误码 / 接口语义：用 `dws devdoc article search --query "<关键词>" --format json` 查官方文档，不编造原因
