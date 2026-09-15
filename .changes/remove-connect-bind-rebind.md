---
category: Removed
---

- **数字员工连接命令**：移除 `dingtalk-tag connect bind` 和 `dingtalk-tag connect rebind`。统一使用 `connect` 登记绑定并接入；更换 Agent 或设备时先完成 `connect unbind`，再执行 `connect`。旧版运行中的连接需先停止，再用 `connect` 补齐绑定。
