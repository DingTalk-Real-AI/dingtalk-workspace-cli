---
category: Fixed
---

- **SafeChat 默认构建与官方产物** — 支持平台的 CGO 构建无需额外 build tag
  即包含 SafeChat 后端，官方六平台 Release 固定使用可校验的交叉编译工具链并拒绝
  发布 CGO-disabled stub 二进制。
- 本地 `make build` / `make rebuild` 默认启用 CGO，并保留显式
  `CGO_ENABLED=0` 的 stub 构建选择。
- Linux 官方构建显式使用 glibc 2.17 链接目标，并校验 ELF 符号版本，
  防止交叉编译镜像升级隐式提高 Linux 系统要求。
- `install.sh` / `install-event.sh` / `install-devapp.sh` 在下载前识别 musl
  发行版（如 Alpine）并明确中止。Linux 产物依赖 glibc 动态加载器，此前这类环境
  会安装成功但连 `dws version` 都无法启动。判定以 `ldd --version` 为准，musl
  加载器文件只在 `ldd` 不报版本时兜底（Alpine 的 BusyBox `ldd` 仅转发给加载器），
  因此额外安装了 `musl` / `musl-tools` 的 glibc 发行版不会被误拒。
- SafeChat cipher 关闭时会等待进行中的加解密结束，不再与 `Close` 并发访问
  vendor client 的初始化状态（`go test -race` 下可复现的数据竞态）。
