# CLI 版本检查与升级提示

`dws auth status`、`dws version` 和 `dws --version` 会检查当前发行渠道是否有新版本。检查使用现有的 GitHub Release 来源配置，不下载或安装软件。

## 关键指令

```sh
dws auth status --format json
dws version --format json
dws --version
```

- `auth status` 和 `version` 的 JSON 保留现有字段，新增 `version_check`。有新版本时还包含 `_notice.update`，其中 `command` 为可执行的升级指令。
- `--version` 保留 stdout 原有的单行版本字符串，检查结果写入 stderr。其他人读格式的检查结果也使用 stderr。
- `version_check.status` 为 `update_available`（有更新）、`up_to_date`（同渠道无更新）、`unknown`（无法确认）或 `skipped`（关闭、开发版本或宿主托管）。只有成功取得有效发布信息才会报告前两种状态。
- 成功查询缓存 24 小时，命中缓存时 `cached=true`，并保留 `checked_at`。缓存按来源地址、仓库、发行版、渠道及当前版本隔离。
- 网络检查最多等待一秒。离线、超时、无有效 Release 或缓存不可用时不会改变原命令的业务结果和退出码。
- `auth status --readonly` 仅读取有效版本缓存，不联网、不写入缓存；没有有效缓存时返回 `unknown`。未知参数导致解析提前停止时，仍保留 `--readonly` 的只读意图。

正式版检查正式渠道；beta 版检查 beta 渠道并提示 `dws upgrade --beta`。开发构建与无法识别的版本不参与比较。检查请求不携带 `GH_TOKEN` 或 `GITHUB_TOKEN`；显式 `dws upgrade` 的原有认证行为不变。

## 未知命令或参数

遇到当前命令树不认识的命令或 flag 时，CLI 在参数解析边界记录该事实，并进行一次限时版本检查。此时跳过日缓存，避免漏掉当天新发布的能力。

有新版时，错误 JSON 附带 `_notice.update`；人读错误附带升级建议。原有错误类别、退出码、拼写建议和帮助指引保留。普通业务错误、缺少必填参数、已知 flag 的非法值、帮助及补全请求不会因此触发检查。

提示只说明“当前版本可能不支持此命令或参数”，不会宣称该命令一定存在于最新版，也不会宣称升级必然修复错误。准确判断某命令的最低支持版本需要独立的版本化命令目录，本功能不提供这项判断。

## 关闭和主动查询

设置 `DWS_NO_UPDATE_CHECK=1` 可关闭上述自动检查；关键指令返回 `skipped`，未知命令保持原错误输出。

```sh
DWS_NO_UPDATE_CHECK=1 dws --version
dws upgrade --check
```

`dws upgrade --check` 保持现有主动查询行为，不受该开关影响。
