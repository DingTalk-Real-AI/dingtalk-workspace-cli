# 版本来源与升级方式

开源版默认从 npm Registry 查询 `dingtalk-workspace-cli` 的发行版本。查询和独立二进制的更新通过 Go 的 HTTP 客户端完成，本机无需安装 npm 或 Node.js。

## 选择版本

```sh
dws upgrade --check --format json
dws upgrade --list
dws upgrade --check --beta
dws upgrade --version v1.0.63 --dry-run
```

- 正式版查询 `/dingtalk-workspace-cli/latest`，beta 查询 `/dingtalk-workspace-cli/beta`。
- `--version` 查询精确版本，不将找不到的版本替换成 latest。
- `--list` 从包的版本元数据筛选所选渠道，排除已弃用版本；Registry 不提供 GitHub Release 正文时，不伪造更新日志。
- 主动查询失败会返回错误，不把无法联网解释成已是最新。

默认 Registry 根地址是 `https://registry.npmjs.org`。`DWS_UPGRADE_REGISTRY` 可指定兼容 Registry 的根地址，使用 HTTPS；本地测试允许 loopback HTTP。Registry 请求不携带 GitHub Token。

显式配置 `DWS_UPGRADE_URL` 或 `DWS_UPGRADE_REPOSITORY` 时，继续使用 GitHub Release API 协议，优先于 Registry 配置。此时 `GITHUB_TOKEN` / `GH_TOKEN` 的行为保留，仍可能遇到对应服务的限流。自定义发行版必须明确配置升级来源；嵌入式运行仍由宿主升级。

## 安装来源决定升级方式

| 当前安装 | 升级方式 |
| --- | --- |
| 官方开源版独立二进制 | npm 确定版本后，从同版本的不可变 GitHub Release 下载当前平台归档和技能包；强制验证大小、SHA256 和二进制精确版本，沿用原子替换和本地备份。 |
| 自定义 Registry / 内部发行版独立二进制 | 保留配置来源的完整 npm tarball 下载、`dist.integrity`、包身份及包内 SHA256 校验；不会自动映射到公共 GitHub 仓库。 |
| npm / pnpm | 识别真实二进制所属安装位置，由对应包管理器安装选定版本；管理器缺失或失败时返回错误，不直接覆盖其管理的文件。 |
| Homebrew | 识别 Cellar 和安装收据，由原 Formula 升级；先核对可安装版本与选定版本一致，不默默切换 stable/beta Formula 或安装其他版本。 |

`--dry-run` 预览所选更新方案，不运行包管理器或修改安装。官方平台下载会读取版本和资产元数据，展示来源与预计大小，不下载归档；`--skip-skills` 不请求技能包。独立二进制支持 `--rollback`。npm/pnpm 的安装脚本会更新技能，因此不支持通过 `dws upgrade --skip-skills` 跳过；包管理器安装的回退交由原包管理器操作。Homebrew 捆绑技能，安装到 Agent 目录仍使用 `dws skill setup`。

npm 安装的 `--force` 会传给 `npm install`；同版本重装还会执行 `npm rebuild`，通过安装脚本从包内资产恢复二进制。此过程仍遵循 npm 的缓存和脚本配置，不保证重新下载整个包；包内资产损坏或脚本被禁用时，更新验证可能失败。

Homebrew 在确认后更新 Formula 索引，再核对目标版本；显式配置的其他 Registry 或 GitHub 来源不能自动映射到原 Formula，需使用原 Homebrew 命令升级。Windows 包管理器安装暂时给出手动升级命令，需退出当前 DWS 进程后执行；无法确认安装归属的 pnpm 存储布局也会给出原管理器指引，不尝试原生覆盖。

## 官方平台资产下载

版本选择仍由 npm 的正式版、beta 或精确版本元数据决定；下载器只查询同一个 `v<version>` tag，不另取 GitHub latest。平台和技能必须来自已公开的 immutable Release，具有有效大小及 SHA256，下载地址必须属于该仓库和精确 tag。纯 `--check` / `--list` 仍只查询 npm。

官方 GitHub Release 元数据查询依次使用 `GITHUB_TOKEN`、`GH_TOKEN`、本机 GitHub CLI（`gh`）已有的 `github.com` 登录凭据；均不可用时使用默认匿名请求，没有内置共享 Token。读取 `gh` 凭据最多等待 2 秒，不触发登录；未安装、未登录、读取失败或超时均回退匿名。已选凭据被服务器拒绝时保留错误，不自动改用匿名重试。

凭据仅用于 `https://api.github.com`，不会写入 DWS 配置或日志，也不会附加到 npm Registry、归档/CDN 下载或跳转后的其他站点。此自动发现逻辑仅适用于官方平台下载路径，显式 GitHub 源保留原有环境变量认证行为。

资产缺失、GitHub API 限流或下载失败会返回错误，不自动回退到全平台 npm 包。大小、SHA256 或二进制版本不匹配时，在替换现有安装前终止。显式 GitHub 配置继续使用原有协议；自定义 Registry 不继承官方平台映射。

例如 `v1.0.64` 的 Mac ARM64 归档与技能包合计约 18.69 MiB，完整 npm 包约 122.2 MiB。npm/pnpm 安装仍下载完整包。下载速度取决于 GitHub 网络；本改动没有接入新的镜像。

旧版 DWS 仍执行其内置升级逻辑。首次升级到包含本修复的版本时可能仍下载全包；安装新版本后，后续升级才采用平台下载。
