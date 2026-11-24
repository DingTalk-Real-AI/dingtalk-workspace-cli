# 平台 npm 分包与升级下载

公共版沿用现有升级入口：默认从 npm 查询版本，独立安装优先使用同版本 GitHub 平台资源；npm/pnpm/Homebrew 安装仍交由原包管理器升级。

## 来源选择

```sh
dws upgrade --source auto
dws upgrade --source github
dws upgrade --source npmmirror
```

`--source` 默认值为 `auto`，仅接受上述三个值，支持命令行补全。无效值按参数校验错误退出（退出码 3）。

| 模式 | 主包版本与弃用检查 | 独立安装的资源下载 |
| --- | --- | --- |
| `auto` | 默认 npmjs，保留 `DWS_UPGRADE_*` 的既有来源配置 | 默认先 GitHub，可重试网络错误时切换同版本 npmmirror 分包 |
| `github` | npmjs 主包 | 固定 GitHub，不回退到镜像 |
| `npmmirror` | npmmirror 主包 | 固定 npmmirror 平台／技能分包，缺少分包时报错 |

显式 `github`、`npmmirror` 覆盖 `DWS_UPGRADE_REGISTRY`、`DWS_UPGRADE_URL` 和 `DWS_UPGRADE_REPOSITORY`，不修改进程环境变量。两种显式模式仅支持公共发行版。`--source` 同样影响 `--check`、`--list` 的主包元数据来源；`--dry-run` 使用相同来源解析资源但不下载。`--rollback` 只使用本地备份，不能配合非 `auto` 来源。

npm/pnpm 安装支持 `auto` 和 `npmmirror`，后者将镜像 Registry 交给原包管理器，仍安装完整主包；`github` 会给出参数错误。Homebrew 安装只接受 `auto`，沿用原 formula。包管理器的下载、重试和输出由其自身管理。

独立安装会显示下载模式、当前来源、切换原因、每个来源／资源的尝试与重试次数、重试等待时间及续传位置；进度行同步显示实际来源、尝试次数、百分比和下载量。响应未提供总大小时显示已下载量。默认每个资源在每个来源最多尝试 3 次（首次加 2 次重试）；切换来源后重新计数。切换原因不输出原始签名 URL 或认证材料。

## 下载行为

- 独立安装的 GitHub 元数据或资源下载出现超时、连接中断、429 或可重试的 5xx 时，尝试读取 npmmirror 的同版本平台分包。
- 显式设置 `DWS_UPGRADE_REGISTRY=https://registry.npmmirror.com` 时，独立安装优先读取平台分包。历史版本的分包返回 404 时，保持原来的整包兼容路径。
- 默认 GitHub 路径回退时若分包尚未同步，报告错误；不会悄悄改为下载 143 MB 的整包。版本、资源 SHA256 或大小不匹配时终止升级。
- 只处理 `X.Y.Z` 和 `X.Y.Z-beta.N` 公共版本。其他发行版、自定义 Registry 与显式 GitHub 源保持原协议。
- 主包仍是版本可用性和弃用的权威；分包存在不能绕过主包撤回。
- 每个资源在单个来源最多尝试三次。连接/TLS 5 秒、响应头 8 秒、正文无进展 15 秒、单次请求最长 10 分钟。保留已下载字节，使用强 ETag、`Range` 和 `If-Match` 续传；核对 Content-Range、ETag 和总大小。
- 200 响应重下；412、416 或资源身份改变清空续传状态。取消、磁盘错误和校验失败不切换来源。临时文件随调用清理，不支持跨命令恢复。

分包 tarball 先按 npm integrity 校验，再校验内部 package.json 身份、归档边界、gzip CRC、checksums.txt 与资源大小/SHA256。GitHub 归档和 npm 外层 tarball 的断点互不混用，不执行 npm 生命周期脚本。

## 公共分包

包名前缀为 `dingtalk-workspace-cli`，分包后缀为：

- `darwin-arm64`、`darwin-x64`
- `linux-arm64`、`linux-x64`
- `win32-arm64`、`win32-x64`
- `skills`

每包只包含 package.json、一个现有发布归档及对应 checksums.txt 记录。平台包声明 npm 的 os/cpu；技能包跨平台共享。原来的全平台主包与安装脚本保持兼容。

## 打包和发布

对已有公共发布资源（六个平台归档、技能包及 checksums.txt）执行：

```sh
node scripts/release/stage-npm-splits.cjs /absolute/path/to/dist 1.0.69
node scripts/release/publish-npm-splits.cjs /absolute/path/to/dist/npm-split --publish
```

先将全部资源与 checksums.txt 匹配，生成七包，再准备 tarball 并检查所有精确版本的发布状态；已存在版本只有 integrity 相同才复用。使用 `release-VERSION` dist-tag，不移动主包 latest/beta。发布中断后可重跑，不能覆盖已经发布的不同内容。

npm 接受上传后可能仍在处理。发布脚本先提交分包，再等待精确版本元数据可读并核对 integrity；最多等待 10 分钟，不将“上传已接受”直接视为“发布已验证”。

公共 Release 工作流通过仓库变量 `DWS_STAGE_NPM_SPLITS=1` 开启此步骤，正式版和 beta 共用同一流程，并在主包渠道更新前发布分包。启用前须在七个包上分别配置 GitHub Actions Trusted Publisher，绑定 `DingTalk-Real-AI/dingtalk-workspace-cli` 的 `release.yml`（Environment 留空），并允许 `npm publish`；CI 通过 OIDC 发布，不注入 NPM_TOKEN。不存在的包须先由维护者初始化所有权；新建信任配置须在 2 天内首次成功发布，配置时间应与上线协调。上述脚本在本地运行时仍需维护者交互认证，不能使用 GitHub Actions OIDC。历史主包修复路径不重打包旧主包；独立分包脚本可补发旧版本。不改变部署流程或其他发行版的发布流程。

npmmirror 是 npmjs 的只读镜像；先发布到 npmjs，再等待镜像同步或使用 `cnpm sync PACKAGE` 请求同步。镜像版本元数据可读后，还需核对 tarball 下载与 integrity，不能仅凭同步请求成功判断资源可用。

本次代码不含对 npm/pnpm 主包安装体积的重构；包管理器安装仍按现有主包处理。

