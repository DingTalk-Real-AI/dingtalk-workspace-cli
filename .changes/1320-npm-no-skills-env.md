---
category: Added
---

- **npm 安装支持 `DWS_NO_SKILLS`** (#1320) — `npm install -g dingtalk-workspace-cli` 现在识别 `DWS_NO_SKILLS=1`，设置后只安装 CLI 本体并跳过 skill 安装，与 `scripts/install.sh`、`scripts/install.ps1` 已有语义对齐。适用于自行分发 skill、或需要规避 skill 写入失败（如 Windows 上 `renameSync` 抛 EPERM）导致整个 npm 安装失败的场景。
