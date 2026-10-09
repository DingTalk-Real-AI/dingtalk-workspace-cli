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
| 独立二进制 | 下载 Registry 返回的 tarball，强制验证 `dist.integrity`，仅提取当前平台归档、技能包和校验文件；验证 SHA256、解压、验证新二进制后，沿用原子替换和本地备份。 |
| npm / pnpm | 识别真实二进制所属安装位置，由对应包管理器安装选定版本；管理器缺失或失败时返回错误，不直接覆盖其管理的文件。 |
| Homebrew | 识别 Cellar 和安装收据，由原 Formula 升级；先核对可安装版本与选定版本一致，不默默切换 stable/beta Formula 或安装其他版本。 |

`--dry-run` 预览所选更新方案，不运行包管理器或修改安装。独立二进制支持 `--skip-skills` 和 `--rollback`。npm/pnpm 的安装脚本会更新技能，因此不支持通过 `dws upgrade --skip-skills` 跳过；包管理器安装的回退交由原包管理器操作。Homebrew 捆绑技能，安装到 Agent 目录仍使用 `dws skill setup`。

npm 安装的 `--force` 会传给 `npm install`；同版本重装还会执行 `npm rebuild`，通过安装脚本从包内资产恢复二进制。此过程仍遵循 npm 的缓存和脚本配置，不保证重新下载整个包；包内资产损坏或脚本被禁用时，更新验证可能失败。

Homebrew 在确认后更新 Formula 索引，再核对目标版本；显式配置的其他 Registry 或 GitHub 来源不能自动映射到原 Formula，需使用原 Homebrew 命令升级。Windows 包管理器安装暂时给出手动升级命令，需退出当前 DWS 进程后执行；无法确认安装归属的 pnpm 存储布局也会给出原管理器指引，不尝试原生覆盖。

包完整性、清单身份或文件校验失败会终止更新，不通过换源绕过校验。当前 npm 发布包包含全部平台归档，因此原生更新的下载量大于单个平台归档；版本检查只获取元数据，不下载发布包。
