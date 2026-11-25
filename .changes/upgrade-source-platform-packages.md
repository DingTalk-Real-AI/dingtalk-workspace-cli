---
category: Added
---

- **升级下载来源**：`dws upgrade --source auto|github|npmmirror` 支持自动回退或固定下载来源，并显示来源切换原因、重试次数、续传位置和下载进度。
- **平台分包与可靠下载**：公共版独立安装可使用同版本 npmmirror 平台和技能分包；分包摘要绑定 GitHub Release 或官方主包声明，缺少校验依据时拒绝安装。下载支持有界重试、HTTP/2 断流重试、GitHub 二级限流回退、停滞超时和强 ETag 断点续传。
- **npm 发布认证**：正常发布和 npm 修复改用 Trusted Publishing；安装验证在无 OIDC 发布权限的独立 job 执行，发布 job 仅接收并核对同一份 tarball 和完整交接摘要。可选发布六个平台分包和一个技能分包；历史主包保留原始打包字节并跳过不支持的分包，Windows 发布通过 Node 调用 npm JavaScript 入口。
