---
category: Removed
---

- **移除顶层 `dws safechat` 命令（破坏性变更）** — 删除 `dws safechat selftest` 与 `dws safechat decrypt`。这两个命令在 `1.0.62-beta.3` (#1051) 交付，但只在显式 `-tags safechat` 的源码构建中存在：官方 Release 一直是 `CGO_ENABLED=0`，stub 的 `newSafeChatCommand()` 返回 `nil`，因此官方二进制从未包含该命令，受影响的只有自行打 tag 构建并升级的用户。SafeChat 现在是 `internal/msgcrypto` 的内部后端，仅通过聊天消息加解密路径暴露，不再提供独立顶层命令。
- **迁移方式** — `dws safechat decrypt` 的等价入口是 `dws chat crypto decrypt`，它走同一套 SafeChat 后端并按策略解密。`dws safechat selftest`（真实取码与密钥获取的端到端自检）没有等价命令；需要验证后端可用性时改用 `dws chat crypto decrypt` 对一条真实密文做一次解密。
