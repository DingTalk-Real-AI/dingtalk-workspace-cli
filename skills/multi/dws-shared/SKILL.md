---
name: dws-shared
description: dws 多 skill 模式的公共参考——认证、全局参数、Schema 命令发现、多组织 / --profile 规则、安全底线。所有 dingtalk-* 子 skill 执行前先读本 skill。命令前缀：dws。
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

每个 dingtalk-* 子 skill 都把本 skill 列为 PREREQUISITE：执行任何产品命令前先读这里的认证、全局参数、Schema 命令发现与多组织规则。`dws` 必须在 PATH 上。

## 认证
- `dws auth login`（新账号新增 profile，同一 `corpId + userId` 重登只刷新该账号）；`--device` 无头 / SSH 登录
- `dws auth status [--profile <selector>]` 查看组织当前账号或指定账号；会尝试刷新，失败时保留真实错误
- macOS 出现 `ciphertext_key_mismatch` 且普通终端仍可登录时：先运行 `env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --dry-run --format json`，通过后加 `--yes` 迁移；不要直接 `auth reset`

## 全局参数
- 所有命令加 `--format json` 取可解析输出
- 全局 `--profile <selector>`：支持 `corpId:userId`、`corpId:userName`、`corpName:userId`、`corpName:userName`，也兼容单独的 corpId、唯一 corpName 和本地 profile 名；均不改默认 profile
- 以 leaf Schema 的 `confirmation` 为准：`user_required` 才先确认并在同意后加 `--yes`；`not_required` 不得仅因 `effect=write` 自行升级确认要求

## 命令发现：Schema + --help

选择命令、读取参数映射/组合约束与安全语义时，优先渐进查询当前二进制内嵌的 leaf Schema；真正组装参数前，用 `--help` 确认 Cobra 当前接受的 flags：

本节适用于进入 Runtime Schema 的基础/原子命令。`+` shortcut 是明确例外，使用下方独立 shortcut catalog 契约。

```bash
dws schema                                      # 产品概览
dws schema calendar                             # 产品工具列表
dws schema "calendar event"                     # 命令分组
dws schema "calendar event create" --compact    # 完整 leaf 契约
```

`--all` 会导出全部工具的完整 leaf，输出很大。仅在用户明确要求全量导出，或执行 CI、Catalog 审计、参数防丢 baseline 时使用 `dws schema --all --format json`；普通业务任务必须渐进查询，不得把全量结果注入上下文。完整 baseline 不得使用会裁剪 provenance 的 `--all --compact`。

| 要确认的信息 | 事实源 |
|-------------|--------|
| 命令是否存在、Cobra 接受哪些 flags | `dws <cli_path> --help` |
| Agent 选命令、参数映射/约束、risk/confirmation（原子/基础命令） | `dws schema "<cli_path>"` |
| shortcut 的参数、约束、risk/confirmation、示例 | `dws shortcut list --service <service> --format json` |
| 钉钉中的文档、日程、消息等业务数据 | 真正执行对应的 `read` / `search` / `list` 命令 |

Schema 与 Help 冲突属于契约漂移：参数只用 Help 接受的 flags，并报告漂移；安全语义冲突时采用更保守的确认方式。Schema 只描述命令契约，不能替代业务查询。

## Shortcut 可用性
- `shortcut` 是对常用操作的高层封装。先按产品 skill 的意图表、脚本和 recipe 路由：存在精确覆盖场景的专用脚本/recipe 时按其执行；否则可见 shortcut 优先于手写等价原子流程。
- shortcut 有独立 catalog，按设计不进入 Runtime Schema。用 `dws shortcut list --service <service> --format json` 读取 flags、required/enum、跨参数 constraints、risk/confirmation 和 examples；不要对 `+` 路径调用 `dws schema`。
- 真正组装参数前用叶子帮助 `dws <service> +<verb> --help` 核对 Cobra 接受的 flags；父级 `dws <service> --help` 只能用于发现子命令。
- shortcut catalog 中 `confirmation=user_required` 时先获得用户确认，确认后才加 `--yes`；`not_required` 不额外确认。
- 如果 shortcut 不在 help / list 中，改用产品参考里的原子命令、脚本或标准流程；不要猜测未展示的 `+` 命令。

## 多组织 / --profile（关键规则）
dws 可同时登录多个钉钉账号，同一组织也可保留多个账号。一个 profile 由 `corpId + userId` 唯一确定；`dws profile list --format json` 默认显示全部账号，稳定选择器读取每项 `profile`。

- **选择规则**：名称只用于输入，自动化使用 `profile=corpId:userId`。名称重名时按报错候选改用稳定选择器，不得猜测。
- **组织默认账号**：只传组织时使用唯一 `isOrgCurrent=true` 的账号。多账号组织没有默认账号时先让用户指定账号；禁止选择第一项、最近登录或最近使用账号。
- **跨组织铁律**：任何读 / 搜在当前组织没命中、且按 `corpId` 去重后有 ≥2 个组织时，每个组织使用唯一组织默认账号的 `profile` 各搜一遍。禁止把同组织多个账号重复执行。
- **单组织**：只有 1 个组织时按当前账号处理；用户明确指定账号时使用该账号的稳定 `profile`。
- **安全护栏**：自动跨组织只对「读 / 搜」；写 / 发 / 删 / 撤回等操作默认只在当前组织做，确需带 `--profile` 跨组织写时先与用户确认目标组织；持久切换 `dws profile switch`（改默认组织）属写操作，未经用户明确要求不得执行。
- 完整命令与跨组织聚合见 `dingtalk-profile` skill。

## 错误处理
- `unknown command` / `unknown flag`：先跑 `dws <path> --help` 查证再修正一次，别把自然语言当命令 / flag
- 服务端 token 过期：提示用户 `dws auth login` 重新登录；本地密钥不匹配按上面的 macOS 迁移流程处理，不要混为 token 过期
- 业务错误码 / 接口语义：用 `dws devdoc article search --query "<关键词>" --format json` 查官方文档，不编造原因
