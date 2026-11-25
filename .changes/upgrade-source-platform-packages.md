---
category: Added
---

- **升级下载来源**：`dws upgrade --source auto|github|npmmirror` 支持自动回退或固定下载来源，并显示来源切换原因、重试次数、续传位置和下载进度。
- **平台分包与可靠下载**：公共版独立安装可使用同版本 npmmirror 平台和技能分包；分包摘要绑定 GitHub Release 或官方主包声明，摘要冲突时拒绝安装，未声明平台资源的历史镜像主包保留整包 SRI 校验路径。归档和元数据正文支持 HTTP/2 断流恢复，下载支持有界重试、GitHub 二级限流回退、停滞超时和强 ETag 断点续传。
- **npm 发布认证**：正常发布和 npm 修复改用 Trusted Publishing；安装验证在无 OIDC 发布权限的独立 job 执行，发布 job 仅接收并核对同一份 tarball 和完整交接摘要。可选发布六个平台分包和一个技能分包；历史主包保留原始打包字节并跳过不支持的分包，Windows 发布通过 Node 调用 npm JavaScript 入口。
- **发布恢复**：发布重跑重新核验撤回标记、封板 tag 和 Release 状态；准备失败后可重新验包补发。历史版本沿用其安装验收契约，OSS 安装脚本随封板资源一并交接，交接文件保留 31 天；POSIX npm 版本管理 shim 继续可用。
