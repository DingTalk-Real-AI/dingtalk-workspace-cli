---
category: Added
---

- **Windows 按 Agent 隔离凭据存储** — `DWS_KEYCHAIN_DIR` 在 Windows 上用于选择独立的 DPAPI 保护 HKCU 注册表命名空间，使同一台机器上的多个 Agent 不再共享登录凭据。需与 `DWS_CONFIG_DIR` 成对设置，并在每次调用（登录、业务命令、logout、reset）时传入相同的 Agent 专属绝对路径；凭据仍保存在注册表，不会作为文件写入该目录。目录会先清理、尽可能解析为绝对路径并做大小写折叠后再哈希，等价写法映射到同一命名空间；未设置该变量时沿用历史注册表键，已有登录不受影响。升级后显式设置该变量会进入新的命名空间并需要重新登录；logout/reset 只清理所选命名空间。macOS/Linux/OpenHarmony 上 `DWS_KEYCHAIN_DIR` 仍作为加密文件存储根目录，`DWS_DISABLE_KEYCHAIN` 依旧仅对 macOS 生效。
