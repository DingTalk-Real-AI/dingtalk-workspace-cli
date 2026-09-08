---
category: Fixed
---

- **npm postinstall skill 安装不再阻断 CLI 安装** (#1320) — skill 安装失败（如 Windows 上 `publishCacheAtomically` 的 `renameSync` 抛 EPERM）此前会让整个 `npm install` 以非零退出码失败，即使 CLI 二进制已解压可用；现在改为仅打印告警并提示运行 `dws skill setup` 补装。同时 npm 安装路径开始识别 `DWS_NO_SKILLS=1`，与 `scripts/install.sh`、`scripts/install.ps1` 已有语义对齐。
