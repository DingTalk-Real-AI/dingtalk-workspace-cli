## 下载与安装

这是本 fork 的预发布包，包含数字员工 `dingtalk-tag` 命令。按操作系统和 CPU 架构选择附件：

| 平台 | 安装包 |
| --- | --- |
| macOS Apple Silicon | `dws-darwin-arm64.tar.gz` |
| macOS Intel | `dws-darwin-amd64.tar.gz` |
| Linux x86_64 | `dws-linux-amd64.tar.gz` |
| Linux ARM64 | `dws-linux-arm64.tar.gz` |
| Windows x86_64 | `dws-windows-amd64.zip` |
| Windows ARM64 | `dws-windows-arm64.zip` |

`checksums.txt` 提供 SHA-256 校验值。下载后核对所选安装包的校验值再解压。

为与已安装的官方 DWS 共存，将解压得到的 `dws` 重命名为 `dws-deap`（Windows 为 `dws-deap.exe`），放入独立目录；不要覆盖原有 `dws`。可通过完整路径运行，或将该目录加入 PATH。解压目录只是临时目录，不是必须安装到 `/tmp`。

安装后查看帮助：

```sh
dws-deap --version
dws-deap dingtalk-tag --help
dws-deap dingtalk-tag manage --help
```

本地 Agent 接入使用 `dws-deap dingtalk-tag connect`；A2A 或其他数字员工 DWS 登录场景使用 `dws-deap dingtalk-tag manage login --agent-uuid <数字员工ID>`。登录后按命令返回的精确 Profile 使用全局 `--profile` 参数选择身份。

普通数字员工创建或更新可以不传 `mainProgramType`，默认按 `open_code` 处理；只有明确需要本地 Agent 时才选择 `local_agent`。

二进制改名不代表配置隔离：仍需检查当前组织、Profile 和 MCP 环境。预发测试配置不会因为换安装包自动变成线上配置。不要对本 fork 包执行 `upgrade`，更新请下载本 fork 的新 Release。
