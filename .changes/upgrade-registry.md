---
category: Fixed
---

- **升级来源** — 开源版的 `upgrade --check`、`--list` 和指定版本查询默认使用 npm Registry，避免依赖 GitHub REST API 的匿名请求额度；保留显式配置的 GitHub 镜像和仓库。
- **升级方式** — npm、pnpm、Homebrew 安装交由对应包管理器升级；独立二进制直接下载 npm 发布包，校验包完整性和平台资产 SHA256 后自更新，无需 npm 或 Node.js。保留原生安装的备份、回滚与技能包更新。
