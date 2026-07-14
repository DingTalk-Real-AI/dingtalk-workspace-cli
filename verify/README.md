# 六渠道发布后验证

该目录用于发版质量保障 SOP 的 **T+1 线上回归**。脚本对每个可在当前主机运行的公开渠道执行：隔离环境清理、安装、`dws version` 版本检查、`dws --help` 冒烟、清理。

```bash
git clone https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli.git /tmp/dws-verify
cd /tmp/dws-verify/verify
bash verify-all-channels.sh
```

支持通过 `DWS_VERIFY_REPO=owner/repo` 验证 fork。输出状态含义：

- `PASS`：本机真实完成了安装、版本检查和冒烟。
- `FAIL`：公开渠道验证失败。
- `SKIP`：当前操作系统或依赖无法运行该渠道；不能计为通过，需由对应平台补测。

| 渠道 | macOS | Linux | Windows |
|---|---:|---:|---:|
| curl installer | ✅ | ✅ | — |
| PowerShell installer | — | — | ✅ |
| npm stable (`latest`) | ✅ | ✅ | ✅* |
| npm beta (`beta`) | ✅ | ✅ | ✅* |
| Homebrew | ✅ | ✅ | — |
| `dws upgrade` | ✅ | ✅ | ✅* |

`*` 当前总入口是 Bash；Windows 原生渠道由 PowerShell 安装器本身验证。Windows npm/upgrade 需要对应 Windows runner 补测，不能用非 Windows 上的 `pwsh` 结果代替。

Homebrew stable 与 keg-only beta Formula 都和代码位于同一个主仓库。由于这是自定义 remote，安装时必须显式指定仓库 URL；同一个 `homebrew` 验证步骤会依次安装并冒烟 stable、beta。脚本不会卸载用户已有的 Homebrew 安装；发现已有安装会失败退出，由验证者改用干净环境重跑。
