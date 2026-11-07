---
category: Fixed
---

- **Windows cgo 崩溃**：重新编译 Windows amd64/arm64 预编译 `libsafechat.a`（消除 legacy `_vsnprintf`/`__imp__*` dllimport 引用），并移除已无必要的 `msvcrt_compat_windows.c` 旧 CRT 桥接层，修复官方发布包在 Windows 上初始化 safechat（cgo）即崩溃的问题。
