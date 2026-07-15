# 六渠道发布后验证

该目录用于发版质量保障 SOP 的 **T+1 线上回归**。脚本对每个可在当前主机运行的公开渠道执行：隔离环境清理、安装、**版本断言**（比对 `dws version` 输出与该渠道自身声明的期望版本，不一致即判 `FAIL`）、`dws --help` 冒烟、清理。

期望版本来源：Homebrew 取 `brew list --versions` 记录的 Formula 版本，npm 取 `npm view <pkg>@<tag> version`，curl 与 `dws upgrade` 取 GitHub `releases/latest` 的 tag。断言防止“装了旧版本却报 PASS”。

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

Homebrew stable 与 keg-only beta Formula 都和代码位于同一个主仓库。由于这是自定义 remote，安装时必须显式指定仓库 URL。`homebrew` 步骤先装 stable 并记录其版本、二进制 SHA 与 `$(brew --prefix)/bin/dws` 链接指向，然后在 **stable 仍在安装状态下** 装 keg-only beta，再断言 stable 的版本、二进制 SHA 与链接三者均未被 beta 覆盖，且 PATH 上的 `dws` 仍解析到 stable，以此证明两渠道共存。脚本会拒绝在用户已装有任一 Formula 时运行；清理阶段只卸载脚本自己安装的 stable 与 beta 两个 Formula，并按需 untap，不动用户的预装版本。

在合并前用 PR head 的 Formula 验证时，通过 `DWS_VERIFY_HOMEBREW_TAP=<owner/repo>` 与 `DWS_VERIFY_HOMEBREW_REPO_URL=<fork git url>` 指向包含本 PR Formula 的 tap；npm/curl 渠道同理可用 `DWS_VERIFY_NPM_PACKAGE`、`DWS_VERIFY_REPO` 覆盖。
