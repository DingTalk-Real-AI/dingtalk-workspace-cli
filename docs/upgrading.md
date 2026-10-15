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
| 独立二进制 | 目标版本提供 `dwsUpgrade` 清单时，只下载当前平台归档和所需技能，并按清单校验大小与 SHA256。旧版本或资产网络失败时下载完整 npm 包，强制验证 `dist.integrity`、包名、版本及包内 SHA256。验证新二进制后沿用原子替换和本地备份。 |
| npm / pnpm | 识别真实二进制所属安装位置，由对应包管理器安装选定版本；管理器缺失或失败时返回错误，不直接覆盖其管理的文件。 |
| Homebrew | 识别 Cellar 和安装收据，由原 Formula 升级；先核对可安装版本与选定版本一致，不默默切换 stable/beta Formula 或安装其他版本。 |

`--dry-run` 预览所选更新方案，不运行包管理器或修改安装。独立二进制支持 `--skip-skills` 和 `--rollback`。npm/pnpm 的安装脚本会更新技能，因此不支持通过 `dws upgrade --skip-skills` 跳过；包管理器安装的回退交由原包管理器操作。Homebrew 捆绑技能，安装到 Agent 目录仍使用 `dws skill setup`。

npm 安装的 `--force` 会传给 `npm install`；同版本重装还会执行 `npm rebuild`，通过安装脚本从包内资产恢复二进制。此过程仍遵循 npm 的缓存和脚本配置，不保证重新下载整个包；包内资产损坏或脚本被禁用时，更新验证可能失败。

Homebrew 在确认后更新 Formula 索引，再核对目标版本；显式配置的其他 Registry 或 GitHub 来源不能自动映射到原 Formula，需使用原 Homebrew 命令升级。Windows 包管理器安装暂时给出手动升级命令，需退出当前 DWS 进程后执行；无法确认安装归属的 pnpm 存储布局也会给出原管理器指引，不尝试原生覆盖。

## 下载、重试与回退

- 新发行版本在 npm 的精确版本清单中携带 `dwsUpgrade`（版本 1），由打包步骤从实际资产和 `checksums.txt` 生成。单平台下载采用该清单给出的地址、字节数与 SHA256，不依赖 npm/Node。`--skip-skills` 不下载技能文件。
- 当前完整 npm 包仍包含全部平台，供 npm/pnpm 安装及历史版本兼容。旧版本没有平台清单时仍须完整下载；本次修复不会改写已发布版本。
- 默认公开 npm 包的兼容下载优先尝试 `registry.npmmirror.com`；`DWS_UPGRADE_NPM_MIRROR` 可替换这个下载镜像。网络不可用时回到 Registry 原始 tarball，始终按原始 Registry 的 `dist.integrity` 校验。自定义 Registry 或非规范 tarball 地址保持原地址，不应用此镜像配置。
- 按平台文件和非最终镜像源的单源总预算为 2 分钟；完整 npm 原始源为 30 分钟，预算包含重试等待。所有下载连续 30 秒没有收到数据即中止。每个来源最多尝试三次，临时 HTTP 429/5xx、连接中断和读取超时可重试。
- 同一来源的部分下载有强 ETag 或 Last-Modified 时，使用 `Range` / `If-Range` 续传，并核对响应范围、总大小与版本标记。来源忽略 Range 时覆盖重下；没有版本标记、切换来源或结束本次调用时不复用部分文件。
- 交互输出显示下载源、尝试次数、已保留字节、下载量、总大小、速度与重试原因。下载网络错误归为 `api`，超时 reason 为 `download_timeout`，退出码 1；本地文件写入失败保持本地错误，用户取消仍原样透传。

只有网络或 HTTP 下载失败可以切换下载源或回退完整 npm 包。清单、大小、SHA256、SRI、包身份、归档边界及重定向安全校验失败会立即终止，不通过回退绕过。版本检查只获取元数据，不下载发布包。已安装的旧 DWS 必须先通过可用安装渠道更新到包含本修复的版本，之后才能使用上述下载策略。

## 自定义完整 npm 包下载镜像

独立安装可仅替换全包下载镜像，版本查询及 `dist.integrity` 仍来自官方 npm Registry：

```sh
DWS_UPGRADE_NPM_MIRROR=https://mirror.example.com/npm dws upgrade
```

镜像必须提供标准 npm tarball 路径，例如上述根地址对应
`https://mirror.example.com/npm/dingtalk-workspace-cli/-/dingtalk-workspace-cli-1.0.64.tgz`。
支持路径前缀和末尾斜杠；仅接受无凭据、查询参数或片段的 HTTPS 地址，本机测试允许 loopback HTTP。
镜像返回的包仍须通过官方 SRI、包名、版本及包内 SHA256 校验；校验失败直接终止。
自定义镜像网络失败后直接回退官方 tarball，不再额外尝试默认 npmmirror。

需要仅使用官方源时，把下载镜像也设为官方 Registry；相同地址会去重：

```sh
DWS_UPGRADE_NPM_MIRROR=https://registry.npmjs.org dws upgrade
```

该变量只影响独立升级中的完整 npm 包下载，不修改 npm 配置，也不自动读取 `npm config get registry`。
按平台下载仍使用发布清单中的地址；npm/pnpm/Homebrew 托管升级仍交给原包管理器。
若要连版本查询一起切换，使用已有的 `DWS_UPGRADE_REGISTRY`；此时以该 Registry 返回的 tarball 和完整性摘要为准，忽略 `DWS_UPGRADE_NPM_MIRROR`。
自定义发行版仅设置下载镜像不算配置版本来源，仍须显式设置其升级 Registry 或 GitHub 来源。
