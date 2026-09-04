---
category: Fixed
---

- **SafeChat 默认构建与官方产物** — 支持平台的 CGO 构建无需额外 build tag
  即包含 SafeChat 后端，官方六平台 Release 固定使用可校验的交叉编译工具链并拒绝
  发布 CGO-disabled stub 二进制。
