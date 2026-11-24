---
category: Added
---

- **升级下载来源**：`dws upgrade --source auto|github|npmmirror` 支持自动回退或固定下载来源，并显示来源切换原因、重试次数、续传位置和下载进度。
- **平台分包与可靠下载**：公共版独立安装可使用同版本 npmmirror 平台和技能分包；下载支持有界重试、停滞超时和强 ETag 断点续传，保留版本、完整性和资源摘要校验。
- **npm 发布认证**：正常发布和 npm 修复改用 Trusted Publishing；可选发布六个平台分包和一个技能分包，现有主包安装与历史修复保持兼容。
